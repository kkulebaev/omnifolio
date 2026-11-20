# Omnifolio — Design Document

Приложение для учёта личных инвестиций. Агрегирует портфели из разных источников
(российские бумаги через T-Invest, крипта через Bybit и Binance, остальное
вручную) и показывает их по отдельности и в сумме.

Документ описывает архитектуру v0.1 — текущее состояние кодовой базы.
История milestone-решений сюда больше не входит — она в `git log`.

---

## 1. Цель и scope

**Главная задача**: снапшот текущей стоимости и распределения активов
плюс ручной журнал пополнений/выводов и расчёт доходности портфеля поверх него.

- Что показываем: сколько всего сейчас, в каких активах, в каких валютах,
  динамику суммарной стоимости портфеля (daily snapshots) с линией «вложено»,
  журнал deposits (пополнения и выводы с датой и валютой) и доходность
  портфеля целиком: вложено (нетто), P&L, XIRR, TWR (см. §3.10).
- Что НЕ показываем (отложено): журнал сделок, P&L по позициям, налоговый
  учёт, ребалансировка, дивиденды, корпоративные действия.

Решение строит модель данных вокруг текущего состояния (positions snapshot),
а не вокруг истории транзакций. Deposits добавлены отдельной таблицей —
это единственный счётчик «вложенных денег», независимый от позиций.
Динамика во времени берётся из ежедневных снимков агрегата `/portfolio`
(`portfolio_snapshots`, см. §3.7), а не из реконструкции по транзакциям.

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

**Personal instruments**. С миграции 0008 у `instruments` есть nullable
`user_id`. Строки с `user_id IS NULL` — глобальный канонический каталог
(биржевые инструменты + cash), пишется только cron-ом / admin-эндпоинтами.
Строки с `user_id IS NOT NULL` — личные активы (квартиры, машины, прочее
имущество), полные CRUD-права у владельца. Двойственность жёсткая на уровне
БД: CHECK `instruments_scope_class_check` гарантирует
`(user_id IS NULL ↔ asset_class ∈ {ru_*, us_*, crypto, cash})` и
`(user_id IS NOT NULL ↔ asset_class ∈ {real_estate, vehicle, other_asset})`.
Уникальность тоже партиционирована — две partial unique indexes: для
глобальных `(LOWER(ticker), asset_class)`, для личных
`(user_id, LOWER(ticker), asset_class)` — личный «AAPL» у юзера не
конфликтует с глобальным AAPL.

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

instruments              (id, user_id NULL, ticker, asset_class, currency, name,
                          created_at, updated_at)
                         -- asset_class IN ('ru_stock','ru_bond','ru_etf',
                         --                 'us_stock','us_etf','crypto','cash',
                         --                 'real_estate','vehicle','other_asset')
                         -- CHECK (user_id IS NULL  ↔ asset_class биржевые/cash)
                         -- CHECK (user_id NOT NULL ↔ asset_class manual)
                         -- UNIQUE (LOWER(ticker), asset_class)        WHERE user_id IS NULL
                         -- UNIQUE (user_id, LOWER(ticker), asset_class) WHERE user_id IS NOT NULL
instrument_external_ids  (source, native_id, instrument_id,
                          PRIMARY KEY(source, native_id))

positions  (account_id, instrument_id, quantity NUMERIC(38,18),
            created_at, updated_at,
            PRIMARY KEY(account_id, instrument_id))
prices     (instrument_id PK, price NUMERIC(20,8), fetched_at)
fx_rates   (date, from_ccy, to_ccy, rate NUMERIC(20,10),
            PRIMARY KEY(date, from_ccy, to_ccy))

deposits   (id, user_id, date DATE, kind, currency, amount NUMERIC(20,2),
            created_at, updated_at)
           -- kind IN ('deposit','withdrawal'), currency ~ '^[A-Z0-9]{3,10}$'
           -- amount > 0 (направление — в kind)

