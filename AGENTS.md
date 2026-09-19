# Size Module — agent guide

Material optimization platform: it decides how to divide stock material into
products with the least waste, and keeps the leftovers in circulation. The first
vertical slice is a **cut optimizer** — give it parts + stock (sheets, panels,
bars) and it returns a validated cutting plan with 2D/3D visualisation, waste
breakdown and shop-floor cut steps.

## Repo map

| Path | What it is | Read first |
|---|---|---|
| `backend/` | Go module `github.com/size-module/backend`: REST API + pure-Go optimizer engine | `backend/AGENTS.md` |
| `db/` | PostgreSQL schema (goose), sqlc queries, seeds, smoke tests, PowerShell scripts | `db/AGENTS.md` |
| `frontend/` | React 19 + TS + Vite + Redux Toolkit + Tailwind/shadcn + three.js SPA | `frontend/AGENTS.md` |
| `deploy/` | docker-compose: local PostgreSQL (:5433) and Adminer (:8081) only | `deploy/AGENTS.md` |
| `docs/` | Domain glossary and solver notes | `docs/AGENTS.md` |
| `graphify-out/` | Generated knowledge graph: `graph.json`, `GRAPH_REPORT.md`, `graph.html` | — |
| `.venv/` | Project-local Python venv with Graphify — tooling only, not part of the app | — |

## Commands

Backend (`backend/`):
- `go run ./cmd/cutoptics` — API on :8080 (works without PostgreSQL)
- `go test ./...` — solver tests + golden benchmark gate
- `go run ./cmd/cutoptics bench -dir testdata/benchmarks -random 6 -check` — quality gate
- `go run ./cmd/cutoptics -demo` / `-demo-bar` — demo plan as JSON

Frontend (`frontend/`):
- `pnpm dev` — Vite on :5173, proxies `/api` and `/healthz` to :8080
- `pnpm build` (`tsc -b && vite build`), `pnpm lint` (oxlint), `pnpm preview`

Database (`db/scripts/`): `up.ps1` → `migrate.ps1` → `seed.ps1` → `test.ps1`;
`generate.ps1` after any `db/queries/*.sql` change; `reset.ps1` for a clean slate.

Knowledge graph (CLI is inside the project venv, not on PATH — use the wrapper):
`.\graphify.ps1 query "<question>"` · `.\graphify.ps1 path "A" "B"` ·
`.\graphify.ps1 explain "Concept"` · `.\graphify.ps1 update .` (refresh after
code changes — AST only, no API cost).

## Non-negotiable conventions

- **Units**: every length in the DB, API and solvers is an integer micrometer
  (`int64` / BIGINT). Floats only for cost, percentages and areas in m²; the
  frontend converts at the edge (`frontend/src/lib/format.ts`).
- **One solver seam**: everything implements `core.Solver`; register new solvers
  instead of branching in callers. Every solution passes `validator.Validate`
  (including guillotine cut-tree feasibility) before it is shown or stored.
- **Generated code**: `backend/internal/platform/db/*` is sqlc output — edit
  `db/queries/*.sql` and run `db/scripts/generate.ps1`; never hand-edit it.
- **Determinism**: same problem + seed + solver version ⇒ identical plan.

## graphify

This project has a knowledge graph at graphify-out/ with god nodes, community structure, and cross-file relationships.

When the user types `/graphify`, use the installed graphify skill or instructions before doing anything else.

Rules:
- For codebase questions, first run `graphify query "<question>"` when graphify-out/graph.json exists. Use `graphify path "<A>" "<B>"` for relationships and `graphify explain "<concept>"` for focused concepts. These return a scoped subgraph, usually much smaller than GRAPH_REPORT.md or raw grep output.
- Dirty graphify-out/ files are expected after hooks or incremental updates; dirty graph files are not a reason to skip graphify. Only skip graphify if the task is about stale or incorrect graph output, or the user explicitly says not to use it.
- If graphify-out/wiki/index.md exists, use it for broad navigation instead of raw source browsing.
- Read graphify-out/GRAPH_REPORT.md only for broad architecture review or when query/path/explain do not surface enough context.
- After modifying code, run `graphify update .` to keep the graph current (AST-only, no API cost).
