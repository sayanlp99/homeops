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
	menuAt          = 7 * 60  // 07:00 local: today's menu
	expiryAt        = 9 * 60  // 09:00 local: pantry items to use soon
)

var slotLabels = map[string]string{"breakfast": "Breakfast", "lunch": "Lunch", "snacks": "Snacks", "dinner": "Dinner"}

// menuDigest tells both of you what's planned today.
func (s *Server) menuDigest(ctx context.Context, h *Household, today string) error {
	rows, err := s.db.Query(ctx, `SELECT slot, title FROM meal_plan_items WHERE household_id = $1 AND date = $2
		ORDER BY array_position(ARRAY['breakfast','lunch','snacks','dinner'], slot), position`, h.ID, today)
	if err != nil {
		return err
	}
	bySlot := map[string][]string{}
	for rows.Next() {
		var slot, title string
		if err := rows.Scan(&slot, &title); err != nil {
			rows.Close()
			return err
		}
		bySlot[slot] = append(bySlot[slot], title)
	}
	rows.Close()
	if len(bySlot) == 0 {
		return nil
	}
	var parts []string
	for _, slot := range mealSlots {
		if d := bySlot[slot]; len(d) > 0 {
			parts = append(parts, slotLabels[slot]+": "+strings.Join(d, ", "))
		}
	}
	return s.notifyHousehold(ctx, h.ID, notification{
		Kind: "meals", DedupeKey: "menu:" + today, Title: "Today's menu", Body: strings.Join(parts, " · "), URL: "/meals",
	})
}

// prepReminders fire `prep_lead_hours` before a meal whose recipe needs preparation,
// e.g. "Soak rajma overnight" 10 hours before lunch.
func (s *Server) prepReminders(ctx context.Context, h *Household, now time.Time) error {
	loc := h.Location()
	today := localToday(h, now)
	rows, err := s.db.Query(ctx, `SELECT m.id, m.date, m.slot, m.title, r.prep_note, r.prep_lead_hours
		FROM meal_plan_items m JOIN recipes r ON r.id = m.recipe_id
		WHERE m.household_id = $1 AND m.date BETWEEN $2 AND $3 AND r.prep_lead_hours > 0 AND m.cooked_at IS NULL AND NOT m.leftover`,
		h.ID, today, today.AddDate(0, 0, 4))
	if err != nil {
		return err
	}
	type due struct{ id, title, note, slot string }
	var send []due
	for rows.Next() {
		var id, slot, title, note string
		var date time.Time
		var lead int
		if err := rows.Scan(&id, &date, &slot, &title, &note, &lead); err != nil {
			rows.Close()
			return err
		}
		mins, err := parseClock(h.MealTimes[slot])
		if err != nil {
			mins = 13 * 60
		}
		mealAt := time.Date(date.Year(), date.Month(), date.Day(), mins/60, mins%60, 0, 0, loc)
		remindAt := mealAt.Add(-time.Duration(lead) * time.Hour)
		if !now.Before(remindAt) && now.Before(mealAt) {
			send = append(send, due{id, title, note, slot})
		}
	}
	rows.Close()
	for _, d := range send {
		body := d.note
		if body == "" {
			body = "Time to start prepping."
		}
		if err := s.notifyHousehold(ctx, h.ID, notification{
			Kind: "meals", DedupeKey: "prep:" + d.id, Title: "Prep for " + d.title + " (" + strings.ToLower(slotLabels[d.slot]) + ")", Body: body, URL: "/meals",
		}); err != nil {
			return err
		}
	}
	return nil
}

// expiryAlert lists pantry items that expire within two days.
func (s *Server) expiryAlert(ctx context.Context, h *Household, today string) error {
	rows, err := s.db.Query(ctx, `SELECT i.name, (p.expires_on - $2::date) FROM pantry_items p JOIN ingredients i ON i.id = p.ingredient_id
		WHERE p.household_id = $1 AND p.expires_on IS NOT NULL AND p.expires_on <= $2::date + 2
		AND (p.quantity IS NULL OR p.quantity > 0) ORDER BY p.expires_on LIMIT 8`, h.ID, today)
	if err != nil {
		return err
	}
	var names []string
	var days []int
	for rows.Next() {
		var n string
		var d int
		if err := rows.Scan(&n, &d); err != nil {
			rows.Close()
			return err
		}
		names, days = append(names, n), append(days, d)
	}
	rows.Close()
	if len(names) == 0 {
		return nil
	}
	return s.notifyHousehold(ctx, h.ID, notification{
		Kind: "meals", DedupeKey: "expiry:" + today, Title: fmt.Sprintf("Use soon: %d %s", len(names), plural(len(names), "item", "items")),
		Body: expiringSummary(names, days), URL: "/pantry",
	})
}

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
		if minute >= menuAt && minute < 11*60 {
			logErr("scheduler: menu", s.menuDigest(ctx, h, today))
		}
		if minute >= expiryAt && minute < 21*60 {
			logErr("scheduler: expiry", s.expiryAlert(ctx, h, today))
			logErr("scheduler: bills", s.billReminders(ctx, h, localToday(h, now)))
		}
		logErr("scheduler: prep", s.prepReminders(ctx, h, now))
	}
	if now.Minute() == 0 && now.Hour() == 4 {
		logErr("scheduler: prune files", s.pruneOrphanFiles(ctx))
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
		FROM members u JOIN tasks t ON t.household_id = u.household_id AND (t.assignee_id = u.id OR t.assignee_id IS NULL)
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
			HouseholdID: h.ID, UserID: uid, Kind: kind, DedupeKey: prefix + ":" + today, Title: title, Body: body, URL: url,
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