portfolio_snapshots (user_id, snapshot_date DATE, display_currency,
                     grand_total NUMERIC(24,8),
                     by_asset_class JSONB, by_currency JSONB, by_account JSONB,
                     created_at,
                     PRIMARY KEY(user_id, snapshot_date))
                    -- by_currency хранит НАТИВНЫЕ суммы по валютам;
                    --   grand_total / by_asset_class / by_account — в display_currency снимка
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
  делает идемпотентным admin-seed (cron-flow). С миграции 0008 этот
  индекс заменён на два partial: глобальный и per-user (см. §3.2).
- `cash` как asset_class добавлен миграцией 0003. Личные активы
  (`real_estate`, `vehicle`, `other_asset`) добавлены миграцией 0008
  (вместе с `user_id` и двумя CHECK).

### 3.6. Deposits (журнал пополнений и выводов)

Отдельная сущность — внешние денежные потоки пользователя: сколько, когда и
в какой валюте он завёл в портфель или вывел из него. Это вход для расчёта
доходности (§3.10) и линии «вложено» на графике истории. Таблица и путь API
по историческим причинам называются `deposits`, хотя хранят и выводы.

- `date` — точная дата операции (миграция 0009 переименовала `month` →
  `date`; старые записи остались на 1-м числе месяца). Дата не может быть
  в будущем больше чем на 1 день — защита от опечатки в годе.
- `kind` — `'deposit' | 'withdrawal'` (VARCHAR + CHECK, §3.8).
  `amount` всегда > 0, направление потока — только в `kind`.
- `currency` — валюта операции (default `RUB`), `amount` —
  `NUMERIC(20, 2)`: у USD/EUR бывают центы.
- Для расчётов каждый поток конвертируется в целевую валюту по курсу ЦБ
  **на дату потока** (`fx.GetRateAt`): последний курс с датой ≤ даты
  потока; если такого нет — ближайший более поздний, и ответ помечается
  `fxApproximated`.
- Редактирование разрешено (`PUT /deposits/{id}`); чужой/несуществующий
  id → 404. Удаление — hard, без soft-delete.
- На фронте итоги журнала считаются только по валютам отдельно — без
  смешивания валют: конвертация по историческим курсам есть только на бэке.
- Down-миграция 0009 lossy: удаляет выводы, сворачивает даты к 1-му числу,
  округляет суммы и теряет валюту.

### 3.7. Portfolio snapshots

Ежедневный снимок агрегата `/portfolio` — фундамент под графики динамики
стоимости портфеля. Не подменяет deposits / future trades log: это
другая модель — daily snapshot **текущего** состояния, не история сделок.

Таблица `portfolio_snapshots`:

- **PK** — composite `(user_id, snapshot_date)` (по образцу `fx_rates`,
  не surrogate UUID — натуральный time-series ключ).
- `snapshot_date` — UTC `CURRENT_DATE` на момент запуска cron.
- `display_currency` — что было у юзера в `users.display_currency` на
  момент снимка. При смене валюты исторические точки **остаются в старой**;
  фронт показывает их as-stored (см. §6 contract).
- `grand_total` / `by_asset_class` / `by_account` — в `display_currency`
  снимка (готовы к чтению без пересчёта).
- `by_currency` — НАТИВНЫЕ суммы по валютам (`USD: 100`, `RUB: 5000`).
  Это «сырьё» для пересчёта истории в другую валюту через исторические
  `fx_rates` — так считает TWR расчёт доходности (§3.10).
- UPSERT через `ON CONFLICT (user_id, snapshot_date) DO UPDATE` —
  последний запуск выигрывает (полезно для повторных run-ов в течение дня
  после fix-ов в Compute).
- Skip-правила: если у юзера нет позиций или **все** позиции stale
  (`grand_total = 0` при непустом `Positions`) — snapshot не пишется
  (только log.warn). Частичная stale (одна-две позиции из 20) — пишем
  как есть; на графике точка слегка просядет.

### 3.8. Конвенции БД

- **PK**: `UUIDv7` через `uuid.NewV7()` в Go. Postgres-side
  `gen_random_uuid()` (v4) не используется.
- **Timestamps**: `TIMESTAMPTZ`, UTC. `created_at` + `updated_at` на всех
  бизнес-таблицах; `updated_at` обновляется триггером
  `trigger_set_updated_at()`.
- **Удаление**: hard delete, FK `ON DELETE CASCADE`. Soft delete не
  используем.
