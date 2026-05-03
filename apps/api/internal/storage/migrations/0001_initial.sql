-- +goose Up
-- +goose StatementBegin
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE OR REPLACE FUNCTION trigger_set_updated_at() RETURNS trigger AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TABLE users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    display_currency VARCHAR(8) NOT NULL DEFAULT 'RUB' CHECK (display_currency ~ '^[A-Z]{3,5}$'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER users_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TABLE sessions (
    token_hash BYTEA PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ NOT NULL,
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);
CREATE INDEX sessions_expires_at_idx ON sessions(expires_at);

CREATE TABLE accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    source_type TEXT NOT NULL CHECK (source_type IN ('manual', 'tinvest', 'bybit')),
    name TEXT NOT NULL,
    last_synced_at TIMESTAMPTZ,
    last_sync_status TEXT CHECK (last_sync_status IN ('success', 'failed', 'pending')),
    last_sync_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX accounts_user_id_idx ON accounts(user_id);
CREATE TRIGGER accounts_updated_at BEFORE UPDATE ON accounts
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TABLE account_credentials (
    account_id UUID PRIMARY KEY REFERENCES accounts(id) ON DELETE CASCADE,
    ciphertext BYTEA NOT NULL,
    nonce BYTEA NOT NULL,
    key_version INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE TRIGGER account_credentials_updated_at BEFORE UPDATE ON account_credentials
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TABLE instruments (
    id UUID PRIMARY KEY,
    ticker TEXT NOT NULL,
    asset_class TEXT NOT NULL CHECK (asset_class IN ('ru_stock', 'ru_bond', 'ru_etf', 'us_stock', 'us_etf', 'crypto')),
    currency VARCHAR(8) NOT NULL CHECK (currency ~ '^[A-Z]{3,5}$'),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX instruments_ticker_idx ON instruments(ticker);
CREATE INDEX instruments_asset_class_idx ON instruments(asset_class);
CREATE TRIGGER instruments_updated_at BEFORE UPDATE ON instruments
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TABLE instrument_external_ids (
    source TEXT NOT NULL,
    native_id TEXT NOT NULL,
    instrument_id UUID NOT NULL REFERENCES instruments(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source, native_id)
);
CREATE INDEX instrument_external_ids_instrument_id_idx ON instrument_external_ids(instrument_id);

CREATE TABLE positions (
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    instrument_id UUID NOT NULL REFERENCES instruments(id) ON DELETE RESTRICT,
    quantity NUMERIC(38, 18) NOT NULL CHECK (quantity > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (account_id, instrument_id)
);
CREATE INDEX positions_instrument_id_idx ON positions(instrument_id);

CREATE TABLE prices (
    instrument_id UUID PRIMARY KEY REFERENCES instruments(id) ON DELETE CASCADE,
    price NUMERIC(20, 8) NOT NULL,
    fetched_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE fx_rates (
    date DATE NOT NULL,
    from_ccy VARCHAR(8) NOT NULL,
    to_ccy VARCHAR(8) NOT NULL,
    rate NUMERIC(20, 10) NOT NULL,
    PRIMARY KEY (date, from_ccy, to_ccy)
);

CREATE TABLE portfolios (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, name)
);
CREATE TRIGGER portfolios_updated_at BEFORE UPDATE ON portfolios
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

CREATE TABLE portfolio_accounts (
    portfolio_id UUID NOT NULL REFERENCES portfolios(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    PRIMARY KEY (portfolio_id, account_id)
);
CREATE INDEX portfolio_accounts_account_id_idx ON portfolio_accounts(account_id);

-- +goose Down
DROP TABLE IF EXISTS portfolio_accounts;
DROP TABLE IF EXISTS portfolios;
DROP TABLE IF EXISTS fx_rates;
DROP TABLE IF EXISTS prices;
DROP TABLE IF EXISTS positions;
DROP TABLE IF EXISTS instrument_external_ids;
DROP TABLE IF EXISTS instruments;
DROP TABLE IF EXISTS account_credentials;
DROP TABLE IF EXISTS accounts;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS users;
DROP FUNCTION IF EXISTS trigger_set_updated_at();
