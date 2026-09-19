# docs/ — domain and solver documentation

| File | Contents | Update when |
|---|---|---|
| `glossary.md` | canonical vocabulary: material, spec, stock format, part, routing, allowance, cut size, kerf, trim, remnant/offcut, scrap, guillotine cut, cut tree, pattern, placement, plan, rules profile, objective, yield, waste, baseline — plus the micrometer unit rule | any new domain term or redefinition |
| `solver.md` | how the optimizer is built: the `Solver` seam, each shipped solver, benchmark harness, golden gate, current numbers, how to add a solver, known limits | solver behaviour, ranks, or benchmark numbers change |

Related, but stored elsewhere:

- `../plan.txt` — the original v1 plan (scope, milestones, risks). Historical:
  some choices drifted (Redux instead of TanStack/Zustand, no codegen yet).
- `../README.md` — human-facing overview + quickstart.
- `../AGENTS.md` — agent guide and repo map.

Keep `glossary.md` and `solver.md` short and factual; they are the
no-drift contract for names used across `db/`, `backend/` and `frontend/`.
