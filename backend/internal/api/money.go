package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
)

// Visibility: shared rows (owner_id NULL) plus the current user's private rows.
const visible = `(owner_id IS NULL OR owner_id = $2)`

type MoneyCategory struct {
	ID           string    `json:"id" db:"id"`
	OwnerID      *string   `json:"ownerId" db:"owner_id"`
	Name         string    `json:"name" db:"name"`
	Icon         string    `json:"icon" db:"icon"`
	Color        string    `json:"color" db:"color"`
	Kind         string    `json:"kind" db:"kind"`
	MonthlyLimit int64     `json:"monthlyLimit" db:"monthly_limit"`
	Rollover     bool      `json:"rollover" db:"rollover"`
	Position     int       `json:"position" db:"position"`
	Archived     bool      `json:"archived" db:"archived"`
	CreatedAt    time.Time `json:"createdAt" db:"created_at"`
}

const moneyCategoryColumns = `id, owner_id, name, icon, color, kind, monthly_limit, rollover, position, archived, created_at`

var defaultMoneyCategories = []struct{ name, icon, color, kind string }{
	{"Groceries", "shopping-basket", "#16a34a", "expense"},
	{"Vegetables & Fruits", "carrot", "#65a30d", "expense"},
	{"Dining Out", "utensils", "#ea580c", "expense"},
	{"Utilities", "plug", "#0891b2", "expense"},
	{"Rent / EMI", "home", "#7c3aed", "expense"},
	{"Transport", "car", "#2563eb", "expense"},
	{"Household", "sofa", "#a16207", "expense"},
	{"Health", "heart-pulse", "#dc2626", "expense"},
	{"Shopping", "shopping-bag", "#db2777", "expense"},
	{"Subscriptions", "repeat", "#4f46e5", "expense"},
	{"Misc", "circle", "#64748b", "expense"},
	{"Income", "wallet", "#15803d", "income"},
}

// ensureMoneyDefaults seeds the household's shared categories on first use.
func (s *Server) ensureMoneyDefaults(ctx context.Context, householdID string) error {
	var n int
	if err := s.db.QueryRow(ctx, `SELECT count(*) FROM money_categories WHERE household_id = $1 AND owner_id IS NULL`, householdID).Scan(&n); err != nil || n > 0 {
		return err
	}
	for i, c := range defaultMoneyCategories {
		if _, err := s.db.Exec(ctx, `INSERT INTO money_categories (household_id, name, icon, color, kind, position) VALUES ($1, $2, $3, $4, $5, $6)
			ON CONFLICT DO NOTHING`, householdID, c.name, c.icon, c.color, c.kind, i); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) moneyCategories(ctx context.Context, u *User) ([]MoneyCategory, error) {
	if err := s.ensureMoneyDefaults(ctx, u.HouseholdID); err != nil {
		return nil, err
	}
	rows, _ := s.db.Query(ctx, `SELECT `+moneyCategoryColumns+` FROM money_categories WHERE household_id = $1 AND `+visible+`
		ORDER BY owner_id NULLS FIRST, position, lower(name)`, u.HouseholdID, u.ID)
	return pgx.CollectRows(rows, pgx.RowToStructByName[MoneyCategory])
}

func (s *Server) listMoneyCategories(w http.ResponseWriter, r *http.Request) error {
	cats, err := s.moneyCategories(r.Context(), currentUser(r))
	if err != nil {
		return err
	}
	return ok(w, cats)
}

func (s *Server) saveMoneyCategory(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		Name         *string `json:"name"`
		Icon         *string `json:"icon"`
		Color        *string `json:"color"`
		Kind         *string `json:"kind"`
		MonthlyLimit *int64  `json:"monthlyLimit"`
		Rollover     *bool   `json:"rollover"`
		Position     *int    `json:"position"`
		Archived     *bool   `json:"archived"`
		Personal     *bool   `json:"personal"` // create only
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.MonthlyLimit != nil && (*in.MonthlyLimit < 0 || *in.MonthlyLimit > 1e12) {
		return errBadRequest("Invalid budget")
	}
	if in.Kind != nil && *in.Kind != "expense" && *in.Kind != "income" {
		return errBadRequest("Kind must be expense or income")
	}
	if in.Color != nil && !isHexColor(*in.Color) {
		return errBadRequest("Invalid colour")
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 40 {
			return errBadRequest("Category name is required (max 40 characters)")
		}
		in.Name = &n
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		if in.Name == nil {
			return errBadRequest("Category name is required")
		}
		var owner *string
		if in.Personal != nil && *in.Personal {
			owner = &u.ID
		}
		if err := s.db.QueryRow(ctx, `INSERT INTO money_categories (household_id, owner_id, name, icon, color, kind, monthly_limit, rollover, position)
			VALUES ($1, $2, $3, coalesce($4, 'circle'), coalesce($5, '#64748b'), coalesce($6, 'expense'), coalesce($7, 0), coalesce($8, false),
				(SELECT coalesce(max(position), 0) + 1 FROM money_categories WHERE household_id = $1)) RETURNING id`,
			u.HouseholdID, owner, *in.Name, in.Icon, in.Color, in.Kind, in.MonthlyLimit, in.Rollover).Scan(&id); err != nil {
			return err
		}
	} else {
		tag, err := s.db.Exec(ctx, `UPDATE money_categories SET name = coalesce($3, name), icon = coalesce($4, icon), color = coalesce($5, color),
			kind = coalesce($6, kind), monthly_limit = coalesce($7, monthly_limit), rollover = coalesce($8, rollover),
			position = coalesce($9, position), archived = coalesce($10, archived)
			WHERE id = $1 AND household_id = $11 AND `+visible,
			id, u.ID, in.Name, in.Icon, in.Color, in.Kind, in.MonthlyLimit, in.Rollover, in.Position, in.Archived, u.HouseholdID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return errNotFound("Category not found")
		}
	}
	s.publishMoney(u, r)
	rows, _ := s.db.Query(ctx, `SELECT `+moneyCategoryColumns+` FROM money_categories WHERE id = $1`, id)
	c, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[MoneyCategory])
	if err != nil {
		return err
	}
	return ok(w, c)
}

