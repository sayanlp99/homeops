package api

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestFridgeNotes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	srv, ts, db := startTestServer(t, 54336)
	a := newClient(t, ts.URL)
	a.do("POST", "/api/setup", map[string]any{"householdName": "Home", "email": "asha@example.com", "username": "asha", "displayName": "Asha", "password": "hunter2hunter2"}, 204, nil)
	var inv map[string]any
	a.do("POST", "/api/invites", map[string]string{"email": "ravi@example.com"}, 200, &inv)
	b := newClient(t, ts.URL)
	b.do("POST", "/api/auth/invite/accept", map[string]any{"code": inv["code"], "username": "ravi", "displayName": "Ravi", "password": "correcthorse"}, 204, nil)

	a.do("POST", "/api/notes", map[string]any{"text": "   "}, 400, nil)
	a.do("POST", "/api/notes", map[string]any{"text": "Hi", "color": "purple"}, 400, nil)
	a.do("POST", "/api/notes", map[string]any{"text": strings.Repeat("দ", 501)}, 400, nil)

	var milk, gas FridgeNote
	a.do("POST", "/api/notes", map[string]any{"id": "6f1c1c39-6d7e-4a8e-9b3e-1d7a5b0c2e11", "text": " Buy milk on the way home "}, 201, &milk)
	if milk.Text != "Buy milk on the way home" || milk.Color != "yellow" || milk.Pinned {
		t.Fatalf("note: %+v", milk)
	}
	var again FridgeNote
	a.do("POST", "/api/notes", map[string]any{"id": milk.ID, "text": "Buy milk on the way home"}, 200, &again) // offline retry
	b.do("POST", "/api/notes", map[string]any{"text": "গ্যাস বুক করতে হবে " + strings.Repeat("🔥", 150), "color": "pink"}, 201, &gas)

	noteAlerts := func(c *testClient) []inboxItem {
		var inbox struct{ Items []inboxItem }
		c.do("GET", "/api/notifications", nil, 200, &inbox)
		var out []inboxItem
		for _, n := range inbox.Items {
			if n.Kind == "notes" {
				out = append(out, n)
			}
		}
		return out
	}
	if n := noteAlerts(b); len(n) != 1 || !strings.Contains(n[0].Title, "Asha left a note") {
		t.Fatalf("partner notification: %+v", n)
	}
	if n := noteAlerts(a); len(n) != 1 || !strings.HasPrefix(n[0].Body, "গ্যাস") || len(n[0].Body) >= len(gas.Text) {
		t.Fatalf("author's own note shouldn't notify them; preview cut cleanly: %+v", n)
	}

	// Either partner can edit and pin; pinned notes come first.
	b.do("PATCH", "/api/notes/"+milk.ID, map[string]any{"pinned": true, "color": "blue"}, 200, nil)
	b.do("PATCH", "/api/notes/"+milk.ID, map[string]any{"text": ""}, 400, nil)
	var list []FridgeNote
	a.do("GET", "/api/notes", nil, 200, &list)
	if len(list) != 2 || list[0].ID != milk.ID || !list[0].Pinned || list[0].Color != "blue" || list[0].Text != milk.Text {
		t.Fatalf("list: %+v", list)
	}

	a.do("DELETE", "/api/notes/"+gas.ID, nil, 204, nil)
	a.do("GET", "/api/notes", nil, 200, &list)
	if len(list) != 1 {
		t.Fatalf("after delete: %+v", list)
	}

	// Checklists: a title is optional, blank items are dropped, ticks are per item.
	a.do("POST", "/api/notes", map[string]any{"text": "", "items": []map[string]any{{"text": "  "}}}, 400, nil)
	many := make([]map[string]any, 31)
	for i := range many {
		many[i] = map[string]any{"text": "x"}
	}
	a.do("POST", "/api/notes", map[string]any{"items": many}, 400, nil)
	var shop FridgeNote
	a.do("POST", "/api/notes", map[string]any{"items": []map[string]any{{"text": "Atta"}, {"text": " "}, {"text": "Curd", "done": true}, {"text": "দই"}}}, 201, &shop)
	if shop.Text != "" || len(shop.Items) != 3 || shop.Items[0].ID == "" || shop.Items[1].Text != "Curd" || !shop.Items[1].Done {
		t.Fatalf("checklist: %+v", shop)
	}
	if n := noteAlerts(b); len(n) != 2 || n[0].Body != "Atta, Curd, দই" {
		t.Fatalf("checklist notification: %+v", n)
	}
	// Both tick different items "at once": neither undoes the other.
	b.do("PATCH", "/api/notes/"+shop.ID+"/items/"+shop.Items[0].ID, map[string]any{"done": true}, 200, nil)
	var ticked FridgeNote
	a.do("PATCH", "/api/notes/"+shop.ID+"/items/"+shop.Items[2].ID, map[string]any{"done": true}, 200, &ticked)
	if !ticked.Items[0].Done || !ticked.Items[1].Done || !ticked.Items[2].Done || ticked.Items[0].Text != "Atta" {
		t.Fatalf("ticks: %+v", ticked.Items)
	}
	a.do("PATCH", "/api/notes/"+shop.ID+"/items/"+shop.Items[1].ID, map[string]any{"done": false}, 200, &ticked)
	if ticked.Items[1].Done {
		t.Fatalf("untick: %+v", ticked.Items)
	}
	a.do("PATCH", "/api/notes/"+shop.ID+"/items/nope", map[string]any{"done": true}, 404, nil)
	a.do("PATCH", "/api/notes/"+milk.ID+"/items/"+shop.Items[0].ID, map[string]any{"done": true}, 404, nil) // item of another note
	// Editing the list keeps ids; clearing every item from an untitled note is refused.
	var edited FridgeNote
	a.do("PATCH", "/api/notes/"+shop.ID, map[string]any{"text": "Shopping", "items": []any{ticked.Items[0], map[string]any{"text": "Eggs"}}}, 200, &edited)
	if edited.Text != "Shopping" || len(edited.Items) != 2 || edited.Items[0].ID != shop.Items[0].ID || !edited.Items[0].Done || edited.Items[1].ID == "" {
		t.Fatalf("edit checklist: %+v", edited)
	}
	a.do("PATCH", "/api/notes/"+shop.ID, map[string]any{"text": "", "items": []any{}}, 400, nil)
	a.do("PATCH", "/api/notes/"+shop.ID, map[string]any{"text": "Eggs and atta", "items": []any{}}, 200, &edited)
	if len(edited.Items) != 0 || edited.Text != "Eggs and atta" {
		t.Fatalf("back to a plain note: %+v", edited)
	}
	a.do("DELETE", "/api/notes/"+shop.ID, nil, 204, nil)

	// Expiry: a note shows through its last day, then comes down by itself.
	ctx := context.Background()
	var hid string
	db.QueryRow(ctx, `SELECT household_id FROM fridge_notes WHERE id = $1`, milk.ID).Scan(&hid)
	today, err := srv.householdToday(ctx, hid)
	if err != nil {
		t.Fatal(err)
	}
	day, _ := time.Parse(time.DateOnly, today)
	yesterday, tomorrow := day.AddDate(0, 0, -1).Format(time.DateOnly), day.AddDate(0, 0, 1).Format(time.DateOnly)
	a.do("POST", "/api/notes", map[string]any{"text": "Too late", "expiresOn": yesterday}, 400, nil)
	a.do("POST", "/api/notes", map[string]any{"text": "Bad date", "expiresOn": "next week"}, 400, nil)
	a.do("POST", "/api/notes", map[string]any{"text": "Bad date", "expiresOn": 7}, 400, nil)
	var today1, soon FridgeNote
	a.do("POST", "/api/notes", map[string]any{"text": "Plumber comes today", "expiresOn": today}, 201, &today1)
	if today1.ExpiresOn == nil || *today1.ExpiresOn != today {
		t.Fatalf("expiry: %+v", today1)
	}
	a.do("PATCH", "/api/notes/"+milk.ID, map[string]any{"expiresOn": tomorrow}, 200, &soon)
	if soon.ExpiresOn == nil || *soon.ExpiresOn != tomorrow || soon.Text != milk.Text {
		t.Fatalf("set expiry: %+v", soon)
	}
	a.do("PATCH", "/api/notes/"+milk.ID, map[string]any{"pinned": false}, 200, &soon)
	if soon.ExpiresOn == nil {
		t.Fatal("a patch without expiresOn must keep it")
	}
	a.do("PATCH", "/api/notes/"+milk.ID, map[string]any{"expiresOn": nil}, 200, &soon)
	if soon.ExpiresOn != nil {
		t.Fatalf("clear expiry: %+v", soon)
	}
	// Yesterday's note is hidden at once and deleted by the scheduler.
	db.Exec(ctx, `UPDATE fridge_notes SET expires_on = $2::date WHERE id = $1`, today1.ID, yesterday)
	a.do("GET", "/api/notes", nil, 200, &list)
	if len(list) != 1 || list[0].ID != milk.ID {
		t.Fatalf("expired note should be hidden: %+v", list)
	}
	h, _ := srv.getHousehold(ctx, hid)
	if err := srv.removeExpiredNotes(ctx, h, today); err != nil {
		t.Fatal(err)
	}
	var left int
	db.QueryRow(ctx, `SELECT count(*) FROM fridge_notes`).Scan(&left)
	if left != 1 {
		t.Fatalf("expired note should be deleted, %d left", left)
	}
}
