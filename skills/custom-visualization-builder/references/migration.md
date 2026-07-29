# Migrating a legacy custom viz

Legacy Splunk custom vizzes use RequireJS and a class-based `SplunkVisualizationBase` API with lifecycle methods: `getInitialDataParams`, `formatData`, and `updateView`. The new framework replaces all of that with a single data listener (JS) or hooks (React).

## Identify what the legacy viz does

Before rewriting, read the existing viz and note:
- What `getInitialDataParams` returns — this controls data shape (`output_mode`: row, column, or raw)
- What `formatData` does — usually transforms the raw search result into something `updateView` can render
- What `updateView` renders — this is the core visualization logic to preserve
- Any token bindings, drilldown handlers, or options it reads from `getCurrentConfig()`

## What maps to what

| Legacy | New framework |
|---|---|
| `getInitialDataParams` | Not needed — data arrives as `{ fields, columns }` via listener/hooks |
| `formatData(data, config)` | Your own conversion helper — columns to rows if needed |
| `updateView(data, config)` | `render()` function (JS) or component body (React) |
| `getCurrentConfig()` / options | `VisualizationAPI.addOptionsListener` (JS) or `useOptions()` (React) |
| `this.trigger('drilldown', ...)` | `VisualizationAPI.triggerDrilldown(...)` — see `drilldown-and-tokens.md` |
| Token bindings | `VisualizationAPI.addTokensListener` / `useTokens()` — see `drilldown-and-tokens.md` |
| RequireJS `define([...], fn)` wrapper | Remove entirely — use ES module `import` instead |
| `splunkjs`, `mvc`, Backbone deps | Remove entirely — not available in the new iframe context |

## Migration steps

1. **Scaffold the new project** using the CLI (step 3 in SKILL.md). JavaScript is usually the right choice for existing Canvas/D3 vizzes.
2. **Copy rendering logic** from `updateView` into the new `render()` function.
3. **Replace data access** — the new framework delivers `{ fields, columns }`. If the legacy viz expected row-major data, convert with:
   ```js
   const { fields, columns } = state.data;
   const fieldNames = fields.map(f => f.name);
   const rows = columns[0].map((_, i) => Object.fromEntries(fieldNames.map((n, j) => [n, columns[j][i]])));
   ```
4. **Replace config access** — use `addOptionsListener`. Always provide fallback values.
5. **Replace RequireJS imports** — remove the `define([...], function(...) { })` wrapper. Use ES module `import`.
6. **Remove Backbone/splunkjs dependencies** — not available in the iframe context.
7. **Preserve CSS** — copy the existing stylesheet into `src/visualization.css`. The build plugin injects it automatically.
8. **Wire up drilldown/tokens if needed** — see `drilldown-and-tokens.md`.

## Data shape note

Legacy vizzes received data pre-transformed by `formatData` per `output_mode`. The new framework always delivers columnar `{ fields, columns }`. All values are strings — parse numerics with `parseFloat` / `parseInt`.
