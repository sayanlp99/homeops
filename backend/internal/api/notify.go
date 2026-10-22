package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type notification struct {
	UserID    string
	Kind      string // matches a notifyKinds preference key
	DedupeKey string
	Title     string
	Body      string
	URL       string
}

// notify records an inbox entry and queues it for push, unless the user opted out of
// this kind. Re-sending the same DedupeKey is a no-op, which makes scheduled jobs idempotent.
func (s *Server) notify(ctx context.Context, n notification) error {
	var prefs map[string]bool
	var householdID string
	if err := s.db.QueryRow(ctx, `SELECT notify_prefs, household_id FROM users WHERE id = $1`, n.UserID).Scan(&prefs, &householdID); err != nil {
		return err
	}
	if enabled, set := prefs[n.Kind]; set && !enabled {
		return nil
	}
	if n.URL == "" {
		n.URL = "/"
	}
	tag, err := s.db.Exec(ctx, `INSERT INTO notifications (user_id, kind, dedupe_key, title, body, url)
		VALUES ($1, $2, $3, $4, $5, $6) ON CONFLICT (user_id, dedupe_key) DO NOTHING`,
		n.UserID, n.Kind, n.DedupeKey, truncate(n.Title, 200), truncate(n.Body, 500), n.URL)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.hub.Publish(householdID, eventNotificationsFor(n.UserID))
		s.push.wake()
	}
	return nil
}

func (s *Server) notifyHousehold(ctx context.Context, householdID string, n notification) error {
	rows, _ := s.db.Query(ctx, `SELECT id FROM users WHERE household_id = $1`, householdID)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		n.UserID = id
		errs = append(errs, s.notify(ctx, n))
	}
	return errors.Join(errs...)
}

func eventNotificationsFor(userID string) (ev realtimeEvent) {
	ev = eventNotifications.from(nil)
	ev.UserID = userID
	return ev
}

// ---- inbox endpoints ----

type inboxItem struct {
	ID        string     `json:"id" db:"id"`
	Kind      string     `json:"kind" db:"kind"`
	Title     string     `json:"title" db:"title"`
	Body      string     `json:"body" db:"body"`
	URL       string     `json:"url" db:"url"`
	CreatedAt time.Time  `json:"createdAt" db:"created_at"`
	ReadAt    *time.Time `json:"readAt" db:"read_at"`
}

func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	rows, _ := s.db.Query(r.Context(), `SELECT id, kind, title, body, url, created_at, read_at FROM notifications
		WHERE user_id = $1 ORDER BY created_at DESC LIMIT 50`, u.ID)
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[inboxItem])
	if err != nil {
		return err
	}
	unread, err := s.unreadCount(r.Context(), u.ID)
	if err != nil {
		return err
	}
	return ok(w, map[string]any{"items": items, "unread": unread})
}