// setMonthBudget overrides one category's limit for one month (amount < 0 clears it).
func (s *Server) setMonthBudget(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Amount int64 `json:"amount"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	month, err := parseMonth(chi.URLParam(r, "month"))
	if err != nil {
		return err
	}
	id := chi.URLParam(r, "id")
	var exists bool
	if err := s.db.QueryRow(r.Context(), `SELECT EXISTS (SELECT 1 FROM money_categories WHERE id = $1 AND household_id = $3 AND `+visible+`)`,
		id, u.ID, u.HouseholdID).Scan(&exists); err != nil || !exists {
		return errNotFound("Category not found")
	}
	if in.Amount < 0 {
		_, err = s.db.Exec(r.Context(), `DELETE FROM money_budget_months WHERE category_id = $1 AND month = $2`, id, month)
	} else {
		_, err = s.db.Exec(r.Context(), `INSERT INTO money_budget_months (category_id, month, amount) VALUES ($1, $2, $3)
			ON CONFLICT (category_id, month) DO UPDATE SET amount = EXCLUDED.amount`, id, month, in.Amount)
	}
	if err != nil {
		return err
	}
	s.publishMoney(u, r)
	return noContent(w)
}

func parseMonth(s string) (time.Time, error) {
	m, err := time.Parse("2006-01", s)
	if err != nil {
		return m, errBadRequest("Month must be YYYY-MM")
	}
	return m, nil
}

// publishMoney tells clients to refresh money data. Private changes still trigger a
// refetch on the partner's device, but the API never returns the other person's rows.
func (s *Server) publishMoney(u *User, r *http.Request) {
	s.hub.Publish(u.HouseholdID, eventMoney.from(r))
}

// ---- accounts ----

type MoneyAccount struct {
	ID       string  `json:"id" db:"id"`
	OwnerID  *string `json:"ownerId" db:"owner_id"`
	Name     string  `json:"name" db:"name"`
	Kind     string  `json:"kind" db:"kind"`
	Archived bool    `json:"archived" db:"archived"`
	Mapped   bool    `json:"hasImportMapping" db:"mapped"`
}

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT id, owner_id, name, kind, archived, import_mapping IS NOT NULL AS mapped
		FROM money_accounts WHERE household_id = $1 AND `+visible+` ORDER BY archived, lower(name)`, u.HouseholdID, u.ID)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[MoneyAccount])
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) saveAccount(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		Name     *string `json:"name"`
		Kind     *string `json:"kind"`
		Archived *bool   `json:"archived"`
		Personal bool    `json:"personal"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.Kind != nil && !slices.Contains([]string{"bank", "card", "cash", "wallet"}, *in.Kind) {
		return errBadRequest("Account type must be bank, card, cash or wallet")
	}
	if in.Name != nil {
		n := strings.TrimSpace(*in.Name)
		if n == "" || len(n) > 40 {
			return errBadRequest("Account name is required")
		}
		in.Name = &n
	}
	id := chi.URLParam(r, "id")
	if id == "" {
		if in.Name == nil {
			return errBadRequest("Account name is required")
		}
		var owner *string
		if in.Personal {
			owner = &u.ID
		}
		if err := s.db.QueryRow(r.Context(), `INSERT INTO money_accounts (household_id, owner_id, name, kind) VALUES ($1, $2, $3, coalesce($4, 'bank')) RETURNING id`,
			u.HouseholdID, owner, *in.Name, in.Kind).Scan(&id); err != nil {
			return err
		}
	} else if _, err := s.db.Exec(r.Context(), `UPDATE money_accounts SET name = coalesce($3, name), kind = coalesce($4, kind), archived = coalesce($5, archived)
		WHERE id = $1 AND household_id = $6 AND `+visible, id, u.ID, in.Name, in.Kind, in.Archived, u.HouseholdID); err != nil {
		return err
	}
	s.publishMoney(u, r)
	return ok(w, map[string]string{"id": id})
}

// ---- transactions ----

type Transaction struct {
	ID          string    `json:"id" db:"id"`
	OwnerID     *string   `json:"ownerId" db:"owner_id"`
	Kind        string    `json:"kind" db:"kind"`
	Amount      int64     `json:"amount" db:"amount"`
	Date        string    `json:"date" db:"date"`
	Payee       string    `json:"payee" db:"payee"`
	Description string    `json:"description" db:"description"`
	Note        string    `json:"note" db:"note"`
	CategoryID  *string   `json:"categoryId" db:"category_id"`
	AccountID   *string   `json:"accountId" db:"account_id"`
	PaidBy      *string   `json:"paidBy" db:"paid_by"`
	Receipt     string    `json:"receipt" db:"receipt"`
	Source      string    `json:"source" db:"source"`
	ImportID    *string   `json:"importId" db:"import_id"`
	CreatedBy   *string   `json:"createdBy" db:"created_by"`
	CreatedAt   time.Time `json:"createdAt" db:"created_at"`
}

const txnColumns = `id, owner_id, kind, amount, date::text AS date, payee, description, note, category_id, account_id, paid_by, receipt,
	source, import_id, created_by, created_at`

func (s *Server) listTransactions(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	q := r.URL.Query()
	where := []string{`household_id = $1`, visible}
	args := []any{u.HouseholdID, u.ID}
	add := func(cond string, v any) {
		args = append(args, v)
		where = append(where, fmt.Sprintf(cond, len(args)))
	}
	if m := q.Get("month"); m != "" {
		month, err := parseMonth(m)
		if err != nil {
			return err
		}
		add(`date >= $%d`, month)
		add(`date < $%d`, month.AddDate(0, 1, 0))
	}
	if c := q.Get("category"); c == "none" {
		where = append(where, `category_id IS NULL AND kind <> 'transfer'`)
	} else if c != "" {
		add(`category_id = $%d`, c)
	}
	if a := q.Get("account"); a != "" {
		add(`account_id = $%d`, a)
	}
	if p := q.Get("paidBy"); p != "" {
		add(`paid_by = $%d`, p)
	}
	if t := strings.TrimSpace(q.Get("q")); t != "" {
		add(`(payee ILIKE '%%' || $%[1]d || '%%' OR description ILIKE '%%' || $%[1]d || '%%' OR note ILIKE '%%' || $%[1]d || '%%')`, t)
	}
	rows, _ := s.db.Query(r.Context(), `SELECT `+txnColumns+` FROM money_transactions WHERE `+strings.Join(where, " AND ")+
		` ORDER BY date DESC, created_at DESC LIMIT 2000`, args...)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[Transaction])
	if err != nil {
		return err
	}
	return ok(w, out)
}

type txnInput struct {
	ID         string  `json:"id"`
	Kind       string  `json:"kind"`
	Amount     int64   `json:"amount"`
	Date       string  `json:"date"`
	Payee      string  `json:"payee"`
	Note       string  `json:"note"`
	CategoryID *string `json:"categoryId"`
	AccountID  *string `json:"accountId"`
	PaidBy     *string `json:"paidBy"`
	Receipt    string  `json:"receipt"`
	Personal   bool    `json:"personal"`
}

// checkRefs verifies the category/account are visible to the user and match privacy:
// a shared transaction can't use a personal category, and vice versa.
func (s *Server) checkRefs(ctx context.Context, u *User, in *txnInput) error {
	if in.CategoryID != nil && *in.CategoryID != "" {
		var owner *string
		var kind string
		if err := s.db.QueryRow(ctx, `SELECT owner_id, kind FROM money_categories WHERE id = $1 AND household_id = $3 AND `+visible,
			*in.CategoryID, u.ID, u.HouseholdID).Scan(&owner, &kind); err != nil {
			return errBadRequest("Unknown category")
		}
		if owner != nil {
			in.Personal = true
		} else if in.Personal {
			// Personal spending stays out of shared envelopes, so both partners see the
			// same numbers for shared categories.
			in.CategoryID = nil
		}
		if in.Kind == "transfer" {
			in.CategoryID = nil
		} else if kind != in.Kind && in.Kind != "" {
			in.Kind = kind
		}
	} else {
		in.CategoryID = nil
	}
	if in.CategoryID == nil && in.Kind == "" {
		in.Kind = "expense"
	}
	if in.AccountID != nil && *in.AccountID != "" {
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM money_accounts WHERE id = $1 AND household_id = $3 AND `+visible+`)`,
			*in.AccountID, u.ID, u.HouseholdID).Scan(&exists); err != nil || !exists {
			return errBadRequest("Unknown account")
		}
	} else {
		in.AccountID = nil
	}
	if in.PaidBy != nil && *in.PaidBy != "" {
		var exists bool
		if err := s.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM members WHERE id = $1 AND household_id = $2)`, *in.PaidBy, u.HouseholdID).Scan(&exists); err != nil || !exists {
			return errBadRequest("Unknown member")
		}
	} else {
		in.PaidBy = &u.ID
	}
	if in.Receipt != "" && !fileIDRe.MatchString(in.Receipt) {
		return errBadRequest("Invalid receipt")
	}
	return nil
}

func (s *Server) createTransaction(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in txnInput
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.ID == "" {
		in.ID = randomUUID()
	} else if t, err := s.getTxn(ctx, u, in.ID); err == nil {
		return ok(w, t) // offline retry
	}
	if in.Kind == "" {
		in.Kind = "expense"
	}
	if !slices.Contains([]string{"expense", "income", "transfer"}, in.Kind) {
		return errBadRequest("Kind must be expense, income or transfer")
	}
	if in.Amount == 0 || in.Amount > 1e12 || in.Amount < -1e12 {
		return errBadRequest("Enter an amount")
	}
	if in.Date == "" {
		h, err := s.getHousehold(ctx, u.HouseholdID)
		if err != nil {
			return err
		}
		in.Date = localToday(h, time.Now()).Format(time.DateOnly)
	} else if _, err := parseDate(in.Date); err != nil {
		return err
	}
	in.Payee = truncate(strings.TrimSpace(in.Payee), 80)
	if in.CategoryID == nil && in.Payee != "" && in.Kind != "transfer" {
		if c, _ := s.classify(ctx, u, in.Payee, in.Kind == "expense"); c != nil {
			in.CategoryID = &c.categoryID
		}
	}
	if err := s.checkRefs(ctx, u, &in); err != nil {
		return err
	}
	var owner *string
	if in.Personal {
		owner = &u.ID
	}
	if _, err := s.db.Exec(ctx, `INSERT INTO money_transactions (id, household_id, owner_id, kind, amount, date, payee, note, category_id,
		account_id, paid_by, receipt, source, created_by) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, 'manual', $13)`,
		in.ID, u.HouseholdID, owner, in.Kind, in.Amount, in.Date, in.Payee, truncate(in.Note, 300), in.CategoryID, in.AccountID, in.PaidBy, in.Receipt, u.ID); err != nil {
		return err
	}
	t, err := s.getTxn(ctx, u, in.ID)
	if err != nil {
		return err
	}
	s.publishMoney(u, r)
	s.checkBudgetAlerts(ctx, u.HouseholdID, t.CategoryID, t.Date)
	return writeJSON(w, http.StatusCreated, t)
}

func (s *Server) getTxn(ctx context.Context, u *User, id string) (*Transaction, error) {
	rows, _ := s.db.Query(ctx, `SELECT `+txnColumns+` FROM money_transactions WHERE id = $1 AND household_id = $3 AND `+visible, id, u.ID, u.HouseholdID)
	return pgx.CollectExactlyOneRow(rows, pgx.RowToAddrOfStructByName[Transaction])
}

func (s *Server) updateTransaction(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	t, err := s.getTxn(ctx, u, chi.URLParam(r, "id"))
	if err != nil {
		return err
	}
	in := txnInput{ID: t.ID, Kind: t.Kind, Amount: t.Amount, Date: t.Date, Payee: t.Payee, Note: t.Note, CategoryID: t.CategoryID,
		AccountID: t.AccountID, PaidBy: t.PaidBy, Receipt: t.Receipt, Personal: t.OwnerID != nil}
	var patch struct {
		Kind       *string        `json:"kind"`
		Amount     *int64         `json:"amount"`
		Date       *string        `json:"date"`
		Payee      *string        `json:"payee"`
		Note       *string        `json:"note"`
		CategoryID optionalString `json:"categoryId"`
		AccountID  optionalString `json:"accountId"`
		PaidBy     *string        `json:"paidBy"`
		Receipt    *string        `json:"receipt"`
		Personal   *bool          `json:"personal"`
		Learn      bool           `json:"learn"` // remember payee → category for future imports
	}
	patch.CategoryID.v, patch.AccountID.v = t.CategoryID, t.AccountID
	if err := decode(r, &patch); err != nil {
		return err
	}
	if patch.Kind != nil {
		if !slices.Contains([]string{"expense", "income", "transfer"}, *patch.Kind) {
			return errBadRequest("Invalid kind")
		}
		in.Kind = *patch.Kind
	}
	if patch.Amount != nil {
		if *patch.Amount == 0 {
			return errBadRequest("Enter an amount")
		}
		in.Amount = *patch.Amount
	}
	if patch.Date != nil {
		if _, err := parseDate(*patch.Date); err != nil {
			return err
		}
		in.Date = *patch.Date
	}
	if patch.Payee != nil {
		in.Payee = truncate(strings.TrimSpace(*patch.Payee), 80)
	}
	if patch.Note != nil {
		in.Note = truncate(*patch.Note, 300)
	}
	in.CategoryID, in.AccountID = patch.CategoryID.v, patch.AccountID.v
	if patch.PaidBy != nil {
		in.PaidBy = patch.PaidBy
	}
	if patch.Receipt != nil {
		in.Receipt = *patch.Receipt
	}
	if patch.Personal != nil {
		in.Personal = *patch.Personal
	}
	if err := s.checkRefs(ctx, u, &in); err != nil {
		return err
	}
	var owner *string
	if in.Personal {
		owner = &u.ID
	}
	if _, err := s.db.Exec(ctx, `UPDATE money_transactions SET owner_id = $2, kind = $3, amount = $4, date = $5, payee = $6, note = $7,
		category_id = $8, account_id = $9, paid_by = $10, receipt = $11, updated_at = now() WHERE id = $1`,
		t.ID, owner, in.Kind, in.Amount, in.Date, in.Payee, in.Note, in.CategoryID, in.AccountID, in.PaidBy, in.Receipt); err != nil {
		return err
	}
	if patch.Learn && in.CategoryID != nil {
		key := ruleKey(in.Payee)
		if key != "" {
			_, err := s.db.Exec(ctx, `INSERT INTO money_rules (household_id, owner_id, pattern, category_id, kind) VALUES ($1, $2, $3, $4, $5)
				ON CONFLICT (household_id, coalesce(owner_id, '00000000-0000-0000-0000-000000000000'::uuid), pattern)
				DO UPDATE SET category_id = EXCLUDED.category_id, kind = EXCLUDED.kind`, u.HouseholdID, owner, key, *in.CategoryID, in.Kind)
			logErr("learn rule", err)
		}
	}
	updated, err := s.getTxn(ctx, u, t.ID)
	if err != nil {
		return err
	}
	s.publishMoney(u, r)
	s.checkBudgetAlerts(ctx, u.HouseholdID, updated.CategoryID, updated.Date)
	return ok(w, updated)
}

func (s *Server) deleteTransaction(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM money_transactions WHERE id = $1 AND household_id = $3 AND `+visible,
		chi.URLParam(r, "id"), u.ID, u.HouseholdID); err != nil {
		return err
	}
	s.publishMoney(u, r)
	return noContent(w)
}

