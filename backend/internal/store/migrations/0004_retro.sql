-- Phase 4: retrospectives and kudos.

CREATE TABLE retros (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    sprint_id     uuid NOT NULL UNIQUE REFERENCES sprints(id) ON DELETE CASCADE,
    -- collecting: cards are private to their author; revealed: everyone sees and votes;
    -- closed: action items have been turned into tasks.
    status        text NOT NULL DEFAULT 'collecting' CHECK (status IN ('collecting', 'revealed', 'closed')),
    revealed_at   timestamptz,
    closed_at     timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE retro_cards (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    retro_id     uuid NOT NULL REFERENCES retros(id) ON DELETE CASCADE,
    author_id    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    col          text NOT NULL CHECK (col IN ('went_well', 'improve', 'action')),
    text         text NOT NULL,
    assignee_id  uuid REFERENCES users(id) ON DELETE SET NULL,  -- for action items
    task_id      uuid REFERENCES tasks(id) ON DELETE SET NULL,  -- task created on close
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX retro_cards_retro_idx ON retro_cards (retro_id);

CREATE TABLE retro_votes (
    card_id  uuid NOT NULL REFERENCES retro_cards(id) ON DELETE CASCADE,
    user_id  uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (card_id, user_id)
);

CREATE TABLE kudos (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    from_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    to_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    task_id       uuid REFERENCES tasks(id) ON DELETE SET NULL,
    emoji         text NOT NULL DEFAULT '👏',
    message       text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    CHECK (from_id <> to_id)
);
CREATE INDEX kudos_household_idx ON kudos (household_id, created_at DESC);
CREATE UNIQUE INDEX kudos_once_per_task_idx ON kudos (from_id, task_id) WHERE task_id IS NOT NULL;