func (s *Server) unreadCount(ctx context.Context, userID string) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (s *Server) markNotificationsRead(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	var in struct {
		IDs []string `json:"ids"`
		All bool     `json:"all"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	var err error
	if in.All {
		_, err = s.db.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL`, u.ID)
	} else if len(in.IDs) > 0 {
		_, err = s.db.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND id = ANY($2::uuid[]) AND read_at IS NULL`, u.ID, in.IDs)
	}
	if err != nil {
		return err
	}
	ev := eventNotificationsFor(u.ID)
	ev.Origin = clientID(r)
	s.hub.Publish(u.HouseholdID, ev)
	return noContent(w)
}

// ---- push subscriptions ----

func (s *Server) pushSubscribe(w http.ResponseWriter, r *http.Request) error {
	var in webpush.Subscription
	if err := decode(r, &in); err != nil {
		return err
	}
	if !strings.HasPrefix(in.Endpoint, "https://") || in.Keys.P256dh == "" || in.Keys.Auth == "" {
		return errBadRequest("Invalid push subscription")
	}
	// An endpoint belongs to one browser; if someone else signs in there, it moves to them.
	_, err := s.db.Exec(r.Context(), `INSERT INTO push_subscriptions (endpoint, user_id, p256dh, auth, user_agent)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = EXCLUDED.user_id, p256dh = EXCLUDED.p256dh, auth = EXCLUDED.auth,
			user_agent = EXCLUDED.user_agent, failure_count = 0`,
		in.Endpoint, currentUser(r).ID, in.Keys.P256dh, in.Keys.Auth, truncate(r.UserAgent(), 300))
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) pushUnsubscribe(w http.ResponseWriter, r *http.Request) error {
	var in struct{ Endpoint string }
	if err := decode(r, &in); err != nil {
		return err
	}
	if _, err := s.db.Exec(r.Context(), `DELETE FROM push_subscriptions WHERE endpoint = $1 AND user_id = $2`, in.Endpoint, currentUser(r).ID); err != nil {
		return err
	}
	return noContent(w)
}

// pushTest sends straight to every device of the user, bypassing prefs and quiet hours,
// and reports per-device results so delivery problems are visible in Settings.
func (s *Server) pushTest(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	unread, _ := s.unreadCount(r.Context(), u.ID)
	results, err := s.push.sendToUser(r.Context(), u.ID, pushPayload{
		ID: "test-" + randomToken(6), Title: "Test notification", Body: "Push notifications are working on this device.",
		URL: "/settings/notifications", Tag: "test", Badge: unread,
	})
	if err != nil {
		return err
	}
	return ok(w, results)
}

// ---- delivery ----

type pushPayload struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
	Badge int    `json:"badge"`
}

type pushResult struct {
	Device string `json:"device"`
	OK     bool   `json:"ok"`
	Status int    `json:"status"`
	Error  string `json:"error,omitempty"`
}

type pusher struct {
	db         *pgxpool.Pool
	publicKey  string
	privateKey string
	subject    string
	client     *http.Client
	wakeCh     chan struct{}
}

func newPusher(ctx context.Context, db *pgxpool.Pool, subject string) (*pusher, error) {
	p := &pusher{db: db, subject: subject, client: &http.Client{Timeout: 15 * time.Second}, wakeCh: make(chan struct{}, 1)}
	var keys struct{ Public, Private string }
	err := db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = 'vapid'`).Scan(&keys)
	if errors.Is(err, pgx.ErrNoRows) {
		keys.Private, keys.Public, err = webpush.GenerateVAPIDKeys()
		if err != nil {
			return nil, err
		}
		// If another instance won the race, use its keys.
		if _, err := db.Exec(ctx, `INSERT INTO app_settings (key, value) VALUES ('vapid', $1) ON CONFLICT DO NOTHING`, keys); err != nil {
			return nil, err
		}
		err = db.QueryRow(ctx, `SELECT value FROM app_settings WHERE key = 'vapid'`).Scan(&keys)
	}
	if err != nil {
		return nil, err
	}
	p.publicKey, p.privateKey = keys.Public, keys.Private
	return p, nil
}

func (p *pusher) wake() {
	select {
	case p.wakeCh <- struct{}{}:
	default:
	}
}

// run delivers pending notifications as they arrive, and re-checks every 30s so
// messages held during quiet hours go out once those end.
func (p *pusher) run(ctx context.Context, s *Server) {
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		p.deliverPending(ctx)
		select {
		case <-ctx.Done():
			return
		case <-p.wakeCh:
		case <-tick.C:
		}
	}
}

