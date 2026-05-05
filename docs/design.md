# Omnifolio — Design Document

Приложение для учёта личных инвестиций. Агрегирует портфели из разных источников
(российские бумаги через T-Invest, крипта через Bybit и Binance, остальное
вручную) и показывает их по отдельности и в сумме.

Документ описывает архитектуру v0.1 — текущее состояние кодовой базы. Ниже —
живой обзор. Решения, принятые на отдельных milestone-интервью, остались
в виде исторических разделов §15–§19. Сводку отклонений от них — см. §20.

---

## 1. Цель и scope

**Главная задача**: снапшот текущей стоимости и распределения активов
плюс ручной журнал ежемесячных пополнений как фундамент под будущий расчёт
доходности.

- Что показываем: сколько всего сейчас, в каких активах, в каких валютах,
  плюс список deposits (помесячные взносы) — для будущего TWR/XIRR.
- Что НЕ показываем (отложено): доходность во времени (TWR/XIRR), журнал сделок,
  P&L по позициям, налоговый учёт, ребалансировка, дивиденды, корпоративные действия.

Решение строит модель данных вокруг текущего состояния (positions snapshot),
а не вокруг истории транзакций. Deposits добавлены отдельной таблицей —
это единственный счётчик «вложенных денег», независимый от позиций.

---

## 2. Источники данных

### 2.1. Position sources (откуда берём позиции)

Гибридная стратегия: где есть API — синкаем; остальное руками.

| Источник | Как тянем позиции | Статус |
|---|---|---|
| Manual | Ручной ввод через UI | реализовано |
| T-Invest | REST `https://invest-public-api.tinkoff.ru/rest/...` (Bearer) | реализовано |
| Bybit | REST `/v5/account/wallet-balance` (HMAC) | реализовано |
| Binance | REST `/api/v3/account` (HMAC) | реализовано |
| IBKR | Client Portal Web API (требует gateway) | **backlog** |

Реализация в `apps/api/internal/source/{tinvest,bybit,binance}/`. Manual —
не отдельный source-пакет: позиции пишутся напрямую в `positions` через
обычный CRUD-эндпоинт, без `Sync`/`Resolve`.

Архитектурный интерфейс (`internal/source/source.go`):

```go
type PositionSource interface {
    ListSubAccounts(ctx context.Context, creds []byte) ([]SubAccount, error)
    Sync(ctx context.Context, creds []byte, subAccountID string) ([]Position, error)
    ResolveInstrument(ctx context.Context, creds []byte, nativeID string) (InstrumentSeed, error)
}
```

### 2.2. Price providers (откуда берём цены)

Раздельный слой, не привязан к position source. В отличие от MVP-наброска,
цены не подтягиваются «лениво при запросе `/portfolio`», а пушатся
расписанием из отдельного бинаря `apps/cron` (см. §4) через admin-эндпоинты
API. Сам API хранит последний снимок в `prices` и просто читает его при
расчёте портфеля.

| Asset class | Провайдер | Источник в коде |
|---|---|---|
| `ru_stock`, `ru_bond`, `ru_etf` | T-Invest `MarketDataService.GetLastPrices` (REST) | `internal/source/tinvest` + `apps/cron` |
| `us_stock`, `us_etf` | Finnhub (`finnhub.io`) | `apps/cron` |
| `crypto` | Bybit public market data (`/v5/market/tickers`) | `apps/cron` |
| FX (`USD/RUB`, `EUR/RUB`, …) | ЦБ РФ XML daily (`cbr.ru/scripts/XML_daily.asp`) | `apps/cron` + `internal/fx` |
| Stablecoins (USDT, USDC) и `cash` в нативной валюте | Хардкод 1:1 | API-сервис `internal/portfolio` |

Routing — по `instrument.asset_class`.

---

## 3. Модель данных

### 3.1. Account как единица учёта

- **Account** — физический счёт у источника (один T-Invest брокерский счёт = один Account).
  Содержит позиции, синкается из source.
- Все агрегаты (`/portfolio`, дашборд) считаются по всем аккаунтам пользователя.

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

### 3.5. Схема

```sql
users               (id, email, password_hash, display_currency, created_at, updated_at)
sessions            (token_hash BYTEA PK, user_id, expires_at, last_seen_at, created_at)

accounts            (id, user_id, source_type, name,
                     last_synced_at, last_sync_status, last_sync_error,
                     created_at, updated_at)
                    -- source_type IN ('manual','tinvest','bybit','binance')
account_credentials (account_id PK, ciphertext BYTEA, nonce BYTEA, key_version,
                     created_at, updated_at)

instruments              (id, ticker, asset_class, currency, name,
                          created_at, updated_at)
                         -- asset_class IN ('ru_stock','ru_bond','ru_etf',
                         --                 'us_stock','us_etf','crypto','cash')
                         -- UNIQUE (LOWER(ticker), asset_class)
instrument_external_ids  (source, native_id, instrument_id,
                          PRIMARY KEY(source, native_id))

positions  (account_id, instrument_id, quantity NUMERIC(38,18),
            created_at, updated_at,
            PRIMARY KEY(account_id, instrument_id))
prices     (instrument_id PK, price NUMERIC(20,8), fetched_at)
fx_rates   (date, from_ccy, to_ccy, rate NUMERIC(20,10),
            PRIMARY KEY(date, from_ccy, to_ccy))

deposits   (id, user_id, month DATE, amount NUMERIC(20,0),
            created_at, updated_at)
           -- month = первое число месяца (CHECK date_trunc)
           -- amount > 0
```

