-- +goose Up
CREATE UNIQUE INDEX instruments_ticker_asset_class_uidx
ON instruments (LOWER(ticker), asset_class);

-- +goose Down
DROP INDEX IF EXISTS instruments_ticker_asset_class_uidx;
