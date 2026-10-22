package api

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

const hdfcSample = `HDFC BANK Ltd.,,,,,,
Date,Narration,Chq./Ref.No.,Value Dt,Withdrawal Amt.,Deposit Amt.,Closing Balance
01/10/26,UPI-SWIGGY-SWIGGY8@YBL-YESB0YBLUPI-427512345678-PAYMENT FROM PHONE,0000427512345678,01/10/26,456.00,,"1,23,544.00"
02/10/26,NEFT CR-CITI0000001-ACME TECHNOLOGIES PVT LTD-ASHA-CITIN52026100212345,CITIN52026100212345,02/10/26,,"1,50,000.00","2,73,544.00"
03/10/26,UPI-RAMU TEA STALL-RAMU@OKSBI-SBIN0001234-427599990000-UPI,0000427599990000,03/10/26,40.00,,"2,73,504.00"
03/10/26,UPI-RAMU TEA STALL-RAMU@OKSBI-SBIN0001234-427599990000-UPI,0000427599990000,03/10/26,40.00,,"2,73,464.00"
04/10/26,CC 000412XXXXXX1234 AUTOPAY SI-TAD,0000000000000000,04/10/26,"12,000.00",,"2,61,464.00"
05/10/26,UPI-ASHA ANAND-9000000001@AXL-UBIN0500001-400000000001-PAYMENT FROM PHONE,0000400000000001,05/10/26,"5,000.00",,"2,56,464.00"`

