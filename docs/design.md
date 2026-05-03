# Omnifolio — Design Document

Приложение для учёта личных инвестиций. Агрегирует портфели из разных источников
(российские бумаги через T-Invest, крипта через Bybit, остальное вручную)
и показывает их по отдельности и в сумме.

Документ зафиксирован после интервью-сессии и описывает MVP v0.1.

---

## 1. Цель и scope

**Главная задача**: снапшот текущей стоимости и распределения активов.

- Что показываем: сколько всего сейчас, в каких активах, в каких валютах.
- Что НЕ показываем (отложено): доходность во времени (TWR/XIRR), журнал сделок,
  P&L по позициям, налоговый учёт, ребалансировка, дивиденды, корпоративные действия.

Решение строит модель данных вокруг текущего состояния (positions snapshot),
а не вокруг истории транзакций.

---

## 2. Источники данных

### 2.1. Position sources (откуда берём позиции)

Гибридная стратегия: где есть API — синкаем; остальное руками.

| Источник | Как тянем позиции | Статус MVP |
|---|---|---|
| T-Invest | gRPC `OperationsService.GetPortfolio` | M2 |
| Bybit | REST `/v5/account/wallet-balance` (HMAC) | M3 |
| Manual | Ручной ввод через UI | M1 |
| IBKR | Client Portal Web API (требует gateway) | **Отложено** |

Архитектурный интерфейс:

```go
type PositionSource interface {
    Sync(ctx context.Context, account Account) ([]Position, error)
    Resolve(ctx context.Context, nativeID string) (InstrumentSeed, error)
}
```

### 2.2. Price providers (откуда берём цены)

Раздельный слой, не привязан к position source.

| Asset class | Провайдер | Notes |
|---|---|---|
| RU stocks/bonds/ETF | T-Invest `MarketDataService.GetLastPrices` | Используем тот же gRPC |
| Crypto | CoinGecko (free, no auth) | Бесплатно, лимит 30 req/min |
| US stocks | Finnhub / Yahoo (отложено) | Решается когда появятся не-T-Invest US-позиции |
| FX (RUB↔USD) | ЦБ РФ XML daily | `cbr.ru/scripts/XML_daily.asp` |
| USDT/USD | Фиксированно 1:1 | Приближение, погрешность <0.1% |

Архитектурный интерфейс:

```go
type PriceProvider interface {
    GetPrices(ctx context.Context, instruments []Instrument) (map[InstrumentID]Money, error)
}
```

Роутинг: по `instrument.asset_class` → выбор провайдера.

---

## 3. Модель данных

### 3.1. Двухуровневая модель Account / Portfolio

- **Account** — физический счёт у источника (один T-Invest брокерский счёт = один Account).
  Содержит позиции, синкается из source.
- **Portfolio** — логическая группа (например, «Долгосрок», «Спекуляции», «Всё вместе»).
  В MVP тривиально мапится на Account-ы (M:N через `portfolio_accounts`).

### 3.2. Канонический справочник инструментов

Один реальный инструмент = одна строка в `instruments`. Маппинг от источников
через таблицу `instrument_external_ids`. AAPL из T-Invest и AAPL из IBKR ссылаются
на ту же `instrument_id`.

Это критично для корректной агрегации в "всё вместе" и для разделения
"position source ↔ price provider".

### 3.3. Хранение денег

- Postgres: `NUMERIC(38, 18)` для quantity, `NUMERIC(20, 8)` для price,
  `NUMERIC(20, 10)` для fx_rate.
- Go: `github.com/shopspring/decimal` всюду. Никаких float.
- Currency: ISO 4217 строкой (`'USD'`, `'RUB'`) + спец-коды для крипты (`'USDT'`, `'BTC'`).
  CHECK constraint на формат.

### 3.4. Валютная агрегация

- **Хранение**: цены и стоимости в нативной валюте инструмента.
- **Отображение**: пользователь выбирает валюту в UI (per-view setting в Pinia).
- **Конвертация**: на лету через таблицу `fx_rates`. Запрос `total в USD`:
  `SUM(quantity × price × fx_to_usd)` — один JOIN.

### 3.5. Схема (черновик)

```sql
users               (id, email, password_hash, created_at)
sessions            (token, user_id, expires_at, last_seen_at)

accounts            (id, user_id, source_type, name, last_synced_at)
account_credentials (account_id PK, ciphertext BYTEA, nonce BYTEA, key_version, updated_at)

instruments              (id, ticker, asset_class, currency, name)
instrument_external_ids  (source, native_id, instrument_id, PRIMARY KEY(source, native_id))

positions  (account_id, instrument_id, quantity NUMERIC(38,18), updated_at)
prices     (instrument_id, ts, price NUMERIC(20,8))
fx_rates   (date, from_ccy, to_ccy, rate NUMERIC(20,10))

portfolios          (id, user_id, name)
portfolio_accounts  (portfolio_id, account_id)
```

---

## 4. Синхронизация

### 4.1. Позиции (меняются редко)

- Cron в фоне каждый час + кнопка "Обновить" в UI на странице аккаунта.
- Архитектура: одна goroutine с `time.NewTicker` внутри API-процесса.
  Не выделяем отдельный бинарь worker.

### 4.2. Цены (меняются постоянно)

- **Lazy cache** с TTL per asset_class:
  - crypto: 60s
  - stocks: 5min
  - FX: 1h
- При HTTP-запросе на `/portfolio` для каждой позиции проверяется
  `prices.fetched_at`; устаревшие подгружаются батчем; UPSERT в БД.
- WebSocket-стримы — отложено (M5+).

### 4.3. FX курсы

- Cron daily, тянет XML с ЦБ, UPSERT в `fx_rates`.

---

## 5. Auth и секреты

### 5.1. User auth

- Session cookie: httpOnly, Secure, SameSite=Lax. Server-side session storage в Postgres (`sessions`).
- Password hash: argon2id (`golang.org/x/crypto/argon2`). Параметры:
  `time=1, memory=64MB, threads=4`.
- Single-user реально, но схема и middleware готовы под multi-user (везде `user_id`).
- Без 2FA, OAuth, magic links на MVP.

### 5.2. Broker secrets

- AES-256-GCM шифрование при хранении в `account_credentials`.
- Master key — `MASTER_KEY` из env (32 байта base64). Приложение падает на старте,
  если не задан.
- Поле `key_version INT` для будущей ротации.
- Дамп БД без env-key не раскрывает токены.

---

## 6. API контракт

- **OpenAPI 3.x** — единый источник правды (`api/openapi.yaml`).
- Бэк: `oapi-codegen` генерит chi-handlers интерфейсы; реализация — вручную.
- Фронт: `orval --client vue-query` генерит TS-клиент с готовыми
  `useXxxQuery` / `useXxxMutation` хуками для TanStack Query.
