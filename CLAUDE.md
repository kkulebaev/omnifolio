# Omnifolio

Личный трекер инвестиций: агрегирует портфели из разных источников (российские
бумаги через T-Invest, крипта через Bybit, остальное вручную) и показывает их
по аккаунтам и в сумме с конвертацией в выбранную валюту отображения.

Полная архитектура и roadmap — `docs/design.md`.
Операционка по Railway — `docs/railway.md`.

## Стек

- **Backend**: Go 1.26 (chi, sqlc, pgx, goose, argon2id, AES-GCM, robfig/cron, slog).
- **Frontend**: Vue 3 + Vite + Pinia + vue-router + TanStack Query + orval + Tailwind v4 + radix-vue + vee-validate/zod.
- **DB**: Postgres 16.
- **Контракт**: OpenAPI как single source of truth — `api/openapi.yaml`.
- **Deploy**: Railway (services: `api`, `web`, `cron`, managed `postgres`), auto-deploy с `main`.

## Структура монорепы

```
api/openapi.yaml          # единственный источник правды для контракта
apps/
  web/                    # Vue 3 SPA (pnpm workspace)
    src/api/generated/    # orval — НЕ редактировать руками
    src/features/<f>/     # фичи: auth, dashboard, account, instrument, settings
    src/components/ui/    # shadcn-vue-style примитивы (button, card, dialog, …)
    src/stores/           # pinia (auth, ui)
    src/lib/              # utils, formatters
  api/                    # Go API
    cmd/api/              # entrypoint
    internal/
      config/ storage/{migrations,queries}/ server/{,oapi}/
      scheduler/ crypto/ auth/ account/ instrument/ position/
      fx/ portfolio/ source/{tinvest,bybit}/ syncer/
  cron/                   # отдельный Go-бинарь — daily refresh цен/FX, exit при завершении
docs/                     # design.md, railway.md
compose.yml Makefile pnpm-workspace.yaml
```

`apps/web` — единственный пакет в pnpm workspace. `apps/api` и `apps/cron` —
независимые Go-модули.

## Локальный запуск

```sh
make dev          # postgres + api в docker compose + web (vite, foreground)
make services     # только бэкенд в фоне; make services-down — выключить
make logs         # хвост логов compose
make test         # go test ./... + vitest --run
make generate     # sqlc + oapi-codegen + orval
```

Порты: API `:8080`, Web `:5173` (proxy `/api` → `localhost:8080`), Postgres `:5432`.
Bootstrap-юзер берётся из `BOOTSTRAP_USER_EMAIL` / `BOOTSTRAP_USER_PASSWORD`
(см. `compose.yml`).

В `apps/web` импорты идут через alias `@` → `src` (vite + tsconfig).

## Codegen workflow

OpenAPI и SQL-запросы — единственные источники для сгенерированного кода:

- `api/openapi.yaml` → `apps/api/internal/server/oapi/*` (oapi-codegen) и
  `apps/web/src/api/generated/*` (orval, vue-query клиент через mutator
  `apps/web/src/api/mutator.ts`).
- `apps/api/internal/storage/queries/*.sql` → sqlc.

После правок в `openapi.yaml` или `queries/*.sql` обязательно `make generate`
и закоммитить регенерированные файлы. Сгенерированные файлы редактировать руками нельзя.

`pnpm --filter web build` и `pnpm --filter web typecheck` уже включают
`orval` как пред-шаг — отдельный `generate` для них не нужен.

## Frontend conventions

Tailwind, формы, темизация и прочие соглашения по фронту — `docs/design.md` §8.1.
Перед тем как писать ручную обёртку над `ref`/`watch`/DOM/Storage — проверить готовый композабл из `@vueuse/core`. Каталог соответствий и правила использования — `docs/vueuse.md`.
