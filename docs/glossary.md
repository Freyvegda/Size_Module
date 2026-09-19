# Domain glossary

The vocabulary is deliberately material-agnostic. Glass, sheet metal, wood and
fabric are all described with the same words; only the rules profile changes.

| Term | Meaning |
|---|---|
| **Material** | A family of stock, e.g. `GLASS`, `ALU`, `MDF`. |
| **Material spec** | A concrete variant of a material: thickness, finish, colour, grade. |
| **Stock format** | A catalog size that can be ordered or cut: `SHEET-3210x2250`, `BAR-6000`, with an on-hand quantity. |
| **Stock item** | A physical piece of a format: a specific sheet, bar or remnant with a label, status and lineage. |
| **Remnant / offcut** | A leftover piece large enough to be reused. Its minimum size is policy (`offcutMinW`, `offcutMinH`, `offcutMinLength`). Accepted plans register remnants as labelled stock items. |
| **Remnant-first allocation** | The policy (`preferRemnants`) that offers physical remnants to the solver before fresh catalog stock, and does not charge remnant sheets as new sheets in the objective. |
| **Plan acceptance** | Freezing a plan and applying it to the shop: consume the physical pieces used, decrement catalog on-hand quantities, register labelled remnants for reusable offcuts, write an audit entry. |
| **Plan version** | An immutable plan row; an edit or re-solve stores the next version (`version + 1`, `parent_plan_id`) and archives the source. |
| **Locked placement (pin)** | A placement a planner pinned; a re-solve must keep it exactly where it is and pack around it (`pinned-2d` / `pinned-1d`). |
| **Export** | A downloadable plan artefact: CSV cut list, SVG drawing, DXF R12 (CAD/CAM) or PDF. |
| **Cost breakdown** | The value of a plan: new material, remnants taken, part/trim/kerf/scrap shares, offcut credit and the resulting net cost, per part and per m². |
| **Net cost** | Material taken minus the value of leftovers that stay in stock: what the order actually consumed. |
| **Realized yield (KPI)** | Yield over **accepted** plans in a window; "created" covers every plan, so pipeline and shop reality can be compared. |
| **Campaign** | An ordered set of jobs planned against one shared stock budget; running an item consumes sheets and returns its offcuts to the pool for the items that follow. |
| **Campaign budget** | The remaining stock list of a campaign: it shrinks as sheets are used and grows as `CMP-…` offcuts re-enter. Re-entering offcuts are registered as real available stock items, so the budget and the physical pool stay one ledger. |
| **Scrap** | Leftover material too small to be reusable. |
| **Part** | A thing to produce, defined by its **finished** size. |
| **Assembly (product)** | A thing the shop *builds* from subparts, e.g. a window or a table: an overall size, a bound **material spec** and an ordered component list. Not itself a cut piece — but it can be *exploded* into cut parts (Products → Run cut plan). A product that mixes materials/dimensions is planned as one order per **(material spec, dimension profile)**. |
| **Component (subpart)** | One box of an assembly: a role (`glass`, `leg`, …), a kind (`beam`/`panel`/`custom`), a quantity, its dimensions, an optional material spec (else the product's) and its position in the assembly's local frame (origin bottom-left-front; x right, y up, z out of the wall). Drives both the 2D elevation and the 3D view. |
| **Routing** | The ordered operations applied to a part (grinding, edge deletion, ...), each with an **allowance**. |
| **Allowance** | Extra material needed by an operation, per edge or total. |
| **Cut size** | Finished size + allowances. This is what the solver receives. |
| **Kerf** | Material consumed by the tool on each cut (blade width). |
| **Trim** | The strip of material next to a stock edge that cannot be used. |
| **Guillotine cut** | A straight cut across the whole piece from edge to edge. Panel saws and glass cutters need these; CNCs do not. |
| **Cut tree** | The recursive structure of guillotine cuts that produces a layout. If no tree exists, the layout is not cuttable. |
| **Pattern** | A specific layout of parts on one sheet. Fewer distinct patterns means fewer machine setups. |
| **Placement** | One part at one position on one sheet. |
| **Plan** | A complete answer: which sheets, which placements, which offcuts. |
| **Solution** | A plan plus metrics, notes and validations, as returned by a solver. |
| **Rules profile** | A stored set of constraints (kerf, trim, rotation, grain, cut mode, offcut policy). The only place vertical specifics live. |
| **Objective** | The weighted goal: fulfil priority demand, minimise sheets, scrap, pattern count, offcut area, cost. |
| **Yield** | Part area ÷ stock area, as a percentage. |
| **Waste** | 100% − yield, split into trim, kerf, scrap and offcuts. |
| **Baseline** | A deliberately simple solver (e.g. `shelf-2d`) used to judge improvements honestly. |

## Unit convention

Every length in the database, the API and the solvers is an integer number of
**micrometers** (`µm`). `1 mm = 1000 µm`. Floats are only used for costs,
percentages and areas in m². Conversions happen at the edges (UI formatting).

## Why guillotine feasibility is checked

A layout can look fine on screen and still be impossible to produce: the classic
"pinwheel" arrangement of four pieces has no edge-to-edge cut that separates the
parts. The validator rebuilds the cut tree for every sheet before a plan is
shown or stored, which is why the engine can promise that a plan is cuttable.
