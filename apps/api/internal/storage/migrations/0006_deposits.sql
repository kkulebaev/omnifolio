-- +goose Up
CREATE TABLE deposits (
    id          UUID PRIMARY KEY,
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    month       DATE NOT NULL CHECK (month = date_trunc('month', month)::date),
    amount      NUMERIC(20, 0) NOT NULL CHECK (amount > 0),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX deposits_user_id_month_idx ON deposits(user_id, month DESC);
CREATE TRIGGER deposits_updated_at BEFORE UPDATE ON deposits
    FOR EACH ROW EXECUTE FUNCTION trigger_set_updated_at();

-- +goose Down
DROP TABLE IF EXISTS deposits;
