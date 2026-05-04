# Railway operations

Project: **omnifolio** (env `production`). All app services deploy from `kkulebaev/omnifolio` `main` branch. High-level deploy intent (env vars, watch-pattern policy, dockerfile paths) lives in [`README.md`](../README.md#production-railway); this file is the operational reference.

## Services

| Name             | Service ID                              | Notes                                      |
| ---------------- | --------------------------------------- | ------------------------------------------ |
| `omnifolio`      | `fdd67108-a289-4b90-8792-ebaa28baccfe`  | Vue web. URL: https://omnifolio.up.railway.app |
| `omnifolio-api`  | `b99b1fa9-8c8f-4487-b5cc-06d3db16e9ba`  | Go HTTP API.                               |
| `omnifolio-cron` | `3ece9033-8bc4-4afe-891f-0e4e5f5c6d9a`  | Go one-shot cron worker, replicas 0/1.     |
| `Postgres`       | `3bbc085d-4d87-46a7-96ad-eb86bb120b75`  | Managed Postgres-SSL 18, volume `postgres-volume`. |

All app services build **from the repo root** (Root Directory not set) using `apps/<svc>/Dockerfile`. Each Dockerfile copies only its own `apps/<svc>/` subtree, so builds are self-contained — no shared Go modules between api/cron, and watch patterns can be scoped to `apps/<svc>/**`.

## Watch patterns (current state)

| Service          | Configured on Railway                                                                |
| ---------------- | ------------------------------------------------------------------------------------ |
| `omnifolio`      | `["apps/web/**", "api/**", "pnpm-lock.yaml", "pnpm-workspace.yaml", "package.json"]` |
| `omnifolio-api`  | `["apps/api/**", "api/**"]`                                                          |
| `omnifolio-cron` | `["apps/cron/**"]`                                                                   |

Patterns match the intent documented in [`README.md`](../README.md#production-railway).

## Managing service settings

The Railway CLI doesn't expose source-settings flags directly. Use the Railway Agent, which has direct access to update service config:

```sh
railway agent -s <service-name-or-id> -p "<request>"
```

Example — read current config:

```sh
railway agent -s omnifolio-cron -p "Show Source settings: Root Directory, Watch Patterns, Config-as-code path, branch."
```

Example — set watch patterns:

```sh
railway agent -s omnifolio-cron -p "Set Watch Patterns to apps/cron/** and read back to confirm."
```

Add `--json` for scriptable output, `--thread-id <id>` to continue a previous session.
