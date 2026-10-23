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
	HouseholdID string
	UserID      string
	Kind        string // matches a notifyKinds preference key
	DedupeKey   string
	Title       string
	Body        string
	URL         string
}

// notify records an inbox entry and queues it for push, unless the user opted out of
// this kind. Re-sending the same DedupeKey is a no-op, which makes scheduled jobs idempotent.
func (s *Server) notify(ctx context.Context, n notification) error {
	var prefs map[string]bool
	householdID := n.HouseholdID
	// Only current members hear about a household.
	err := s.db.QueryRow(ctx, `SELECT u.notify_prefs FROM memberships m JOIN users u ON u.id = m.user_id
		WHERE m.user_id = $1 AND m.household_id = $2`, n.UserID, householdID).Scan(&prefs)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if enabled, set := prefs[n.Kind]; set && !enabled {
		return nil
	}
	if n.URL == "" {
		n.URL = "/"
	}
	tag, err := s.db.Exec(ctx, `INSERT INTO notifications (user_id, household_id, kind, dedupe_key, title, body, url)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (user_id, household_id, dedupe_key) DO NOTHING`,
		n.UserID, householdID, n.Kind, n.DedupeKey, truncate(n.Title, 200), truncate(n.Body, 500), n.URL)
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
	rows, _ := s.db.Query(ctx, `SELECT id FROM members WHERE household_id = $1`, householdID)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return err
	}
	var errs []error
	n.HouseholdID = householdID
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
		WHERE user_id = $1 AND household_id = $2 ORDER BY created_at DESC LIMIT 50`, u.ID, u.HouseholdID)
	items, err := pgx.CollectRows(rows, pgx.RowToStructByName[inboxItem])
	if err != nil {
		return err
	}
	var unread int
	err = s.db.QueryRow(r.Context(), `SELECT count(*) FROM notifications WHERE user_id = $1 AND household_id = $2 AND read_at IS NULL`,
		u.ID, u.HouseholdID).Scan(&unread)
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
		_, err = s.db.Exec(r.Context(), `UPDATE notifications SET read_at = now() WHERE user_id = $1 AND household_id = $2 AND read_at IS NULL`, u.ID, u.HouseholdID)
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

	rows, _ := p.db.Query(ctx, `SELECT n.id, n.user_id, n.kind, n.title, n.body, n.url || CASE WHEN n.url LIKE '%?%' THEN '&' ELSE '?' END || 'hh=' || n.household_id,
			u.quiet_start, u.quiet_end, h.timezone
		FROM notifications n JOIN users u ON u.id = n.user_id JOIN households h ON h.id = n.household_id
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
		service := pushService(sb.Endpoint)
		if err != nil {
			res.Error = err.Error()
			slog.Warn("push failed", "user", userID, "service", service, "device", res.Device, "err", err)
			p.recordFailure(ctx, sb.Endpoint, false, res.Error)
			results = append(results, res)
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		resp.Body.Close()
		res.Status = resp.StatusCode
		reason := strings.TrimSpace(string(body))
		switch {
		case resp.StatusCode < 300:
			res.OK = true
			slog.Info("push sent", "user", userID, "service", service, "device", res.Device, "status", resp.StatusCode)
			_, err := p.db.Exec(ctx, `UPDATE push_subscriptions SET last_success_at = now(), failure_count = 0, last_error = '' WHERE endpoint = $1`, sb.Endpoint)
			logErr("record push success", err)
		case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
			// The push service says this subscription no longer exists; the device
			// re-registers automatically the next time the app is opened there.
			res.Error = "subscription expired"
			slog.Warn("push subscription removed", "user", userID, "service", service, "device", res.Device, "status", resp.StatusCode, "reason", reason)
			p.recordFailure(ctx, sb.Endpoint, true, res.Error)
		default:
			res.Error = resp.Status
			if reason != "" {
				res.Error += ": " + truncate(reason, 160)
			}
			slog.Warn("push rejected", "user", userID, "service", service, "device", res.Device, "status", resp.StatusCode, "reason", reason)
			p.recordFailure(ctx, sb.Endpoint, false, res.Error)
		}
		results = append(results, res)
	}
	return results, nil
}

func (p *pusher) recordFailure(ctx context.Context, endpoint string, gone bool, reason string) {
	var err error
	if gone {
		_, err = p.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1`, endpoint)
	} else {
		_, err = p.db.Exec(ctx, `UPDATE push_subscriptions SET failure_count = failure_count + 1, last_error = $2, last_failure_at = now()
			WHERE endpoint = $1`, endpoint, truncate(reason, 300))
		if err == nil {
			_, err = p.db.Exec(ctx, `DELETE FROM push_subscriptions WHERE endpoint = $1 AND failure_count >= 20`, endpoint)
		}
	}
	logErr("record push failure", err)
}

// pushService names the browser vendor's push service from a subscription endpoint.
func pushService(endpoint string) string {
	switch {
	case strings.Contains(endpoint, "fcm.googleapis.com"):
		return "Google (Chrome/Edge/Android)"
	case strings.Contains(endpoint, "push.apple.com"):
		return "Apple (Safari/iOS)"
	case strings.Contains(endpoint, "mozilla.com"):
		return "Mozilla (Firefox)"
	case strings.Contains(endpoint, "notify.windows.com"):
		return "Microsoft (Edge/Windows)"
	}
	return "Other"
}

// pushDevices lists the current user's registered devices, for Settings.
func (s *Server) pushDevices(w http.ResponseWriter, r *http.Request) error {
	rows, _ := s.db.Query(r.Context(), `SELECT endpoint, user_agent, created_at, last_success_at, failure_count, last_error, last_failure_at
		FROM push_subscriptions WHERE user_id = $1 ORDER BY created_at`, currentUser(r).ID)
	type device struct {
		Endpoint      string     `json:"endpoint"`
		Device        string     `json:"device"`
		Browser       string     `json:"browser"`
		Service       string     `json:"service"`
		CreatedAt     time.Time  `json:"createdAt"`
		LastSuccessAt *time.Time `json:"lastSuccessAt"`
		Failures      int        `json:"failures"`
		LastError     string     `json:"lastError"`
		LastFailureAt *time.Time `json:"lastFailureAt"`
	}
	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (device, error) {
		var d device
		var ua string
		err := row.Scan(&d.Endpoint, &ua, &d.CreatedAt, &d.LastSuccessAt, &d.Failures, &d.LastError, &d.LastFailureAt)
		d.Device, d.Browser, d.Service = deviceName(ua), browserName(ua), pushService(d.Endpoint)
		return d, err
	})
	if err != nil {
		return err
	}
	return ok(w, out)
}

func browserName(ua string) string {
	switch {
	case strings.Contains(ua, "EdgA") || strings.Contains(ua, "Edg/"):
		return "Edge"
	case strings.Contains(ua, "CriOS"):
		return "Chrome (iOS)"
	case strings.Contains(ua, "FxiOS"), strings.Contains(ua, "Firefox/"):
		return "Firefox"
	case strings.Contains(ua, "SamsungBrowser"):
		return "Samsung Internet"
	case strings.Contains(ua, "Chrome/"):
		return "Chrome"
	case strings.Contains(ua, "Safari/"):
		return "Safari"
	}
	return "Browser"
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