- Swagger/Scalar UI на `/docs` для ручного дёрганья.

---

## 7. Backend стек (Go)

| Слой | Выбор |
|---|---|
| HTTP роутер | `chi` (stdlib-совместимый, native-adapter в oapi-codegen) |
| Query layer | `sqlc` + `pgx` + `decimal.Decimal` через override |
| Migrations | `goose`, plain SQL, embed через `//go:embed` |
| Logging | `log/slog` (stdlib), JSON в проде |
| Config | `caarlos0/env` (12-factor, env-only) |
| Decimal | `shopspring/decimal` |
| Crypto | `golang.org/x/crypto/argon2`, `crypto/aes`+`crypto/cipher` (GCM) |
| Tests | `testing` + `testify` + `testcontainers-go` (real Postgres) |

---

## 8. Frontend стек

| Слой | Выбор |
|---|---|
| Build | Vite |
| UI | Vue 3 (Composition API, `<script setup>`) + shadcn-vue + Tailwind v4 |
| State (client) | Pinia |
| State (server) | TanStack Query (vue-query) |
| Routing | vue-router (history mode) |
| API client | orval из OpenAPI |
| Forms | vee-validate + zod |
| Tests | Vitest (unit + component). Playwright **отложен**. |
| Package manager | pnpm |
| TS | strict + noUncheckedIndexedAccess |

Разделение state:
- **Server-state** (списки, дашборд, цены) — TanStack Query.
- **Client-state** (выбранная валюта, выбранный portfolio, UI флаги) — Pinia.

---

## 9. Структура репозитория

Monorepo, pnpm-workspace.

```
omnifolio/
  api/
    openapi.yaml                # source of truth контракта
  apps/
    api/                        # Go backend
      cmd/api/main.go
      internal/
        server/                 # chi + handlers (oapi-codegen)
        sync/                   # cron + position sync
        price/                  # price providers + cache
        fx/                     # ЦБ FX cron
        auth/                   # session + argon2
        domain/                 # entities, value objects
        storage/                # sqlc generated + repositories
      migrations/*.sql          # goose
      sqlc.yaml
      go.mod
    web/                        # Vue frontend
      src/
        components/
        composables/
        routes/
        stores/
        api/                    # orval-generated
      orval.config.ts
      vite.config.ts
      package.json
  compose.yml                   # dev (postgres + api + web vite)
  compose.prod.yml              # prod (postgres + api + caddy)
  Caddyfile
  pnpm-workspace.yaml
  Makefile                      # generate / migrate / dev / test / build
  .github/workflows/ci.yml
  docs/
    design.md                   # этот файл
```

---

## 10. Деплой

