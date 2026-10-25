package api

import (
	"strings"
	"testing"
)

func TestCookingTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("integration test")
	}
	_, ts, _ := startTestServer(t, 54337)
	c := newClient(t, ts.URL)
	c.do("POST", "/api/setup", map[string]any{"householdName": "Home", "email": "cook@example.com", "username": "cook", "displayName": "Cook", "password": "hunter2hunter2"}, 204, nil)

	var meal MealItem
	c.do("POST", "/api/meals", map[string]any{"date": "2031-03-03", "slot": "lunch", "title": "Toor dal", "servings": 2}, 201, &meal)

	// A cooking task remembers its dish.
	var task Task
	c.do("POST", "/api/tasks", map[string]any{"title": "Cook Toor dal", "description": "Lunch on Mon 3 Mar · 2 servings", "dueDate": "2031-03-03", "mealId": meal.ID}, 201, &task)
	if task.MealID == nil || *task.MealID != meal.ID {
		t.Fatalf("meal link: %+v", task.MealID)
	}
	c.do("POST", "/api/tasks", map[string]any{"title": "Cook nothing", "mealId": "00000000-0000-0000-0000-000000000000"}, 400, nil)

	// Moving the dish moves its open task, day and description both.
	c.do("PATCH", "/api/meals/"+meal.ID, map[string]any{"date": "2031-03-04"}, 200, nil)
	var tasks []Task
	c.do("GET", "/api/tasks", nil, 200, &tasks)
	if len(tasks) != 1 || *tasks[0].DueDate != "2031-03-04" || !strings.HasPrefix(tasks[0].Description, "Lunch on Tue 4 Mar") {
		t.Fatalf("moved task: %+v", tasks)
	}

	// A done task stays where it was.
	c.do("PATCH", "/api/tasks/"+task.ID, map[string]any{"status": "done"}, 200, nil)
	c.do("PATCH", "/api/meals/"+meal.ID, map[string]any{"date": "2031-03-05"}, 200, nil)
	c.do("GET", "/api/tasks", nil, 200, &tasks)
	if *tasks[0].DueDate != "2031-03-04" || tasks[0].MealID == nil {
		t.Fatalf("done task moved: %+v", tasks[0])
	}

	// Removing the dish keeps the task, unlinked.
	c.do("DELETE", "/api/meals/"+meal.ID, nil, 204, nil)
	c.do("GET", "/api/tasks", nil, 200, &tasks)
	if len(tasks) != 1 || tasks[0].MealID != nil {
		t.Fatalf("after removing the dish: %+v", tasks)
	}
}