Заметки:
- `cash` как asset_class добавлен миграцией 0003 — отдельные строки для
  валютных остатков (`RUB`, `USD`, `EUR`) с `price=1.00` в нативной валюте,
  чтобы баланс на брокерском счёте попадал в агрегаты.
- Таблицы `portfolios` / `portfolio_accounts` (заложенные в первоначальном
  дизайне для "named портфелей") **удалены** миграцией 0004. Портфель
  пересчитывается on-the-fly по всем accounts юзера; именованные группы
  отложены до явной потребности.
- Уникальность инструмента — `UNIQUE(LOWER(ticker), asset_class)`
  (миграция 0002), что разводит `MMM/us_stock` и `MMM/ru_stock`, но
  делает `POST /instruments` идемпотентным.

### 3.6. Deposits (журнал пополнений)

Отдельная сущность — фиксирует помесячные взносы пользователя в условной
"эффективной" валюте учёта. Таблица минимальна (`user_id`, `month`,
`amount`) и существует ради будущего расчёта доходности (TWR/XIRR), для
которого нужна история cash flows. На MVP UI показывает её как простой
список и используется при оценке "сколько вложено" на дашборде.

- `month` хранится как первое число месяца (CHECK), чтобы один депозит
  за период был естественно один — без дубликатов на разные дни.
- `amount` — `NUMERIC(20, 0)` (целые копейки/рубли — фиксированная
  валюта учёта пользователя).
- Удаление — hard, без soft-delete и без редактирования (создал-удалил-
  пересоздал, если ошибся).

---

## 4. Синхронизация

Реализация разделена между двумя процессами:

- **`apps/api`** — внутренний `robfig/cron/v3` scheduler в `internal/scheduler/`,
  который раз в час пробегает по всем брокерским аккаунтам и подтягивает
  свежие позиции (плюс служебный sessions cleanup и daily FX refetch).
- **`apps/cron`** — отдельный одноразовый Go-бинарь, запускаемый Railway
  Cron по расписанию. Он наполняет каталог инструментов, актуальные цены и
  курсы валют через admin-эндпоинты API. Не работает в фоне — отрабатывает
  все шаги и завершается.

### 4.1. Позиции (cron в API + on-demand)

- Внутренний scheduler API: `0 * * * *` — `syncerSvc.SyncAll()` обходит
  accounts с `source_type IN ('tinvest','bybit','binance')`.
- On-demand: `POST /accounts/:id/sync` — синхронный, ждёт результат.
- На создание T-Invest аккаунта — async первый sync сразу после INSERT
  (`lastSyncStatus='pending'`).
- Применение снимка — одна транзакция: UPSERT текущих позиций + DELETE
  ушедших + UPDATE `last_sync_*`. Concurrency-защита —
  `pg_try_advisory_xact_lock(hashtext('sync:'||account_id))`.
- В рамках того же sync собираются `instrument_id` всех новых позиций и
  для T-Invest сразу UPSERT-ятся цены через `MarketDataService.GetLastPrices`.

### 4.2. Цены и каталог инструментов (бинарь `apps/cron`)

`apps/cron/cmd/cron/main.go` — однопроходный воркер. Запускается Railway
Cron, делает все шаги последовательно и `os.Exit`. Аутентифицируется на
API через `Authorization: Bearer ${ADMIN_API_KEY}`.

Шаги:

1. Загрузить статический seed инструментов (US акции/ETF, облигации) из
   embedded JSON.
2. Подтянуть актуальный список MOEX-инструментов из T-Invest (если задан
   `TINVEST_TOKEN`).
3. Подтянуть USDT-spot инструменты Bybit (public market data).
4. `POST /admin/instruments` — UPSERT каталога.
5. `GET /admin/instruments` — verify.
6. Котировки: Finnhub для `us_stock`/`us_etf`, T-Invest для MOEX,
   Bybit public для `crypto`. `cash` instrument-ы получают цену 1.00.
7. `POST /admin/prices` — UPSERT в `prices`.
8. ЦБ РФ XML → курсы валют.
9. `POST /admin/fx` — UPSERT в `fx_rates`.
10. Exit.

Это push-модель: API больше не лезет за ценами при запросе `/portfolio`
(старая идея «lazy cache с TTL» отменена). Если cron не отработал, в
`/portfolio` поля `priceFetchedAt` стареют, и API помечает позицию
`priceStale=true`.

### 4.3. FX курсы

