-- A cooking task remembers the planned dish it was made for, so the meal planner can
-- show whether each dish has a task and which sprint it's in.
ALTER TABLE tasks ADD COLUMN meal_id uuid REFERENCES meal_plan_items(id) ON DELETE SET NULL;
CREATE INDEX tasks_meal_id_idx ON tasks (meal_id) WHERE meal_id IS NOT NULL;

-- Link the cooking tasks made before this: "Cook <dish>" due on the dish's day. Two
-- dishes with the same name that day (rotis at lunch and dinner) pair up in order.
WITH t AS (
  SELECT id, household_id, due_date, substr(title, 6) AS dish,
         row_number() OVER (PARTITION BY household_id, due_date, title ORDER BY number) AS n
  FROM tasks WHERE title LIKE 'Cook %' AND due_date IS NOT NULL AND recurring_id IS NULL
), m AS (
  SELECT id, household_id, date, title,
         row_number() OVER (PARTITION BY household_id, date, title
           ORDER BY array_position(ARRAY['breakfast','lunch','snacks','dinner'], slot), position) AS n
  FROM meal_plan_items WHERE NOT leftover
)
UPDATE tasks SET meal_id = m.id
FROM t JOIN m ON m.household_id = t.household_id AND m.date = t.due_date AND m.title = t.dish AND m.n = t.n
WHERE tasks.id = t.id;
