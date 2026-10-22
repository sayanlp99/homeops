-- Phase 1: identity, sprints, tasks, recurring chores, notifications.

CREATE TABLE households (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name               text NOT NULL,
    timezone           text NOT NULL DEFAULT 'Asia/Kolkata',
    -- Sprint cadence. Day-of-week values follow Go's time.Weekday (0 = Sunday).
    sprint_start_dow   int  NOT NULL DEFAULT 1 CHECK (sprint_start_dow BETWEEN 0 AND 6),
    planning_dow       int  NOT NULL DEFAULT 0 CHECK (planning_dow BETWEEN 0 AND 6),
    planning_time      text NOT NULL DEFAULT '20:00',
    auto_roll          boolean NOT NULL DEFAULT true,
    next_task_number   int  NOT NULL DEFAULT 1,
    created_at         timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE users (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id   uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    username       text NOT NULL UNIQUE,
    display_name   text NOT NULL,
    password_hash  text NOT NULL,
    role           text NOT NULL DEFAULT 'member' CHECK (role IN ('admin', 'member')),
    color          text NOT NULL DEFAULT '#2563eb',
    notify_prefs   jsonb NOT NULL DEFAULT '{}',
    quiet_start    text,  -- 'HH:MM' local time, null = no quiet hours
    quiet_end      text,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    token_hash    bytea PRIMARY KEY,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_agent    text NOT NULL DEFAULT '',
    created_at    timestamptz NOT NULL DEFAULT now(),
    last_seen_at  timestamptz NOT NULL DEFAULT now(),
    expires_at    timestamptz NOT NULL
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE passkeys (
    credential_id  bytea PRIMARY KEY,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name           text NOT NULL,
    data           jsonb NOT NULL,  -- serialized webauthn.Credential
    created_at     timestamptz NOT NULL DEFAULT now(),
    last_used_at   timestamptz
);
CREATE INDEX passkeys_user_idx ON passkeys (user_id);

CREATE TABLE invites (
    code          text PRIMARY KEY,
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    created_by    uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at    timestamptz NOT NULL,
    used_at       timestamptz
);

CREATE TABLE app_settings (
    key    text PRIMARY KEY,
    value  jsonb NOT NULL
);

CREATE TABLE labels (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    name          text NOT NULL,
    color         text NOT NULL,
    UNIQUE (household_id, name)
);

CREATE TABLE sprints (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    number        int  NOT NULL,
    goal          text NOT NULL DEFAULT '',
    starts_on     date NOT NULL,
    ends_on       date NOT NULL,
    status        text NOT NULL DEFAULT 'planned' CHECK (status IN ('planned', 'active', 'completed')),
    started_at    timestamptz,
    completed_at  timestamptz,
    UNIQUE (household_id, number)
);
CREATE UNIQUE INDEX sprints_one_active_idx ON sprints (household_id) WHERE status = 'active';

CREATE TABLE recurring_tasks (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id      uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    title             text NOT NULL,
    description       text NOT NULL DEFAULT '',
    priority          int  NOT NULL DEFAULT 2 CHECK (priority BETWEEN 0 AND 4),
    points            int,
    labels            uuid[] NOT NULL DEFAULT '{}',
    -- Assignment: a fixed user, or rotate between members each occurrence.
    assignee_id       uuid REFERENCES users(id) ON DELETE SET NULL,
    rotate            boolean NOT NULL DEFAULT false,
    rotate_index      int NOT NULL DEFAULT 0,
    freq              text NOT NULL CHECK (freq IN ('daily', 'weekly', 'monthly')),
    interval          int  NOT NULL DEFAULT 1 CHECK (interval >= 1),
    weekdays          int[] NOT NULL DEFAULT '{}',  -- for weekly: 0..6
    month_day         int CHECK (month_day BETWEEN 1 AND 31),  -- for monthly; clamps to month end
    anchor_date       date NOT NULL,  -- interval counting starts here
    active            boolean NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id            uuid PRIMARY KEY,
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    number        int  NOT NULL,
    title         text NOT NULL,
    description   text NOT NULL DEFAULT '',
    status        text NOT NULL DEFAULT 'todo' CHECK (status IN ('backlog', 'todo', 'in_progress', 'done')),
    priority      int  NOT NULL DEFAULT 2 CHECK (priority BETWEEN 0 AND 4),
    points        int,
    labels        uuid[] NOT NULL DEFAULT '{}',
    assignee_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    reporter_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    sprint_id     uuid REFERENCES sprints(id) ON DELETE SET NULL,
    due_date      date,
    rank          double precision NOT NULL DEFAULT 0,
    recurring_id  uuid REFERENCES recurring_tasks(id) ON DELETE SET NULL,
    completed_at  timestamptz,
    completed_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (household_id, number)
);
CREATE INDEX tasks_household_status_idx ON tasks (household_id, status);
CREATE INDEX tasks_sprint_idx ON tasks (sprint_id);
CREATE INDEX tasks_completed_idx ON tasks (household_id, completed_at);

-- Ledger of generated recurring occurrences, so deleting a generated task doesn't
-- cause it to be regenerated.
CREATE TABLE recurring_occurrences (
    recurring_id  uuid NOT NULL REFERENCES recurring_tasks(id) ON DELETE CASCADE,
    occurs_on     date NOT NULL,
    PRIMARY KEY (recurring_id, occurs_on)
);

CREATE TABLE push_subscriptions (
    endpoint         text PRIMARY KEY,
    user_id          uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    p256dh           text NOT NULL,
    auth             text NOT NULL,
    user_agent       text NOT NULL DEFAULT '',
    created_at       timestamptz NOT NULL DEFAULT now(),
    last_success_at  timestamptz,
    failure_count    int NOT NULL DEFAULT 0
);
CREATE INDEX push_subscriptions_user_idx ON push_subscriptions (user_id);

-- Inbox + push outbox. A row is pushed once (pushed_at), deferred during quiet hours.
CREATE TABLE notifications (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind        text NOT NULL,
    dedupe_key  text NOT NULL,
    title       text NOT NULL,
    body        text NOT NULL DEFAULT '',
    url         text NOT NULL DEFAULT '/',
    created_at  timestamptz NOT NULL DEFAULT now(),
    read_at     timestamptz,
    push_state  text NOT NULL DEFAULT 'pending' CHECK (push_state IN ('pending', 'sent', 'skipped')),
    pushed_at   timestamptz,
    UNIQUE (user_id, dedupe_key)
);
CREATE INDEX notifications_user_created_idx ON notifications (user_id, created_at DESC);
CREATE INDEX notifications_pending_idx ON notifications (created_at) WHERE push_state = 'pending';
