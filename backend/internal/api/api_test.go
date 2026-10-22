package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/jackc/pgx/v5/pgxpool"

	"homeops/internal/config"
	"homeops/internal/store"
)

// TestAPI runs the main flows against a real, throwaway Postgres.
func TestAPI(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	ctx := context.Background()
	s, ts, db := startTestServer(t, 54329)
	alice := newClient(t, ts.URL)
	bob := newClient(t, ts.URL)

	var boot map[string]any
	alice.do("GET", "/api/bootstrap", nil, 200, &boot)
	if boot["setupRequired"] != true {
		t.Fatalf("expected setupRequired, got %v", boot)
	}

	// CSRF header is enforced on writes.
	req, _ := http.NewRequest("POST", ts.URL+"/api/auth/login", strings.NewReader(`{}`))
	if resp, _ := http.DefaultClient.Do(req); resp.StatusCode != 403 {
		t.Fatalf("expected 403 without CSRF header, got %d", resp.StatusCode)
	}

	alice.do("POST", "/api/setup", map[string]any{"householdName": "Home", "username": "Sayan", "displayName": "Sayan", "password": "hunter2hunter2"}, 204, nil)
	alice.do("POST", "/api/setup", map[string]any{"username": "xx", "displayName": "x", "password": "hunter2hunter2"}, 409, nil)
	alice.do("GET", "/api/bootstrap", nil, 200, &boot)
	me := boot["user"].(map[string]any)
	if me["username"] != "sayan" || me["role"] != "admin" {
		t.Fatalf("unexpected user %v", me)
	}
	if n := len(boot["labels"].([]any)); n != 7 {
		t.Fatalf("expected 7 default labels, got %d", n)
	}
	if boot["vapidPublicKey"] == "" {
		t.Fatal("missing VAPID key")
	}

	var sprints []Sprint
	alice.do("GET", "/api/sprints", nil, 200, &sprints)
	if len(sprints) != 2 || sprints[0].Status != "planned" || sprints[1].Status != "active" {
		t.Fatalf("expected active + planned sprint, got %+v", sprints)
	}
	active, next := sprints[1], sprints[0]

	// Login / logout / wrong password.
	alice.do("POST", "/api/auth/logout", nil, 204, nil)
	alice.do("GET", "/api/tasks", nil, 401, nil)
	alice.do("POST", "/api/auth/login", map[string]string{"username": "sayan", "password": "wrong"}, 401, nil)
	alice.do("POST", "/api/auth/login", map[string]string{"username": "SAYAN", "password": "hunter2hunter2"}, 204, nil)

	// Successful sign-ins never count toward the lockout; failures do.
	for i := 0; i < 12; i++ {
		alice.do("POST", "/api/auth/login", map[string]string{"username": "sayan", "password": "hunter2hunter2"}, 204, nil)
	}
	for i := 0; i < 9; i++ { // one failure already happened above
		alice.do("POST", "/api/auth/login", map[string]string{"username": "sayan", "password": "nope"}, 401, nil)
	}
	alice.do("POST", "/api/auth/login", map[string]string{"username": "sayan", "password": "hunter2hunter2"}, 429, nil)
	s.limiter = newRateLimiter(10, 15*time.Minute)

	// Invite the partner.
	var inv map[string]string
	alice.do("POST", "/api/invites", nil, 200, &inv)
	bob.do("GET", "/api/auth/invite/"+inv["code"], nil, 200, nil)
	bob.do("POST", "/api/auth/invite/accept", map[string]any{"code": inv["code"], "username": "partner", "displayName": "Partner", "password": "correcthorse"}, 204, nil)
	bob.do("POST", "/api/auth/invite/accept", map[string]any{"code": inv["code"], "username": "p2", "displayName": "P", "password": "correcthorse"}, 404, nil)
	bob.do("POST", "/api/invites", nil, 403, nil) // members can't invite
	var bobBoot map[string]any
	bob.do("GET", "/api/bootstrap", nil, 200, &bobBoot)
	bobID := bobBoot["user"].(map[string]any)["id"].(string)

	// Bob listens for live events.
	events, stopEvents := bob.sse("/api/events")
	defer stopEvents()

	// Tasks: create with a client ID (idempotent), assign, complete.
	id := "6f1c2c1e-0000-4000-8000-000000000001"
	var task Task
	alice.do("POST", "/api/tasks", map[string]any{"id": id, "title": "Mop floors", "sprintId": active.ID, "points": 3,
		"labels": []string{boot["labels"].([]any)[0].(map[string]any)["id"].(string)}}, 201, &task)
	if task.Number != 1 || task.Status != "todo" || *task.Points != 3 || len(task.Labels) != 1 {
		t.Fatalf("unexpected task %+v", task)
	}
	alice.do("POST", "/api/tasks", map[string]any{"id": id, "title": "Mop floors"}, 200, &task)
	if task.Number != 1 {
		t.Fatal("retry created a duplicate")
	}
	expectEvent(t, events, "tasks")

	alice.do("PATCH", "/api/tasks/"+id, map[string]any{"assigneeId": bobID, "dueDate": "2030-01-05"}, 200, &task)
	if task.AssigneeID == nil || *task.AssigneeID != bobID || *task.DueDate != "2030-01-05" {
		t.Fatalf("assign failed %+v", task)
	}
	var inbox struct {
		Items  []inboxItem
		Unread int
	}
	bob.do("GET", "/api/notifications", nil, 200, &inbox)
	if inbox.Unread != 1 || !strings.Contains(inbox.Items[0].Title, "assigned you HO-1") {
		t.Fatalf("expected assignment notification, got %+v", inbox)
	}
	bob.do("POST", "/api/notifications/read", map[string]any{"all": true}, 204, nil)

	bob.do("PATCH", "/api/tasks/"+id, map[string]any{"status": "done"}, 200, &task)
	if task.CompletedAt == nil || *task.CompletedBy != bobID {
		t.Fatalf("completion not recorded %+v", task)
	}
	bob.do("PATCH", "/api/tasks/"+id, map[string]any{"points": nil, "dueDate": nil}, 200, &task)
	if task.Points != nil || task.DueDate != nil {
		t.Fatalf("null should clear fields %+v", task)
	}
	alice.do("PATCH", "/api/tasks/"+id, map[string]any{"status": "bogus"}, 400, nil)
	alice.do("PATCH", "/api/tasks/"+id, map[string]any{"assigneeId": "6f1c2c1e-0000-4000-8000-00000000dead"}, 400, nil)

	// An unfinished task to carry over.
	var open Task
	alice.do("POST", "/api/tasks", map[string]any{"title": "Pay electricity bill", "sprintId": active.ID}, 201, &open)

	// Recurring: every day, rotating. Generates into the active + planned sprints from today.
	var rec Recurring
	alice.do("POST", "/api/recurring", map[string]any{"title": "Water plants", "freq": "daily", "rotate": true}, 200, &rec)
	var tasks []Task
	alice.do("GET", "/api/tasks", nil, 200, &tasks)
	gen := filter(tasks, func(t Task) bool { return t.RecurringID != nil })
	if len(gen) < 7 {
		t.Fatalf("expected at least a week of generated tasks, got %d", len(gen))
	}
	assignees := map[string]bool{}
	for _, g := range gen {
		assignees[*g.AssigneeID] = true
	}
	if len(assignees) != 2 {
		t.Fatalf("rotation should alternate between both members, got %v", assignees)
	}
	alice.do("DELETE", "/api/tasks/"+gen[len(gen)-1].ID, nil, 204, nil)
	if err := s.ensureSprints(ctx, task.householdIDFor(t, s)); err != nil {
		t.Fatal(err)
	}
	alice.do("GET", "/api/tasks", nil, 200, &tasks)
	if n := len(filter(tasks, func(t Task) bool { return t.RecurringID != nil })); n != len(gen)-1 {
		t.Fatalf("deleted occurrence was regenerated: %d vs %d", n, len(gen)-1)
	}

	// Complete the sprint: open work moves to the next sprint.
	alice.do("POST", "/api/sprints/"+next.ID+"/start", nil, 409, nil)
	alice.do("POST", "/api/sprints/"+active.ID+"/complete", map[string]any{"moveTo": "next"}, 204, nil)
	alice.do("GET", "/api/tasks", nil, 200, &tasks)
	for _, tk := range tasks {
		if tk.ID == open.ID && (tk.SprintID == nil || *tk.SprintID != next.ID) {
			t.Fatalf("open task not carried over: %+v", tk)
		}
	}
	alice.do("POST", "/api/sprints/"+next.ID+"/start", nil, 204, nil)
	alice.do("GET", "/api/sprints", nil, 200, &sprints)
	if sprints[0].Status != "planned" || sprints[1].ID != next.ID || sprints[1].Status != "active" || sprints[2].Status != "completed" {
		t.Fatalf("unexpected sprint states %+v", sprints)
	}
	if sprints[2].DoneCount != 1 {
		t.Fatalf("completed sprint should keep its done task, got %+v", sprints[2])
	}

	// Digest + dedupe.
	h, _ := s.getHousehold(ctx, task.householdIDFor(t, s))
	today := localToday(h, time.Now()).Format(time.DateOnly)
	alice.do("POST", "/api/tasks", map[string]any{"title": "Buy milk", "dueDate": today}, 201, nil)
	if err := s.dueDigest(ctx, h, today, false); err != nil {
		t.Fatal(err)
	}
	if err := s.dueDigest(ctx, h, today, false); err != nil {
		t.Fatal(err)
	}
	var n int
	db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE dedupe_key = $1`, "due:"+today).Scan(&n)
	if n != 2 {
		t.Fatalf("expected one digest per member, got %d", n)
	}

	// Push subscription upsert + pref opt-out.
	alice.do("POST", "/api/push/subscribe", map[string]any{"endpoint": "https://push.example.invalid/x", "keys": map[string]string{"p256dh": "a", "auth": "b"}}, 204, nil)
	alice.do("PATCH", "/api/me", map[string]any{"notifyPrefs": map[string]bool{"ceremony": false}, "quietStart": "22:00", "quietEnd": "07:00"}, 200, nil)
	alice.do("PATCH", "/api/me", map[string]any{"quietStart": "25:99"}, 400, nil)

	// Sessions + password change signs out other devices.
	other := newClient(t, ts.URL)
	other.do("POST", "/api/auth/login", map[string]string{"username": "sayan", "password": "hunter2hunter2"}, 204, nil)
	alice.do("POST", "/api/me/password", map[string]string{"current": "hunter2hunter2", "new": "newpassword1"}, 204, nil)
	other.do("GET", "/api/tasks", nil, 401, nil)
	alice.do("GET", "/api/tasks", nil, 200, nil)
}

// startTestServer boots a throwaway Postgres and an API server; both stop with the test.
func startTestServer(t *testing.T, port uint32) (*Server, *httptest.Server, *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	pg := embeddedpostgres.NewDatabase(embeddedpostgres.DefaultConfig().
		Port(port).RuntimePath(dir + "/rt").DataPath(dir + "/data").CachePath(os.TempDir() + "/homeops-pg-cache").
		Logger(io.Discard))
	if err := pg.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { pg.Stop() })
	db, err := store.Open(ctx, fmt.Sprintf("postgres://postgres:postgres@localhost:%d/postgres?sslmode=disable", port))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	if err := store.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx, db); err != nil { // idempotent
		t.Fatal(err)
	}
	cfg := config.Config{PublicURL: "http://localhost:5173", VAPIDEmail: "t@example.com", DataDir: dir + "/files"}
	if ep := os.Getenv("HOMEOPS_TEST_S3_ENDPOINT"); ep != "" { // run against real object storage
		cfg.S3Endpoint, cfg.S3Bucket = ep, fmt.Sprintf("homeops-test-%d", port)
		cfg.S3AccessKey, cfg.S3SecretKey = os.Getenv("HOMEOPS_TEST_S3_ACCESS_KEY"), os.Getenv("HOMEOPS_TEST_S3_SECRET_KEY")
	}
	s, err := New(ctx, cfg, db)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(s.Routes())
	t.Cleanup(ts.Close)
	return s, ts, db
}

func (t Task) householdIDFor(tb testing.TB, s *Server) string {
	var hid string
	if err := s.db.QueryRow(context.Background(), `SELECT household_id FROM tasks WHERE id = $1`, t.ID).Scan(&hid); err != nil {
		tb.Fatal(err)
	}
	return hid
}

func filter[T any](xs []T, f func(T) bool) []T {
	var out []T
	for _, x := range xs {
		if f(x) {
			out = append(out, x)
		}
	}
	return out
}

func expectEvent(t *testing.T, ch <-chan string, topic string) {
	t.Helper()
	timeout := time.After(3 * time.Second)
	for {
		select {
		case msg := <-ch:
			if strings.Contains(msg, `"topic":"`+topic+`"`) {
				return
			}
		case <-timeout:
			t.Fatalf("no %s event received", topic)
		}
	}
}

type testClient struct {
	t    *testing.T
	base string
	http *http.Client
}

func newClient(t *testing.T, base string) *testClient {
	jar, _ := cookiejar.New(nil)
	return &testClient{t: t, base: base, http: &http.Client{Jar: jar}}
}

func (c *testClient) do(method, path string, body any, wantStatus int, out any) {
	c.t.Helper()
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.base+path, rd)
	req.Header.Set("X-Requested-With", "homeops")
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != wantStatus {
		c.t.Fatalf("%s %s: want %d, got %d: %s", method, path, wantStatus, resp.StatusCode, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			c.t.Fatalf("%s %s: decode: %v: %s", method, path, err, raw)
		}
	}
}

func (c *testClient) sse(path string) (<-chan string, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", c.base+path, nil)
	resp, err := c.http.Do(req)
	if err != nil || resp.StatusCode != 200 {
		c.t.Fatal(fmt.Sprint("sse connect: ", err))
	}
	ch := make(chan string, 32)
	go func() {
		defer resp.Body.Close()
		sc := bufio.NewScanner(resp.Body)
		for sc.Scan() {
			if line := sc.Text(); strings.HasPrefix(line, "data: ") {
				ch <- line[6:]
			}
		}
	}()
	return ch, cancel
}

func jsonDecode(t *testing.T, r io.Reader, v any) {
	t.Helper()
	if err := json.NewDecoder(r).Decode(v); err != nil {
		t.Fatal(err)
	}
}
