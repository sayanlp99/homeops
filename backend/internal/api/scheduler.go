package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	morningDigestAt = 8 * 60  // 08:00 local: what's due today
	eveningNudgeAt  = 19 * 60 // 19:00 local: what's still open
)

func (s *Server) runScheduler(ctx context.Context) {
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		s.schedulerTick(ctx, time.Now())
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (s *Server) schedulerTick(ctx context.Context, now time.Time) {
	rows, _ := s.db.Query(ctx, `SELECT `+householdColumns+` FROM households`)
	households, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[Household])
	if err != nil {
		logErr("scheduler: load households", err)
		return
	}
	for _, h := range households {
		logErr("scheduler: sprints", s.ensureSprints(ctx, h.ID), "household", h.ID)
		local := now.In(h.Location())
		minute := local.Hour()*60 + local.Minute()
		today := localToday(h, now).Format(time.DateOnly)

		// Each job runs on the first tick at/after its time; dedupe keys stop repeats.
		if minute >= morningDigestAt && minute < 22*60 {
			logErr("scheduler: morning digest", s.dueDigest(ctx, h, today, false))
		}
		if minute >= eveningNudgeAt && minute < 22*60 {
			logErr("scheduler: evening nudge", s.dueDigest(ctx, h, today, true))
		}
		logErr("scheduler: ceremonies", s.ceremonyReminders(ctx, h, local))
	}
	// Housekeeping.
	_, err = s.db.Exec(ctx, `DELETE FROM sessions WHERE expires_at < now()`)
	logErr("scheduler: expire sessions", err)
	_, err = s.db.Exec(ctx, `DELETE FROM notifications WHERE created_at < now() - interval '90 days'`)
	logErr("scheduler: prune notifications", err)
}

// dueDigest sends each member one summary of their open tasks due today (morning) or
// still open from today and before (evening). Unassigned tasks go to everyone.
func (s *Server) dueDigest(ctx context.Context, h *Household, today string, evening bool) error {
	cond := `t.due_date = $2::date`
	kind, prefix := "task_due", "due"
	if evening {
		cond = `t.due_date <= $2::date`
		kind, prefix = "task_overdue", "open"
	}
	rows, _ := s.db.Query(ctx, `SELECT u.id, t.number, t.title, t.due_date < $2::date AS overdue
		FROM users u JOIN tasks t ON t.household_id = u.household_id AND (t.assignee_id = u.id OR t.assignee_id IS NULL)
		WHERE u.household_id = $1 AND t.status <> 'done' AND `+cond+`
		ORDER BY u.id, t.priority DESC, t.rank`, h.ID, today)
	type item struct {
		userID  string
		number  int
		title   string
		overdue bool
	}
	items, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (item, error) {
		var it item
		err := row.Scan(&it.userID, &it.number, &it.title, &it.overdue)
		return it, err
	})
	if err != nil {
		return err
	}
	byUser := map[string][]item{}
	var order []string
	for _, it := range items {
		if _, seen := byUser[it.userID]; !seen {
			order = append(order, it.userID)
		}
		byUser[it.userID] = append(byUser[it.userID], it)
	}
	for _, uid := range order {
		list := byUser[uid]
		overdue := 0
		titles := make([]string, 0, 3)
		for i, it := range list {
			if it.overdue {
				overdue++
			}
			if i < 3 {
				titles = append(titles, it.title)
			}
		}
		body := strings.Join(titles, " · ")
		if len(list) > 3 {
			body += fmt.Sprintf(" +%d more", len(list)-3)
		}
		var title string
		if evening {
			title = fmt.Sprintf("%d %s still open", len(list), plural(len(list), "task", "tasks"))
			if overdue > 0 {
				title += fmt.Sprintf(" (%d overdue)", overdue)
			}
		} else {
			title = fmt.Sprintf("Today: %d %s due", len(list), plural(len(list), "task", "tasks"))
		}
		url := "/board?mine=1"
		if len(list) == 1 {
			url = fmt.Sprintf("/tasks/%d", list[0].number)
		}
		if err := s.notify(ctx, notification{
			UserID: uid, Kind: kind, DedupeKey: prefix + ":" + today, Title: title, Body: body, URL: url,
		}); err != nil {
			return err
		}
	}
	return nil
}

// ceremonyReminders nudges both members 30 minutes before, and at, sprint planning.
func (s *Server) ceremonyReminders(ctx context.Context, h *Household, local time.Time) error {
	if int(local.Weekday()) != h.PlanningDow {
		return nil
	}
	at, err := parseClock(h.PlanningTime)
	if err != nil {
		return nil
	}
	minute := local.Hour()*60 + local.Minute()
	date := local.Format(time.DateOnly)
	switch {
	case minute >= at && minute < at+60:
		return s.notifyHousehold(ctx, h.ID, notification{
			Kind: "ceremony", DedupeKey: "planning-now:" + date,
			Title: "Sprint planning time", Body: "Review last week, then plan next week's tasks together.", URL: "/backlog",
		})
	case minute >= at-30 && minute < at:
		return s.notifyHousehold(ctx, h.ID, notification{
			Kind: "ceremony", DedupeKey: "planning-soon:" + date,
			Title: "Sprint planning in 30 minutes", Body: "Grab a cup of chai ☕", URL: "/backlog",
		})
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