- Daily — внутренним scheduler-ом API (`0 6 * * *`), плюс ещё раз через
  тот же `apps/cron` (overlap намеренный — гарантия свежих курсов
  независимо от того, какой из процессов жив).

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
- Бэк: `oapi-codegen` (strict-server + chi-server) генерит интерфейсы
  в `internal/server/oapi/`; реализация — вручную в `internal/server/handlers.go`.
- Фронт: `orval --client vue-query` (через mutator
  `apps/web/src/api/mutator.ts`) генерит TS-клиент с готовыми
  `useXxxQuery` / `useXxxMutation` хуками в `apps/web/src/api/generated/`.

Текущие пути (по тегам):

- **auth**: `POST /auth/login`, `POST /auth/logout`, `GET /auth/me`.
- **accounts**: `GET/POST /accounts`, `GET/PUT/DELETE /accounts/{id}`,
  `POST /accounts/tinvest/preview`, `POST /accounts/{id}/sync`.
- **positions**: `GET /accounts/{id}/positions`,
  `POST /accounts/{id}/positions`,
  `PUT/DELETE /accounts/{id}/positions/{instrumentId}`.
- **instruments**: `GET /instruments` (с фильтром+пагинацией),
  `GET /instruments/search`.
- **portfolio**: `GET /portfolio?currency=...`.
- **deposits**: `GET/POST /deposits`, `DELETE /deposits/{id}`.
- **system**: `GET /healthz`.

Admin-эндпоинты для cron-воркера живут вне OpenAPI спеки и регистрируются
напрямую в `internal/server/admin.go` (chi routes под middleware
`auth.RequireAdmin(ADMIN_API_KEY)`):

- `POST /admin/instruments` — bulk UPSERT каталога + `instrument_external_ids`.
- `GET /admin/instruments` — verify.
- `POST /admin/prices` — bulk UPSERT в `prices`.
- `POST /admin/fx` — bulk UPSERT в `fx_rates`.

Они нарочно не в публичной OpenAPI: их клиент — только наш собственный
`apps/cron`, отдельный контракт между процессами не нужен.

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
| UI | Vue 3 (Composition API, `<script setup>`) + Tailwind v4 |
| Component primitives | radix-vue (headless) + собственные обёртки в `components/ui/` |
| Icons | lucide-vue-next |
| State (client) | Pinia (v3) + `pinia-plugin-persistedstate` |
| State (server) | TanStack Query (vue-query v5) |
| Composables | `@vueuse/core` (см. §8.2 и `docs/vueuse.md`) |
| Routing | vue-router (history mode) |
| API client | orval из OpenAPI (`vue-query` client, custom fetch mutator) |
| Forms | vee-validate + zod (`@vee-validate/zod`) |
| Toasts | vue-sonner |
| Tests | Vitest (unit + component). Playwright **отложен**. |
| Package manager | pnpm |
| TS | strict + noUncheckedIndexedAccess |

Разделение state:
- **Server-state** (списки, дашборд, цены) — TanStack Query.
- **Client-state** (выбранная валюта, UI-флаги, тема) — Pinia.

UI-примитивы — небольшая собственная библиотека (`button`, `card`,
`checkbox`, `confirm`, `dialog`, `input`, `label`, `table`) поверх
radix-vue. CLI `shadcn-vue` для добавления компонентов **не используется**:
тяжёлые / редкие компоненты не нужны, а написать тонкую обёртку проще,
чем тащить генератор и оверрайдить его шаблоны.

### 8.1. Tailwind conventions

- Брать значения только из стандартной шкалы Tailwind (`gap-1`, `text-sm`, `w-48`, `rounded-md` и т.п.). Не вводить новые arbitrary values в квадратных скобках (`gap-[3px]`, `text-[12.5px]`, `w-[188px]`).
- Если точного значения в стандартной шкале нет — выбрать ближайший токен или предложить пользователю варианты. Не вписывать `[Npx]` молча.

### 8.2. VueUse

Перед написанием ручной обёртки над `ref`/`watch`/DOM/Storage — проверить
готовый composable в `@vueuse/core` (`useToggle`, `useStorage`,
`useMediaQuery`, `useEventListener` и т. д.). Каталог типовых соответствий
и правила использования — `docs/vueuse.md`.

### 8.3. Mobile

UI адаптирован под мобильные viewport-ы — sidebar становится off-canvas
(toggle через Pinia `useUiStore`), таблицы переходят в card-вид,
sticky-summary на дашборде отключён на узких экранах (см. коммит
`fix(web): drop sticky summary on mobile dashboard`). Адаптация целиком
делается через Tailwind responsive-utilities (`md:`, `lg:`); отдельный
JS-определитель «mobile» специально не вводится.

---

## 9. Структура репозитория

Monorepo, pnpm-workspace. `apps/web` — единственный пакет в pnpm workspace;
`apps/api` и `apps/cron` — независимые Go-модули.

