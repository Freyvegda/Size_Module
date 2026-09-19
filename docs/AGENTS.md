# docs/ — domain and solver documentation

| File | Contents | Update when |
|---|---|---|
| `glossary.md` | canonical vocabulary: material, spec, stock format, stock item, part, routing, allowance, cut size, kerf, trim, remnant/offcut, remnant-first allocation, plan acceptance, plan version, locked placement (pin), export, cost breakdown, net cost, realized yield, campaign, campaign budget, assembly, scrap, guillotine cut, cut tree, pattern, placement, plan, rules profile, objective, yield, waste, baseline — plus the micrometer unit rule | any new domain term or redefinition |
| `solver.md` | how the optimizer is built: the `Solver` seam, each shipped solver (including the pinned re-solve solvers), pinned placements, costing output, benchmark harness, golden gate, current numbers, how to add a solver, known limits | solver behaviour, ranks, or benchmark numbers change |

Related, but stored elsewhere:

- `../plan.txt` — the live forward plan (Phases 0–5: baseline hygiene, closing
  the open seams, platform hardening, product depth, ops and solver reach).
- `../plan-v1.txt` — the original v1 plan (scope, milestones, risks). Historical:
  some choices drifted (Redux instead of TanStack/Zustand, no codegen yet).
- `../README.md` — human-facing overview + quickstart.
- `../AGENTS.md` — agent guide and repo map.

Keep `glossary.md` and `solver.md` short and factual; they are the
no-drift contract for names used across `db/`, `backend/` and `frontend/`.
