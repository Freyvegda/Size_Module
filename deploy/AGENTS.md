# deploy/ — local infrastructure

Local-development infrastructure only. The Go API and the React app run on the
host (`backend/`, `frontend/`); compose runs just the database services.

`compose/docker-compose.yml` (project name `size-module`):

| Service | Image | Host port | Notes |
|---|---|---|---|
| `postgres` | `postgres:16-alpine` | `${POSTGRES_PORT:-5433}` → 5432 | volume `pgdata`, `pg_isready` healthcheck |
| `adminer` | `adminer:4` | `${ADMINER_PORT:-8081}` → 8080 | depends on healthy postgres |

- Copy `compose/.env.example` to `compose/.env` to override credentials/ports.
- The PostgreSQL host port defaults to **5433** because a native install often
  owns 5432. Keep `POSTGRES_PORT` here in sync with `DATABASE_URL` in `db/.env`.
- Start/stop through `db/scripts/up.ps1` / `down.ps1`, not raw compose, so the
  right env file is picked up.
- There is intentionally no API/frontend Dockerfile yet (single-plant
  self-hosted deployment comes later — see `plan.txt`).