// ---- categorization ----

type match struct {
	categoryID string
	kind       string
	source     string // "rule" or "builtin"
}

// Built-in hints for common Indian merchants, mapped to default category names.
var merchantHints = []struct {
	category string
	kind     string
	words    []string
}{
	{"", "transfer", []string{"credit card payment", "credit card", "credit c", "creditcard", "cc payment", "card payment", "cred club", "cred.club", "billdesk-cc", "autopay si", "self transfer", "own account",
		"national pension", "npstrust", "nps", "zerodha", "groww", "upstox", "kuvera", "mutual fund", "indian clearing", "iccl", "sip", "ppf", "fixed deposit", "recurring deposit"}},
	{"Income", "income", []string{"salary", "sal cr", "interest credit", "interest paid", "int.pd", "int pd", "dividend", "refund"}},
	{"Misc", "expense", []string{"atm cash withdrawal"}},
	{"Groceries", "expense", []string{"bigbasket", "blinkit", "zepto", "instamart", "dmart", "avenue supermarts", "jiomart", "more retail", "reliance fresh", "reliance smart", "spencer", "star bazaar", "nature basket", "grofers", "kirana", "milkbasket", "country delight"}},
	{"Vegetables & Fruits", "expense", []string{"vegetable", "sabzi", "fruits", "otipy", "kisankonnect"}},
	{"Dining Out", "expense", []string{"swiggy", "zomato", "eatsure", "dominos", "domino's", "mcdonald", "kfc", "pizza hut", "starbucks", "cafe coffee day", "ccd", "haldiram", "restaurant", "dhaba", "biryani", "chaayos", "burger king", "subway", "barbeque",
		"food court", "canteen", "cafe", "tea", "chai", "bakery", "sweets", "juice", "darshini", "tiffin", "mess"}},
	{"Transport", "expense", []string{"uber", "ola", "rapido", "irctc", "redbus", "indigo", "air india", "vistara", "akasa", "metro", "fastag", "petrol", "fuel", "hpcl", "bpcl", "iocl", "indian oil", "shell", "parking", "makemytrip", "cleartrip", "ixigo"}},
	{"Utilities", "expense", []string{"electricity", "cesc", "bescom", "tneb", "msedcl", "tata power", "adani elec", "torrent power", "wbsedcl", "bses", "water bill", "gas bill", "indane", "bharat gas", "hp gas", "mahanagar gas", "igl", "airtel", "jio", "vodafone", "vi prepaid", "bsnl", "act fibernet", "hathway", "tata play", "dish tv", "broadband", "recharge", "utility", "tv-internet", "internet", "postpaid"}},
	{"Subscriptions", "expense", []string{"netflix", "prime video", "amazon prime", "hotstar", "disney", "spotify", "youtube", "google one", "google play", "apple.com", "icloud", "sonyliv", "zee5", "jiocinema", "audible", "linkedin", "chatgpt", "openai", "anthropic"}},
	{"Rent / EMI", "expense", []string{"rent", "emi", "loan", "nobroker", "society maintenance", "maintenance"}},
	{"Health", "expense", []string{"apollo", "pharmeasy", "1mg", "tata 1mg", "netmeds", "medplus", "hospital", "clinic", "diagnostic", "lal path", "thyrocare", "practo", "pharmacy", "chemist", "medical"}},
	{"Shopping", "expense", []string{"amazon", "flipkart", "myntra", "ajio", "nykaa", "meesho", "tata cliq", "croma", "reliance digital", "decathlon", "ikea", "lifestyle", "westside", "pantaloons", "lenskart", "firstcry",
		"bastralaya", "vastralaya", "textiles", "garments", "fashion", "saree", "mi com", "xiaomi", "samsung", "apple store"}},
	{"Household", "expense", []string{"urban company", "urbanclap", "laundry", "dry clean", "housejoy", "hardware", "pepperfry", "home centre"}},
}

