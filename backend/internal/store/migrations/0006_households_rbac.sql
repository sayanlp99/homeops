-- Multi-household: people join households through memberships with a role, sign in
-- with a username or email, and are onboarded by email invite.

ALTER TABLE users ADD COLUMN email text;
CREATE UNIQUE INDEX users_email_idx ON users (lower(email));
ALTER TABLE users ADD COLUMN is_platform_admin boolean NOT NULL DEFAULT false;

-- owner: everything incl. deleting the household and managing admins
-- admin: household settings, invites, members
-- member: day-to-day use; viewer: read-only
CREATE TABLE memberships (
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    user_id       uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role          text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member', 'viewer')),
    created_at    timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (household_id, user_id)
);
CREATE INDEX memberships_user_idx ON memberships (user_id);

INSERT INTO memberships (household_id, user_id, role, created_at)
SELECT household_id, id, CASE role WHEN 'admin' THEN 'owner' ELSE 'member' END, created_at FROM users;
-- Whoever ran first-time setup administers the whole installation.
UPDATE users SET is_platform_admin = true WHERE role = 'admin';

-- Each session is "in" one household; new sign-ins resume the last one used.
ALTER TABLE users ADD COLUMN last_household_id uuid REFERENCES households(id) ON DELETE SET NULL;
UPDATE users SET last_household_id = household_id;
ALTER TABLE sessions ADD COLUMN household_id uuid REFERENCES households(id) ON DELETE SET NULL;
UPDATE sessions s SET household_id = u.household_id FROM users u WHERE u.id = s.user_id;

-- Notifications belong to the household they're about.
ALTER TABLE notifications ADD COLUMN household_id uuid REFERENCES households(id) ON DELETE CASCADE;
UPDATE notifications n SET household_id = u.household_id FROM users u WHERE u.id = n.user_id;
ALTER TABLE notifications ALTER COLUMN household_id SET NOT NULL;
ALTER TABLE notifications DROP CONSTRAINT notifications_user_id_dedupe_key_key;
ALTER TABLE notifications ADD UNIQUE (user_id, household_id, dedupe_key);

ALTER TABLE users DROP COLUMN household_id;
ALTER TABLE users DROP COLUMN role;

-- One row per person per household, so household-scoped queries stay simple joins.
CREATE VIEW members AS
SELECT u.id, m.household_id, m.role, u.username, u.display_name, u.email, u.color, m.created_at
FROM memberships m JOIN users u ON u.id = m.user_id;

-- Invites are addressed to an email and carry the role the person will get.
DELETE FROM invites WHERE used_at IS NULL;
ALTER TABLE invites ADD COLUMN email text NOT NULL DEFAULT '';
ALTER TABLE invites ADD COLUMN role text NOT NULL DEFAULT 'member' CHECK (role IN ('owner', 'admin', 'member', 'viewer'));
ALTER TABLE invites ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE invites ADD COLUMN used_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE invites ALTER COLUMN created_by DROP NOT NULL;
ALTER TABLE invites DROP CONSTRAINT invites_created_by_fkey;
ALTER TABLE invites ADD FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE SET NULL;
CREATE INDEX invites_household_idx ON invites (household_id);

CREATE TABLE password_resets (
    token_hash  bytea PRIMARY KEY,
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    used_at     timestamptz
);
CREATE INDEX password_resets_user_idx ON password_resets (user_id);
