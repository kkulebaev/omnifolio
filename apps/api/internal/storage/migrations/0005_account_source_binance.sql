-- +goose Up
ALTER TABLE accounts DROP CONSTRAINT accounts_source_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_source_type_check
    CHECK (source_type IN ('manual', 'tinvest', 'bybit', 'binance'));

-- +goose Down
ALTER TABLE accounts DROP CONSTRAINT accounts_source_type_check;
ALTER TABLE accounts ADD CONSTRAINT accounts_source_type_check
    CHECK (source_type IN ('manual', 'tinvest', 'bybit'));
