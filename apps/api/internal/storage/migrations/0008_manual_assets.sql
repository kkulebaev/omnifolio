-- +goose Up
ALTER TABLE instruments
    ADD COLUMN user_id UUID REFERENCES users(id) ON DELETE CASCADE;

ALTER TABLE instruments DROP CONSTRAINT instruments_asset_class_check;
ALTER TABLE instruments ADD CONSTRAINT instruments_asset_class_check
    CHECK (asset_class IN (
        'ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto','cash',
        'real_estate','vehicle','other_asset'
    ));

-- Structural invariant: global rows ↔ exchange/cash classes; personal rows ↔ manual classes.
ALTER TABLE instruments ADD CONSTRAINT instruments_scope_class_check CHECK (
    (user_id IS NULL     AND asset_class IN ('ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto','cash'))
 OR (user_id IS NOT NULL AND asset_class IN ('real_estate','vehicle','other_asset'))
);

-- Replace global unique index with two partial indexes scoped by ownership.
DROP INDEX instruments_ticker_asset_class_uidx;
CREATE UNIQUE INDEX instruments_ticker_asset_class_global_uidx
    ON instruments (LOWER(ticker), asset_class) WHERE user_id IS NULL;
CREATE UNIQUE INDEX instruments_ticker_asset_class_user_uidx
    ON instruments (user_id, LOWER(ticker), asset_class) WHERE user_id IS NOT NULL;

CREATE INDEX instruments_user_id_idx ON instruments(user_id) WHERE user_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS instruments_user_id_idx;
DROP INDEX IF EXISTS instruments_ticker_asset_class_user_uidx;
DROP INDEX IF EXISTS instruments_ticker_asset_class_global_uidx;
CREATE UNIQUE INDEX instruments_ticker_asset_class_uidx
    ON instruments (LOWER(ticker), asset_class);

ALTER TABLE instruments DROP CONSTRAINT instruments_scope_class_check;
ALTER TABLE instruments DROP CONSTRAINT instruments_asset_class_check;
ALTER TABLE instruments ADD CONSTRAINT instruments_asset_class_check
    CHECK (asset_class IN ('ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto','cash'));

ALTER TABLE instruments DROP COLUMN user_id;
