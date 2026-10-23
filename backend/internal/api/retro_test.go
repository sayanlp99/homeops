package api

import (
	"strings"
	"testing"
)

func TestRetroAndInsights(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, ts, _ := startTestServer(t, 54332)
	a := newClient(t, ts.URL)
	a.do("POST", "/api/setup", map[string]any{"householdName": "Home", "username": "asha", "displayName": "Asha", "password": "hunter2hunter2"}, 204, nil)
	var inv map[string]string
	a.do("POST", "/api/invites", nil, 200, &inv)
	b := newClient(t, ts.URL)
	b.do("POST", "/api/auth/invite/accept", map[string]any{"code": inv["code"], "username": "ravi", "displayName": "Ravi", "password": "correcthorse"}, 204, nil)
	var boot map[string]any
	a.do("GET", "/api/bootstrap", nil, 200, &boot)
	aID := boot["user"].(map[string]any)["id"].(string)
	b.do("GET", "/api/bootstrap", nil, 200, &boot)
	bID := boot["user"].(map[string]any)["id"].(string)

	var sprints []Sprint
	a.do("GET", "/api/sprints", nil, 200, &sprints)
	active, next := sprints[1], sprints[0]
	a.do("GET", "/api/retro?sprint="+next.ID, nil, 400, nil) // planned sprint

	type retroResp struct {
		Retro  Retro
		Cards  []RetroCard
		Hidden map[string]int
	}
	var ra, rb retroResp
	a.do("GET", "/api/retro?sprint="+active.ID, nil, 200, &ra)
	if ra.Retro.Status != "collecting" {
		t.Fatalf("retro: %+v", ra.Retro)
	}
	var c1 map[string]string
	a.do("POST", "/api/retros/"+ra.Retro.ID+"/cards", map[string]any{"col": "went_well", "text": "Kitchen stayed clean all week"}, 200, &c1)
	a.do("POST", "/api/retros/"+ra.Retro.ID+"/cards", map[string]any{"col": "improve", "text": "Too many takeaway dinners"}, 200, nil)
	a.do("POST", "/api/retros/"+ra.Retro.ID+"/cards", map[string]any{"col": "nope", "text": "x"}, 400, nil)

	// Private until reveal: the partner sees a count, not the text, and can't edit it.
	b.do("GET", "/api/retro?sprint="+active.ID, nil, 200, &rb)
	if len(rb.Cards) != 0 || rb.Hidden[aID] != 2 {
		t.Fatalf("collecting privacy: %+v", rb)
	}
	b.do("PATCH", "/api/retro-cards/"+c1["id"], map[string]any{"text": "edited"}, 403, nil)
	b.do("DELETE", "/api/retro-cards/"+c1["id"], nil, 403, nil)
	var act map[string]string
	b.do("POST", "/api/retros/"+ra.Retro.ID+"/cards", map[string]any{"col": "action", "text": "Plan dinners on Sunday", "assigneeId": aID}, 200, &act)
	b.do("POST", "/api/retro-cards/"+act["id"]+"/vote", nil, 400, nil) // voting opens after reveal
	b.do("POST", "/api/retros/"+ra.Retro.ID+"/close", nil, 400, nil)   // must reveal first

	b.do("POST", "/api/retros/"+ra.Retro.ID+"/reveal", nil, 204, nil)
	var inbox struct{ Items []inboxItem }
	a.do("GET", "/api/notifications", nil, 200, &inbox)
	found := false
	for _, n := range inbox.Items {
		found = found || strings.Contains(n.Title, "revealed the retro")
	}
	if !found {
		t.Fatalf("reveal notification missing: %+v", inbox.Items)
	}
	a.do("GET", "/api/retro?sprint="+active.ID, nil, 200, &ra)
	if len(ra.Cards) != 3 || len(ra.Hidden) != 0 {
		t.Fatalf("revealed: %+v", ra)
	}
	a.do("POST", "/api/retro-cards/"+act["id"]+"/vote", nil, 204, nil)
	b.do("POST", "/api/retro-cards/"+act["id"]+"/vote", nil, 204, nil)
	b.do("POST", "/api/retro-cards/"+act["id"]+"/vote", nil, 204, nil) // toggle off
	b.do("POST", "/api/retro-cards/"+act["id"]+"/vote", nil, 204, nil) // and on
	a.do("GET", "/api/retro?sprint="+active.ID, nil, 200, &ra)
	for _, c := range ra.Cards {
		if c.ID == act["id"] && c.Votes != 2 {
			t.Fatalf("votes: %+v", c)
		}
	}

	var closed map[string]int
	a.do("POST", "/api/retros/"+ra.Retro.ID+"/close", nil, 200, &closed)
	if closed["tasksCreated"] != 1 {
		t.Fatalf("close: %v", closed)
	}
	var tasks []Task
	a.do("GET", "/api/tasks", nil, 200, &tasks)
	var actionTask *Task
	for i := range tasks {
		if tasks[i].Title == "Plan dinners on Sunday" {
			actionTask = &tasks[i]
		}
	}
	if actionTask == nil || *actionTask.SprintID != next.ID || *actionTask.AssigneeID != aID || actionTask.Priority != 3 {
		t.Fatalf("action task: %+v", actionTask)
	}
	a.do("POST", "/api/retros/"+ra.Retro.ID+"/cards", map[string]any{"col": "improve", "text": "late"}, 400, nil)
	var list []map[string]any
	a.do("GET", "/api/retros", nil, 200, &list)
	if len(list) != 1 || list[0]["status"] != "closed" || list[0]["actions"].(float64) != 1 {
		t.Fatalf("retro list: %v", list)
	}

	// Insights: a pointed task done by Asha moves burndown, fairness and her streak.
	var done Task
	a.do("POST", "/api/tasks", map[string]any{"title": "Deep clean fridge", "sprintId": active.ID, "points": 3}, 201, &done)
	a.do("POST", "/api/tasks", map[string]any{"title": "Fix tap", "sprintId": active.ID, "points": 2}, 201, nil)
	a.do("PATCH", "/api/tasks/"+done.ID, map[string]any{"status": "done"}, 200, nil)
	var bd struct {
		Total int
		Unit  string
		Days  []struct {
			Date      string
			Remaining *int
		}
	}
	a.do("GET", "/api/metrics/burndown?sprint="+active.ID, nil, 200, &bd)
	if bd.Total != 5 || bd.Unit != "points" || len(bd.Days) != 7 {
		t.Fatalf("burndown: %+v", bd)
	}
	var last *int
	for _, d := range bd.Days {
		if d.Remaining != nil {
			last = d.Remaining
		}
	}
	if last == nil || *last != 2 {
		t.Fatalf("burndown remaining today: %v", last)
	}
	var team struct {
		Sprints []struct {
			ID       string
			Planned  int
			Done     int
			ByMember map[string]int
		}
		Streaks map[string]struct {
			Current int
			Today   bool
		}
	}
	a.do("GET", "/api/metrics/team", nil, 200, &team)
	cur := team.Sprints[len(team.Sprints)-1]
	if cur.ID != active.ID || cur.Done != 3 || cur.ByMember[aID] != 3 || team.Streaks[aID].Current != 1 || !team.Streaks[aID].Today {
		t.Fatalf("team metrics: %+v", team)
	}

	// Kudos: to the partner only, once per task, with a notification.
	b.do("POST", "/api/kudos", map[string]any{"toId": aID, "taskId": done.ID, "emoji": "🌟", "message": "Fridge looks new!"}, 200, nil)
	b.do("POST", "/api/kudos", map[string]any{"toId": aID, "taskId": done.ID}, 409, nil)
	b.do("POST", "/api/kudos", map[string]any{"toId": bID}, 400, nil)
	b.do("POST", "/api/kudos", map[string]any{"toId": aID, "emoji": "💩"}, 400, nil)
	a.do("GET", "/api/notifications", nil, 200, &inbox)
	if !strings.Contains(inbox.Items[0].Title, "Kudos from Ravi") {
		t.Fatalf("kudos notification: %+v", inbox.Items[0])
	}
}