- **Прод**: один VPS, `docker compose up -d`.
- **Reverse proxy**: Caddy с auto-HTTPS (Let's Encrypt в одну директиву).
- **Serving**: Caddy раздаёт статику фронта (`apps/web/dist`), проксирует `/api/*` на API-контейнер.
- **Postgres**: в контейнере, volume на хосте.
- **Backup**: `pg_dump` в cron на хосте + ротация (TODO в M5).
- **k8s/PaaS не используем** — single-user, одна машина.

---

## 11. Observability

В MVP — ничего. `slog` JSON в `docker logs`.

Добавляем по необходимости:
- Prometheus + `/metrics` — когда захочется метрики.
- OpenTelemetry — когда multi-user или запутанная распределёнка.
- Sentry — когда заболит трекинг ошибок.

---

## 12. MVP Roadmap

### M0 — Skeleton (1–2 дня)
- Monorepo по layout. compose.yml. Caddyfile (опционально для dev).
- pnpm-workspace, Makefile, CI workflow.
- Healthcheck `/healthz`, hello-world Vue страница, shadcn-vue button.

### M1 — Manual portfolio + auth (3–5 дней)
- Goose миграции всей схемы из §3.5.
- Auth: argon2id, sessions, middleware, `/login` `/me` `/logout`.
- OpenAPI: `/accounts` (CRUD type=manual), `/accounts/:id/positions`,
  `/portfolio` (агрегация), `/instruments/search`.
- sqlc queries, service layer, chi handlers.
- ЦБ FX cron daily.
- Frontend: login, dashboard (table + total), accounts page (CRUD).
- **Acceptance**: руками завести manual-аккаунт, добавить AAPL × 10 @ 200 USD,
  увидеть total в RUB.

### M2 — T-Invest source (3–5 дней)
- gRPC клиент к T-Invest.
- `TInvestPositionSource` (Sync + Resolve).
- `TInvestPriceProvider` для RU инструментов.
- AES-GCM encryption для credentials.
- UI: account type=tinvest, форма для readonly-токена, кнопка "Sync now".
- Background sync cron 1h.
- **Acceptance**: завести T-Invest token → увидеть реальные позиции и стоимость.

### M3 — Bybit source + crypto prices (2–3 дня)
- `BybitPositionSource` через REST + HMAC.
- `CoinGeckoPriceProvider` для крипты.
- UI: account type=bybit, форма api_key + api_secret.
- **Acceptance**: завести Bybit read-only key → увидеть крипто-холдинги.

### M4 — Lazy price cache polish (2 дня)
- TTL per asset_class, batch fetch, retry/backoff.
- UI: индикация "stale at HH:MM" при отказе провайдера.

### M5 — Polish + prod (3–5 дней)
- Pinia store для UI-state (selected currency, selected portfolio).
- Portfolio CRUD (логические группы).
- Charts (asset_class breakdown, по аккаунтам).
- `compose.prod.yml`, Caddy, MASTER_KEY ротация-инструкция в README.
- Backup `pg_dump` cron на хосте.
- **Release**: v0.1.

---

## 13. Backlog (после v0.1)

- IBKR интеграция (Client Portal Web API + gateway).
- Snapshots / историчность портфеля (фундамент для TWR/XIRR).
- WebSocket-стримы цен (Bybit, T-Invest).
- 2FA / multi-user UI / OAuth.
- Дивиденды, корп. действия.
- Налоговый учёт.
- US-stocks через Finnhub/Yahoo (когда появятся не-T-Invest US-позиции).
- On-chain кошельки (Metamask address tracking) — отдельный класс источников.
- Mobile (PWA или native).

---

## 14. Открытые мелочи (не блокеры)

- Семантика частичного отказа: показать "N/A" или "stale at HH:MM" вместо
  краха при недоступности source/provider.
- Charts library: recharts-vue / vue-chartjs / unovis — выбор в M5.
- US-stocks price provider: Finnhub vs Yahoo vs Polygon — решается при появлении US-позиций.
- Backup retention policy: сколько дней хранить pg_dump'ы.

---

## 15. Решения M1 (детализация)

Решения, зафиксированные после интервью на тему M1 (вопросы 17–25). Обязательны
к чтению перед реализацией.

### 15.1. Конвенции БД

- **PK**: `UUIDv7`, генерируется в Go через `github.com/google/uuid` (`uuid.NewV7()`).
  Postgres-side `gen_random_uuid()` не используется (даёт v4).
- **Timestamps**: всегда `TIMESTAMPTZ`, UTC внутри.
- **Audit columns**: `created_at` + `updated_at` на всех таблицах (кроме
  ассоциативных). `updated_at` обновляется триггером
  `trigger_set_updated_at()`. `created_by`/`updated_by` — нет.
- **Удаление**: hard delete через FK `ON DELETE CASCADE`. Soft delete не вводим.
- **Naming**: snake_case в БД, plural таблицы (`accounts`, `positions`),
  ассоциативные `<a>_<b>` (`portfolio_accounts`).

### 15.2. Auth flow

- **Bootstrap первого юзера**: на старте API проверяет таблицу `users`. Если
  пуста и заданы env `BOOTSTRAP_USER_EMAIL` + `BOOTSTRAP_USER_PASSWORD` —
  создаёт юзера. Никакого `/register` endpoint в M1.
- **Идентификация**: email (lowercase normalize) + password.
- **Session expiry**: idle timeout 30 минут + absolute timeout 30 дней.
- **Cookie**: имя `sid`, `HttpOnly`, `SameSite=Lax`, `Path=/`. `Secure`
  ставится conditionally — в production (https), в dev (http) — нет.
- **CSRF**: не используется. Защита через `SameSite=Lax` + проверка `Origin`
  header на state-changing requests на бэке. Same-origin SPA (Caddy раздаёт
  фронт + проксирует `/api`) делает это достаточным.
- **Login errors**: всегда generic "Неверный email или пароль". При несуществующем
  email делаем dummy `argon2.IDKey` против фиксированного хеша — защита от
  timing-атак.
- **Endpoints M1**: `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`.

### 15.3. API conventions

- **Errors**: RFC 7807 Problem Details (`application/problem+json`), поля
  `type/title/status/detail/instance`, расширения `fields` для валидации.
- **JSON naming**: camelCase в JSON (`createdAt`, `userId`).
- **Envelope**: голый ресурс для single (`GET /accounts/:id` → `{id, name, ...}`),
  обёртка для list (`{items: [...], total, nextCursor}`).
- **Pagination**: cursor-based для potentially-large (`positions`, `prices`),
  без пагинации для small (`accounts`, `portfolios`).
- **IDs**: UUID-строка везде.
- **Validation errors**: `fields` объект (`{"email": "invalid format"}`).
- **HTTP status codes**: 200/201/204 success, 400 syntactic, 401 unauth,
  403 forbidden, 404 not found, 409 conflict, **422 для семантической
  валидации**, 500 internal.

### 15.4. Schema-specific

- **Session token**: 32 random bytes из `crypto/rand`, в cookie — base64url,
  в БД — SHA-256 хеш (`BYTEA(32)`). PK на `token_hash`.
- **Enums** (`source_type`, `asset_class`, `last_sync_status`): VARCHAR + CHECK
  constraint, не Postgres `ENUM`.
- **Prices**: одна строка на инструмент (`prices(instrument_id PK, price,
  fetched_at)`), UPSERT при обновлении. Истории нет.
- **Positions PK**: composite `(account_id, instrument_id)`.
- **Instruments uniqueness**: НЕТ UNIQUE на `ticker`. Источник истины —
  `instrument_external_ids(source, native_id)` UNIQUE.
- **Account sync state**: `last_synced_at` + `last_sync_status` + `last_sync_error`.
- **Account is_active**: НЕТ. Hard delete + recreate.

### 15.5. Go layout

```
apps/api/
  cmd/api/main.go               # composition root
  internal/
    config/                     # envconfig.Config
    server/                     # chi mux, oapi-codegen impl, middleware, problem.go
    auth/                       # users + sessions + crypto, service + errors
    account/                    # accounts + credentials, service + errors
    portfolio/                  # portfolios + portfolio_accounts
    instrument/                 # canonical instruments + external_ids
    price/                      # PriceProvider interface, cache, providers/
    fx/                         # ЦБ FX cron + lookups
    storage/
      queries/*.sql             # все sqlc inputs
      *.sql.go                  # generated
      models.go                 # generated
      db.go                     # pgxpool + storage.New(pool)
  migrations/
    0001_initial.sql            # схема M1
  sqlc.yaml
  go.mod
```

- **Packaging**: by feature.
- **Layering внутри feature**: `handler → service → repository`. Repository =
  sqlc-generated `Queries` (или тонкая обёртка).
- **DI**: manual в `main.go`. Без `wire`/`fx`.
- **Errors**: sentinel в feature-пакете + `fmt.Errorf("...: %w", err)`.
  HTTP-слой делает `errors.Is(err, account.ErrNotFound)` → status code mapping
  в едином `problem.go`.
- **sqlc**: глобальный пакет `internal/storage`, не per-feature.
- **HTTP handlers**: один тип в `internal/server/`, реализует
  `oapi-codegen ServerInterface`, делегирует в feature services.

### 15.6. M1 endpoints

**Auth:**
- `POST /auth/login` `{email, password}` → 200 `{user}` + Set-Cookie
- `POST /auth/logout` → 204 + Clear-Cookie
- `GET /auth/me` → 200 `{user}` / 401

**Accounts:**
- `POST /accounts` `{name, type: "manual"}` → 201 `{account}`
- `GET /accounts` → 200 `{items: [account...]}`
- `GET /accounts/:id` → 200 `{...account, positions: [...]}`
- `PATCH /accounts/:id` `{name}` → 200 `{account}`
- `DELETE /accounts/:id` → 204

**Positions:**
- `POST /accounts/:id/positions` `{instrumentId, quantity}` → 201 `{position}`
- `PUT /accounts/:id/positions/:instrumentId` `{quantity}` → 200 `{position}`
- `DELETE /accounts/:id/positions/:instrumentId` → 204

**Portfolio:**
- `GET /portfolio?currency=USD` → 200 композитный response:
  ```json
  {
    "summary": {
      "displayCurrency": "USD",
      "grandTotal": "12345.67",
      "byAssetClass": {...},
      "byCurrency":   {...},
      "byAccount":    {...}
    },
    "positions": [
      {
        "accountId": "...", "accountName": "...",
        "instrumentId": "...", "ticker": "AAPL",
        "assetClass": "us_stock", "quantity": "10",
        "price": "200", "currency": "USD",
        "valueNative": "2000.00", "valueDisplay": "2000.00",
        "priceFetchedAt": "...", "priceStale": false
      }
    ]
  }
  ```

**Instruments:**
- `GET /instruments/search?q=AAPL` → 200 `{items: [instrument...]}`
- `POST /instruments` `{ticker, assetClass, currency, name}` → 201 `{instrument}`

**Manual instrument flow**: сначала search в локальной таблице, если не нашёл —
явный `POST /instruments`, дальше `POST /accounts/:id/positions` с полученным
`instrumentId`. Two-step, не inline create.

### 15.7. Frontend структура

```
apps/web/src/
  main.ts
  App.vue                       # bootstrap + ready spinner
  router.ts                     # routes + meta guards
  style.css
  lib/
    utils.ts                    # cn()
    api-client.ts               # orval entry / fetch interceptor (credentials: 'include')
    http-error.ts               # типы Problem, isAuthError, isValidationError
  components/
    ui/                         # shadcn-vue (CLI generated)
    layout/
      AppLayout.vue
      AuthLayout.vue
      Header.vue
  features/
    auth/
      pages/LoginPage.vue
      composables/useAuth.ts
      stores/auth-store.ts
    account/
      pages/AccountListPage.vue
      pages/AccountDetailPage.vue
      components/CreateAccountDialog.vue
      components/AddPositionDialog.vue
      components/PositionTable.vue
      composables/useAccounts.ts
    dashboard/
      pages/DashboardPage.vue
      components/SummaryCards.vue
      components/PositionsTable.vue
      composables/usePortfolio.ts
  stores/
    ui-store.ts                 # displayCurrency (с localStorage), theme
  api/generated/                # orval output: TS client + zod schemas
```

- **Routes**: `/login`, `/`, `/accounts`, `/accounts/:id`. Logout — action, без route.
- **Layouts**: `AuthLayout` (centered card) для `/login`, `AppLayout`
  (header + nav + main) для остального.
- **Auth guard**: per-route `meta: { requiresAuth }` / `requiresGuest`.
  Глобальный `beforeEach` ждёт `auth.ready`, читает meta.
- **Bootstrap**: `App.vue onMounted` → `auth.bootstrap()` (вызывает
  `GET /auth/me`, ставит `ready=true`). До `ready` — spinner.
- **State разделение**:
  - **TanStack Query** — все server-state (`me`, `accounts`, `portfolio`, …).
  - **Pinia** — UI state (`displayCurrency`, theme), auth-store как обёртка
    над `useMeQuery`.
- **Errors**: globally в `queryCache.onError` (toast + 401 → /login),
  per-mutation `onError` для form validation.

### 15.8. Validation + crypto

- **Server validation**: oapi-codegen middleware валидирует request shape
  против OpenAPI (`OAPIValidator`). Поверх — `go-playground/validator` теги
  для cross-field/business правил, нормализация и uniqueness check в service
  layer.
- **Client validation**: orval генерит zod-схемы из OpenAPI
  (`zod: { generate: true }`), используются в `vee-validate` через
  `@vee-validate/zod` adapter.
- **Source of truth**: OpenAPI. Server и client получают согласованную
  валидацию shape; семантику фронт получает через 422 + `fields`.
- **Constraints**:
  - Email: `format: email`, max 254, lowercase normalize.
  - Password: min 12, max 128, без complexity-rules (NIST SP 800-63B-3).
  - Account name: 1..100.
  - Instrument: ticker `^[A-Z0-9._-]{1,20}$`, asset_class enum, currency
    `^[A-Z]{3,5}$`, name 1..200.
  - Position quantity: NUMERIC `> 0`.
- **Argon2id parameters**: `time=1, memory=64MB, threads=4`. Хранение в
  PHC-формате (`$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>`), salt 16 bytes.
- **Session token**: 32 bytes из `crypto/rand`, base64url в cookie, SHA-256
  hash в БД (`BYTEA(32)`).
- **Sessions cleanup**: lookup проверяет expiry без DELETE; cron каждый час
  `DELETE FROM sessions WHERE expires_at < now() - interval '1 day'`.

### 15.9. Runtime

- **Migrations**: embedded goose (`//go:embed migrations/*.sql`),
  автоприменение на старте API через `goose.Up`.
- **pgxpool config (explicit)**:
  - `MaxConns=10`, `MinConns=2`
  - `MaxConnLifetime=1h`, `MaxConnIdleTime=30m`
  - `HealthCheckPeriod=1m`
- **Health endpoints**: только `/healthz` (всегда 200 пока процесс жив).
  Compose healthcheck для Postgres — через `pg_isready`, для API — HTTP
  `/healthz`.
- **Graceful shutdown**: SIGINT/SIGTERM → `srv.Shutdown(10s)` →
  `scheduler.Stop()` → `pool.Close()`.
- **Cron**: библиотека `robfig/cron/v3`. M1 jobs: sessions cleanup
  (`0 * * * *`), FX fetch (`0 6 * * *`).
- **Startup sequence main.go**:
  1. parse config (envconfig)
  2. init logger (slog)
  3. open pgxpool
  4. ping pool (fail-fast)
  5. run goose migrations
  6. bootstrap user (если первый запуск + env)
  7. wire services (manual DI)
  8. setup scheduler, register jobs, start
  9. setup chi router
  10. http.Server.ListenAndServe в goroutine
  11. wait SIGINT/SIGTERM
  12. graceful shutdown

### 15.10. M1 acceptance test

1. `docker compose up -d --build` — postgres + api поднимаются, миграции
   применяются автоматически.
2. С env `BOOTSTRAP_USER_EMAIL` + `BOOTSTRAP_USER_PASSWORD` — первый юзер
   создаётся.
3. `pnpm --filter web dev` — открывается `http://localhost:5173`,
   редиректит на `/login`.
4. Логинимся.
5. На `/accounts` создаём manual-аккаунт «Бумажные».
6. На `/accounts/:id`:
   - Кнопка «Add position».
   - В диалоге: search «AAPL» → ничего не найдено → «Создать вручную»
     → ticker=AAPL, asset_class=us_stock, currency=USD, name=«Apple Inc.».
   - Quantity=10. Save.
7. На `/` (dashboard): видим позицию AAPL × 10, цена pending (нет цены,
   M1 не ходит за US-ценами). Total пока 0 или N/A.
8. **Альтернативный acceptance**: вручную через SQL вставить цену в `prices`
   и убедиться что аггрегация считает `valueNative = 10 × price` и
   конвертит в RUB через `fx_rates` (M1 кладёт USD/RUB через ЦБ FX cron).

---

## 16. Решения M1.2 (детализация)

Решения, зафиксированные после интервью на тему реализации auth M1.2
(вопросы 26–29). Дополняют §15 конкретикой codegen/crypto/sqlc/behavior.

### 16.1. oapi-codegen integration

- **Mode**: strict-server поверх chi-server adapter.
  - `strict-server: true` — методы получают типизированные `*RequestObject`
    и возвращают `*ResponseObject` (oneOf вариантов 200/401/422).
    oapi-codegen сам маршалит и проставляет статусы.
  - `chi-server: true` даёт `HandlerFromMux` для wiring.
- **Размер интерфейса**: один глобальный `ServerInterface` со всеми
  методами M1. Реализуется одним типом `*server.Server`. Per-tag splits — нет.
- **Where**: `internal/server/oapi/` — generated файлы (`api.gen.go`,
  `spec.gen.go`, `types.gen.go`) рядом с реализацией.
- **Runtime валидация**: `oapi-codegen/nethttp-middleware` —
  `OapiRequestValidator(spec)` как chi middleware. Spec embed через
  `//go:embed api/openapi.yaml` (yaml кладётся в `internal/server/oapi/`
  при build context либо копируется из корня monorepo).
- **Generation**:
  - `internal/server/oapi/gen.go` — `//go:generate go run ...oapi-codegen --config=config.yaml ../api/openapi.yaml`.
  - `tools.go` с blank import для версии в `go.mod`.
  - Makefile target `generate` запускает `cd apps/api && go generate ./...`
    и `pnpm --filter web generate`.
- **Middleware order** (chi mux):
  1. RequestID
  2. RealIP
  3. structured request logger
  4. Recoverer (panic → Problem 500)
  5. Timeout (30s)
  6. OapiRequestValidator (валидирует body/params/headers)
  7. (на protected routes) sessionMiddleware
- **Error mapper** (`writeProblem(w, err)`):
  - `errors.Is(err, ErrNotFound)` → 404
  - `errors.Is(err, ErrInvalidCredentials)` → 401
  - `errors.Is(err, ErrConflict)` → 409
  - validator-ошибка с `fields` → 422
  - default → 500 + `log.Error`

### 16.2. Crypto, password hashing, sessions

- **MASTER_KEY encoding**: base64url, 32 bytes plain → 43 char без padding.
  Парсится в config как:
  ```go
  keyBytes, err := base64.RawURLEncoding.DecodeString(cfg.MasterKey)
  if err != nil || len(keyBytes) != 32 { return ErrInvalidMasterKey }
  ```
  В compose dev — placeholder корректной длины, в prod — генерируется
  через `openssl rand -base64 32 | tr '+/' '-_' | tr -d '='`.
- **Domain separation через HKDF**:
  - `credentialsKey := hkdfExpand(masterKey, "credentials.v1", 32)` —
    для AES-GCM шифрования broker secrets.
  - Будущие назначения (sessions MAC, signing) — через свои labels.
  - Используем `golang.org/x/crypto/hkdf`.
- **AES-GCM**:
  - 12-byte nonce из `crypto/rand`, хранится в `account_credentials.nonce`.
  - **AAD = account_id (16 bytes)** — защита от swap-атаки на дамп БД
    (расшифровка чужих credentials под видом своих не пройдёт integrity check).
  - Encrypt: `gcm.Seal(nil, nonce, plaintext, accountIDBytes)`.
- **Argon2 hash format**:
  - PHC string: `$argon2id$v=19$m=65536,t=1,p=4$<salt_b64>$<hash_b64>`
  - Параметры в строке → upgrade-path: при login считаем оба варианта
    параметров; при успехе со старыми — пере-хэшируем под новые и UPDATE.
- **Library**: `github.com/alexedwards/argon2id` (тонкая обёртка ~150 строк
  без транзитивных deps).
  ```go
  hash, _ := argon2id.CreateHash(password, &argon2id.Params{
      Memory: 64 * 1024, Iterations: 1, Parallelism: 4,
      SaltLength: 16, KeyLength: 32,
  })
  match, _, _ := argon2id.ComparePasswordAndHash(password, hash)
  ```
- **Session token**:
  - 32 bytes из `crypto/rand` → `base64.RawURLEncoding` (43 char).
  - В cookie: plaintext base64.
  - В БД (`sessions.token_hash BYTEA(32)`): SHA-256 без соли (input — 256-bit
    random, brute-force нерелевантен).
- **Session middleware** кладёт в `request.Context()`:
  - Полный `User { ID, Email, DisplayCurrency }` — чтобы хендлеры не
    делали extra DB lookup.
  - Helpers: `UserFromContext(ctx)`, `MustUserFromContext(ctx)` (panic
    если missing — для protected routes).
- **Cookie attributes**:
  - Name: `sid`
  - HttpOnly: true
  - Secure: `cfg.IsProduction()` (в dev http localhost — false)
  - SameSite: Lax
  - Path: `/`
  - Max-Age: 30 дней (absolute timeout)
- **CORS**: НЕ используется. Same-origin: dev — Vite proxy `/api → :8080`,
  prod — Caddy reverse proxy на одном домене.

### 16.3. sqlc integration

- **Engine**: `sql_package: "pgx/v5"` — native pgx-типы, `pgxpool.Pool`
  как `DBTX`.
- **Output package**: один `internal/storage` (`storage.New(pool) *Queries`).
  Per-resource splits на пакеты — нет.
- **Queries SQL files** — per-resource в `internal/storage/queries/`:
  ```
  internal/storage/queries/
    users.sql
    sessions.sql
    accounts.sql
    account_credentials.sql
    instruments.sql
    instrument_external_ids.sql
    positions.sql
    prices.sql
    fx_rates.sql
    portfolios.sql
  ```
  Generated outputs — в `internal/storage/{users,sessions,…}.sql.go`,
  всё в одном `package storage`.
- **Type overrides**:
  - `numeric` → `github.com/shopspring/decimal.Decimal`
    (nullable → `decimal.NullDecimal`)
  - `uuid` → `github.com/google/uuid.UUID`
    (nullable → `uuid.NullUUID`)
- **NULL**: `emit_pointers_for_null_types: true` →
  `LastSyncedAt *time.Time` вместо `pgtype.Timestamptz`.
- **Generation**: `sqlc` бинарь установлен в Dockerfile.dev
  (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.27.0`). Запуск через
  Makefile: `cd apps/api && sqlc generate`.
- **Naming clash**: sqlc генерит `internal/storage/db.go` со своим `DBTX`
  interface и `Queries`. Наш текущий `db.go` (с `NewPool`) переименован
  в `pool.go` — конфликта нет.
- **sqlc.yaml** (примерное содержимое):
  ```yaml
  version: "2"
  sql:
    - schema: "internal/storage/migrations"
      queries: "internal/storage/queries"
      engine: "postgresql"
      gen:
        go:
          package: "storage"
          out: "internal/storage"
          sql_package: "pgx/v5"
          emit_pointers_for_null_types: true
          emit_json_tags: false
          overrides:
            - db_type: "uuid"
              go_type: "github.com/google/uuid.UUID"
            - db_type: "uuid"
              nullable: true
              go_type:
                type: "uuid.NullUUID"
                import: "github.com/google/uuid"
            - db_type: "numeric"
              go_type: "github.com/shopspring/decimal.Decimal"
            - db_type: "numeric"
              nullable: true
              go_type:
                type: "decimal.NullDecimal"
                import: "github.com/shopspring/decimal"
  ```
- **Транзакции**: `tx, _ := pool.Begin(ctx); qtx := q.WithTx(tx); ...; tx.Commit(ctx)`.

### 16.4. Behaviors

- **Bootstrap user**:
  - `BOOTSTRAP_USER_EMAIL` + `BOOTSTRAP_USER_PASSWORD` есть, юзера с этим
    email **нет** → создаём, лог `INFO bootstrap user created`.
  - `BOOTSTRAP_..._EMAIL` есть, юзер с этим email **уже есть** → no-op,
    лог `INFO bootstrap skipped: user exists`. Env-пароль не перезаписывает.
  - `users` пуста, env-переменные **не заданы** → лог
    `WARN bootstrap skipped: BOOTSTRAP_USER_EMAIL not set; first user
    must be created via /auth/register (not yet implemented)`. Приложение
    стартует, но залогиниться нельзя.
- **Login при существующей сессии**: создаётся новая сессия параллельно.
  Multi-device. Существующие сессии **не** инвалидируются.
- **`last_seen_at` coalescing**: middleware читает session, если
  `time.Since(LastSeenAt) > 1 minute` — UPDATE, иначе skip. Точность
  idle-чека ±1 минута (приемлемо при idle=30мин).
- **Logout**: `DELETE FROM sessions WHERE token_hash = $1` + Set-Cookie
  с `Max-Age=0` для немедленной очистки. Без BroadcastChannel (M5+).
- **Validator package в M1.2**: НЕ подключаем `go-playground/validator`.
  Shape-валидация — OpenAPI middleware. Бизнес-правила — sentinel errors
  в service layer. Подключим в M2 при появлении cross-field правил.
- **Auth events logging**: явные `log.Info("auth: login ok", "user_id",
  ..., "ip", ...)` и `log.Warn("auth: login failed", "email", ...,
  "ip", ...)` в auth handlers — security-relevant audit. Никогда не
  логируем password.

### 16.5. Tests

- **Unit (без БД)**: crypto helpers (`hkdf`, `aesgcm.Seal/Open`),
  argon2 wrapper.
- **Integration (testcontainers-go)**: auth service (login, logout, me,
  bootstrap), session middleware, account/instrument/position service.
  Реальный Postgres per-suite, миграции применяются на старте suite.
- **Handler-level**: `httptest.Server` поверх chi с реальным sessions
  middleware и testcontainers-Queries. Smoke на login flow + auth
  enforcement.
- **E2E (Playwright)**: отложено до M5+.

### 16.6. Что НЕ в M1.2

- Email verification, password reset, 2FA, account lockout, captcha,
  rate limit на login.
- Refresh tokens, JWT (отвергнуты в дизайне).
- CORS (same-origin).
- Audit log как отдельная таблица (использyetsя explicit logging).
- Sessions UI ("где я залогинен", revoke).
- BroadcastChannel logout sync между табами.

---

## 17. Решения M1.3 (детализация)

Решения, зафиксированные после интервью на тему accounts/instruments/positions
(вопросы 30–33). Конкретизируют §15 для CRUD-фич.

### 17.1. Authorization enforcement

- **Hybrid service + repository**:
  - Service-сигнатуры всегда принимают `userID` первым параметром:
    `Get(ctx, userID, accountID)`, `Delete(ctx, userID, accountID)`.
  - Repository (sqlc) queries фильтруют по `WHERE id = $1 AND user_id = $2`
    везде, где сущность user-owned. Defense-in-depth: даже если service забыл
    проверку, repo не вернёт чужой ресурс.
- **Handler boilerplate**: явный вызов
  `user := auth.MustUserFromContext(ctx); res, err := svc.X(ctx, user.ID, params)`
  в каждом handler. Не выделяем custom oapi middleware.
- **Error на чужой ресурс — 404 Not Found** (security best practice):
  не палим существование. Repository query без results → `ErrNotFound` →
  404. Не различаем "не существует" / "не твой".
- **Multi-step ownership** (positions через accounts): один SQL с JOIN, не
  два запроса. Атомарно, без race-окна.
  ```sql
  SELECT p.*, i.* FROM positions p
    JOIN accounts a ON p.account_id = a.id
    JOIN instruments i ON p.instrument_id = i.id
  WHERE a.user_id = $1 AND a.id = $2 AND p.instrument_id = $3
  ```
- **Concurrency на дублирующий POST**: чистый INSERT и catch
  `pgconn.PgError.Code == "23505"` (unique_violation) → `ErrConflict` → 409.
  Без UPSERT.
- **Без ownership-cache в session** — каждый запрос делает свежий lookup
  через JOIN. Дёшево, корректно.

### 17.2. Account types в M1.3

- **Только `manual`**. POST /accounts с `type: tinvest|bybit` отвергается:
  422 + `fields: {type: "manual is the only supported type in this version"}`.
  OpenAPI спека описывает все три (forward-compat), service enforce-ит.
- **`name`**: обязательный, 1..100 chars, без uniqueness per user.
- **Без `default_currency` на account** — валюта позиции определяется
  `instruments.currency`. Per-account валюта не нужна.
- **Без cosmetic полей** в M1.3 (color, description, sort_order). Добавим
  в M5 polish при необходимости.
- **POST /accounts response** — возвращает `Account` с пустым массивом
  `positions[]` (унификация с GET /accounts/:id формой `AccountDetail`).
- **manual flow** в M1.3: `account.SourceType = 'manual'`,
  `LastSyncStatus = nil`, `LastSyncError = nil`. Без credentials.
  Без sync.

### 17.3. Instruments lifecycle

- **Anyone creates global**: instruments — глобальный канонический
  справочник без `user_id`. Любой залогиненный юзер может создать.
- **Search**: простой `ILIKE '%' || $1 || '%' OR name ILIKE ...`,
  hard-limit 20, ORDER BY exact-match-first затем алфавит. Без full-text
  и trigram до M5+.
- **Hard-dedup через UNIQUE constraint**:
  - Миграция `0002_instruments_unique.sql`:
    ```sql
    CREATE UNIQUE INDEX instruments_ticker_asset_class_uidx
    ON instruments (LOWER(ticker), asset_class);
    ```
  - POST /instruments в service: SELECT по `(LOWER(ticker), asset_class)`,
    если найден — возвращаем существующий instrument (idempotent), иначе
    INSERT. На race с конкурентным INSERT — catch `23505` и повторный
    SELECT.
  - Это пересмотр Q20e (там сказано "никакого UNIQUE на ticker"). Уточнение:
    UNIQUE не на `ticker` сам по себе, а на пару `(LOWER(ticker), asset_class)`.
    `MMM/us_stock` и `MMM/ru_stock` остаются разными — задача решена.
- **No DELETE** через API в M1. Каталог append-only.
- **Global visibility**: search возвращает любые instruments всем юзерам.

### 17.4. Position lifecycle

- **Quantity > 0** обязательно. Удаление — DELETE endpoint, не quantity=0.
  Negative (shorts) — out of scope MVP.
- **PUT строго update**: PUT `/accounts/:id/positions/:instrumentId` →
  обновляет quantity существующей позиции; если нет → 404. UPSERT-семантику
  делаем через POST.
- **POST конфликт**: POST `/accounts/:id/positions` с `instrumentId`,
  который уже есть → 409 (с сообщением "use PUT to update"). Без accumulate
  и без overwrite.
- **DELETE**: 204 если удалили, 404 если не было. Не idempotent, чтобы
  юзер видел расхождение если кликнул дважды.
- **Несуществующий instrumentId в POST**: 422 с
  `fields: {instrumentId: "not found"}`. Семантическая, не shape-валидация.

### 17.5. AccountDetail composition

- **Один SQL с JOIN** `positions JOIN instruments` для построения списка
  positions с embedded `Instrument`. sqlc возвращает кастомную row
  с префиксированными колонками (`i_id`, `i_ticker`, …).
- **Без цен в Position**: AccountDetail возвращает позиции без price/value.
  OpenAPI Position schema не имеет `price`. Цены и valuation — в
  PortfolioPosition (M1.4 endpoint `/portfolio`).
- UI логика: `/accounts/:id` показывает "AAPL × 10" без $-значений; для
  total — переходит на dashboard `/`.

### 17.6. Sentinel errors

```go
// internal/account
var (
    ErrNotFound         = errors.New("account: not found")
    ErrTypeNotSupported = errors.New("account: type not supported in this version")
)

// internal/instrument
var (
    ErrNotFound = errors.New("instrument: not found")
)

// internal/position
var (
    ErrAccountNotFound    = errors.New("position: account not found")
    ErrInstrumentNotFound = errors.New("position: instrument not found")
    ErrAlreadyExists      = errors.New("position: already exists")
    ErrNotFound           = errors.New("position: not found")
)
```

Mapper в `internal/server/handlers.go`:
- `account.ErrNotFound`, `instrument.ErrNotFound`, `position.ErrNotFound`,
  `position.ErrAccountNotFound` → 404
- `account.ErrTypeNotSupported`, `position.ErrInstrumentNotFound` → 422
  с `fields`
- `position.ErrAlreadyExists` → 409

### 17.7. Транзакции в M1.3

Не нужны — все операции single-statement (CREATE/UPDATE/DELETE). UPSERT
для instruments — single SQL. POST /accounts → INSERT.

Транзакции появятся в M2 при account+credentials атомарном создании.

### 17.8. M1.3 acceptance (см. ниже §18 для M1.5 acceptance)

1. Login (M1.2 уже есть).
2. POST `/accounts {name: "Бумажные", type: "manual"}` → 201 + Account.
3. POST `/instruments {ticker: "AAPL", assetClass: "us_stock", currency: "USD", name: "Apple Inc."}` → 201 + Instrument.
4. Повторный POST с тем же payload → 200/201 с тем же `id` (idempotent).
5. POST `/accounts/:id/positions {instrumentId, quantity: "10"}` → 201 + Position.
6. Повторный POST → 409.
7. PUT `/accounts/:id/positions/:instrumentId {quantity: "15"}` → 200 + Position.
8. GET `/accounts/:id` → AccountDetail с positions=[{instrument: {ticker: "AAPL", ...}, quantity: "15"}].
9. GET `/instruments/search?q=app` → top-20, AAPL первой строкой.
10. DELETE `/accounts/:id/positions/:instrumentId` → 204.
11. DELETE повторно → 404.
12. DELETE `/accounts/:id` → 204; CASCADE удаляет оставшиеся positions.
13. GET `/accounts/:id` чужого юзера (через раздельный токен) → 404.

---

## 18. Решения M1.5 (детализация)

Решения, зафиксированные после грилла на frontend M1.5 (вопросы 34–36).
Конкретизируют §15.7 для веб-интерфейса.

### 18.1. UI setup

- **shadcn-vue CLI init** (`pnpm dlx shadcn-vue@latest init`), затем
  `pnpm dlx shadcn-vue@latest add <component>` для каждого нужного компонента.
- **Минимальный набор M1.5**: Button, Card, Input, Label, Form (`<Form>`,
  `<FormField>`, `<FormItem>`, `<FormLabel>`, `<FormControl>`, `<FormMessage>`),
  Dialog, Table, Select, Sonner (toasts), Skeleton, DropdownMenu.
- **Locale**: всё UI на русском. Технические идентификаторы (тикеры, asset_class
  значения, валюты) — оставляем как есть (английские).
- **i18n не подключаем** в M1.5 (один пользователь, один язык). Когда понадобится
  multi-language — добавим vue-i18n.
- **Theme**: light + dark с переключателем в Header DropdownMenu. CSS variables
  уже подготовлены в `style.css`.

### 18.2. Forms

- **Pattern**: shadcn-vue `<Form>` обёртки над vee-validate. Каждое поле в
  `<FormField name="x">` → `<FormItem>` → `<FormLabel>` → `<FormControl>` →
  `<FormMessage>`. Связывание автоматическое.
- **zod**: orval-generated zod-схемы из OpenAPI как baseline. Для feature-specific
  правил — `.extend()`. Single source of truth — OpenAPI.
- **`@vee-validate/zod`** через `toTypedSchema(schema)` — мост между zod и vee-validate.

### 18.3. API client

- **orval** генерит TS-клиент в `src/api/generated/`:
  - `--client vue-query` — готовые `useXxxQuery` / `useXxxMutation`.
  - `--mutator src/api/mutator.ts` — custom fetcher.
- **Custom mutator** делает fetch с `credentials: 'include'` (cookie sid),
  парсит non-2xx как `Problem`, бросает typed `HttpError`.
- **Generation triggers**: Makefile target `generate` → `pnpm --filter web generate`
  → orval CLI. После каждого изменения OpenAPI спеки — пересборка.

### 18.4. Auth bootstrap flow

- **`App.vue setup`** → `onMounted(() => authStore.bootstrap())`.
- **`authStore.bootstrap()`** делает `meQuery.refetch()` (или прямой fetch через
  mutator) → ставит `ready=true` независимо от результата.
- До `ready === true` — глобальный spinner вместо `<RouterView>`.
- **Router guard**:
  ```ts
  router.beforeEach(async (to) => {
    await authStore.ready  // Promise resolves after bootstrap
    if (to.meta.requiresAuth && !authStore.user) {
      return { name: 'login', query: { redirect: to.fullPath } }
    }
    if (to.meta.requiresGuest && authStore.user) {
      return { name: 'dashboard' }
    }
  })
  ```

### 18.5. Pinia stores

- **`useAuthStore`** — обёртка над TanStack Query:
  - `user = computed(() => meQuery.data.value)` — без duplicate state.
  - `ready: Promise<void>` — резолвится после первого me-запроса.
  - `login(credentials)` — мутация → setQueryData([me]).
  - `logout()` — fetch /auth/logout → queryClient.clear() → router.push('/login').
- **`useUiStore`** — UI client-state:
  - `displayCurrency: string` (default = `user.displayCurrency`, persist via
    localStorage через `pinia-plugin-persistedstate`).
  - `theme: 'light' | 'dark'` (persist).

### 18.6. Routes

- `/login` (meta `requiresGuest: true`) → AuthLayout + LoginPage.
- `/` (meta `requiresAuth: true`) → AppLayout + DashboardPage.
- `/accounts` (`requiresAuth`) → AppLayout + AccountListPage.
- `/accounts/:id` (`requiresAuth`) → AppLayout + AccountDetailPage.

### 18.7. Login flow

- `useLoginMutation` → on success: `queryClient.setQueryData(['/auth/me'], user)`,
  router push на `route.query.redirect ?? '/'`.
- on error 401 (Invalid credentials) → vee-validate `setErrors({email: 'Неверный email или пароль'})` (или toast).
- on error 422 → `setErrors` с серверными `fields`.

### 18.8. Logout flow (M1.5)

- `useLogoutMutation` → fetch `/auth/logout` → `queryClient.clear()` →
  `authStore.user = null` → router push `/login`.
- Без BroadcastChannel (M5+).

### 18.9. Dashboard composition

- **Summary cards** (3 шт.):
  - "Всего" — grandTotal в displayCurrency.
  - "По классу активов" — top asset_class с долей % и значением.
  - "Устаревшие цены" — счётчик позиций с `priceStale: true`.
- **Currency selector в Header** (DropdownMenu), 3 валюты: RUB / USD / EUR
  (хардкод; больше — в M5).
- **Positions table** — все позиции, sort default by `valueDisplay desc`,
  колонки: Ticker / Asset class / Quantity / Price / Value display.
- **Без charts в M1.5** — отложено в M5.

### 18.10. UI states

- **Loading**: `<Skeleton>` placeholders, имитирующие финальный layout.
- **Empty (no accounts)**: `<EmptyState>` с CTA "Создать первый аккаунт →"
  → router push `/accounts`.
- **Empty (no positions on dashboard)**: `<EmptyState>` "Нет позиций. Добавь
  через [Аккаунты]".
- **Error 5xx**: toast (Sonner) с retry кнопкой, cached data остаётся.
- **Error 401 (на любом protected запросе)**: глобальный `queryCache.onError` →
  authStore.logout() → router /login.

### 18.11. Forms specifics

- **LoginForm**: email + password. zod из orval `loginRequestZod`.
- **CreateAccountDialog**: name + type (select). Type ограничен `manual`
  на UI уровне (один option), хотя backend описывает три (forward-compat).
- **AddPositionDialog (hybrid flow)**:
  1. Tab 1 "Поиск": `<InstrumentSearch>` (autocomplete по name/ticker через
     `useSearchInstrumentsQuery({ q })` debounced 300ms).
  2. Если выбрал — quantity input → save → `useCreatePositionMutation`.
  3. Если не нашёл — Tab 2 "Создать вручную": ticker / asset_class / currency /
     name → `useCreateInstrumentMutation` → получаем `instrumentId` →
     `useCreatePositionMutation`.
- **EditPositionDialog**: только quantity input → `useUpdatePositionMutation`.
- **Delete confirmations**: AlertDialog (shadcn-vue compoonent, добавим если
  потребуется), хардкод "Уверены?" → mutation.

### 18.12. Error handling layers

- **Mutator (lowest)**: парсит non-2xx → `HttpError(status, problem)`.
- **TanStack queryCache.onError (global)**:
  - 401 → authStore.logout() → /login.
  - 5xx → toast.error("Что-то пошло не так").
- **per-mutation onError (highest)**:
  - 422 + fields → form.setErrors(fields).
  - 409 → toast.error(problem.detail).

### 18.13. Number/date formatting

- **`useFormatters()` composable** в `lib/formatters.ts`:
  - `currency(amount: string|number, code: string): string` — `Intl.NumberFormat`.
  - `quantity(amount: string|number): string` — `Intl.NumberFormat` без currency.
  - `date(d: Date|string): string` — относительное "5 минут назад" /
    `Intl.RelativeTimeFormat` или fallback на абсолютное.

### 18.14. Зависимости (M1.5)

К существующему `apps/web/package.json` добавить:

```
"dependencies":
  "vue-router": "^4.4.5"
  "pinia": "^2.2.6"
  "pinia-plugin-persistedstate": "^4.1.3"
  "@tanstack/vue-query": "^5.62.0"
  "vee-validate": "^4.14.6"
  "zod": "^3.23.8"
  "@vee-validate/zod": "^4.14.6"
  "vue-sonner": "^1.3.0"
  "@vueuse/core": "^11.3.0"
  "radix-vue": "^1.9.10"
  "lucide-vue-next": "^0.460.0"

"devDependencies":
  "orval": "^7.3.0"
```

### 18.15. M1.5 acceptance

1. `make services` поднимает api+postgres. `pnpm --filter web dev` поднимает Vite.
2. `http://localhost:5173/` редиректит на `/login`.
3. Login `dev@local.test` / `devpassword12345` → редирект `/`.
4. Header показывает email + currency dropdown + theme toggle + logout.
5. Empty state "Создать первый аккаунт → /accounts".
6. На `/accounts` создать manual account.
7. На `/accounts/:id` "Add position" → search "AAPL" → пусто → "Создать вручную"
   → ticker=AAPL, asset_class=us_stock, currency=USD, name=Apple Inc. → quantity=10 → Save.
8. На `/` видим позицию AAPL × 10 в таблице, priceStale=true (нет цены).
9. Через SQL вставить price → reload `/` → видим valueDisplay.
10. Переключить валюту USD ↔ RUB в header → grand total меняется.
11. Theme toggle dark/light работает, persist в localStorage.
12. Logout → редирект /login + cleared cache.


