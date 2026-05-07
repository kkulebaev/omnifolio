-- +goose Up
CREATE TABLE portfolio_snapshots (
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    snapshot_date    DATE NOT NULL,
    display_currency VARCHAR(8) NOT NULL CHECK (display_currency ~ '^[A-Z]{3,5}$'),
    grand_total      NUMERIC(24, 8) NOT NULL,
    by_asset_class   JSONB NOT NULL,
    by_currency      JSONB NOT NULL,
    by_account       JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, snapshot_date)
);

-- +goose Down
DROP TABLE IF EXISTS portfolio_snapshots;