func (p *pusher) deliverPending(ctx context.Context) {
	// Anything held for more than 12 hours is stale; keep it in the inbox only.
	_, err := p.db.Exec(ctx, `UPDATE notifications SET push_state = 'skipped' WHERE push_state = 'pending' AND created_at < now() - interval '12 hours'`)
	logErr("expire pending pushes", err)

	rows, _ := p.db.Query(ctx, `SELECT n.id, n.user_id, n.kind, n.title, n.body, n.url, u.quiet_start, u.quiet_end, h.timezone
		FROM notifications n JOIN users u ON u.id = n.user_id JOIN households h ON h.id = u.household_id
		WHERE n.push_state = 'pending' ORDER BY n.created_at LIMIT 100`)
	type pending struct {
		id, userID, kind, title, body, url string
		quietStart, quietEnd               *string
		tz                                 string
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pending, error) {
		var it pending
		err := row.Scan(&it.id, &it.userID, &it.kind, &it.title, &it.body, &it.url, &it.quietStart, &it.quietEnd, &it.tz)
		return it, err
	})
	if err != nil {
		logErr("load pending pushes", err)
		return
	}
	for _, it := range items {
		loc, err := time.LoadLocation(it.tz)
		if err != nil {
			loc = time.UTC
		}
		if inQuietHours(it.quietStart, it.quietEnd, time.Now().In(loc)) {
			continue
		}
		var unread int
		p.db.QueryRow(ctx, `SELECT count(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL`, it.userID).Scan(&unread)
		results, err := p.sendToUser(ctx, it.userID, pushPayload{
			ID: it.id, Title: it.title, Body: it.body, URL: it.url, Tag: it.kind, Badge: unread,
		})
		if err != nil {
			logErr("send push", err)
			continue
		}
		state := "skipped" // no devices subscribed
		for _, r := range results {
			if r.OK {
				state = "sent"
			}
		}
		_, err = p.db.Exec(ctx, `UPDATE notifications SET push_state = $2, pushed_at = now() WHERE id = $1`, it.id, state)
		logErr("mark push", err)
	}
}

func (p *pusher) sendToUser(ctx context.Context, userID string, payload pushPayload) ([]pushResult, error) {
	rows, _ := p.db.Query(ctx, `SELECT endpoint, p256dh, auth, user_agent FROM push_subscriptions WHERE user_id = $1`, userID)
	type sub struct {
		webpush.Subscription
		ua string
	}
	subs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (sub, error) {
		var s sub
		err := row.Scan(&s.Endpoint, &s.Keys.P256dh, &s.Keys.Auth, &s.ua)
		return s, err
	})
	if err != nil {
		return nil, err
	}
	msg, _ := json.Marshal(payload)
	results := make([]pushResult, 0, len(subs))
	for _, sb := range subs {
		res := pushResult{Device: deviceName(sb.ua)}
		resp, err := webpush.SendNotificationWithContext(ctx, msg, &sb.Subscription, &webpush.Options{
			HTTPClient:      p.client,
			Subscriber:      p.subject,
			VAPIDPublicKey:  p.publicKey,
			VAPIDPrivateKey: p.privateKey,
			TTL:             12 * 60 * 60,
			Urgency:         webpush.UrgencyHigh,
		})
		if err != nil {
			res.Error = err.Error()
			p.recordFailure(ctx, sb.Endpoint, false)
			results = append(results, res)
			continue
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		res.Status = resp.StatusCode
		switch {
		case resp.StatusCode < 300:
			res.OK = true
			_, err := p.db.Exec(ctx, `UPDATE push_subscriptions SET last_success_at = now(), failure_count = 0 WHERE endpoint = $1`, sb.Endpoint)
			logErr("record push success", err)
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			// The browser dropped this subscription; the client re-subscribes on next open.
			res.Error = "subscription expired"
			p.recordFailure(ctx, sb.Endpoint, true)
		default:
			res.Error = resp.Status
			slog.Warn("push rejected", "status", resp.StatusCode, "device", res.Device)
			p.recordFailure(ctx, sb.Endpoint, false)
		}
		results = append(results, res)
	}
	return results, nil
}

func (p *pusher) recordFailure(ctx context.Context, endpoint string, gone bool) {
	var err error
	if gone {
		_, err = p.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	} else {
		_, err = p.db.Exec(ctx, `UPDATE push_subscriptions SET failure_count = failure_count + 1 WHERE endpoint = $1`, endpoint)
		if err == nil {
			_, err = p.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1 AND failure_count >= 20`, endpoint)
		}
	}
	logErr("record push failure", err)
}

// inQuietHours handles ranges that cross midnight, e.g. 22:00–07:00.
func inQuietHours(start, end *string, now time.Time) bool {
	if start == nil || end == nil {
		return false
	}
	s, err1 := parseClock(*start)
	e, err2 := parseClock(*end)
	if err1 != nil || err2 != nil || s == e {
		return false
	}
	m := now.Hour()*60 + now.Minute()
	if s < e {
		return m >= s && m < e
	}
	return m >= s || m < e
}
