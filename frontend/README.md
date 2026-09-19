# Size Module — frontend

React 19 + TypeScript + Vite + Tailwind v4 / shadcn/ui + Redux Toolkit +
three.js (via @react-three/fiber).

```powershell
pnpm install
pnpm dev      # http://localhost:5173, proxies /api to http://localhost:8080
pnpm build    # tsc -b && vite build
pnpm lint
```

## Layout

```
src/
  app/                 Redux store and typed hooks
  components/layout/   App shell (sidebar + header)
  components/ui/       shadcn/ui components
  features/
    backend/           health + meta slice
    catalog/           materials, parts and stock-format list/create thunks
    optimizer/         2D/1D demo problems, optimize + comparison + async job slice
    viewer/            PlanViewer, PlanScene (three.js 2D/3D), BarPlanView (1D)
  lib/                 API client, types, solver filtering, formatters, sample plans
  pages/               Dashboard, Plan viewer, Materials, Parts, Stock, Jobs, Settings
```

## Notes

- The Jobs page runs the demo problem with a chosen solver (`?solver=`) or
  compares every compatible solver side by side (dry runs, nothing archived).
  Only solvers the backend accepts for the selected dimension profile and cut
  mode (guillotine / free) are offered — `lib/solvers.ts` mirrors the registry.
- **Async queue**: on a database-backed API the Jobs page can also submit to
  `POST /api/v1/jobs` and watch `GET /api/v1/jobs/{id}/events` (SSE): live
  sheets, pieces, yield and elapsed time, with cancel, and the archived result
  loads into the viewer when the job finishes. Without PostgreSQL, use the
  synchronous Run button.
- **1D bars** are first-class: load the bar demo, run `ffd-1d` / `cg-1d` /
  `best-1d`, and the viewer renders `BarPlanView` tracks with length metrics
  instead of the 3D sheet scene.
- **Materials and parts** can be created from the UI (`POST /api/v1/materials`,
  `POST /api/v1/parts`), which needs PostgreSQL; the pages explain the
  `db/scripts` steps when it is down.
- `features/viewer/PlanScene.tsx` renders 2D plans twice: an orthographic
  top-down drawing and a perspective 3D stack. It is loaded lazily so three.js
  stays out of the initial bundle.
- Redux owns server state on purpose: the optimizer result is global (dashboard,
  jobs page and viewer all read it), and the viewer selection is shared between
  the canvas and the side panels. The catalog slice keeps materials/parts/stock
  in the same store.
- All API lengths are micrometers; formatting helpers in `lib/format.ts` convert
  to millimeters at the edge, and the create forms convert back with
  `mmToMicron`.
