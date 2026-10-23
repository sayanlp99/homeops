package api

import (
	"context"
	"regexp"
	"sync"
	"testing"
	"time"

	"homeops/internal/mail"
)

type captureMailer struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (c *captureMailer) Enabled() bool { return true }
func (c *captureMailer) Send(_ context.Context, m mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, m)
	return nil
}

func (c *captureMailer) last(t *testing.T, to string) mail.Message {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.sent) - 1; i >= 0; i-- {
		if c.sent[i].To == to {
			return c.sent[i]
		}
	}
	t.Fatalf("no email sent to %s", to)
	return mail.Message{}
}

var linkRe = regexp.MustCompile(`http://localhost:5173/(join|reset)/(\S+)`)

func TestHouseholdsAndRoles(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	s, ts, _ := startTestServer(t, 54333)
	mailer := &captureMailer{}
	s.mailer = mailer
	admin, owner, viewer, outsider := newClient(t, ts.URL), newClient(t, ts.URL), newClient(t, ts.URL), newClient(t, ts.URL)

	admin.do("POST", "/api/setup", map[string]any{"householdName": "Ours", "email": "Admin@Example.com", "username": "admin", "displayName": "Admin", "password": "hunter2hunter2"}, 204, nil)
	var boot map[string]any
	admin.do("GET", "/api/bootstrap", nil, 200, &boot)
	ours := boot["household"].(map[string]any)["id"].(string)
	if boot["user"].(map[string]any)["email"] != "admin@example.com" {
		t.Fatalf("email not normalised: %v", boot["user"])
	}

	// Sign in with the email too.
	admin.do("POST", "/api/auth/login", map[string]string{"username": "ADMIN@example.com", "password": "hunter2hunter2"}, 204, nil)

	// Only the platform admin creates households; a new person is invited as owner.
	var created map[string]any
	admin.do("POST", "/api/admin/households", map[string]any{"name": "Parents", "ownerEmail": "mum@example.com"}, 200, &created)
	if created["owner"] != "invited" {
		t.Fatalf("expected owner invite, got %v", created)
	}
	parents := created["id"].(string)
	link := linkRe.FindStringSubmatch(mailer.last(t, "mum@example.com").Text)
	if link == nil || link[1] != "join" {
		t.Fatal("invite email has no join link")
	}
	var info map[string]any
	owner.do("GET", "/api/auth/invite/"+link[2], nil, 200, &info)
	if info["role"] != "owner" || info["email"] != "mum@example.com" || info["accountExists"] != false {
		t.Fatalf("unexpected invite info %v", info)
	}
	owner.do("POST", "/api/auth/invite/accept", map[string]any{"code": link[2], "username": "mum", "displayName": "Mum", "password": "correcthorse"}, 204, nil)
	owner.do("GET", "/api/bootstrap", nil, 200, &boot)
	if boot["user"].(map[string]any)["role"] != "owner" || boot["household"].(map[string]any)["id"] != parents {
		t.Fatalf("owner landed wrong: %v", boot["user"])
	}
	owner.do("GET", "/api/admin/households", nil, 403, nil)

	// The admin isn't a member of Parents and can't see its data.
	owner.do("POST", "/api/tasks", map[string]any{"title": "Parents' chore"}, 201, nil)
	var tasks []Task
	admin.do("GET", "/api/tasks", nil, 200, &tasks)
	if len(tasks) != 0 {
		t.Fatalf("household data leaked: %+v", tasks)
	}
	admin.do("POST", "/api/me/household", map[string]string{"householdId": parents}, 404, nil)

	// The owner invites the admin's existing account as a viewer... the admin joins while signed in.
	owner.do("POST", "/api/invites", map[string]string{"email": "admin@example.com", "role": "viewer"}, 200, nil)
	link = linkRe.FindStringSubmatch(mailer.last(t, "admin@example.com").Text)
	outsider.do("POST", "/api/auth/invite/accept", map[string]any{"code": link[2], "username": "sneaky", "displayName": "S", "password": "correcthorse"}, 409, nil)
	admin.do("POST", "/api/invites/"+link[2]+"/join", nil, 200, nil)
	admin.do("GET", "/api/bootstrap", nil, 200, &boot)
	if boot["household"].(map[string]any)["id"] != parents || boot["user"].(map[string]any)["role"] != "viewer" {
		t.Fatalf("join didn't switch: %v", boot["user"])
	}
	if hs := boot["households"].([]any); len(hs) != 2 {
		t.Fatalf("expected 2 households, got %v", hs)
	}
	// Viewer: reads yes, writes no; leaving is allowed.
	admin.do("GET", "/api/tasks", nil, 200, &tasks)
	if len(tasks) != 1 {
		t.Fatalf("viewer should see the chore, got %d", len(tasks))
	}
	admin.do("POST", "/api/tasks", map[string]any{"title": "nope"}, 403, nil)
	admin.do("POST", "/api/notifications/read", map[string]any{"all": true}, 204, nil)

	// Switch back: data follows the session's household.
	admin.do("POST", "/api/me/household", map[string]string{"householdId": ours}, 204, nil)
	admin.do("GET", "/api/tasks", nil, 200, &tasks)
	if len(tasks) != 0 {
		t.Fatalf("wrong household after switch: %+v", tasks)
	}

	// Role rules in Parents.
	viewer.do("POST", "/api/auth/login", map[string]string{"username": "admin", "password": "hunter2hunter2"}, 204, nil)
	viewer.do("POST", "/api/me/household", map[string]string{"householdId": parents}, 204, nil)
	var b2 map[string]any
	owner.do("GET", "/api/bootstrap", nil, 200, &b2)
	ownerID := b2["user"].(map[string]any)["id"].(string)
	var adminID string
	for _, m := range b2["members"].([]any) {
		if m.(map[string]any)["username"] == "admin" {
			adminID = m.(map[string]any)["id"].(string)
		}
	}
	viewer.do("PATCH", "/api/members/"+ownerID, map[string]string{"role": "member"}, 403, nil)
	owner.do("PATCH", "/api/members/"+ownerID, map[string]string{"role": "admin"}, 400, nil) // last owner
	owner.do("DELETE", "/api/members/"+ownerID, nil, 400, nil)
	owner.do("PATCH", "/api/members/"+adminID, map[string]string{"role": "admin"}, 204, nil)
	viewer.do("PATCH", "/api/members/"+ownerID, map[string]string{"role": "member"}, 403, nil) // admins can't touch owners
	viewer.do("POST", "/api/invites", map[string]string{"email": "boss@example.com", "role": "owner"}, 403, nil)
	viewer.do("POST", "/api/invites", map[string]string{"email": "cook@example.com", "role": "member"}, 200, nil)
	var invites []pendingInvite
	viewer.do("GET", "/api/invites", nil, 200, &invites)
	if len(invites) != 1 || invites[0].Email != "cook@example.com" {
		t.Fatalf("unexpected invites %+v", invites)
	}
	viewer.do("DELETE", "/api/household", map[string]string{"confirmName": "Parents"}, 403, nil)

	// Removing someone puts their open tasks back in the pool and drops them out.
	viewer.do("DELETE", "/api/members/"+adminID, nil, 204, nil) // leave
	boot = nil
	viewer.do("GET", "/api/bootstrap", nil, 200, &boot)
	if boot["household"].(map[string]any)["id"] != ours {
		t.Fatalf("after leaving, expected fallback household, got %v", boot["household"])
	}

	// Password reset by email; unknown emails look the same.
	outsider.do("POST", "/api/auth/password/forgot", map[string]string{"email": "nobody@example.com"}, 204, nil)
	outsider.do("POST", "/api/auth/password/forgot", map[string]string{"email": "mum@example.com"}, 204, nil)
	var resetMsg mail.Message
	for i := 0; i < 50 && resetMsg.To == ""; i++ { // sent in the background
		mailer.mu.Lock()
		for _, m := range mailer.sent {
			if m.To == "mum@example.com" && m.Subject == "Reset your HomeOps password" {
				resetMsg = m
			}
		}
		mailer.mu.Unlock()
		sleepMs(20)
	}
	link = linkRe.FindStringSubmatch(resetMsg.Text)
	if link == nil || link[1] != "reset" {
		t.Fatalf("no reset link in %q", resetMsg.Text)
	}
	outsider.do("GET", "/api/auth/password/reset/"+link[2], nil, 200, nil)
	outsider.do("POST", "/api/auth/password/reset", map[string]string{"token": link[2], "password": "brandnewpass"}, 204, nil)
	outsider.do("POST", "/api/auth/password/reset", map[string]string{"token": link[2], "password": "again12345"}, 404, nil)
	owner.do("GET", "/api/tasks", nil, 401, nil) // old sessions signed out
	owner.do("POST", "/api/auth/login", map[string]string{"username": "mum", "password": "brandnewpass"}, 204, nil)

	// Email changes need the password and must be unique.
	owner.do("PATCH", "/api/me", map[string]string{"email": "mum2@example.com"}, 400, nil)
	owner.do("PATCH", "/api/me", map[string]string{"email": "admin@example.com", "password": "brandnewpass"}, 409, nil)
	owner.do("PATCH", "/api/me", map[string]string{"email": "mum2@example.com", "password": "brandnewpass"}, 200, nil)

	// Deleting a household needs its name.
	owner.do("DELETE", "/api/household", map[string]string{"confirmName": "nope"}, 400, nil)
	owner.do("DELETE", "/api/household", map[string]string{"confirmName": "Parents"}, 204, nil)
	boot = nil
	owner.do("GET", "/api/bootstrap", nil, 200, &boot)
	if boot["household"] != nil {
		t.Fatalf("expected no household, got %v", boot["household"])
	}
	owner.do("GET", "/api/tasks", nil, 403, nil)
}

func sleepMs(n int) { time.Sleep(time.Duration(n) * time.Millisecond) }
