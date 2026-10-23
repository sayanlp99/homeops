-- Remember why a device stopped receiving pushes, so Settings can show it.
ALTER TABLE push_subscriptions ADD COLUMN last_error text NOT NULL DEFAULT '';
ALTER TABLE push_subscriptions ADD COLUMN last_failure_at timestamptz;
