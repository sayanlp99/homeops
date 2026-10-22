-- Phase 2: recipes, meal plan, pantry, grocery list.

ALTER TABLE households
    ADD COLUMN meal_times jsonb NOT NULL DEFAULT '{"breakfast":"08:00","lunch":"13:30","snacks":"17:30","dinner":"21:00"}';

-- Household ingredient catalog: one row per thing you buy, with the aisle it lives in.
CREATE TABLE ingredients (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name          text NOT NULL,
    aisle         text NOT NULL DEFAULT 'Other',
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX ingredients_name_idx ON ingredients (household_id, lower(name));

CREATE TABLE recipes (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id     uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    title            text NOT NULL,
    description      text NOT NULL DEFAULT '',
    category         text NOT NULL DEFAULT 'main',
    meal_types       text[] NOT NULL DEFAULT '{}',
    diet             text NOT NULL DEFAULT 'veg' CHECK (diet IN ('veg', 'egg', 'nonveg', 'vegan')),
    cuisine          text NOT NULL DEFAULT '',
    tags             text[] NOT NULL DEFAULT '{}',
    servings         int  NOT NULL DEFAULT 2 CHECK (servings BETWEEN 1 AND 50),
    prep_minutes     int,
    cook_minutes     int,
    prep_note        text NOT NULL DEFAULT '',  -- e.g. "Soak rajma overnight"
    prep_lead_hours  int  NOT NULL DEFAULT 0,   -- remind this many hours before the meal
    steps            jsonb NOT NULL DEFAULT '[]',
    source_url       text NOT NULL DEFAULT '',
    photo            text NOT NULL DEFAULT '',  -- file id under uploads/recipes
    favorite         boolean NOT NULL DEFAULT false,
    rating           int NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX recipes_household_idx ON recipes (household_id);

CREATE TABLE recipe_ingredients (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    recipe_id      uuid NOT NULL REFERENCES recipes(id) ON DELETE CASCADE,
    position       int  NOT NULL,
    ingredient_id  uuid NOT NULL REFERENCES ingredients(id) ON DELETE RESTRICT,
    quantity       double precision,  -- null = "to taste"
    unit           text NOT NULL DEFAULT '',
    note           text NOT NULL DEFAULT ''
);
CREATE INDEX recipe_ingredients_recipe_idx ON recipe_ingredients (recipe_id, position);

-- A slot (e.g. Tuesday lunch) holds several dishes: dal + sabzi + rice.
CREATE TABLE meal_plan_items (
    id            uuid PRIMARY KEY,
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    date          date NOT NULL,
    slot          text NOT NULL CHECK (slot IN ('breakfast', 'lunch', 'snacks', 'dinner')),
    position      double precision NOT NULL DEFAULT 0,
    recipe_id     uuid REFERENCES recipes(id) ON DELETE SET NULL,
    title         text NOT NULL,  -- recipe title snapshot, or a free-text dish
    servings      int  NOT NULL DEFAULT 2 CHECK (servings BETWEEN 1 AND 50),
    note          text NOT NULL DEFAULT '',
    cooked_at     timestamptz,
    cooked_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    deductions    jsonb NOT NULL DEFAULT '[]',  -- pantry amounts taken when cooked, for undo
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX meal_plan_items_date_idx ON meal_plan_items (household_id, date);
CREATE INDEX meal_plan_items_recipe_idx ON meal_plan_items (recipe_id);

CREATE TABLE pantry_items (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id   uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    ingredient_id  uuid NOT NULL REFERENCES ingredients(id) ON DELETE CASCADE,
    quantity       double precision,  -- null = in stock, amount not tracked
    unit           text NOT NULL DEFAULT '',
    low_threshold  double precision,
    location       text NOT NULL DEFAULT 'pantry' CHECK (location IN ('pantry', 'fridge', 'freezer')),
    expires_on     date,
    updated_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (household_id, ingredient_id)
);

CREATE TABLE grocery_items (
    id             uuid PRIMARY KEY,
    household_id   uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    ingredient_id  uuid REFERENCES ingredients(id) ON DELETE SET NULL,
    name           text NOT NULL,
    quantity       double precision,
    unit           text NOT NULL DEFAULT '',
    aisle          text NOT NULL DEFAULT 'Other',
    note           text NOT NULL DEFAULT '',
    source         text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'plan', 'pantry')),
    checked_at     timestamptz,
    checked_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    created_by     uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX grocery_items_household_idx ON grocery_items (household_id);

-- A finished shopping trip; Phase 3 links these to budget expenses.
CREATE TABLE grocery_trips (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id   uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    completed_by   uuid REFERENCES users(id) ON DELETE SET NULL,
    completed_at   timestamptz NOT NULL DEFAULT now(),
    item_count     int NOT NULL,
    items          jsonb NOT NULL DEFAULT '[]',
    total_amount   double precision,
    store          text NOT NULL DEFAULT ''
);
CREATE INDEX grocery_trips_household_idx ON grocery_trips (household_id, completed_at DESC);
