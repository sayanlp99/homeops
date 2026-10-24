-- Nutrition: health profiles, who eats what, seasons, and ingredient → food links.

-- One per person, shared by all their households. Any non-viewer member of a
-- household the person belongs to may edit it (a couple managing each other's plan).
CREATE TABLE health_profiles (
    user_id     uuid PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    sex         text NOT NULL CHECK (sex IN ('female', 'male')),
    birth_year  int  NOT NULL CHECK (birth_year BETWEEN 1900 AND 2100),
    height_cm   real NOT NULL CHECK (height_cm BETWEEN 100 AND 230),
    weight_kg   real NOT NULL CHECK (weight_kg BETWEEN 25 AND 250),
    activity    text NOT NULL DEFAULT 'light' CHECK (activity IN ('sedentary', 'light', 'moderate', 'active')),
    goal        text NOT NULL DEFAULT 'maintain' CHECK (goal IN ('lose', 'maintain', 'gain')),
    avoid       text[] NOT NULL DEFAULT '{}',   -- food groups: fish, seafood, chicken, mutton, egg, dairy…
    overrides   jsonb NOT NULL DEFAULT '{}',    -- per-nutrient daily targets set by hand
    updated_at  timestamptz NOT NULL DEFAULT now(),
    updated_by  uuid REFERENCES users(id) ON DELETE SET NULL
);

-- Who eats a planned dish and how much: {"<user id>": servings}. NULL = everyone who
-- can eat it has one serving. A leftover is eaten but not cooked again, so it adds
-- nothing to the grocery list or the pantry.
ALTER TABLE meal_plan_items
    ADD COLUMN portions jsonb,
    ADD COLUMN leftover boolean NOT NULL DEFAULT false;

-- Months (1–12) a dish is in season; empty = all year.
ALTER TABLE recipes ADD COLUMN months smallint[] NOT NULL DEFAULT '{}';

-- The food-table entry an ingredient is, when matching by name isn't right.
-- '' = match by name, '-' = leave out of nutrition.
ALTER TABLE ingredients ADD COLUMN food_key text NOT NULL DEFAULT '';