- **Naming**: snake_case, plural таблицы (`accounts`, `positions`),
  ассоциативные `<a>_<b>` (`instrument_external_ids`).
- **Enum-поля**: `VARCHAR + CHECK constraint`, не Postgres `ENUM` (проще
  ALTER при добавлении значения — см. миграции 0003, 0005).
- **Positions PK**: composite `(account_id, instrument_id)`.
- **Sessions PK**: `token_hash BYTEA(32)` — SHA-256 от 32 random bytes
  (cookie несёт base64url plaintext; в БД — только хеш).

### 3.9. Lifecycle позиций и инструментов

- **Position quantity** > 0 обязательно; удаление — DELETE, не
  `quantity=0`. Шорты (negative) — out of scope.
- **POST positions** в существующий `(account_id, instrumentId)` → 409
  (use PUT). Без accumulate / overwrite.
- **Instruments — глобальные** (`user_id IS NULL`): канонический
  справочник без явного владельца. Любой залогиненный юзер триггерит
  admin/cron seed. DELETE через API нет — append-only.
- **Instruments — личные** (`user_id IS NOT NULL`, миграция 0008):
  актив, которым владеет конкретный юзер (`real_estate` / `vehicle` /
  `other_asset`). Полный CRUD для владельца через `/instruments`
  эндпоинты. DELETE возвращает 409 при активных позициях
  (FK `positions.instrument_id ON DELETE RESTRICT`). Currency и
  asset_class неизменяемы у личного инструмента — смена через
  delete + recreate, чтобы не ломать историю.
- **Authorization** (defense-in-depth): service-функции принимают
  `userID` первым параметром; repository-запросы фильтруют
  `WHERE user_id = $1`. На чужой ресурс — 404, не 403 (не палим
  существование).

### 3.10. Расчёт доходности

`GET /portfolio/returns?currency=XXX` (пакет `internal/returns`) считает
доходность портфеля целиком в одной целевой валюте (default —
`users.display_currency`). Чистая математика (`XIRR`, `TWR`,
`Annualize`) отделена от сервиса и покрыта unit-тестами; деньги на входе и
выходе сервиса — `decimal`, `float64` только внутри численных методов.

Входы:

- **Текущая стоимость** — `grand_total` из `portfolio.Compute` в целевой
  валюте.
- **Потоки** — все записи deposits, каждая сконвертирована по курсу на свою
  дату (§3.6).
- **Точки стоимости для TWR** — все `portfolio_snapshots` пользователя.
  Значение точки пересчитывается из `by_currency` (нативные суммы, §3.7)
  по курсу на `snapshot_date`, а не берётся из `grand_total`: так история
  остаётся сопоставимой после смены display currency. Валюта, для которой
  курса нет ни на какую дату, выпадает из точки (как и из `grand_total` в
  `Compute`). После всех снапшотов **всегда** добавляется финальная
  live-точка «сегодня» = текущая стоимость — даже если снапшот за сегодня уже
  есть (утренний снапшот не содержит дневных взносов).
- Даты потоков обрезаются сверху до сегодня (UTC): валидация допускает
  завтрашнюю дату, и без обрезки такой поток выпал бы из TWR и линии
  «вложено».

Метрики:

- `netInvested = Σ deposits − Σ withdrawals`, `profit = currentValue −
  netInvested`, `profitPct = profit / netInvested` (null при
  `netInvested ≤ 0`).
- **XIRR** — денежно-взвешенная годовая доходность. Соглашение инвестора:
  пополнение — отрицательный поток, вывод — положительный, финальный поток —
  `+currentValue` на сегодня. Решается `Σ aᵢ / (1+r)^(tᵢ/365) = 0`
  (ACT/365) методом Ньютона с fallback на бисекцию в `[−0.9999, 100]`.
  null, если в потоках нет смены знака, решатель не сошёлся или с первого
  потока прошло меньше **30 дней** (годовая ставка на коротком отрезке
  вводит в заблуждение). Верхняя граница бисекции 1e6 — чтобы волатильный
  месяц (+50% за 30 дней ≈ +13 800% годовых) не превращался в null.
