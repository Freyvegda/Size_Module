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
| `src/app/` | Redux `store.ts` (slices: `backend`, `assemblies`, `campaigns`, `catalog`, `kpis`, `optimizer`, `viewer`) and typed `hooks.ts` |
| `src/components/layout/AppShell.tsx` | sidebar nav, header, API status badge, `<Outlet/>` |
| `src/components/ui/` | shadcn primitives (button, card, table, select, tabs, …) |
| `src/features/backend/` | health + meta slice (async thunks) |
| `src/features/kpis/` | realized-yield KPI slice (`fetchKpis`, window in days) |
| `src/features/campaigns/` | campaign list/detail slice (create, add/remove item, run-next mutations) |
| `src/features/catalog/` | materials, material types/brands, parts, stock-format and physical-stock slice (list + create + update thunks) |
| `src/features/optimizer/` | 1D/2D demo problems, optimization run + solver comparison + async job queue (submit/cancel, SSE progress) slice, `SolverBadges.tsx` |
| `src/features/viewer/` | `PlanViewer.tsx` (panels + edit/export toolbar), `PlanScene.tsx` (three.js ortho 2D / perspective 3D, draggable in edit mode), `BarPlanView.tsx` (1D bars), `viewerSlice.ts`, `partColor.ts` |
| `src/features/assemblies/` | products (assemblies) slice + preview: `assemblySlice.ts`, `geometry.ts`, `explode.ts` (components → cut parts), `templates.ts` (window/table), `AssemblyViewer.tsx` (2D/3D toggle), `AssemblyElevation.tsx` (SVG), `AssemblyScene.tsx` (lazy three.js) |
| `src/lib/` | `api.ts` (fetch wrapper + `ApiError`), `types.ts` (hand-written TS mirrors of Go contracts), `results.ts` (nil-slice normalization), `solvers.ts` (registry-compatible solver filtering), `format.ts` (µm→mm), `samplePlan.ts` (2D + 1D fallback plans), `utils.ts` |
| `src/pages/` | Dashboard, Jobs, Materials, Parts, Products, Stock, PlanViewer, Settings |
| `public/` | `favicon.svg`, `icons.svg` |

## Conventions

- **Redux Toolkit for all server state** (no TanStack Query/Zustand — `plan-v1.txt`
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
- **KPIs**: the dashboard fetches `/api/v1/kpis?days=…` through the `kpis`
  slice and only when `backend.health.db === 'up'`; `realized` is accepted
  plans, `created` the pipeline, `series` drives the yield bars. Cost
  breakdowns come straight from `result.cost` on any `OptimizeResult`.
- **Campaigns**: `/campaigns` (list + create from catalog formats/remnants) and
  `/campaigns/:id` (budget table, ordered items, Run next, add item from catalog
  parts). Every mutation returns the full detail, which the slice stores, so
  the budget on screen is always the server's. Opening a planned item's plan
  uses `loadPlanDetail` from the optimizer slice.
- **Plan editing**: the viewer's edit mode keeps a draft copy of the sheets plus
  pending `EditOperation`s in `viewerSlice` (`beginEdit`, `movePart`,
  `rotatePart`, `toggleLockPart`, `deletePart`); `PlanScene` drags pieces in 2D
  only and snaps to millimetres. Saving/re-solving goes through `editPlan` /
  `reoptimizePlan` in the optimizer slice, which replace `result` with the new
  plan version and update `lastPlanId`. Export buttons are plain links to
  `GET /api/v1/plans/{id}/exports?format=…`, so the browser downloads them.
- **Nil slices**: Go marshals nil slices as JSON `null` (`unplaced`, `offcuts`,
  and sometimes `sheets`), so every `OptimizeResult` entering the store goes
  through `normalizeResult()` from `lib/results.ts`. Viewer components still
  guard with `?? []` as a second line of defence — a raw null once crashed the
  whole viewer.
- `PlanScene.tsx` is loaded with `React.lazy` so three.js stays out of the
  initial bundle — keep it that way.
- **Products (assemblies)**: `src/pages/ProductsPage.tsx` builds a product from
  subparts under a chosen material spec (type), previews it, and can run a cut
  plan. Components are boxes in the assembly's local frame (origin
  bottom-left-front; x right, y up, z out of the wall). `AssemblyViewer` renders
  the 2D SVG elevation (`AssemblyElevation`) and lazy-loads `AssemblyScene.tsx`
  for 3D, keeping three.js out of the initial bundle. `templates.ts` provides the
  window/table generators; `explode.ts` turns components into cut parts (cut
  face = the two largest dimensions, identical parts merged) and
  `optimizeProblem` plans them against the type's stock, then the viewer opens.
- **Materials & stock entry**: the Materials page creates families and their
  types (`POST /api/v1/material-specs`); the Stock page adds catalog sizes
  (`POST /api/v1/stock-formats`). Both feed the Products material pickers.
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