```
omnifolio/
  api/
    openapi.yaml                # source of truth публичного контракта
  apps/
    api/                        # Go backend (chi + sqlc + oapi-codegen)
      cmd/api/main.go
      internal/
        config/                 # envconfig
        server/                 # chi router, handlers, admin, problem
          oapi/                 # oapi-codegen generated
        auth/                   # users + sessions + argon2 + middleware
        crypto/                 # AES-GCM, HKDF
        account/                # accounts + credentials services
        instrument/             # canonical instruments + external_ids
        position/               # positions service (manual CRUD)
        portfolio/              # /portfolio aggregation (read-only)
        deposits/               # deposits CRUD
        fx/                     # ЦБ FX fetch + lookups
        scheduler/              # robfig/cron jobs registration
        syncer/                 # position sync orchestration
        source/                 # PositionSource implementations
          tinvest/
          bybit/
          binance/
        storage/
          migrations/*.sql      # goose
          queries/*.sql         # sqlc inputs
          *.sql.go              # sqlc generated
      sqlc.yaml
      go.mod
    web/                        # Vue 3 SPA
      src/
        api/                    # orval-generated + mutator
        components/
          layout/
          ui/                   # button, card, dialog, table, ...
        features/
          auth/ account/ dashboard/ deposits/ instrument/ settings/
        lib/                    # utils, formatters, http-error
        stores/                 # auth.ts, ui.ts (pinia)
        router.ts
        App.vue main.ts
      orval.config.ts
      vite.config.ts
      package.json
    cron/                       # отдельный Go-бинарь
      cmd/cron/main.go
      go.mod
  compose.yml                   # dev (postgres + api + опционально cron)
  pnpm-workspace.yaml
  Makefile                      # generate / dev / services / test / build
  docs/
    design.md                   # этот файл
    railway.md                  # ops по Railway
    vueuse.md                   # таблица соответствий и правила использования
```

---

## 10. Деплой

Рантайм — **Railway** (PaaS). Single-user, инфраструктура минимальна;
управляется без k8s. Полное операционное руководство — `docs/railway.md`.

Сервисы в Railway:

- `api` — собирается из `apps/api/Dockerfile`, watch pattern на `apps/api/**`.
  Слушает `:8080`, healthcheck — `GET /healthz`. На старте автоматически
  применяет goose-миграции.
- `web` — собирается из `apps/web/Dockerfile` (Vite build), watch pattern
  на `apps/web/**` и `api/**` (изменение OpenAPI триггерит ребилд фронта
  через orval). Раздаёт статику и проксирует `/api/*` на сервис `api`
  через Railway internal networking.
- `cron` — собирается из `apps/cron/Dockerfile`, запускается Railway Cron
  по расписанию (см. `docs/railway.md`). Не держит долгоживущего HTTP-
  сервера — отрабатывает свои шаги (см. §4.2) и завершается.
- `postgres` — managed plugin Railway.

Auto-deploy — с `main`. Секреты (`DATABASE_URL`, `MASTER_KEY`,
`SESSION_SECRET`, `ADMIN_API_KEY`, `TINVEST_TOKEN`, `FINNHUB_API_KEY`,
`BOOTSTRAP_USER_*`) — в Railway env vars.

**Backup**: managed Postgres-плагин делает снимки сам; собственный
`pg_dump`-cron не вводим — это ответственность Railway.

**Reverse proxy / TLS**: Railway раздаёт публичный домен с auto-HTTPS,
своего Caddy/Nginx нет.

---

## 11. Observability

В MVP — ничего. `slog` JSON в `docker logs`.

Добавляем по необходимости:
- Prometheus + `/metrics` — когда захочется метрики.
- OpenTelemetry — когда multi-user или запутанная распределёнка.
- Sentry — когда заболит трекинг ошибок.

---

## 12. MVP Roadmap

### M0 — Skeleton ✅
- Monorepo по layout. `compose.yml` для dev. pnpm-workspace, Makefile.
- Healthcheck `/healthz`, hello-world Vue.

### M1 — Manual portfolio + auth ✅
- Goose миграции схемы (см. §3.5). Auth: argon2id, sessions, middleware,
  `/login` `/me` `/logout`.
- OpenAPI: `/accounts` (CRUD), `/accounts/:id/positions`, `/portfolio`,
  `/instruments`, `/instruments/search`.
- sqlc queries, service layer, chi handlers.
- Frontend: login, dashboard, accounts page (CRUD), instruments page.

### M2 — T-Invest source ✅
- REST-клиент к T-Invest (`internal/source/tinvest`).
- `TInvestPositionSource` (Sync + Resolve) + цены через
  `MarketDataService.GetLastPrices` в рамках того же sync.
- AES-GCM encryption для credentials, AAD = account_id.
- Two-step preview flow при создании аккаунта (выбор sub-account).
- UI: account type=tinvest, кнопка «Синхронизировать».
- Hourly sync cron в API.

### M3 — Bybit + Binance + crypto prices ✅
- `BybitPositionSource` через REST + HMAC.
- `BinancePositionSource` через `/api/v3/account` (HMAC).
- Crypto-цены — Bybit public market data (CoinGecko **не используется**).
- UI: account type=bybit, type=binance — формы api_key + api_secret.