- Причина null отдаётся в `xirrReason` / `twrReason`: `no_flows`,
  `insufficient_history`, `no_sign_change`, `no_convergence`,
  `no_snapshots`, `no_measurable_period`; фронт показывает её подсказкой.
- **TWR** — время-взвешенная доходность, нейтральная к размеру и моменту
  взносов. Под-периоды между соседними точками; `rᵢ = (Vᵢ − Fᵢ) / Vᵢ₋₁ − 1`,
  где `Fᵢ` — нетто-взнос под-периода; итог `Π(1+rᵢ) − 1`.
  Снапшот пишется в 09:00 UTC, а у потока есть только дата (и зачисление у
  брокера часто T+1), поэтому поток с датой `d` считается **отсутствующим**
  в `V_d`: он относится к под-периоду, который заканчивается первой точкой
  с датой **строго больше** `d`. Поток в день первого снапшота не входит в
  V₀ и уходит в первый под-период; live-точка забирает все потоки с датой ≥
  даты последнего снапшота. Потоки до первого снапшота не участвуют — TWR
  стартует с него (`twrFrom`). Под-период пропускается, если
  `Vᵢ₋₁ ≤ 0` или `Vᵢ − Fᵢ ≤ 0`, чтобы произведение не меняло знак.
- **Аннуализация TWR** — `(1+twr)^(365/days) − 1`, только при
  `days ≥ 365`; на более коротком периоде `twrAnnualized = null`.
- `fxApproximated = true`, если хотя бы один поток или снапшот
  сконвертирован «ближайшим» (более поздним) курсом.
- Если для потока нет курса вообще ни на какую дату — 422
  `FX rate unavailable for XXX→YYY` (крайний случай: ЦБ публикует все
  основные валюты). Для линии «вложено» в `/portfolio/history` такой поток
  пропускается (с `slog.Warn`), чтобы не ломать график, а ответ помечается
  `netInvestedPartial = true` — фронт выводит сноску.
- `fx.GetRateAt` перебирает маршруты direct → inverse → через RUB и
  предпочитает тот, где курс есть на дату; «ближайший более поздний»
  используется, только если точного нет ни на одном маршруте.

`/portfolio/history` отдаёт в каждой точке `netInvested` — накопленный
нетто-поток с датой строго раньше даты точки (то же соглашение, что у TWR:
снапшот дня d ещё не содержит потоков дня d), в `displayCurrency` этой
точки, каждый поток по курсу на свою дату. Фронт рисует его пунктиром поверх линии
стоимости.

---

## 4. Синхронизация

Реализация разделена между двумя процессами:

- **`apps/api`** — внутренний `robfig/cron/v3` scheduler в `internal/scheduler/`,
  который раз в час пробегает по всем брокерским аккаунтам и подтягивает
  свежие позиции (плюс служебный sessions cleanup, daily FX refetch и
  daily portfolio snapshot — см. §4.4).
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

**Cron трогает только глобальные инструменты.** Admin-эндпоинт
`POST /admin/prices` использует SQL-предикат `WHERE user_id IS NULL`
внутри UPSERT (см. `UpsertGlobalPrice`) — попытка обновить цену
личного инструмента вернёт `failed` без побочного эффекта. CHECK
`instruments_scope_class_check` дополнительно гарантирует, что cron
физически не может создать строку с `user_id IS NOT NULL`. Это
структурный инвариант, а не service-layer convention.

**Cash и личные активы никогда не помечаются stale**: цены на них
авторитативны (по определению — для cash; и пользовательски заданные —
для `real_estate`/`vehicle`/`other_asset`), таймстамп `fetched_at`
не несёт сигнал о свежести.

### 4.3. FX курсы

- Daily — внутренним scheduler-ом API (`0 6 * * *`), плюс ещё раз через
  тот же `apps/cron` (overlap намеренный — гарантия свежих курсов
  независимо от того, какой из процессов жив).

### 4.4. Daily portfolio snapshot

Раз в день внутренний scheduler API обходит всех юзеров и пишет в
`portfolio_snapshots` агрегированный снимок их `/portfolio` (см. §3.7).

- Spec: `0 9 * * *` UTC (полдень MSK). Слот выбран после того как
  `apps/cron` на Railway уже отстрелял prices/fx и hourly position-sync
  гарантированно прошёл.
