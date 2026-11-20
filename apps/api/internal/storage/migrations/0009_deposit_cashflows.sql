-- +goose Up
ALTER TABLE deposits RENAME COLUMN month TO date;
-- The inline CHECK from 0006 keeps its auto-generated name after the rename.
ALTER TABLE deposits DROP CONSTRAINT deposits_month_check;

-- Foreign-currency flows (USD, EUR, …) carry cents.
ALTER TABLE deposits ALTER COLUMN amount TYPE NUMERIC(20, 2);

ALTER TABLE deposits
    ADD COLUMN kind VARCHAR(16) NOT NULL DEFAULT 'deposit'
        CONSTRAINT deposits_kind_check CHECK (kind IN ('deposit', 'withdrawal')),
    ADD COLUMN currency VARCHAR(10) NOT NULL DEFAULT 'RUB'
        CONSTRAINT deposits_currency_check CHECK (currency ~ '^[A-Z0-9]{3,10}$');

DROP INDEX deposits_user_id_month_idx;
CREATE INDEX deposits_user_id_date_idx ON deposits(user_id, date DESC);

-- +goose Down
-- Lossy: withdrawals are deleted, dates collapse to the first of their month,
-- amounts are rounded to whole units (minimum 1 to keep amount > 0), and the
-- currency is dropped, so non-RUB flows silently become RUB again.
DROP INDEX IF EXISTS deposits_user_id_date_idx;

DELETE FROM deposits WHERE kind = 'withdrawal';
UPDATE deposits SET date = date_trunc('month', date)::date;

ALTER TABLE deposits DROP COLUMN currency;
ALTER TABLE deposits DROP COLUMN kind;

ALTER TABLE deposits ALTER COLUMN amount TYPE NUMERIC(20, 0)
    USING GREATEST(ROUND(amount), 1);

ALTER TABLE deposits RENAME COLUMN date TO month;
ALTER TABLE deposits ADD CONSTRAINT deposits_month_check
    CHECK (month = date_trunc('month', month)::date);

CREATE INDEX deposits_user_id_month_idx ON deposits(user_id, month DESC);
