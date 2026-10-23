package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// burndown: points (or tasks, when nothing is pointed) left in a sprint at the end of
// each day. Uses each task's current points and its completion time.
func (s *Server) burndown(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	var start, end time.Time
	var number int
	if err := s.db.QueryRow(ctx, `SELECT starts_on, ends_on, number FROM sprints WHERE id = $1 AND household_id = $2`,
		r.URL.Query().Get("sprint"), u.HouseholdID).Scan(&start, &end, &number); err != nil {
		return errNotFound("Sprint not found")
	}
	rows, err := s.db.Query(ctx, `SELECT points, completed_at FROM tasks WHERE sprint_id = $1`, r.URL.Query().Get("sprint"))
	if err != nil {
		return err
	}
	type t struct {
		points *int
		done   *time.Time
	}
	var tasks []t
	pointed := false
	for rows.Next() {
		var x t
		if err := rows.Scan(&x.points, &x.done); err != nil {
			rows.Close()
			return err
		}
		if x.points != nil && *x.points > 0 {
			pointed = true
		}
		tasks = append(tasks, x)
	}
	rows.Close()
	weight := func(x t) int {
		if !pointed {
			return 1
		}
		if x.points == nil {
			return 0
		}
		return *x.points
	}
	total := 0
	for _, x := range tasks {
		total += weight(x)
	}
	loc := h.Location()
	today := localToday(h, time.Now())
	type day struct {
		Date      string `json:"date"`
		Remaining *int   `json:"remaining"` // nil for future days
		Done      int    `json:"done"`      // completed that day
	}
	var days []day
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		dd := day{Date: d.Format(time.DateOnly)}
		endOfDay := time.Date(d.Year(), d.Month(), d.Day(), 23, 59, 59, 0, loc)
		dayStart := time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, loc)
		if !d.After(today) {
			rem := total
			for _, x := range tasks {
				if x.done != nil && !x.done.After(endOfDay) {
					rem -= weight(x)
					if !x.done.Before(dayStart) {
						dd.Done += weight(x)
					}
				}
			}
			dd.Remaining = &rem
		}
		days = append(days, dd)
	}
	unit := "points"
	if !pointed {
		unit = "tasks"
	}
	return ok(w, map[string]any{"sprintNumber": number, "total": total, "unit": unit, "days": days})
}

// teamMetrics: velocity and fairness per sprint, daily streaks and recent kudos.
func (s *Server) teamMetrics(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	h, err := s.getHousehold(ctx, u.HouseholdID)
	if err != nil {
		return err
	}
	n, _ := strconv.Atoi(r.URL.Query().Get("sprints"))
	if n <= 0 || n > 26 {
		n = 8
	}
	// Tasks without points still count as 1 so unpointed chores get credit.
	rows, err := s.db.Query(ctx, `SELECT s.id, s.number, s.starts_on::text, s.status,
		coalesce(sum(coalesce(t.points, 1)), 0)::int,
		coalesce(sum(coalesce(t.points, 1)) FILTER (WHERE t.status = 'done'), 0)::int
		FROM sprints s LEFT JOIN tasks t ON t.sprint_id = s.id
		WHERE s.household_id = $1 AND s.status <> 'planned'
		GROUP BY s.id ORDER BY s.number DESC LIMIT $2`, h.ID, n)
	if err != nil {
		return err
	}
	type sprint struct {
		ID       string         `json:"id"`
		Number   int            `json:"number"`
		StartsOn string         `json:"startsOn"`
		Status   string         `json:"status"`
		Planned  int            `json:"planned"`
		Done     int            `json:"done"`
		ByMember map[string]int `json:"byMember"`
	}
	var sprints []sprint
	ids := []string{}
	for rows.Next() {
		var sp sprint
		if err := rows.Scan(&sp.ID, &sp.Number, &sp.StartsOn, &sp.Status, &sp.Planned, &sp.Done); err != nil {
			rows.Close()
			return err
		}
		sp.ByMember = map[string]int{}
		sprints = append(sprints, sp)
		ids = append(ids, sp.ID)
	}
	rows.Close()
	if len(ids) > 0 {
		rows, err = s.db.Query(ctx, `SELECT sprint_id, completed_by, sum(coalesce(points, 1))::int FROM tasks
			WHERE sprint_id = ANY($1::uuid[]) AND status = 'done' AND completed_by IS NOT NULL GROUP BY 1, 2`, ids)
		if err != nil {
			return err
		}
		for rows.Next() {
			var sid, uid string
			var p int
			if err := rows.Scan(&sid, &uid, &p); err != nil {
				rows.Close()
				return err
			}
			for i := range sprints {
				if sprints[i].ID == sid {
					sprints[i].ByMember[uid] = p
				}
			}
		}
		rows.Close()
	}
	// Oldest first for charts.
	for i, j := 0, len(sprints)-1; i < j; i, j = i+1, j-1 {
		sprints[i], sprints[j] = sprints[j], sprints[i]
	}

	// Streaks: consecutive local days with at least one task completed.
	rows, err = s.db.Query(ctx, `SELECT DISTINCT completed_by, (completed_at AT TIME ZONE $2)::date::text FROM tasks
		WHERE household_id = $1 AND completed_at > now() - interval '400 days' AND completed_by IS NOT NULL`, h.ID, h.Timezone)
	if err != nil {
		return err
	}
	daysBy := map[string]map[string]bool{}
	for rows.Next() {
		var uid, d string
		if err := rows.Scan(&uid, &d); err != nil {
			rows.Close()
			return err
		}
		if daysBy[uid] == nil {
			daysBy[uid] = map[string]bool{}
		}
		daysBy[uid][d] = true
	}
	rows.Close()
	type streak struct {
		Current int  `json:"current"`
		Best    int  `json:"best"`
		Today   bool `json:"today"` // already did something today
	}
	today := localToday(h, time.Now())
	streaks := map[string]streak{}
	for uid, set := range daysBy {
		var st streak
		st.Today = set[today.Format(time.DateOnly)]
		// Today doesn't break the streak until it's over.
		d := today
		if !st.Today {
			d = d.AddDate(0, 0, -1)
		}
		for set[d.Format(time.DateOnly)] {
			st.Current++
			d = d.AddDate(0, 0, -1)
		}
		run := 0
		for d := today.AddDate(0, 0, -400); !d.After(today); d = d.AddDate(0, 0, 1) {
			if set[d.Format(time.DateOnly)] {
				run++
				st.Best = max(st.Best, run)
			} else {
				run = 0
			}
		}
		streaks[uid] = st
	}

	rows, _ = s.db.Query(ctx, `SELECT k.id, k.from_id, k.to_id, k.task_id, coalesce(t.title, ''), k.emoji, k.message, k.created_at
		FROM kudos k LEFT JOIN tasks t ON t.id = k.task_id WHERE k.household_id = $1 ORDER BY k.created_at DESC LIMIT 20`, h.ID)
	kudos, err := pgx.CollectRows(rows, pgx.RowToStructByPos[Kudos])
	if err != nil {
		return err
	}
	received := map[string]int{}
	rows, err = s.db.Query(ctx, `SELECT to_id, count(*)::int FROM kudos WHERE household_id = $1 GROUP BY 1`, h.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id string
		var c int
		rows.Scan(&id, &c)
		received[id] = c
	}
	rows.Close()
	return ok(w, map[string]any{"sprints": sprints, "streaks": streaks, "kudos": kudos, "kudosReceived": received})
}