- Реализация: пакет `internal/snapshot/`. `Service.RunDaily(ctx)` —
  список user-id из `users` (отдельный sqlc-query `ListUserIDs :many`)
  → для каждого `RunForUser` → `portfolio.Compute(...)` →
  `UpsertPortfolioSnapshot`.
- Per-user error: `log.Error + continue`, в конце Job —
  `errors.Join(...)` (один битый юзер не валит остальных).
- Без транзакций — одиночный INSERT на юзера; `Compute` читает
  многотабличные rows как и `/portfolio` handler.
- Backfill ретроспективно невозможен (`positions` хранит только текущее
  состояние, без timeline). Первая точка для существующего юзера
  появляется в ближайший cron run или через ручной триггер
  `POST /admin/snapshots/run` (Bearer `ADMIN_API_KEY`, тот же что у
  `/admin/fx`) — последний выполняет `RunDaily` синхронно. Endpoint
  пригодится и для re-runs после фикса в `Compute`: `ON CONFLICT DO
  UPDATE` на `(user_id, snapshot_date)` делает повторные вызовы
  идемпотентными в рамках UTC-дня.
- Retention — вечно. Объём (~90KB/год/юзер) пренебрежим; downsampling
  и TTL — отложено до явной потребности.
- Snapshot хранит только агрегаты — `grand_total`, `by_asset_class`
  (keyed by class string), `by_currency` (by currency string),
  `by_account` (by account UUID). **Instrument_id нигде не хранится**.
  Удаление личного инструмента (миграция 0008) не оставляет dangling-
  ссылок в исторических снапшотах — суммы уже посчитаны и заморожены.

---

## 5. Auth и секреты

### 5.1. User auth

- **Password hash**: argon2id через `alexedwards/argon2id`, PHC-формат
  `$argon2id$v=19$m=65536,t=1,p=4$<salt>$<hash>` (параметры в строке —
  upgrade-path при login). Salt 16 bytes.
- **Session cookie** `sid`: HttpOnly, SameSite=Lax, Path=/, Secure
  conditionally в prod. Idle timeout 30 минут (`last_seen_at`
  coalescing раз в минуту), absolute 30 дней.
- **Session token**: 32 random bytes из `crypto/rand` → base64url в
  cookie; в `sessions.token_hash` — SHA-256.
- **Bootstrap**: при пустой `users` и заданных
  `BOOTSTRAP_USER_EMAIL`/`BOOTSTRAP_USER_PASSWORD` создаётся первый
  юзер. `/auth/register` нет.
- **Login errors**: всегда generic «Неверный email или пароль» + dummy
  argon2 hash при несуществующем email — защита от timing-атак.
- **CSRF**: не используется. Same-origin SPA + `SameSite=Lax` +
  Origin-проверка на state-changing запросах.
- Single-user, но схема/middleware готовы под multi-user (везде `user_id`).
- Без 2FA, OAuth, magic links, refresh tokens на MVP.

### 5.2. Broker secrets

- **AES-256-GCM** в `account_credentials.ciphertext`. Nonce 12 bytes
  случайный, **AAD = `account_id.bytes()`** — защита от swap-атаки в
  дампе БД (нельзя расшифровать чужие credentials под видом своих).
- **Master key** `MASTER_KEY` из env, 32 bytes в `base64.RawURLEncoding`
  (43 char). Валидируется на старте — приложение падает, если ключ
  невалиден.
- **Domain separation через HKDF**:
  `credentialsKey := hkdfExpand(masterKey, "credentials.v1", 32)`.
  Будущие назначения (signing и пр.) — через свои labels.
- `account_credentials.key_version INT` под будущую ротацию.
- Дамп БД без `MASTER_KEY` токены не раскрывает.

### 5.3. Admin auth

- `Authorization: Bearer ${ADMIN_API_KEY}` — middleware
  `auth.RequireAdmin` на `/admin/*` (см. §6). Используется только из
  `apps/cron`; человеческого UI нет.

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
- **portfolio**: `GET /portfolio?currency=...`,
  `GET /portfolio/history?from=&to=` (опциональны, default — последние
  90 дней; ответ — массив daily snapshots с `displayCurrency` и
  `netInvested` per точка плюс верхнеуровневый `currentDisplayCurrency`),
  `GET /portfolio/returns?currency=...` (вложено, P&L, XIRR, TWR — §3.10).