// ruleKey is the learned-rule pattern for a payee: lowercase, digits stripped.
func ruleKey(payee string) string {
	k := strings.ToLower(strings.TrimSpace(digitRun.ReplaceAllString(payee, "")))
	k = strings.Join(strings.Fields(k), " ")
	if len(k) < 3 {
		return ""
	}
	return truncate(k, 60)
}

// classify picks a category for a narration: learned rules first (longest pattern
// wins), then built-in merchant hints matched against category names.
func (s *Server) classify(ctx context.Context, u *User, text string, debit bool) (*match, error) {
	low := strings.ToLower(text)
	rows, err := s.db.Query(ctx, `SELECT pattern, category_id, kind FROM money_rules WHERE household_id = $1 AND `+visible+`
		ORDER BY length(pattern) DESC`, u.HouseholdID, u.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var p, cid, kind string
		if err := rows.Scan(&p, &cid, &kind); err != nil {
			rows.Close()
			return nil, err
		}
		if strings.Contains(low, p) {
			rows.Close()
			return &match{cid, kind, "rule"}, nil
		}
	}
	rows.Close()
	cats, err := s.moneyCategories(ctx, u)
	if err != nil {
		return nil, err
	}
	byName := map[string]string{}
	for _, c := range cats {
		if !c.Archived {
			byName[strings.ToLower(c.Name)] = c.ID
		}
	}
	for _, h := range merchantHints {
		if h.kind == "income" && debit || h.kind == "expense" && !debit && h.category != "Shopping" {
			continue
		}
		for _, w := range h.words {
			if containsWord(low, w) {
				if h.kind == "transfer" {
					return &match{"", "transfer", "builtin"}, nil
				}
				if id, found := byName[strings.ToLower(h.category)]; found {
					return &match{id, h.kind, "builtin"}, nil
				}
			}
		}
	}
	return nil, nil
}