type Kudos struct {
	ID        string    `json:"id"`
	FromID    string    `json:"fromId"`
	ToID      string    `json:"toId"`
	TaskID    *string   `json:"taskId"`
	TaskTitle string    `json:"taskTitle"`
	Emoji     string    `json:"emoji"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"createdAt"`
}

var kudosEmoji = []string{"👏", "🙌", "❤️", "🌟", "💪", "🎉", "🙏", "🔥"}

func (s *Server) giveKudos(w http.ResponseWriter, r *http.Request) error {
	u := currentUser(r)
	ctx := r.Context()
	var in struct {
		ToID    string  `json:"toId"`
		TaskID  *string `json:"taskId"`
		Emoji   string  `json:"emoji"`
		Message string  `json:"message"`
	}
	if err := decode(r, &in); err != nil {
		return err
	}
	if in.ToID == u.ID {
		return errBadRequest("Kudos are for your partner")
	}
	if err := s.checkMember(ctx, u.HouseholdID, &in.ToID); err != nil {
		return err
	}
	if in.Emoji == "" {
		in.Emoji = "👏"
	}
	valid := false
	for _, e := range kudosEmoji {
		valid = valid || e == in.Emoji
	}
	if !valid {
		return errBadRequest("Pick one of the emoji")
	}
	in.Message = truncate(strings.TrimSpace(in.Message), 200)
	var taskTitle string
	if in.TaskID != nil {
		if err := s.db.QueryRow(ctx, `SELECT title FROM tasks WHERE id = $1 AND household_id = $2`, *in.TaskID, u.HouseholdID).Scan(&taskTitle); err != nil {
			return errBadRequest("Unknown task")
		}
	}
	var id string
	err := s.db.QueryRow(ctx, `INSERT INTO kudos (household_id, from_id, to_id, task_id, emoji, message) VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (from_id, task_id) WHERE task_id IS NOT NULL DO NOTHING RETURNING id`,
		u.HouseholdID, u.ID, in.ToID, in.TaskID, in.Emoji, in.Message).Scan(&id)
	if err == pgx.ErrNoRows {
		return errConflict("You already sent kudos for this task")
	}
	if err != nil {
		return err
	}
	body := in.Message
	if body == "" && taskTitle != "" {
		body = "For: " + taskTitle
	}
	logErr("kudos notify", s.notify(ctx, notification{
		HouseholdID: u.HouseholdID, UserID: in.ToID, Kind: "kudos", DedupeKey: "kudos:" + id, Title: in.Emoji + " Kudos from " + u.DisplayName, Body: body, URL: "/retro",
	}))
	s.hub.Publish(u.HouseholdID, eventRetro.from(r))
	return ok(w, map[string]string{"id": id})
}