- **deposits**: `GET/POST /deposits` (список — `date DESC, createdAt DESC`),
  `PUT/DELETE /deposits/{id}`.
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

### 6.1. Конвенции

- **Errors**: RFC 7807 Problem Details (`application/problem+json`),
  поля `type/title/status/detail/instance`, расширение `fields` для
  валидации.
- **JSON naming**: camelCase (`createdAt`, `userId`).
- **Envelope**: голый ресурс для single (`GET /accounts/:id` →
  `{id, name, ...}`), `{items: [...], total, nextCursor}` для list.
- **Pagination**: cursor-based для potentially-large (`positions`,
  `prices`); без пагинации для small (`accounts`).
- **IDs**: UUID-string везде.
- **Validation**:
  - Shape — OpenAPI middleware (`OapiRequestValidator`) на старте chain.
  - Семантика — sentinel-errors в service layer → mapper в `problem.go`.
- **Status codes**: 200/201/204 success; 400 syntactic; 401 unauth;
  404 для not-found и для чужих ресурсов; 409 conflict; 422 семантическая
  валидация (с `fields`); 500 internal.
- **Two-step instrument flow**: `GET /instruments/search` → если не
  нашёл, `POST /instruments` (идемпотентен) → дальше создаём position.

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
| Component primitives | reka-ui (headless) + собственные обёртки в `components/ui/` |
| Icons | `@lucide/vue` |
| State (client) | Pinia (v4) + `pinia-plugin-persistedstate` |
| State (server) | TanStack Query (vue-query v5) |
| Composables | `@vueuse/core` (см. §8.2 и `docs/vueuse.md`) |
| Routing | vue-router (history mode) |
| API client | orval из OpenAPI (`vue-query` client, custom fetch mutator) |
| Forms | ручная валидация через `ref` + `validate()` в компоненте |
| Toasts | vue-sonner |
| Tests | Vitest Browser Mode (Chromium через `@vitest/browser-playwright`), unit + component. E2E на Playwright **отложен**. |
| Package manager | pnpm |
| TS | strict + noUncheckedIndexedAccess |

Разделение state:
- **Server-state** (списки, дашборд, цены) — TanStack Query.
- **Client-state** (выбранная валюта, UI-флаги, тема) — Pinia.

UI-примитивы — небольшая собственная библиотека (`button`, `card`,
`checkbox`, `confirm`, `dialog`, `input`, `label`, `table`) поверх
reka-ui. Простые компоненты добавляем как тонкие обёртки вручную;
для сложных (например `chart`, который тянет `@unovis/ts` и `@unovis/vue`)
допускаем `npx shadcn-vue@latest add <name>` — он подтягивает нужные
зависимости и shadcn-style обёртки в `components/ui/`. Полный
shadcn-CLI-режим не включаем: компоненты добавляются по факту, а не
оптом.

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
        snapshot/               # daily portfolio snapshot job
        deposits/               # deposits CRUD (пополнения и выводы)
        returns/                # XIRR / TWR + /portfolio/returns
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

## 12. Backlog

В работе / ближайшее:

- Дополнительные графики поверх существующих `portfolio_snapshots`:
  stacked-area `by_asset_class` / `by_currency`, разрез по аккаунтам.
  Сейчас на dashboard рисуется только line `grand_total`.

Дальше / без приоритета:

- IBKR (Client Portal Web API + gateway).
- WebSocket-стримы цен (Bybit, T-Invest).
- Token rotation для брокерских аккаунтов: `PUT /accounts/:id/credentials`
  (сейчас — hard recreate).
- 2FA / multi-user UI / OAuth.
- Дивиденды, корп. действия. Налоговый учёт.
- On-chain кошельки (Metamask address tracking) — отдельный класс источников.
- Mobile native / PWA (сейчас — responsive web, без offline).
- BroadcastChannel logout-sync между табами.
- Общий feedback в UI при on-demand sync failure (поверх существующего
  `priceStale`).
