# Omnifolio

Personal investment tracker for stocks and crypto.

Aggregates portfolios from multiple sources (Russian stocks via T-Invest, crypto via Bybit, anything via manual entry) and shows them per-account and combined, with values converted into a chosen display currency.

See [`docs/design.md`](docs/design.md) for full architecture and milestone roadmap.

## Stack

- **Backend**: Go 1.23 (chi + sqlc + pgx + goose + argon2id + AES-GCM + robfig/cron + slog)
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

`main` branch auto-deploys to Railway via GitHub integration:
- `apps/api/**` and `api/**` changes rebuild the **api** service.
- `apps/web/**`, `api/**`, `pnpm-lock.yaml`, `pnpm-workspace.yaml`, `package.json` changes rebuild the **web** service.

Env vars required per service (set via Railway dashboard or `railway variables`):

**api**
- `DATABASE_URL` — Postgres connection string (auto-injected from `Postgres` service if you reference it)
- `MASTER_KEY` — 32 bytes base64url, used for AES-GCM credential encryption
- `BOOTSTRAP_USER_EMAIL`, `BOOTSTRAP_USER_PASSWORD` — first user to seed if `users` table is empty
- `ENV=prod`, `LOG_LEVEL=info`
- `RAILWAY_DOCKERFILE_PATH=apps/api/Dockerfile`

**web**
- `API_INTERNAL_URL=http://api.railway.internal:8080`
- `RAILWAY_DOCKERFILE_PATH=apps/web/Dockerfile`

**postgres** — managed plugin, no manual config.

## Generating MASTER_KEY

```sh
node -e "console.log(require('crypto').randomBytes(32).toString('base64url'))"
```

## Codegen

```sh
make generate          # sqlc + oapi-codegen + orval (TS client)
```

After modifying `api/openapi.yaml` or `apps/api/internal/storage/queries/*.sql`, re-run `make generate` and commit the regenerated files.
