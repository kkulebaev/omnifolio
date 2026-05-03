-- +goose Up
DROP TABLE IF EXISTS portfolio_accounts;
DROP TABLE IF EXISTS portfolios;

-- +goose Down
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
