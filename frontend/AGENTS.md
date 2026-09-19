# frontend/ — React SPA

React 19 + TypeScript + Vite 8 + Tailwind v4 + shadcn/ui (radix-ui) + Redux
Toolkit + three.js (`@react-three/fiber`, lazy-loaded). The app talks to the Go
API through `/api` and `/healthz`, which the Vite dev server proxies to
`http://localhost:8080`.

## Layout

| Path | Contents |
|---|---|
| `src/main.tsx` | `createRoot` + Redux `Provider` + `BrowserRouter` + `TooltipProvider` |
| `src/App.tsx` | routes nested under `AppShell` (index → Dashboard, `viewer`, `materials`, `parts`, `stock`, `jobs`, `settings`, `*` → `/`) |
| `src/app/` | Redux `store.ts` (slices: `backend`, `catalog`, `optimizer`, `viewer`) and typed `hooks.ts` |
| `src/components/layout/AppShell.tsx` | sidebar nav, header, API status badge, `<Outlet/>` |
| `src/components/ui/` | shadcn primitives (button, card, table, select, tabs, …) |
| `src/features/backend/` | health + meta slice (async thunks) |
| `src/features/catalog/` | materials, parts, stock-format and physical-stock slice (list + create + update thunks) |
| `src/features/optimizer/` | 1D/2D demo problems, optimization run + solver comparison + async job queue (submit/cancel, SSE progress) slice, `SolverBadges.tsx` |
| `src/features/viewer/` | `PlanViewer.tsx` (panels), `PlanScene.tsx` (three.js ortho 2D / perspective 3D), `BarPlanView.tsx` (1D bars), `viewerSlice.ts`, `partColor.ts` |
| `src/lib/` | `api.ts` (fetch wrapper + `ApiError`), `types.ts` (hand-written TS mirrors of Go contracts), `results.ts` (nil-slice normalization), `solvers.ts` (registry-compatible solver filtering), `format.ts` (µm→mm), `samplePlan.ts` (2D + 1D fallback plans), `utils.ts` |
| `src/pages/` | Dashboard, Jobs, Materials, Parts, Stock, PlanViewer, Settings |
| `public/` | `favicon.svg`, `icons.svg` |

## Conventions

- **Redux Toolkit for all server state** (no TanStack Query/Zustand — `plan.txt`
  describes an older plan). Server data is fetched in `createAsyncThunk`s; local
  form/page state uses `useState`.
- **API client**: use `api.get` / `api.post` / `api.patch` from `lib/api.ts`;
  relative paths only (`/api/v1/...`) so the Vite proxy and production work the
  same.
- **Units**: everything from the API is integer micrometers; convert for display
  only, via `lib/format.ts` (`mmToMicron` in the create forms).
- **Solver pickers** must filter with `solversFor()` from `lib/solvers.ts`, which
  mirrors the backend registry (`core.CutModeCompatible` / rank order). A named
  solver for the wrong dimension profile or cut mode is rejected with a `422`,
  so never offer the raw `/api/v1/meta` list.
- **Async jobs**: the Jobs page submits with `startAsyncJob` (`POST
  /api/v1/jobs`) and follows `GET /api/v1/jobs/{id}/events` over SSE. The
  `optimizer` slice tracks `jobRequest` / `jobState` / `jobProgress` and loads
  the archived result into the viewer when the job is done. The queue needs
  PostgreSQL; the Run button uses synchronous `/optimize` and works without it.
- **Free cutting**: `demoProblemFor('2d', 'free')` sends the *complete* default
  rules with `cutMode: free`; the server only fills in defaults when the whole
  rules object is empty, so sending one overridden field would drop kerf/trim.
- **1D plans** come back as `SheetPlan`s whose width is the bar length. The
  viewer detects them via `optimizer.dimension` and renders `BarPlanView`
  instead of the three.js scene; length metrics live in `stockLengthM` /
  `usedLengthM`.
- **Remnants**: the Jobs page toggle sends `includeRemnants=true`; the Jobs and
  Stock pages read `metrics.remnantSheets` and `/api/v1/stock-items`. The plan
  viewer offers **Accept plan** when the optimizer slice holds a `lastPlanId`
  (set from `planId` in an optimize response or a job view); acceptance consumes
  pieces, decrements on-hand quantities and registers labelled remnants, and the
  page refreshes the available pool afterwards.
- **Nil slices**: Go marshals nil slices as JSON `null` (`unplaced`, `offcuts`,
  and sometimes `sheets`), so every `OptimizeResult` entering the store goes
  through `normalizeResult()` from `lib/results.ts`. Viewer components still
  guard with `?? []` as a second line of defence — a raw null once crashed the
  whole viewer.
- `PlanScene.tsx` is loaded with `React.lazy` so three.js stays out of the
  initial bundle — keep it that way.
- `lib/types.ts` must stay in sync with `backend/api/openapi.yaml` manually
  (client codegen is planned, not wired). `SolverCapabilities` includes `Rank`.
- shadcn components live in `components/ui`; use the `cn` helper from `@/lib/utils`
  (re-export of the `cn` package). `components.json` points the `hooks` alias at
  `@/hooks`, which does not exist yet — create it if you add a hook.

## Commands (from `frontend/`)

```powershell
pnpm install
pnpm dev      # :5173, proxies /api and /healthz to :8080
pnpm build    # tsc -b && vite build
pnpm lint     # oxlint
pnpm preview
```

Pages degrade gracefully: the dashboard/jobs pages show API/DB-down messaging,
and the viewer falls back to `lib/samplePlan.ts` (2D or 1D) when the demo
endpoints fail.