// isMemberPayee reports whether a payee is one of you (e.g. "Asha Anand" when
// moving money to your own account at another bank). That's a transfer, not spending.
// Only the payee is checked: NEFT narrations also carry the account holder's own name.
func (s *Server) isMemberPayee(ctx context.Context, householdID, payee string) bool {
	low := strings.ToLower(payee)
	rows, err := s.db.Query(ctx, `SELECT display_name FROM members WHERE household_id = $1`, householdID)
	if err != nil {
		return false
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		rows.Scan(&name)
		if n := strings.ToLower(strings.TrimSpace(name)); len(n) >= 4 && containsWord(low, n) {
			return true
		}
	}
	return false
}

func containsWord(text, w string) bool {
	i := strings.Index(text, w)
	for i >= 0 {
		before := i == 0 || !isAlnum(text[i-1])
		end := i + len(w)
		after := end >= len(text) || !isAlnum(text[end])
		if before && after {
			return true
		}
		next := strings.Index(text[i+1:], w)
		if next < 0 {
			return false
		}
		i += next + 1
	}
	return false
}

func isAlnum(b byte) bool { return b >= 'a' && b <= 'z' || b >= '0' && b <= '9' }

func (s *Server) classifyEndpoint(w http.ResponseWriter, r *http.Request) error {
	m, err := s.classify(r.Context(), currentUser(r), r.URL.Query().Get("text"), r.URL.Query().Get("kind") != "income")
	if err != nil {
		return err
	}
	if m == nil {
		return ok(w, map[string]any{"categoryId": nil})
	}
	return ok(w, map[string]any{"categoryId": nilIfEmpty(m.categoryID), "kind": m.kind, "source": m.source})
}

