-- Phase 3: budget envelopes, transactions, accounts, statement imports, bills.
-- Money is stored as integer paise (₹1 = 100) so sums are exact.
-- owner_id NULL = shared with the household; set = private to that member.

CREATE TABLE money_categories (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id   uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    owner_id       uuid REFERENCES users(id) ON DELETE CASCADE,
    name           text NOT NULL,
    icon           text NOT NULL DEFAULT 'circle',
    color          text NOT NULL DEFAULT '#64748b',
    kind           text NOT NULL DEFAULT 'expense' CHECK (kind IN ('expense', 'income')),
    monthly_limit  bigint NOT NULL DEFAULT 0 CHECK (monthly_limit >= 0),
    rollover       boolean NOT NULL DEFAULT false,
    position       int NOT NULL DEFAULT 0,
    archived       boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX money_categories_name_idx ON money_categories (household_id, coalesce(owner_id, '00000000-0000-0000-0000-000000000000'::uuid), lower(name));

-- Per-month overrides of a category's limit ("Shopping is ₹15k in November").
CREATE TABLE money_budget_months (
    category_id  uuid NOT NULL REFERENCES money_categories(id) ON DELETE CASCADE,
    month        date NOT NULL,  -- first of the month
    amount       bigint NOT NULL CHECK (amount >= 0),
    PRIMARY KEY (category_id, month)
);

CREATE TABLE money_accounts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id    uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    owner_id        uuid REFERENCES users(id) ON DELETE CASCADE,
    name            text NOT NULL,
    kind            text NOT NULL DEFAULT 'bank' CHECK (kind IN ('bank', 'card', 'cash', 'wallet')),
    import_mapping  jsonb,  -- remembered column mapping for statement imports
    archived        boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE money_imports (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    owner_id      uuid REFERENCES users(id) ON DELETE CASCADE,
    account_id    uuid REFERENCES money_accounts(id) ON DELETE SET NULL,
    filename      text NOT NULL,
    row_count     int NOT NULL,
    imported      int NOT NULL,
    created_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE money_bills (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id        uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    owner_id            uuid REFERENCES users(id) ON DELETE CASCADE,
    name                text NOT NULL,
    amount              bigint NOT NULL DEFAULT 0 CHECK (amount >= 0),  -- 0 = varies
    due_day             int NOT NULL CHECK (due_day BETWEEN 1 AND 31),
    category_id         uuid REFERENCES money_categories(id) ON DELETE SET NULL,
    account_id          uuid REFERENCES money_accounts(id) ON DELETE SET NULL,
    remind_days_before  int NOT NULL DEFAULT 3 CHECK (remind_days_before BETWEEN 0 AND 15),
    last_paid_month     date,
    active              boolean NOT NULL DEFAULT true,
    created_at          timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE money_transactions (
    id               uuid PRIMARY KEY,
    household_id     uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    owner_id         uuid REFERENCES users(id) ON DELETE CASCADE,
    kind             text NOT NULL DEFAULT 'expense' CHECK (kind IN ('expense', 'income', 'transfer')),
    amount           bigint NOT NULL,  -- paise; negative expense = refund
    date             date NOT NULL,
    payee            text NOT NULL DEFAULT '',
    description      text NOT NULL DEFAULT '',  -- raw bank narration for imports
    note             text NOT NULL DEFAULT '',
    category_id      uuid REFERENCES money_categories(id) ON DELETE SET NULL,
    account_id       uuid REFERENCES money_accounts(id) ON DELETE SET NULL,
    paid_by          uuid REFERENCES users(id) ON DELETE SET NULL,
    receipt          text NOT NULL DEFAULT '',  -- file id
    source           text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'import', 'grocery', 'bill', 'invoice')),
    import_id        uuid REFERENCES money_imports(id) ON DELETE CASCADE,
    import_hash      text,
    grocery_trip_id  uuid REFERENCES grocery_trips(id) ON DELETE SET NULL,
    bill_id          uuid REFERENCES money_bills(id) ON DELETE SET NULL,
    created_by       uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX money_transactions_date_idx ON money_transactions (household_id, date);
CREATE INDEX money_transactions_category_idx ON money_transactions (category_id, date);
CREATE UNIQUE INDEX money_transactions_import_hash_idx ON money_transactions (household_id, import_hash) WHERE import_hash IS NOT NULL;

-- Learned categorization: narration/payee contains `pattern` → category.
CREATE TABLE money_rules (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    household_id  uuid NOT NULL REFERENCES households(id) ON DELETE CASCADE,
    owner_id      uuid REFERENCES users(id) ON DELETE CASCADE,
    pattern       text NOT NULL,  -- lowercase
    category_id   uuid NOT NULL REFERENCES money_categories(id) ON DELETE CASCADE,
    kind          text NOT NULL DEFAULT 'expense' CHECK (kind IN ('expense', 'income', 'transfer')),
    hits          int NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX money_rules_pattern_idx ON money_rules (household_id, coalesce(owner_id, '00000000-0000-0000-0000-000000000000'::uuid), pattern);

