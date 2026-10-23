package api

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"homeops/internal/invoice"
)

// invoicePreview parses an uploaded grocery invoice (PDF) or stores a bill photo, and
// proposes an expense plus pantry additions. Nothing is recorded until commit.
func (s *Server) invoicePreview(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes+1<<20)
	file, _, err := r.FormFile("file")
	if err != nil {
		return errBadRequest("Choose an invoice PDF or a photo of the bill")
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	out := map[string]any{}
	if !strings.HasPrefix(string(data[:min(len(data), 5)]), "%PDF") {
		// A photo of a paper bill: keep it as the receipt; the amount is entered by hand.
		id, err := s.storeImage(ctx, strings.NewReader(string(data)))
		if err != nil {
			return err
		}
		out["receipt"] = id
		out["invoice"] = nil
		return ok(w, out)
	}
	pages, err := invoice.ReadPDF(data)
	if err != nil {
		return errBadRequest(err.Error())
	}
	inv := invoice.Parse(pages)
	id, err := s.storePDF(ctx, data)
	if err != nil {
		return err
	}
	out["receipt"] = id
	out["invoice"] = inv
	if inv.Date == "" {
		h, err := s.getHousehold(ctx, u.HouseholdID)
		if err != nil {
			return err
		}
		inv.Date = localToday(h, time.Now()).Format(time.DateOnly)
	}
	// Already recorded? (same merchant + order ID)
	if inv.OrderID != "" {
		var existing *string
		s.db.QueryRow(ctx, `SELECT id FROM money_transactions WHERE household_id = $1 AND import_hash = $2`, u.HouseholdID, invoiceHash(inv)).Scan(&existing)
		out["duplicateOf"] = existing
	}
	cats, err := s.moneyCategories(ctx, u)
	if err != nil {
		return err
	}
	for _, c := range cats {
		if c.OwnerID == nil && strings.EqualFold(c.Name, "Groceries") && !c.Archived {
			out["categoryId"] = c.ID
		}
	}
	// Pantry candidates: goods only, with quantity from pack size × count.
	type cand struct {
		Name     string   `json:"name"`
		Quantity *float64 `json:"quantity"`
		Unit     string   `json:"unit"`
		Aisle    string   `json:"aisle"`
		Include  bool     `json:"include"`
	}
	var cands []cand
	for _, it := range inv.Items {
		if it.Fee {
			continue
		}
		c := cand{Name: it.Name, Aisle: classifyAisle(it.Name), Unit: "pcs"}
		q := it.Qty
		if it.PackQty != nil {
			q = it.Qty * *it.PackQty
			c.Unit = it.PackUnit
		}
		c.Quantity = &q
		// Cleaners, wipes, tobacco, ice… don't belong in the kitchen pantry by default.
		c.Include = c.Aisle != "Household" && c.Aisle != "Other"
		cands = append(cands, c)
	}
	out["pantry"] = cands
	return ok(w, out)
}

func invoiceHash(inv *invoice.Invoice) string {
	return "inv:" + strings.ToLower(inv.Merchant) + ":" + inv.OrderID
}

func (s *Server) invoiceCommit(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		txnInput
		OrderID string `json:"orderId"`
		Pantry  []struct {
			Name     string   `json:"name"`
			Quantity *float64 `json:"quantity"`
			Unit     string   `json:"unit"`
		} `json:"pantry"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Amount <= 0 {
		return errBadRequest("Enter the bill total")
	}
	if _, err := parseDate(in.Date); err != nil {
		return err
	}
	in.Kind = "expense"
	if err := s.checkRefs(ctx, u, &in.txnInput); err != nil {
		return err
	}
	var owner *string
	if in.Personal {
		owner = &u.ID
	}
	var hash *string
	if in.OrderID != "" {
		h := invoiceHash(&invoice.Invoice{Merchant: in.Payee, OrderID: in.OrderID})
		hash = &h
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	id := randomUUID()
	tag, err := tx.Exec(ctx, `INSERT INTO money_transactions (id, household_id, owner_id, kind, amount, date, payee, note, category_id, account_id,
		paid_by, receipt, source, import_hash, created_by) VALUES ($1, $2, $3, 'expense', $4, $5, $6, $7, $8, $9, $10, $11, 'invoice', $12, $13)
		ON CONFLICT (household_id, import_hash) WHERE import_hash IS NOT NULL DO NOTHING`,
		id, u.HouseholdID, owner, in.Amount, in.Date, truncate(in.Payee, 80), truncate(in.Note, 300), in.CategoryID, in.AccountID, in.PaidBy, in.Receipt, hash, u.ID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errConflict("This order has already been recorded")
	}
	added := 0
	for _, p := range in.Pantry {
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		iid, aisle, err := ensureIngredient(ctx, tx, u.HouseholdID, p.Name)
		if err != nil {
			return err
		}
		if err := addToPantry(ctx, tx, u.HouseholdID, iid, p.Quantity, normalizeUnit(p.Unit), aisle); err != nil {
			return err
		}
		added++
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.publishMoney(u, r)
	if added > 0 {
		s.hub.Publish(u.HouseholdID, eventPantry.from(r))
	}
	s.checkBudgetAlerts(ctx, u.HouseholdID, in.CategoryID, in.Date)
	return ok(w, map[string]any{"transactionId": id, "pantryAdded": added})
}

// matchExisting finds an already-recorded transaction (an invoice, a manual entry, a
// grocery trip) that a statement line is probably the payment for: same amount within
// three days. Statement imports default such rows to "skip" so nothing counts twice.
func (s *Server) matchExisting(ctx context.Context, u *User, accountID string, rows []importRow) error {
	if len(rows) == 0 {
		return nil
	}
	minD, maxD := rows[0].DateStr, rows[0].DateStr
	for _, r := range rows {
		minD, maxD = min(minD, r.DateStr), max(maxD, r.DateStr)
	}
	from, _ := time.Parse(time.DateOnly, minD)
	to, _ := time.Parse(time.DateOnly, maxD)
	q, err := s.db.Query(ctx, `SELECT id, amount, date, payee, source FROM money_transactions
		WHERE household_id = $1 AND `+visible+` AND source <> 'import' AND kind = 'expense' AND date BETWEEN $3 AND $4`,
		u.HouseholdID, u.ID, from.AddDate(0, 0, -3), to.AddDate(0, 0, 3))
	if err != nil {
		return err
	}
	type ex struct {
		id, payee, source string
		amount            int64
		date              time.Time
		used              bool
	}
	var existing []*ex
	for q.Next() {
		e := &ex{}
		if err := q.Scan(&e.id, &e.amount, &e.date, &e.payee, &e.source); err != nil {
			q.Close()
			return err
		}
		existing = append(existing, e)
	}
	q.Close()
	for i := range rows {
		r := &rows[i]
		if r.Duplicate || !r.Debit {
			continue
		}
		d, _ := time.Parse(time.DateOnly, r.DateStr)
		for _, e := range existing {
			days := d.Sub(e.date).Hours() / 24
			if e.used || e.amount != r.Amount || days < -3 || days > 3 {
				continue
			}
			e.used = true
			label := e.payee
			if label == "" {
				label = "an earlier entry"
			}
			r.AlreadyRecorded = "Already recorded: " + label
			r.Include = false
			break
		}
	}
	return nil
}