### M4 — Cron worker + admin push ✅
- Вынесен отдельный бинарь `apps/cron`, который пушит инструменты, цены
  и FX в API через `/admin/*` (Bearer `ADMIN_API_KEY`).
- Lazy on-request fetch цен — заменён на push-модель.
- API маркирует позиции `priceStale=true`, если `prices.fetched_at`
  старше 24h (или нет FX rate).

### M5 — Polish + Railway ✅
- Pinia store для UI-state (selected currency, theme, sidebar).
- Адаптация под мобильные viewport-ы.
- Deploy на Railway (api/web/cron/postgres). Auto-deploy с `main`.
- Deposits фича — журнал ежемесячных пополнений.
- Settings page.

### В работе / следующее
- Расчёт доходности (TWR/XIRR) поверх deposits + текущего портфеля.
- Графики (asset_class breakdown, динамика total) — chart-библиотека пока
  не выбрана.
- Снапшоты портфеля (нужны как фундамент для динамики).

---

## 13. Backlog

- IBKR интеграция (Client Portal Web API + gateway).
- Snapshots / историчность портфеля (фундамент для TWR/XIRR).
- WebSocket-стримы цен (Bybit, T-Invest).
- 2FA / multi-user UI / OAuth.
- Дивиденды, корп. действия.
- Налоговый учёт.
- On-chain кошельки (Metamask address tracking) — отдельный класс источников.
- Mobile native / PWA (текущая web-адаптация — responsive, без offline).
- BroadcastChannel logout-sync между табами.

---

## 14. Открытые мелочи (не блокеры)

- Семантика частичного отказа: уже частично реализовано через `priceStale`,
  нужен общий feedback в UI при недоступности source при on-demand sync.
- Chart library: recharts-vue / vue-chartjs / unovis — выбор откладывается
  до момента, когда фича будет нужна.
