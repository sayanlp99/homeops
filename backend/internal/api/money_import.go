package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"homeops/internal/statement"
)

var digitRun = regexp.MustCompile(`\d+`)

type importRow struct {
	statement.Row
	Hash       string  `json:"hash"`
	Kind       string  `json:"kind"`
	CategoryID *string `json:"categoryId"`
	Matched    string  `json:"matched"` // "rule", "builtin" or ""
	Duplicate  bool    `json:"duplicate"`
	Include    bool    `json:"include"`
	// Set when this payment was already entered another way (invoice, by hand).
	AlreadyRecorded string `json:"alreadyRecorded,omitempty"`
}

// rowHash identifies a statement line for de-duplication across overlapping imports.
// The occurrence counter keeps two identical same-day purchases distinct.
func rowHash(accountID string, r statement.Row, occurrence int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%v|%s|%s|%d", accountID, r.DateStr, r.Amount, r.Debit,
		strings.ToLower(strings.Join(strings.Fields(r.Description), " ")), r.Ref, occurrence)))
	return hex.EncodeToString(h[:16])
}

// importPreview parses an uploaded statement and proposes categories, without saving.
func (s *Server) importPreview(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
	file, hdr, err := r.FormFile("file")
	if err != nil {
		return errBadRequest("Choose a statement file")
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	accountID := r.FormValue("accountId")
	var mappingJSON []byte
	if accountID != "" {
		if err := s.db.QueryRow(ctx, `SELECT import_mapping FROM money_accounts WHERE id = $1 AND household_id = $3 AND `+visible,
			accountID, u.ID, u.HouseholdID).Scan(&mappingJSON); err != nil {
			return errBadRequest("Unknown account")
		}
	}
	table, err := statement.Read(data)
	if err != nil {
		return errBadRequest(err.Error())
	}
	var m statement.Mapping
	var warnings []string
	switch {
	case r.FormValue("mapping") != "":
		if err := json.Unmarshal([]byte(r.FormValue("mapping")), &m); err != nil {
			return errBadRequest("Invalid column mapping")
		}
	case len(mappingJSON) > 0 && json.Unmarshal(mappingJSON, &m) == nil && m.HeaderRow < len(table):
		// The account's remembered layout, unless the header moved.
		if detected, err := statement.Detect(table); err == nil && detected.HeaderRow != m.HeaderRow {
			m = detected
		}
	default:
		if m, err = statement.Detect(table); err != nil {
			warnings = append(warnings, err.Error())
		}
	}
	var rows []statement.Row
	if m.HeaderRow >= 0 && m.Date >= 0 {
		var w2 []string
		rows, w2 = statement.Extract(table, m)
		warnings = append(warnings, w2...)
	}

	var incomeCat *string
	if cats, err := s.moneyCategories(ctx, u); err == nil {
		for _, c := range cats {
			if c.Kind == "income" && !c.Archived && c.OwnerID == nil {
				id := c.ID
				incomeCat = &id
				break
			}
		}
	}
	out := make([]importRow, 0, len(rows))
	seen := map[string]int{}
	hashes := make([]string, 0, len(rows))
	for _, row := range rows {
		base := rowHash(accountID, row, 0)
		occ := seen[base]
		seen[base]++
		ir := importRow{Row: row, Hash: rowHash(accountID, row, occ), Kind: "expense", Include: true}
		if !row.Debit {
			ir.Kind = "income"
		}
		if s.isMemberPayee(ctx, u.HouseholdID, row.Payee) {
			ir.Kind, ir.Matched = "transfer", "builtin"
		} else if mt, _ := s.classify(ctx, u, row.Description+" "+row.Payee, row.Debit); mt != nil {
			ir.Matched = mt.source
			if mt.kind == "transfer" {
				ir.Kind = "transfer"
			} else if mt.categoryID != "" {
				id := mt.categoryID
				ir.CategoryID = &id
				if !row.Debit && mt.kind == "expense" {
					ir.Kind = "expense" // refund into an expense category
				}
			}
		}
		if ir.Kind == "income" && ir.CategoryID == nil {
			ir.CategoryID = incomeCat
		}
		out = append(out, ir)
		hashes = append(hashes, ir.Hash)
	}
	// Flag rows already imported earlier.
	if len(hashes) > 0 {
		hr, err := s.db.Query(ctx, `SELECT import_hash FROM money_transactions WHERE household_id = $1 AND import_hash = ANY($2)`, u.HouseholdID, hashes)
		if err != nil {
			return err
		}
		dup := map[string]bool{}
		for hr.Next() {
			var h string
			hr.Scan(&h)
			dup[h] = true
		}
		hr.Close()
		for i := range out {
			if dup[out[i].Hash] {
				out[i].Duplicate, out[i].Include = true, false
			}
		}
	}
	if err := s.matchExisting(ctx, u, accountID, out); err != nil {
		return err
	}
	// Banks that print totals let us prove nothing was dropped.
	var check map[string]any
	if sum := statement.ReadSummary(data); sum != nil {
		problem := sum.Check(rows)
		check = map[string]any{"summary": sum, "ok": problem == "", "problem": problem}
		if problem != "" {
			warnings = append(warnings, problem)
		}
	}
	// Columns for the manual mapper: header row (or first row) cells.
	var header []string
	if m.HeaderRow >= 0 && m.HeaderRow < len(table) {
		header = table[m.HeaderRow]
	}
	sample := table
	if len(sample) > 15 {
		sample = sample[:15]
	}
	return ok(w, map[string]any{
		"filename": hdr.Filename, "mapping": m, "header": header, "sample": sample, "rows": out, "warnings": warnings, "check": check,
	})
}

func (s *Server) importCommit(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 8<<20)
	var in struct {
		AccountID *string            `json:"accountId"`
		Filename  string             `json:"filename"`
		Mapping   *statement.Mapping `json:"mapping"`
		Personal  bool               `json:"personal"`
		Rows      []importRow        `json:"rows"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		return errBadRequest("Invalid import")
	}
	if len(in.Rows) == 0 {
		return errBadRequest("Nothing selected to import")
	}
	var owner *string
	if in.Personal {
		owner = &u.ID
	}
	if in.AccountID != nil && *in.AccountID != "" {
		var accOwner *string
		if err := s.db.QueryRow(ctx, `SELECT owner_id FROM money_accounts WHERE id = $1 AND household_id = $3 AND `+visible,
			*in.AccountID, u.ID, u.HouseholdID).Scan(&accOwner); err != nil {
			return errBadRequest("Unknown account")
		}
		if accOwner != nil {
			owner = accOwner // a personal account's statement stays private
		}
	} else {
		in.AccountID = nil
	}
	// Categories the user may use for this privacy level.
	cats, err := s.moneyCategories(ctx, u)
	if err != nil {
		return err
	}
	allowed := map[string]bool{}
	for _, c := range cats {
		if owner != nil || c.OwnerID == nil {
			allowed[c.ID] = true
		}
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var importID string
	if err := tx.QueryRow(ctx, `INSERT INTO money_imports (household_id, owner_id, account_id, filename, row_count, imported, created_by)
		VALUES ($1, $2, $3, $4, $5, 0, $6) RETURNING id`, u.HouseholdID, owner, in.AccountID, truncate(in.Filename, 120), len(in.Rows), u.ID).Scan(&importID); err != nil {
		return err
	}
	imported := 0
	touched := map[string]string{}
	for _, row := range in.Rows {
		if !row.Include {
			continue
		}
		if _, err := time.Parse(time.DateOnly, row.DateStr); err != nil || row.Amount <= 0 || row.Hash == "" {
			return errBadRequest("Invalid row in import")
		}
		kind := row.Kind
		if kind != "expense" && kind != "income" && kind != "transfer" {
			kind = "expense"
		}
		amount := row.Amount
		if kind == "expense" && !row.Debit {
			amount = -amount // refund
		}
		cat := row.CategoryID
		if cat != nil && !allowed[*cat] || kind == "transfer" {
			cat = nil
		}
		tag, err := tx.Exec(ctx, `INSERT INTO money_transactions (id, household_id, owner_id, kind, amount, date, payee, description, category_id,
			account_id, paid_by, source, import_id, import_hash, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 'import', $12, $13, $11)
			ON CONFLICT (household_id, import_hash) WHERE import_hash IS NOT NULL DO NOTHING`,
			randomUUID(), u.HouseholdID, owner, kind, amount, row.DateStr, truncate(row.Payee, 80), truncate(row.Description, 300), cat,
			in.AccountID, u.ID, importID, row.Hash)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			imported++
			if cat != nil {
				touched[*cat] = row.DateStr
			}
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE money_imports SET imported = $2 WHERE id = $1`, importID, imported); err != nil {
		return err
	}
	if in.Mapping != nil && in.AccountID != nil {
		b, _ := json.Marshal(in.Mapping)
		if _, err := tx.Exec(ctx, `UPDATE money_accounts SET import_mapping = $2 WHERE id = $1`, *in.AccountID, b); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.publishMoney(u, r)
	for cat, date := range touched {
		c := cat
		s.checkBudgetAlerts(ctx, u.HouseholdID, &c, date)
	}
	return ok(w, map[string]any{"importId": importID, "imported": imported, "skipped": len(in.Rows) - imported})
}

type MoneyImport struct {
	ID        string    `json:"id" db:"id"`
	AccountID *string   `json:"accountId" db:"account_id"`
	Filename  string    `json:"filename" db:"filename"`
	RowCount  int       `json:"rowCount" db:"row_count"`
	Imported  int       `json:"imported" db:"imported"`
	CreatedBy *string   `json:"createdBy" db:"created_by"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
}

func (s *Server) listImports(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT id, account_id, filename, row_count, imported, created_by, created_at FROM money_imports
		WHERE household_id = $1 AND `+visible+` ORDER BY created_at DESC LIMIT 30`, u.HouseholdID, u.ID)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[MoneyImport])
	if err != nil {
		return err
	}
	return ok(w, out)
}

// undoImport deletes an import and every transaction it created (via ON DELETE CASCADE).
func (s *Server) undoImport(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	tag, err := s.db.Exec(r.Context(), `DELETE FROM money_imports WHERE id = $1 AND household_id = $3 AND `+visible, chi.URLParam(r, "id"), u.ID, u.HouseholdID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errNotFound("Import not found")
	}
	s.publishMoney(u, r)
	return noContent(w)
}
