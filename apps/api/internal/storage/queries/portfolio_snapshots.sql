-- name: UpsertPortfolioSnapshot :exec
INSERT INTO portfolio_snapshots (
    user_id, snapshot_date, display_currency,
    grand_total, by_asset_class, by_currency, by_account
)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, snapshot_date) DO UPDATE SET
    display_currency = EXCLUDED.display_currency,
    grand_total      = EXCLUDED.grand_total,
    by_asset_class   = EXCLUDED.by_asset_class,
    by_currency      = EXCLUDED.by_currency,
    by_account       = EXCLUDED.by_account,
    created_at       = now();

-- name: ListPortfolioSnapshotsByDateRange :many
SELECT user_id, snapshot_date, display_currency,
       grand_total, by_asset_class, by_currency, by_account, created_at
FROM portfolio_snapshots
WHERE user_id = $1
  AND snapshot_date BETWEEN $2 AND $3
ORDER BY snapshot_date ASC;
