package api

import (
	"strings"
	"testing"
)

func TestFridgeNotes(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, ts, _ := startTestServer(t, 54336)
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
}