- Token rotation для брокерских аккаунтов — пока hard recreate
  (см. §19.7), API для `PUT /accounts/:id/credentials` — backlog.

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
  ассоциативные `<a>_<b>` (`instrument_external_ids`).

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
  без пагинации для small (`accounts`).
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
      oapi/                     # generated (api.gen.go, types.gen.go, ...)
    auth/                       # users + sessions, middleware, RequireAdmin
    crypto/                     # AES-GCM, HKDF
    account/                    # accounts + credentials, service + errors
    portfolio/                  # /portfolio aggregation (read-only)
    instrument/                 # canonical instruments + external_ids
    position/                   # positions service (manual CRUD)
    fx/                         # ЦБ FX fetch + lookups
    scheduler/                  # robfig/cron registration
    syncer/                     # position sync orchestration
    source/                     # position sources (tinvest, bybit, binance)
    deposits/                   # deposits CRUD
    storage/
      migrations/*.sql          # goose, embedded //go:embed
      queries/*.sql             # все sqlc inputs
      *.sql.go                  # generated
      models.go                 # generated
      db.go                     # pgxpool + storage.New(pool)
  sqlc.yaml
  go.mod
```

Миграции лежат в `internal/storage/migrations/` (а не на верхнем уровне
модуля), чтобы embedding `//go:embed` шёл прямо из storage-пакета.

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
    portfolio.sql
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
  (`go install github.com/sqlc-dev/sqlc/cmd/sqlc@v1.31.1`). Запуск через
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

---

## 19. Решения M2 (детализация)

Решения для T-Invest интеграции (вопросы 38–42). Дополняют §15 для broker-source
поддержки.

### 19.1. T-Invest клиент и source-архитектура

- **Клиент**: **REST API** (`https://invest-public-api.tinkoff.ru/rest/...`),
  написанный руками поверх `net/http` + `encoding/json`. Без официального SDK
  (он тянет grpc-стек; не используем — REST покрывает нужные методы
  + долгий cold-cache build на cold network). REST endpoint-ы покрывают все
  нужные методы (UsersService.GetAccounts, OperationsService.GetPortfolio,
  MarketDataService.GetLastPrices, InstrumentsService.GetInstrumentBy) и
  достаточны для M2.
- **Trade-off**: теряем gRPC streaming (real-time котировок) — не используем
  в M2 (отказались в M5+); теряем typed protobuf stubs — заменяем ручными
  Go-структурами под нужные нам поля (~10 типов).
- **Sub-accounts (1:1 mapping)**: один Omnifolio account = один T-Invest
  sub-account (брокерский / ИИС / премиум). При создании юзер вводит токен →
  выбирает один sub-account через UI → сохраняем `tinvestAccountId` в credentials.
- **Token type**: рекомендуем read-only invest token. Не enforce-им (T-Invest
  не возвращает права в API), но в UI/README есть warning.
- **Окружение**: только production. Sandbox не нужен (юзер хочет видеть свои
  позиции). Тесты — через httptest mock-server.
- **Расположение**: `internal/source/tinvest/` (`client.go`, `quotation.go`,
  `types.go`, `position_source.go`, `price_provider.go`, `credentials.go`,
  `mapping.go`). Параллельно `internal/source/bybit/` для M3.

**Интерфейсы** (`internal/source/source.go`):

```go
type Position struct {
    NativeInstrumentID string          // FIGI for tinvest, symbol for bybit
    Quantity           decimal.Decimal
}

type InstrumentSeed struct {
    Ticker     string
    AssetClass string
    Currency   string
    Name       string
}

type Price struct {
    Amount    decimal.Decimal
    Currency  string
    FetchedAt time.Time
}

type SubAccount struct {
    ID   string
    Name string
    Type string  // BROKER, IIS, PREMIUM
}

type PositionSource interface {
    ListSubAccounts(ctx context.Context, creds []byte) ([]SubAccount, error)
    Sync(ctx context.Context, creds []byte, subAccountID string) ([]Position, error)
    ResolveInstrument(ctx context.Context, creds []byte, nativeID string) (InstrumentSeed, error)
}

type PriceProvider interface {
    GetPrices(ctx context.Context, creds []byte, instruments []ResolvedInstrument) (map[uuid.UUID]Price, error)
}
```

`account.Service` инжектит `*source.Registry` с `Positions: map[string]PositionSource`
(key = source_type) и `Prices: map[string]PriceProvider` (key = asset_class).

### 19.2. Account creation API (для tinvest)

- **Two-step preview flow**:
  1. `POST /accounts/tinvest/preview` `{token}` → 200 `{subAccounts: [{id, name, type}]}` или 422 `{detail: "Invalid token"}`.
  2. `POST /accounts` с `oneOf` discriminated body — для tinvest:
     `{name, type:"tinvest", token, tinvestAccountId}` → 201 + Account
     с `lastSyncStatus='pending'`.
- **OpenAPI schema**: `oneOf` с `discriminator: type`:
  - `CreateManualAccountRequest`: `{name, type:"manual"}`
  - `CreateTInvestAccountRequest`: `{name, type:"tinvest", token, tinvestAccountId}`
- **Validation**: token проверяется на обоих шагах (preview + create) через
  `GetInfo()` или `GetAccounts()`. Защита от race "токен отозвали между шагами".
- **Credentials storage**: AES-GCM encrypted JSON `{"token":"...", "tinvestAccountId":"..."}`
  в `account_credentials.ciphertext`. AAD = account_id.bytes() (как в M1.2).
- **UI flow**: tab toggle в `CreateAccountDialog`:
  - Tab "Manual": поле name → submit.
  - Tab "T-Invest": шаг 1 (token) → шаг 2 (radio sub-accounts + name) → submit.
- **Sub-account display**: `account.name` (юзеровское имя из Tinkoff) +
  badge русифицированного типа (Брокерский / ИИС / Премиум).

### 19.3. Sync semantics

- **Triggers** (все три):
  1. **At account creation**: async фоновая горутина запускает первый sync
     сразу после INSERT. Account возвращается с `lastSyncStatus='pending'`.
  2. **On-demand**: `POST /accounts/:id/sync` — синхронный, ждёт результат до 30s.
     Возвращает обновлённый account.
  3. **Cron**: `0 * * * *` (каждый час). Для всех accounts с
     `source_type IN ('tinvest','bybit')`.
- **Sync logic** (применение нового снимка):
  ```sql
  BEGIN;
  -- UPSERT всех текущих позиций
  INSERT INTO positions (account_id, instrument_id, quantity)
  VALUES ... ON CONFLICT (account_id, instrument_id)
  DO UPDATE SET quantity = EXCLUDED.quantity, updated_at = now();

  -- Удалить позиции, которых больше нет
  DELETE FROM positions
  WHERE account_id = $1 AND instrument_id != ALL($2::uuid[]);

  -- Обновить статус
  UPDATE accounts
  SET last_synced_at = now(),
      last_sync_status = 'success',
      last_sync_error = NULL
  WHERE id = $1;
  COMMIT;
  ```
- **Atomicity**: одна транзакция, на error → rollback, отдельной транзакцией
  пишем `last_sync_status='failed'` + `last_sync_error`.
- **Concurrency**: `pg_try_advisory_xact_lock(hashtext('sync:'||account_id))`.
  Если lock занят — sync пропускается с `last_sync_skipped` log.
- **Inline price fetch**: после positions UPSERT собираем uniq instrument_id
  → batch `MarketDataService.GetLastPrices(figis)` → UPSERT в `prices`.
  Один sync-проход даёт и позиции, и актуальные цены.
- **Error mapping**:
  - gRPC `Unauthenticated` → status=failed, error="Токен отклонён T-Invest. Удалите и создайте аккаунт заново."
  - gRPC `ResourceExhausted` → status=failed, error="Превышен лимит запросов T-Invest. Следующий sync через час."
  - `context.DeadlineExceeded` → status=failed, error="Превышено время ожидания. Попробуйте позже."
  - default → status=failed, error="Ошибка синхронизации: <message>"

### 19.4. Instrument resolution + asset classes

- **Resolution flow** на каждую позицию из sync:
  1. SELECT `instrument_external_ids` WHERE `(source='tinvest', native_id=figi)`.
  2. Found → return `instrument_id`.
  3. Not found → gRPC `InstrumentsService.GetInstrumentBy(IdType=FIGI, id=figi)`.
  4. Map T-Invest InstrumentInfo → InstrumentSeed (см. ниже).
  5. INSERT `instruments` + INSERT `instrument_external_ids`. Race на 23505 →
     повторный SELECT.
- **Asset class mapping**:
  ```
  share + class_code in MOEX list (TQBR, TQTF, ...)  → ru_stock
  share + class_code in SPB list (SPBXM, MBSE, ...)  → us_stock
  bond                                                → ru_bond
  etf  + MOEX class_code                              → ru_etf
  etf  + SPB class_code                               → us_etf
  currency                                            → cash
  futures, option, sp                                 → SKIP (warning)
  ```
- **MOEX class codes** (hardcoded list): `TQBR, TQTF, TQOB, TQOE, TQBD, TQIF,
  EQRP_INFO, FQBR, MXBD, ...`. Расширяется по факту encounters.
- **Cash positions**: T-Invest `instrument_type='currency'` (RUB, USD, EUR на
  счёте) — track как отдельный asset_class:
  - Миграция `0003_asset_class_cash.sql`: добавить `'cash'` в CHECK constraint.
  - При первом resolve cash-instrument: ticker=RUB/USD/EUR, asset_class=cash,
    currency=RUB/USD/EUR, name="Российский рубль" / "Доллар США" / "Евро".
  - Цена = 1.00 в нативной валюте (UPSERT в `prices`). Конвертация в display
    currency через FX как обычно.
- **Skip with warning**: futures/options/sp/неизвестные types → log.Warn,
  не создаём instrument, не добавляем в positions. Sync продолжается.
  При нулевом прогрессе sync → status=success с пустым `last_sync_skipped_count`
  (counter не вводим в M2; просто warning в логи).

### 19.5. Миграции для M2

**`0003_asset_class_cash.sql`**:
```sql
-- +goose Up
ALTER TABLE instruments DROP CONSTRAINT instruments_asset_class_check;
ALTER TABLE instruments ADD CONSTRAINT instruments_asset_class_check
  CHECK (asset_class IN ('ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto','cash'));

-- +goose Down
ALTER TABLE instruments DROP CONSTRAINT instruments_asset_class_check;
ALTER TABLE instruments ADD CONSTRAINT instruments_asset_class_check
  CHECK (asset_class IN ('ru_stock','ru_bond','ru_etf','us_stock','us_etf','crypto'));
```

Других миграций в M2 не требуется — `account_credentials.ciphertext` уже есть,
`accounts.last_sync_*` тоже.

### 19.6. PriceProvider scope

- **TInvestPriceProvider** обслуживает **только** инструменты с
  `instrument_external_ids.source='tinvest'`. Manual instruments — игнорируются
  (без цен до тех пор, пока не появится подходящий provider).
- **Per-account token**: используем токен того аккаунта, для которого идёт
  sync (он в памяти расшифрован). Нет глобального системного токена.
- **Future**: M3 добавит `CoinGeckoPriceProvider` для `asset_class='crypto'` —
  no-auth public API.
- **Cash prices**: при resolve cash-instrument сразу UPSERT price=1.00. Не
  обновляется через PriceProvider (нет смысла).

### 19.7. Token rotation

В M2 — **hard recreate**: юзер удаляет аккаунт (CASCADE → positions), создаёт
заново с новым токеном. Простота важнее непотери account_id-истории.

В M5+: `PUT /accounts/:id/credentials` (только token) если будет частая нужда.

### 19.8. UI

- **CreateAccountDialog** превращается в tab-based:
  ```
  ┌─ [Manual] [T-Invest] ─────────────────────────┐
  │ T-Invest tab:                                  │
  │   Step 1: Token input → "Далее"               │
  │   Step 2: Radio sub-accounts + Name → "Создать"│
  └────────────────────────────────────────────────┘
  ```
- **AccountDetailPage** — для tinvest показывает:
  - Badge `lastSyncStatus`: pending=spinner, success="Sync 5 мин назад", failed=red icon + tooltip с error.
  - Кнопка "Синхронизировать сейчас" → POST `/accounts/:id/sync` (синхронный, ждёт).
  - На pending — auto-refetch query через 3s.
- **DashboardPage** — без изменений (positions уже из tinvest sync видны).

### 19.9. M2 acceptance

1. На `/accounts` → CreateAccountDialog → Tab "T-Invest".
2. Ввожу real T-Invest read-only token, нажимаю "Далее".
3. Backend → preview → возвращает sub-accounts.
4. Выбираю "Брокерский счёт", ввожу name "T-Invest Брокерский" → "Создать".
5. POST /accounts → 201 с `lastSyncStatus='pending'`. Async sync стартует.
6. UI на странице аккаунта показывает spinner.
7. Через несколько секунд → status=success, появляются позиции (SBER, GAZP, USD на счёте, и т.д.).
8. Дашборд показывает реальные позиции с актуальными ценами. Total в RUB конвертится в USD/EUR через FX.
9. Нажимаю "Sync now" → синхронный refresh.
10. Cron каждый час обновляет в фоне.
11. Если токен реджектнут (тест: меняю в БД ciphertext на мусор) → next sync → status=failed + visible error.
12. Удаляю аккаунт → CASCADE удаляет positions + credentials.

### 19.10. Что НЕ в M2

- US акции через other US-источник (Finnhub/Yahoo) — по дизайну отложено.
  Если позиция US-stock пришла из T-Invest через СПБ — провайдер цены —
  T-Invest. Если manual US-stock — без цены.
- Параллелизм sync (errgroup) — sequential в M2.
- Update credentials endpoint — hard recreate в M2.
- WebSocket стримы цен — M5+.
- Опции/фьючерсы/структурки — skip с warning.
- last_sync_skipped_count counter — на M2 нет колонки.
- Sandbox toggle — нет.

---

## 20. Реализованные изменения после M1-решений

§15–§19 — слепки решений на момент интервью по соответствующим
milestone-ам. Кодовая база с тех пор разошлась с ними в нескольких
точках; ниже — сводка отклонений (актуальная истина выражена в §1–§14).

### 20.1. Архитектура

- **Cron вынесен в отдельный бинарь** `apps/cron`. В §4.2 предполагалось,
  что цены подтягиваются «лениво» внутри API при HTTP-запросе на
  `/portfolio` — это заменено на push-модель: бинарь пушит инструменты,
  цены и FX через `/admin/*` эндпоинты. API при `/portfolio` ничего
  снаружи не дёргает.
- **Admin-эндпоинты `/admin/instruments`, `/admin/prices`, `/admin/fx`**
  добавлены в `internal/server/admin.go`, защищены Bearer
  `ADMIN_API_KEY`. В публичный OpenAPI они нарочно не входят.
- **Отдельные пакеты `internal/scheduler`, `internal/syncer`** —
  scheduler регистрирует robfig/cron-задачи (sessions cleanup, FX
  refresh, hourly sync), syncer — сам флоу sync поверх PositionSource.

### 20.2. Источники

- **Binance** добавлен наравне с Bybit (`internal/source/binance/`,
  миграция 0005 — расширила CHECK на `source_type`). В исходных
  M3-решениях Binance не упоминался.
- **CoinGecko не используется**. Цены крипты тянутся из Bybit public
  market data в рамках того же бинаря `apps/cron`.
- **Finnhub** добавлен как поставщик цен для `us_stock`/`us_etf` (в
  §2.2 он был помечен «отложено»; теперь подключён в `apps/cron`).
- **Manual — не source-пакет**. Ручные позиции пишутся напрямую в
  `positions` через `position` service + соответствующий REST-флоу.
  В §15.5 под него не выделено директории, и так и осталось.

### 20.3. Схема и API

- **Таблицы `portfolios` / `portfolio_accounts` удалены** миграцией
  0004. Идея именованных портфелей (выпала из роадмапа) откладывается
  до явной потребности; агрегация — всегда «всё по юзеру».
- **Deposits фича** — новая таблица + эндпоинты `GET/POST /deposits`,
  `DELETE /deposits/{id}`. В §3.5 / §15 её не было; добавлена как
  фундамент под расчёт доходности (см. §3.6).
- **Schema CreateAccountRequest** — единый объект с conditional required
  по `type` (`token`+`tinvestAccountId` для tinvest, `apiKey`+`apiSecret`
  для bybit/binance). Discriminated `oneOf` (как предлагалось в §19.2)
  не используется — упрощает orval и UI-форму.
- **`asset_class='cash'`** — добавлен миграцией 0003; описан в §19.4
  и теперь действительно реализован.

### 20.4. Frontend

- **shadcn-vue CLI не используется**. Вместо неё — собственные тонкие
  обёртки в `components/ui/` поверх radix-vue (см. §8). Перечень текущих
  примитивов: `button, card, checkbox, confirm, dialog, input, label,
  table`.
- **Routes расширены**: добавлены `/deposits`, `/instruments`,
  `/settings`. В §15.7 / §18.6 был только базовый набор `/`,
  `/accounts`, `/accounts/:id`, `/login`.
- **`@vueuse/core` принят в стек**. Появилась конвенция: перед ручной
  обёрткой над `ref/watch/DOM/Storage` сначала проверить готовый
  composable. Каталог соответствий — `docs/vueuse.md`.
- **Mobile**: реализована responsive-адаптация (off-canvas sidebar,
  card-layout таблиц на мобильных, snapshot dashboard без sticky).
  В исходном плане раздел про мобилу отсутствовал.
- **Charts**: ни одна chart-библиотека не подключена — фича
  оставлена в открытых вопросах §14.

### 20.5. Деплой

- **Railway вместо VPS+Caddy** (см. §10). `compose.prod.yml` и
  `Caddyfile` (упоминаемые в §9 / §10 первоначального дизайна) не
  созданы и не нужны.
- **Backups** — managed Railway Postgres плагин; собственный
  `pg_dump`-cron, заявленный в M5, не реализован (и не планируется).
