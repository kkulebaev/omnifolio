# Omnifolio

Personal investment tracker for stocks and crypto.

Aggregates portfolios from multiple sources (Russian stocks via T-Invest, crypto via Bybit, anything via manual entry) and shows them per-account and combined, with values converted into a chosen display currency.

See [`docs/design.md`](docs/design.md) for full architecture and milestone roadmap.

## Stack

- **Backend**: Go 1.26 (chi + sqlc + pgx + goose + argon2id + AES-GCM + robfig/cron + slog)
- **Frontend**: Vue 3 + Vite + Pinia + vue-router + TanStack Query + orval + Tailwind v4
- **DB**: Postgres 16
- **Contract**: OpenAPI single source of truth (`api/openapi.yaml`)
- **Deploy**: Railway (api + web + Postgres), GitHub auto-deploy on push to main

## Local development

```sh
make services           # postgres + api in docker-compose with hot reload
pnpm --filter web dev   # vite on :5173
```

The web app proxies `/api/*` to `localhost:8080`. Bootstrap user is created from `BOOTSTRAP_USER_EMAIL` + `BOOTSTRAP_USER_PASSWORD` env (set in `compose.yml`).

## Production (Railway)

`main` branch auto-deploys to Railway via GitHub integration. Intended watch-pattern scope per service:
- `apps/api/**` and `api/**` changes rebuild the **api** service.
- `apps/web/**`, `api/**`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `package.json` changes rebuild the **web** service.
- `apps/cron/**` changes rebuild the **cron** service.

For service IDs, current watch-patterns state, and how to manage Railway settings, see [`docs/railway.md`](docs/railway.md).

Env vars required per service (set via Railway dashboard or `railway variables`):

**api**
- `DATABASE_URL` — Postgres connection string (auto-injected from `Postgres` service if you reference it)
- `MASTER_KEY` — 32 bytes base64url, used for AES-GCM credential encryption
- `ADMIN_API_KEY` — shared secret for service-to-service `/admin/*` calls (used by the **cron** service)
- `BOOTSTRAP_USER_EMAIL`, `BOOTSTRAP_USER_PASSWORD` — first user to seed if `users` table is empty
- `ENV=prod`, `LOG_LEVEL=info`
- `RAILWAY_DOCKERFILE_PATH=apps/api/Dockerfile`

**web**
- `API_INTERNAL_URL=http://api.railway.internal:8080`
- `RAILWAY_DOCKERFILE_PATH=apps/web/Dockerfile`

**cron** — daily price refresh, runs once per invocation and exits. Configure as a Railway cron service with schedule `0 6 * * *` (06:00 UTC ≈ 09:00 МСК).
- `API_URL=http://api.railway.internal:8080`
- `ADMIN_API_KEY` — same value as set on the **api** service
- `FINNHUB_API_KEY` — quote provider for `us_stock` / `us_etf` instruments (optional; if unset, those classes aren't refreshed)
- `TINVEST_TOKEN` — read-only invest token; quote provider for `ru_stock`, also drives the MOEX share catalog snapshot (optional; if unset, the ru channel is skipped entirely)
- `RAILWAY_DOCKERFILE_PATH=apps/cron/Dockerfile`

**postgres** — managed plugin, no manual config.

## Generating MASTER_KEY

```sh
node -e "console.log(require('crypto').randomBytes(32).toString('base64url'))"
```

## Cron service (price refresh)

`apps/cron` is a one-shot Go binary that on each invocation:
1. Seeds the canonical instruments catalog from three sources: a static US list from the embedded `apps/cron/cmd/cron/instruments.json`; if `TINVEST_TOKEN` is set, every MOEX share (`class_code='TQBR'`) returned by T-Invest `InstrumentsService.Shares`; and every USDT-quoted spot pair currently trading on Bybit (`/v5/market/instruments-info`, public — stablecoins excluded). All three lists are POSTed to `/admin/instruments` (idempotent — existing rows are no-ops).
2. Pulls quotes from external providers — Finnhub for `us_stock` / `us_etf`, T-Invest `MarketDataService.GetLastPrices` for `ru_stock`, Bybit `/v5/market/tickers` for `crypto` — and writes them via `POST /admin/prices`.
3. Pulls daily FX rates from cbr.ru (`XML_daily.asp`, every published currency vs RUB) and writes them via `POST /admin/fx`. The api uses these for `/portfolio` currency conversion; without a recent cron run, conversions degrade to `ErrRateUnavailable`.

It does not talk to the DB directly. To add/remove tracked US tickers, edit `apps/cron/cmd/cron/instruments.json` and redeploy the **cron** service. The Russian and crypto universes are rebuilt from T-Invest and Bybit on every run, so no manual list is maintained for them. After a fresh deploy run the cron once manually so `/portfolio` has FX rates and prices to work with. Trigger manually in dev:

```sh
docker compose --profile cron run --rm \
  -e FINNHUB_API_KEY=$FINNHUB_API_KEY \
  -e TINVEST_TOKEN=$TINVEST_TOKEN \
  cron
```

Broker-bound prices are not the cron's job — Bybit and T-Invest positions are still synced hourly by the api's in-process `sync-brokerage-accounts` job, but that job no longer touches the `prices` table.

## Codegen

```sh
make generate          # sqlc + oapi-codegen + orval (TS client)
```

After modifying `api/openapi.yaml` or `apps/api/internal/storage/queries/*.sql`, re-run `make generate` and commit the regenerated files.