func TestMoneyFlow(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	s, ts, db := startTestServer(t, 54331)
	a := newClient(t, ts.URL)
	a.do("POST", "/api/setup", map[string]any{"householdName": "Home", "username": "asha", "displayName": "Asha", "password": "hunter2hunter2"}, 204, nil)
	var inv map[string]string
	a.do("POST", "/api/invites", nil, 200, &inv)
	b := newClient(t, ts.URL)
	b.do("POST", "/api/auth/invite/accept", map[string]any{"code": inv["code"], "username": "partner", "displayName": "Partner", "password": "correcthorse"}, 204, nil)

	var cats []MoneyCategory
	a.do("GET", "/api/money/categories", nil, 200, &cats)
	if len(cats) != 12 {
		t.Fatalf("default categories: %d", len(cats))
	}
	catID := map[string]string{}
	for _, c := range cats {
		catID[c.Name] = c.ID
	}
	month := time.Now().Format("2006-01")
	today := time.Now().Format(time.DateOnly)

	// Budget + 80% alert to both members.
	a.do("PATCH", "/api/money/categories/"+catID["Groceries"], map[string]any{"monthlyLimit": 1000000}, 200, nil)
	var t1 Transaction
	a.do("POST", "/api/money/transactions", map[string]any{"amount": 850000, "categoryId": catID["Groceries"], "payee": "DMart"}, 201, &t1)
	if t1.Date != today || *t1.PaidBy == "" || t1.Kind != "expense" {
		t.Fatalf("txn defaults: %+v", t1)
	}
	for _, c := range []*testClient{a, b} {
		var inbox struct{ Items []inboxItem }
		c.do("GET", "/api/notifications", nil, 200, &inbox)
		if len(inbox.Items) == 0 || !strings.Contains(inbox.Items[0].Title, "Groceries at 85%") {
			t.Fatalf("budget alert missing: %+v", inbox.Items)
		}
	}
	// Payee-based auto category on manual entry.
	var t2 Transaction
	a.do("POST", "/api/money/transactions", map[string]any{"amount": 35000, "payee": "Swiggy"}, 201, &t2)
	if t2.CategoryID == nil || *t2.CategoryID != catID["Dining Out"] {
		t.Fatalf("auto category: %+v", t2)
	}

	// Privacy: the partner's personal category and spending stay invisible to Sayan.
	var pc MoneyCategory
	b.do("POST", "/api/money/categories", map[string]any{"name": "My hobbies", "personal": true, "monthlyLimit": 300000}, 200, &pc)
	var pt Transaction
	b.do("POST", "/api/money/transactions", map[string]any{"amount": 120000, "categoryId": pc.ID, "payee": "Guitar strings"}, 201, &pt)
	if pt.OwnerID == nil {
		t.Fatal("transaction in a personal category must be personal")
	}
	a.do("GET", "/api/money/categories", nil, 200, &cats)
	for _, c := range cats {
		if c.ID == pc.ID {
			t.Fatal("partner's personal category leaked")
		}
	}
	var list []Transaction
	a.do("GET", "/api/money/transactions?month="+month, nil, 200, &list)
	for _, x := range list {
		if x.ID == pt.ID {
			t.Fatal("partner's personal transaction leaked")
		}
	}
	a.do("PATCH", "/api/money/transactions/"+pt.ID, map[string]any{"note": "x"}, 404, nil)
	a.do("POST", "/api/money/transactions", map[string]any{"amount": 100, "categoryId": pc.ID}, 400, nil)
	var bsum map[string]any
	b.do("GET", "/api/money/summary?month="+month, nil, 200, &bsum)
	if bsum["spent"].(float64) != 850000+35000+120000 {
		t.Fatalf("partner summary spent: %v", bsum["spent"])
	}
	var asum map[string]any
	a.do("GET", "/api/money/summary?month="+month, nil, 200, &asum)
	if asum["spent"].(float64) != 850000+35000 {
		t.Fatalf("sayan summary spent: %v", asum["spent"])
	}

	// Statement import: preview, commit, re-import is all duplicates, undo.
	var acc map[string]string
	a.do("POST", "/api/money/accounts", map[string]any{"name": "HDFC Savings", "kind": "bank"}, 200, &acc)
	prev := importPreview(t, a, acc["id"], hdfcSample)
	if len(prev.Rows) != 6 || prev.Rows[5].Kind != "transfer" {
		t.Fatalf("preview rows: %+v", prev)
	}
	r := prev.Rows
	if r[0].Payee != "Swiggy" || r[0].CategoryID == nil || *r[0].CategoryID != catID["Dining Out"] || r[0].Kind != "expense" {
		t.Fatalf("swiggy row: %+v", r[0])
	}
	if r[1].Kind != "income" || r[1].CategoryID == nil || *r[1].CategoryID != catID["Income"] {
		t.Fatalf("salary row: %+v", r[1])
	}
	if r[2].Hash == r[3].Hash {
		t.Fatal("identical same-day rows must hash differently")
	}
	if r[4].Kind != "transfer" {
		t.Fatalf("card autopay should be a transfer: %+v", r[4])
	}
	var res map[string]any
	a.do("POST", "/api/money/import/commit", map[string]any{"accountId": acc["id"], "filename": "oct.csv", "mapping": prev.Mapping, "rows": r}, 200, &res)
	if res["imported"].(float64) != 6 {
		t.Fatalf("commit: %v", res)
	}
	again := importPreview(t, a, acc["id"], hdfcSample)
	for _, x := range again.Rows {
		if !x.Duplicate || x.Include {
			t.Fatalf("re-import should be all duplicates: %+v", x)
		}
	}
	a.do("POST", "/api/money/import/commit", map[string]any{"accountId": acc["id"], "rows": r}, 200, &res)
	if res["imported"].(float64) != 0 {
		t.Fatalf("duplicate commit imported %v", res["imported"])
	}

	// Learn: categorize the tea stall once, the next import picks it up.
	a.do("GET", "/api/money/transactions?month=2026-10&q=ramu", nil, 200, &list)
	if len(list) != 2 {
		t.Fatalf("tea rows: %d", len(list))
	}
	a.do("PATCH", "/api/money/transactions/"+list[0].ID, map[string]any{"categoryId": catID["Dining Out"], "learn": true}, 200, nil)
	var rules []MoneyRule
	a.do("GET", "/api/money/rules", nil, 200, &rules)
	if len(rules) != 1 || rules[0].Pattern != "ramu tea stall" {
		t.Fatalf("rules: %+v", rules)
	}
	var imports []MoneyImport
	a.do("GET", "/api/money/imports", nil, 200, &imports)
	a.do("DELETE", "/api/money/imports/"+imports[len(imports)-1].ID, nil, 204, nil)
	learned := importPreview(t, a, acc["id"], hdfcSample)
	if learned.Rows[2].Matched != "rule" || *learned.Rows[2].CategoryID != catID["Dining Out"] || learned.Rows[2].Duplicate {
		t.Fatalf("learned rule / undo: %+v", learned.Rows[2])
	}

	// Rollover: unused budget from last month carries forward.
	var rc MoneyCategory
	a.do("POST", "/api/money/categories", map[string]any{"name": "Gifts", "monthlyLimit": 200000, "rollover": true}, 200, &rc)
	db.Exec(t.Context(), `UPDATE money_categories SET created_at = now() - interval '40 days' WHERE id = $1`, rc.ID)
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01") + "-10"
	a.do("POST", "/api/money/transactions", map[string]any{"amount": 50000, "categoryId": rc.ID, "date": lastMonth}, 201, nil)
	a.do("GET", "/api/money/summary?month="+month, nil, 200, &asum)
	found := false
	for _, c := range asum["categories"].([]any) {
		m := c.(map[string]any)
		if m["categoryId"] == rc.ID {
			found = true
			if m["carry"].(float64) != 150000 {
				t.Fatalf("carry: %v", m)
			}
		}
	}
	if !found {
		t.Fatal("rollover category missing from summary")
	}

	// Bills: reminder 2 days ahead, paying records an expense and stops reminders.
	h, _ := s.getHousehold(t.Context(), func() string { var id string; db.QueryRow(t.Context(), `SELECT id FROM households`).Scan(&id); return id }())
	day := localToday(h, time.Now())
	due := day.AddDate(0, 0, 2)
	if due.Month() != day.Month() {
		due = day // keep it in this month for the test
	}
	var bill map[string]string
	a.do("POST", "/api/money/bills", map[string]any{"name": "Electricity", "amount": 184000, "dueDay": due.Day(), "categoryId": catID["Utilities"], "remindDaysBefore": 3}, 200, &bill)
	if err := s.billReminders(t.Context(), h, day); err != nil {
		t.Fatal(err)
	}
	var inbox struct{ Items []inboxItem }
	b.do("GET", "/api/notifications", nil, 200, &inbox)
	if !strings.Contains(inbox.Items[0].Title, "Electricity") {
		t.Fatalf("bill reminder: %+v", inbox.Items[0])
	}
	a.do("POST", "/api/money/bills/"+bill["id"]+"/pay", map[string]any{}, 200, nil)
	var bills []Bill
	a.do("GET", "/api/money/bills", nil, 200, &bills)
	if bills[0].LastPaidMonth == nil || *bills[0].LastPaidMonth != day.Format("2006-01") {
		t.Fatalf("bill not marked paid: %+v", bills[0])
	}

	// Grocery trip with a total becomes a Groceries expense.
	var gi GroceryItem
	a.do("POST", "/api/grocery", map[string]any{"text": "1 kg onions"}, 201, &gi)
	a.do("PATCH", "/api/grocery/"+gi.ID, map[string]any{"checked": true}, 200, nil)
	a.do("POST", "/api/grocery/complete", map[string]any{"total": 486.5, "store": "DMart"}, 200, nil)
	a.do("GET", "/api/money/transactions?month="+month+"&category="+catID["Groceries"], nil, 200, &list)
	if len(list) != 2 || list[0].Amount != 48650 || list[0].Source != "grocery" {
		t.Fatalf("grocery expense: %+v", list)
	}
	// Invoice → expense + pantry; the same order can't be recorded twice; the bank line
	// that paid for it is recognised on statement import.
	invBody := map[string]any{"amount": 31000, "date": "2026-10-20", "payee": "Zepto", "orderId": "ORD123", "categoryId": catID["Groceries"],
		"pantry": []map[string]any{{"name": "Potato chips", "quantity": 160, "unit": "g"}}}
	a.do("POST", "/api/money/invoice/commit", invBody, 200, nil)
	a.do("POST", "/api/money/invoice/commit", invBody, 409, nil)
	var pantry []PantryItem
	a.do("GET", "/api/pantry", nil, 200, &pantry)
	if pantryQty(pantry, "Potato chips") != 160 {
		t.Fatalf("invoice pantry: %+v", pantry)
	}
	stmt := "Date,Narration,Withdrawal Amt.,Deposit Amt.\n21/10/26,UPI-ZEPTO MARKETPLACE PRIVATE LIMITED-ZEPTO.PAYU@HDFCBANK-HDFC0MERUPI-123456789012-UPIINTENT,310.00,\n"
	pv := importPreview(t, a, acc["id"], stmt)
	if len(pv.Rows) != 1 || pv.Rows[0].AlreadyRecorded == "" || pv.Rows[0].Include {
		t.Fatalf("statement match: %+v", pv.Rows)
	}

	if formatRupees(12345678) != "₹1,23,457" || formatRupees(99) != "₹1" || formatRupees(100000000) != "₹10,00,000" {
		t.Fatalf("rupee format: %s %s %s", formatRupees(12345678), formatRupees(99), formatRupees(100000000))
	}
}

type previewResult struct {
	Mapping json.RawMessage
	Rows    []importRow
}

func importPreview(t *testing.T, c *testClient, accountID, csv string) previewResult {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "statement.csv")
	io.WriteString(fw, csv)
	mw.WriteField("accountId", accountID)
	mw.Close()
	req, _ := http.NewRequest("POST", c.base+"/api/money/import/preview", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Requested-With", "homeops")
	resp, err := c.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("preview: %d %s", resp.StatusCode, raw)
	}
	var out previewResult
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
