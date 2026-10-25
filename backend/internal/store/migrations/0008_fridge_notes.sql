-- Fridge notes: sticky notes on the home screen that everyone in the household sees.

CREATE TABLE fridge_notes (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    author_id     uuid REFERENCES users(id) ON DELETE SET NULL,
    text          text NOT NULL CHECK (length(text) BETWEEN 1 AND 500),
    color         text NOT NULL DEFAULT 'yellow' CHECK (color IN ('yellow', 'pink', 'blue', 'green', 'orange')),
    pinned        boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fridge_notes_household_idx ON fridge_notes (household_id, created_at DESC);
