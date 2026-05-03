-- +goose Up
ALTER TABLE instruments DROP CONSTRAINT instruments_asset_class_check;
ALTER TABLE instruments ADD CONSTRAINT instruments_asset_class_check
    CHECK (asset_class IN ('ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto','cash'));

-- +goose Down
ALTER TABLE instruments DROP CONSTRAINT instruments_asset_class_check;
ALTER TABLE instruments ADD CONSTRAINT instruments_asset_class_check
    CHECK (asset_class IN ('ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto'));