type MoneyRule struct {
	ID         string    `json:"id" db:"id"`
	OwnerID    *string   `json:"ownerId" db:"owner_id"`
	Pattern    string    `json:"pattern" db:"pattern"`
	CategoryID string    `json:"categoryId" db:"category_id"`
	Kind       string    `json:"kind" db:"kind"`
	CreatedAt  time.Time `json:"createdAt" db:"created_at"`
}

func (s *Server) listRules(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT id, owner_id, pattern, category_id, kind, created_at FROM money_rules
		WHERE household_id = $1 AND `+visible+` ORDER BY pattern`, u.HouseholdID, u.ID)
	out, err := pgx.CollectRows(rows, pgx.RowToStructByName[MoneyRule])
	if err != nil {
		return err
	}
	return ok(w, out)
}

func (s *Server) deleteRule(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	if _, err := s.db.Exec(r.Context(), `DELETE FROM money_rules WHERE id = $1 AND household_id = $3 AND `+visible, chi.URLParam(r, "id"), u.ID, u.HouseholdID); err != nil {
		return err
	}
	s.publishMoney(u, r)
	return noContent(w)
}

// ---- monthly summary ----

type CategorySummary struct {
	CategoryID string `json:"categoryId"`
	Limit      int64  `json:"limit"`
	Carry      int64  `json:"carry"` // rolled over from earlier months (can be negative)
	Spent      int64  `json:"spent"`
	Count      int    `json:"count"`
}

func (s *Server) moneySummary(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	month, err := parseMonth(r.URL.Query().Get("month"))
	if err != nil {
		return err
	}
	cats, err := s.moneyCategories(ctx, u)
	if err != nil {
		return err
	}
	from := month.AddDate(-12, 0, 0)
	next := month.AddDate(0, 1, 0)

	// Spend per category per month over the last year (for rollover) + this month.
	type key struct {
		cat   string
		month string
	}
	spent := map[key]int64{}
	counts := map[string]int{}
	rows, err := s.db.Query(ctx, `SELECT coalesce(category_id::text, ''), to_char(date, 'YYYY-MM'), sum(amount), count(*)
		FROM money_transactions WHERE household_id = $1 AND `+visible+` AND kind = 'expense' AND date >= $3 AND date < $4
		GROUP BY 1, 2`, u.HouseholdID, u.ID, from, next)
	if err != nil {
		return err
	}
	cur := month.Format("2006-01")
	for rows.Next() {
		var c, m string
		var sum int64
		var n int
		if err := rows.Scan(&c, &m, &sum, &n); err != nil {
			rows.Close()
			return err
		}
		spent[key{c, m}] = sum
		if m == cur {
			counts[c] = n
		}
	}
	rows.Close()
	overrides := map[key]int64{}
	rows, err = s.db.Query(ctx, `SELECT b.category_id::text, to_char(b.month, 'YYYY-MM'), b.amount FROM money_budget_months b
		JOIN money_categories c ON c.id = b.category_id WHERE c.household_id = $1 AND (c.owner_id IS NULL OR c.owner_id = $2) AND b.month >= $3 AND b.month < $4`,
		u.HouseholdID, u.ID, from, next)
	if err != nil {
		return err
	}
	for rows.Next() {
		var c, m string
		var a int64
		if err := rows.Scan(&c, &m, &a); err != nil {
			rows.Close()
			return err
		}
		overrides[key{c, m}] = a
	}
	rows.Close()

	limitFor := func(c MoneyCategory, m string) int64 {
		if v, found := overrides[key{c.ID, m}]; found {
			return v
		}
		return c.MonthlyLimit
	}
	var out []CategorySummary
	var budgeted, total int64
	for _, c := range cats {
		if c.Kind != "expense" {
			continue
		}
		cs := CategorySummary{CategoryID: c.ID, Limit: limitFor(c, cur), Spent: spent[key{c.ID, cur}], Count: counts[c.ID]}
		if c.Rollover {
			created := time.Date(c.CreatedAt.Year(), c.CreatedAt.Month(), 1, 0, 0, 0, 0, time.UTC)
			for m := maxDate(from, created); m.Before(month); m = m.AddDate(0, 1, 0) {
				ms := m.Format("2006-01")
				cs.Carry += limitFor(c, ms) - spent[key{c.ID, ms}]
			}
		}
		if c.Archived && cs.Spent == 0 {
			continue
		}
		budgeted += cs.Limit
		out = append(out, cs)
	}
	for k, v := range spent {
		if k.month == cur {
			total += v
		}
	}

	// Income, who paid, daily spend, six-month trend and top payees.
	var income int64
	s.db.QueryRow(ctx, `SELECT coalesce(sum(amount), 0) FROM money_transactions WHERE household_id = $1 AND `+visible+`
		AND kind = 'income' AND date >= $3 AND date < $4`, u.HouseholdID, u.ID, month, next).Scan(&income)
	var uncategorized int
	s.db.QueryRow(ctx, `SELECT count(*) FROM money_transactions WHERE household_id = $1 AND `+visible+`
		AND kind <> 'transfer' AND category_id IS NULL AND date >= $3 AND date < $4`, u.HouseholdID, u.ID, month, next).Scan(&uncategorized)

	byPerson := map[string]int64{}
	rows, err = s.db.Query(ctx, `SELECT coalesce(paid_by::text, ''), sum(amount) FROM money_transactions WHERE household_id = $1 AND `+visible+`
		AND kind = 'expense' AND date >= $3 AND date < $4 GROUP BY 1`, u.HouseholdID, u.ID, month, next)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p string
		var v int64
		rows.Scan(&p, &v)
		byPerson[p] = v
	}
	rows.Close()

	days := next.AddDate(0, 0, -1).Day()
	daily := make([]int64, days)
	rows, err = s.db.Query(ctx, `SELECT extract(day FROM date)::int, sum(amount) FROM money_transactions WHERE household_id = $1 AND `+visible+`
		AND kind = 'expense' AND date >= $3 AND date < $4 GROUP BY 1`, u.HouseholdID, u.ID, month, next)
	if err != nil {
		return err
	}
	for rows.Next() {
		var d int
		var v int64
		rows.Scan(&d, &v)
		if d >= 1 && d <= days {
			daily[d-1] = v
		}
	}
	rows.Close()

	type monthTotal struct {
		Month  string `json:"month"`
		Spent  int64  `json:"spent"`
		Income int64  `json:"income"`
	}
	var trend []monthTotal
	for i := 5; i >= 0; i-- {
		m := month.AddDate(0, -i, 0).Format("2006-01")
		var t int64
		for k, v := range spent {
			if k.month == m {
				t += v
			}
		}
		trend = append(trend, monthTotal{Month: m, Spent: t})
	}
	rows, err = s.db.Query(ctx, `SELECT to_char(date, 'YYYY-MM'), sum(amount) FROM money_transactions WHERE household_id = $1 AND `+visible+`
		AND kind = 'income' AND date >= $3 AND date < $4 GROUP BY 1`, u.HouseholdID, u.ID, month.AddDate(0, -5, 0), next)
	if err == nil {
		for rows.Next() {
			var m string
			var v int64
			rows.Scan(&m, &v)
			for i := range trend {
				if trend[i].Month == m {
					trend[i].Income = v
				}
			}
		}
		rows.Close()
	}

	type payee struct {
		Payee  string `json:"payee"`
		Amount int64  `json:"amount"`
		Count  int    `json:"count"`
	}
	var payees []payee
	rows, err = s.db.Query(ctx, `SELECT payee, sum(amount), count(*) FROM money_transactions WHERE household_id = $1 AND `+visible+`
		AND kind = 'expense' AND payee <> '' AND date >= $3 AND date < $4 GROUP BY payee ORDER BY 2 DESC LIMIT 6`, u.HouseholdID, u.ID, month, next)
	if err != nil {
		return err
	}
	for rows.Next() {
		var p payee
		rows.Scan(&p.Payee, &p.Amount, &p.Count)
		payees = append(payees, p)
	}
	rows.Close()

	return ok(w, map[string]any{
		"month": cur, "categories": out, "budgeted": budgeted, "spent": total, "income": income,
		"uncategorized": uncategorized, "byPerson": byPerson, "daily": daily, "trend": trend, "topPayees": payees,
	})
}

// checkBudgetAlerts notifies when a category crosses 80% or 100% of its monthly budget.
// Shared categories notify both members; personal ones only their owner.
func (s *Server) checkBudgetAlerts(ctx context.Context, householdID string, categoryID *string, date string) {
	if categoryID == nil {
		return
	}
	d, err := time.Parse(time.DateOnly, date)
	if err != nil {
		return
	}
	month := time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, time.UTC)
	var name string
	var owner *string
	var limit, spent int64
	err = s.db.QueryRow(ctx, `SELECT c.name, c.owner_id,
		coalesce((SELECT amount FROM money_budget_months WHERE category_id = c.id AND month = $2), c.monthly_limit),
		coalesce((SELECT sum(amount) FROM money_transactions t WHERE t.category_id = c.id AND t.kind = 'expense' AND t.date >= $2 AND t.date < $3), 0)
		FROM money_categories c WHERE c.id = $1 AND c.kind = 'expense'`, *categoryID, month, month.AddDate(0, 1, 0)).Scan(&name, &owner, &limit, &spent)
	if err != nil || limit <= 0 {
		return
	}
	pct := spent * 100 / limit
	level := 0
	switch {
	case pct >= 100:
		level = 100
	case pct >= 80:
		level = 80
	default:
		return
	}
	n := notification{
		HouseholdID: householdID,
		Kind:        "budget",
		DedupeKey:   fmt.Sprintf("budget:%s:%s:%d", *categoryID, month.Format("2006-01"), level),
		URL:         "/budget",
	}
	if level == 100 {
		n.Title = fmt.Sprintf("%s budget used up", name)
		n.Body = fmt.Sprintf("%s spent of %s this month (%d%%).", formatRupees(spent), formatRupees(limit), pct)
	} else {
		n.Title = fmt.Sprintf("%s at %d%% of budget", name, pct)
		n.Body = fmt.Sprintf("%s left of %s this month.", formatRupees(limit-spent), formatRupees(limit))
	}
	if owner != nil {
		n.UserID = *owner
		logErr("budget alert", s.notify(ctx, n))
		return
	}
	logErr("budget alert", s.notifyHousehold(ctx, householdID, n))
}

// formatRupees renders paise as "₹1,23,456" with Indian digit grouping.
func formatRupees(paise int64) string {
	neg := paise < 0
	if neg {
		paise = -paise
	}
	rupees := (paise + 50) / 100
	s := fmt.Sprint(rupees)
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		var parts []string
		for len(head) > 2 {
			parts = append([]string{head[len(head)-2:]}, parts...)
			head = head[:len(head)-2]
		}
		if head != "" {
			parts = append([]string{head}, parts...)
		}
		s = strings.Join(parts, ",") + "," + tail
	}
	if neg {
		return "-₹" + s
	}
	return "₹" + s
}
