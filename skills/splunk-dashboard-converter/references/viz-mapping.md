# Visualization Mapping Reference

Maps classic `<chart>`/`<single>`/`<table>`/`<event>`/`<map>` types and their options to Studio `splunk.*` viz types, plus theme/color parity rules.

## When to load

Load for **any** conversion that has a `<chart>`, `<single>`, `<table>`, `<event>`, or `<map>` — i.e. essentially always.

## Chart type → Studio type

| Classic `charting.chart` value | Studio `type` |
|---|---|
| `bar` | `splunk.bar` |
| `column` | `splunk.column` |
| `line` | `splunk.line` |
| `area` | `splunk.area` |
| `pie` | `splunk.pie` |
| `scatter` | `splunk.scatter` |
| `bubble` | `splunk.bubble` |
| `radialGauge` | `splunk.singlevalueradial` |
| `fillerGauge` | `splunk.fillergauge` |
| `markerGauge` | `splunk.markergauge` |

`<single>` → `splunk.singlevalue`
`<table>` → `splunk.table`
`<event>` → `splunk.events`
`<map>` (geom-based) → `splunk.map` with `layers: [{ "type": "choropleth" }]`

For unmapped or third-party viz types, default to `splunk.table` and note the substitution in the report.

## Common chart option translations

| Classic option | Studio option |
|---|---|
| `charting.axisTitleX.text` | `xAxisTitleText` |
| `charting.axisTitleY.text` | `yAxisTitleText` |
| `charting.legend.placement` (`bottom`/`right`/`top`/`left`) | `legendDisplay` (same values) |
| `charting.legend.placement="none"` | `legendDisplay="off"` |

## Single value option translations

| Classic option | Studio option | Notes |
|---|---|---|
| `<option name="underLabel">` | `underLabel` | Caption below the major value — not `label` |

## Single value sparkline DSL

Classic `<single>` panels that feed a time-series search should use the sparkline DSL pattern to get the sparkline bar, major value, and trend arrow. Without this, Studio shows only a scalar number with no sparkline or trend.

```json
"viz_sv": {
  "type": "splunk.singlevalue",
  "options": {
    "sparklineValues": "> primary | seriesByName('<fieldName>')",
    "majorValue": "> sparklineValues | lastPoint()",
    "trendValue": "> sparklineValues | delta(-2)",
    "underLabel": "<caption>"
  }
}
```

`<fieldName>` is the numeric field from the SPL (e.g. `count`, `total`, `sizeOnDiskMB`). The search must return a time-series result (e.g. `timechart` or `bucket _time | stats`) for the sparkline to have points to plot.

## Visual parity — theme & colors

### Theme & background

| Classic source | Studio `theme` | `backgroundColor` |
|---|---|---|
| `<dashboard theme="dark">` | `"dark"` | `"#000000"` |
| `<dashboard theme="light">` | `"light"` | `"#FFFFFF"` |
| No `theme` attribute | `"light"` | `"#FFFFFF"` |

Set `backgroundColor` inside `layoutDefinitions.layout_1.options`, not `layout.options`.

### Chart colors — per-type rules

| Chart type | Recommended option | Why |
|---|---|---|
| `splunk.bar` | `fieldColors` keyed by category value | Both engines honor it; survives data reordering |
| `splunk.pie` | `seriesColors` (positional) | Engines disagree on `fieldColors` semantics for pie |
| `splunk.column` | `seriesColors` (positional) | Same disagreement as pie |
| `splunk.line` / `splunk.area` | `fieldColors` keyed by series name | For multi-series timecharts |
| Scatter / bubble | `seriesColors` (positional) | Value-keyed binding unreliable across engines |

**Do not use** `fieldColors` on `splunk.pie` or `splunk.column` — silently ignored.
**Do not use** app-level CSS or `data/ui/themes/` for chart palette overrides — brittle and version-fragile.

### Color workflow

1. **Explicit colors in classic XML** (highest priority) — translate verbatim:
   - `charting.fieldColors` → `fieldColors`
   - `charting.seriesColors` → `seriesColors`

   > **`seriesColors` format conversion**: Classic stores the value as a bare string like `[0x6BB7C8,0x999999,0xD85E3D]`. Studio requires a JSON array of strings: `["0x6BB7C8","0x999999","0xD85E3D"]`. Parse the classic string, split on `,`, strip `[` `]`, and quote each hex value.
2. **No explicit colors + user asked for parity** — derive from data and inject the right option per chart type above. Update **both** dashboards in the same pass.
3. **No explicit colors + no parity request** — leave unset, let Studio use its native palette.

### Single value
Looks identical across formats when single-value rules are followed: numeric values from SPL (never `tostring()`), formatting via `unit` / `unitPosition` / `numberPrecision`.

## Report notes

- Which theme + background was applied and why
- Whether chart colors were preserved, derived, or left as Studio defaults
- If `fieldColors` were derived from data, list the value→hex assignments for user review
- Any unmapped/third-party viz type that defaulted to `splunk.table`
