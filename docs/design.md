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

