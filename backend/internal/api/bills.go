package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

type Bill struct {
	ID               string  `json:"id" db:"id"`
	OwnerID          *string `json:"ownerId" db:"owner_id"`
	Name             string  `json:"name" db:"name"`
	Amount           int64   `json:"amount" db:"amount"`
	DueDay           int     `json:"dueDay" db:"due_day"`
	CategoryID       *string `json:"categoryId" db:"category_id"`
	AccountID        *string `json:"accountId" db:"account_id"`
	RemindDaysBefore int     `json:"remindDaysBefore" db:"remind_days_before"`
	LastPaidMonth    *string `json:"lastPaidMonth" db:"last_paid_month"`
	Active           bool    `json:"active" db:"active"`
}

const billColumns = `id, owner_id, name, amount, due_day, category_id, account_id, remind_days_before, to_char(last_paid_month, 'YYYY-MM') AS last_paid_month, active`

func (s *Server) listBills(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT `+billColumns+` FROM money_bills WHERE household_id = $1 AND `+visible+` ORDER BY due_day, lower(name)`,
		u.HouseholdID, u.ID)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[Bill])
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) saveBill(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Name             string  `json:"name"`
		Amount           int64   `json:"amount"`
		DueDay           int     `json:"dueDay"`
		CategoryID       *string `json:"categoryId"`
		AccountID        *string `json:"accountId"`
		RemindDaysBefore int     `json:"remindDaysBefore"`
		Active           *bool   `json:"active"`
		Personal         bool    `json:"personal"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 60 {
		return errBadRequest("Bill name is required")
	}
	if in.DueDay < 1 || in.DueDay > 31 {
		return errBadRequest("Due day must be 1–31")
	}
	if in.Amount < 0 || in.RemindDaysBefore < 0 || in.RemindDaysBefore > 15 {
		return errBadRequest("Invalid amount or reminder")
	}
	ti := txnInput{CategoryID: in.CategoryID, AccountID: in.AccountID, Personal: in.Personal, Kind: "expense"}
	if err := s.checkRefs(ctx, u, &ti); err != nil {
		return err
	}
	var owner *string
	if ti.Personal {
		owner = &u.ID
	}
	active := in.Active == nil || *in.Active
	id := chi.URLParam(r, "id")
	if id == "" {
		if err := s.db.QueryRow(ctx, `INSERT INTO money_bills (household_id, owner_id, name, amount, due_day, category_id, account_id, remind_days_before, active)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING id`, u.HouseholdID, owner, in.Name, in.Amount, in.DueDay, ti.CategoryID, ti.AccountID,
			in.RemindDaysBefore, active).Scan(&id); err != nil {
			return err
		}
	} else {
		tag, err := s.db.Exec(ctx, `UPDATE money_bills SET owner_id = $3, name = $4, amount = $5, due_day = $6, category_id = $7, account_id = $8,
			remind_days_before = $9, active = $10 WHERE id = $1 AND household_id = $11 AND `+visible,
			id, u.ID, owner, in.Name, in.Amount, in.DueDay, ti.CategoryID, ti.AccountID, in.RemindDaysBefore, active, u.HouseholdID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound("Bill not found")
		}
	}
	s.publishMoney(u, r)
	return ok(w, map[string]string{"id": id})
}

func (s *Server) deleteBill(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM money_bills WHERE id = $1 AND household_id = $3 AND `+visible, chi.URLParam(r, "id"), u.ID, u.HouseholdID); err != nil {
		return err
	}
	s.publishMoney(u, r)
	return noContent(w)
}

// payBill records this month's payment as an expense and silences its reminders.
func (s *Server) payBill(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Amount int64  `json:"amount"`
		Date   string `json:"date"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	rows, _ := s.db.Query(ctx, `SELECT `+billColumns+` FROM money_bills WHERE id = $1 AND household_id = $3 AND `+visible, chi.URLParam(r, "id"), u.ID, u.HouseholdID)
	b, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Bill])
	if err != nil {
		return err
	}
	if in.Amount <= 0 {
		in.Amount = b.Amount
	}
	if in.Amount <= 0 {
		return errBadRequest("Enter the amount paid")
	}
	if in.Date == "" {
		h, err := s.getHousehold(ctx, u.HouseholdID)
		if err != nil {
			return err
		}
		in.Date = localToday(h, time.Now()).Format(time.DateOnly)
	}
	d, err := parseDate(in.Date)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	id := randomUUID()
	if _, err := tx.Exec(ctx, `INSERT INTO money_transactions (id, household_id, owner_id, kind, amount, date, payee, category_id, account_id,
		paid_by, source, bill_id, created_by) VALUES ($1, $2, $3, 'expense', $4, $5, $6, $7, $8, $9, 'bill', $10, $9)`,
		id, u.HouseholdID, b.OwnerID, in.Amount, in.Date, b.Name, b.CategoryID, b.AccountID, u.ID, b.ID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE money_bills SET last_paid_month = $2 WHERE id = $1`, b.ID, time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.publishMoney(u, r)
	s.checkBudgetAlerts(ctx, u.HouseholdID, b.CategoryID, in.Date)
	return ok(w, map[string]string{"transactionId": id})
}

// billReminders runs daily: "Electricity due in 3 days", on the day, and once if overdue.
func (s *Server) billReminders(ctx context.Context, h *Household, today time.Time) error {
	rows, _ := s.db.Query(ctx, `SELECT `+billColumns+` FROM money_bills WHERE household_id = $1 AND active`, h.ID)
	bills, err := pgx.CollectRows(rows, pgx.RowToStructByName[Bill])
	if err != nil {
		return err
	}
	month := today.Format("2006-01")
	lastDay := time.Date(today.Year(), today.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	for _, b := range bills {
		if b.LastPaidMonth != nil && *b.LastPaidMonth == month {
			continue
		}
		due := time.Date(today.Year(), today.Month(), min(b.DueDay, lastDay), 0, 0, 0, 0, time.UTC)
		days := int(due.Sub(today).Hours() / 24)
		var phase, title string
		amount := ""
		if b.Amount > 0 {
			amount = " · " + formatRupees(b.Amount)
		}
		switch {
		case days == 0:
			phase, title = "today", fmt.Sprintf("%s is due today%s", b.Name, amount)
		case days > 0 && days <= b.RemindDaysBefore:
			phase, title = "soon", fmt.Sprintf("%s due in %d %s%s", b.Name, days, plural(days, "day", "days"), amount)
		case days < 0 && days >= -3:
			phase, title = "overdue", fmt.Sprintf("%s is overdue%s", b.Name, amount)
		default:
			continue
		}
		n := notification{Kind: "budget", DedupeKey: fmt.Sprintf("bill:%s:%s:%s", b.ID, month, phase), Title: title,
			Body: fmt.Sprintf("Due on the %s. Mark it paid in HomeOps once done.", ordinal(due.Day())), URL: "/budget/bills"}
		if b.OwnerID != nil {
			n.UserID = *b.OwnerID
			err = s.notify(ctx, n)
		} else {
			err = s.notifyHousehold(ctx, h.ID, n)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func ordinal(n int) string {
	suffix := "th"
	if n%100 < 11 || n%100 > 13 {
		switch n % 10 {
		case 1:
			suffix = "st"
		case 2:
			suffix = "nd"
		case 3:
			suffix = "rd"
		}
	}
	return fmt.Sprintf("%d%s", n, suffix)
}
