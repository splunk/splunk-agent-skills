# UDF Options Reference

Authoritative option keys for Studio viz types, data sources, and inputs. Sourced from `@splunk/visualizations/*.config.js` and `@splunk/dashboard-inputs` schemas.

## When to load

Load for **any** conversion — these are the exact option keys the model must use when writing Studio JSON. Prevents hallucinated option names.

## Supported conversions

- Classic `charting.*` options → viz-specific option keys listed below
- Classic `<input>` options (`token`, `defaultValue`, `fieldForLabel`, `fieldForValue`) → input option keys
- Classic `<query>` + `<earliest>`/`<latest>` → `ds.search` `queryParameters`
- Classic `<search base="...">` chains → `ds.chain`

## Unsupported — warn or ask

- `charting.*` options not listed in this file have no Studio equivalent — note in report and skip
- `input.multiselect` has no native `delimiter`/`prefix`/`suffix` — handled in `references/form-inputs.md`

---

## Conversion rules

### Data sources

**`ds.search`**

| Option | Type | Notes |
|---|---|---|
| `query` | string | SPL string, preserve verbatim |
| `queryParameters.earliest` | string | e.g. `-24h@h`, `$tok.earliest$` |
| `queryParameters.latest` | string | e.g. `now`, `$tok.latest$` |
| `queryParameters.searchMode` | `fast` \| `smart` \| `verbose` | |
| `queryParameters.sampleRatio` | number \| string | |
| `refresh` | number \| string | Interval in seconds or time expression |
| `refreshType` | `delay` \| `interval` | `delay` = after search completes; `interval` = when dispatched |

**`ds.chain`**

| Option | Type | Notes |
|---|---|---|
| `query` | string | SPL appended to the base search |
| `extend` | string | Key of the base `ds.search` to chain from |

---

### Viz options — common (most chart types)

| Option | Type | Values |
|---|---|---|
| `backgroundColor` | string | Hex color or token |
| `legendDisplay` | string | `right` \| `left` \| `top` \| `bottom` \| `off` |
| `legendLabels` | array | Override legend label strings |
| `legendMode` | string | `standard` \| `seriesCompare` |
| `seriesColors` | array | Ordered hex colors per series |
| `seriesColorsByField` | object | `{ "fieldName": "#hex" }` |
| `resultLimit` | number | Max series to render |
| `dataValuesDisplay` | string | `off` \| `all` \| `minmax` |
| `xAxisTitleText` | string | X-axis label |
| `yAxisTitleText` | string | Primary Y-axis label |
| `y2AxisTitleText` | string | Secondary Y-axis label |
| `xAxisTitleVisibility` | string | `show` \| `hide` |
| `yAxisTitleVisibility` | string | `show` \| `hide` |
| `xAxisLabelVisibility` | string | `show` \| `hide` |
| `yAxisLabelVisibility` | string | `show` \| `hide` |
| `xAxisLabelRotation` | number | Degrees |
| `yAxisMin` | number | |
| `yAxisMax` | number | |
| `yAxisScale` | string | `linear` \| `log` |
| `showYAxisWithZero` | boolean | Force Y-axis to include zero |
| `showXMajorGridLines` | boolean | |
| `showYMajorGridLines` | boolean | |
| `showSplitSeries` | boolean | Separate each series into its own panel |
| `splitByLayout` | string | `trellis` to enable trellis |

### Viz options — `splunk.line` / `splunk.area`

| Option | Type | Values |
|---|---|---|
| `nullValueDisplay` | string | `zero` \| `connect` \| `gaps` |
| `lineDashStyle` | string | `solid` \| `shortDash` \| `longDash` |
| `lineWidth` | number | Stroke width in px |
| `showLineSmoothing` | boolean | |
| `markerDisplay` | string | `off` \| `auto` (line only) |
| `stackMode` | string | `auto` \| `stacked` \| `stacked100` (area only) |
| `overlayFields` | array | Fields to render on Y2 axis |

### Viz options — `splunk.bar` / `splunk.column`

| Option | Type | Values |
|---|---|---|
| `stackMode` | string | `auto` \| `stacked` \| `stacked100` |
| `overlayFields` | array | Fields to render on Y2 axis |

### Viz options — `splunk.pie`

| Option | Type | Values |
|---|---|---|
| `labelDisplay` | string | `valuesAndPercentage` \| `percentage` \| `value` \| `label` |
| `labelField` | string | Field for slice labels |
| `valueField` | string | Field for slice values |
| `showDonutHole` | boolean | Render as donut |
| `collapseThreshold` | number | Collapse slices below this % |
| `collapseLabel` | string | Label for collapsed "other" slice |

### Viz options — `splunk.table`

| Option | Type | Values |
|---|---|---|
| `count` | number | Rows per page |
| `showRowNumbers` | boolean | |
| `headerVisibility` | string | `show` \| `hide` |
| `showFooterTotals` | boolean | |
| `showFooterPercentages` | boolean | |
| `columnFormat` | object | Per-column formatting config |

### Viz options — `splunk.singlevalue`

| Option | Type | Values |
|---|---|---|
| `majorValue` | string | DSL expression (default `> majorValue`) |
| `majorValueField` | string | Field to use as the major value |
| `majorColor` | string | Hex color |
| `majorFontSize` | number | px |
| `underLabel` | string | Caption below the major value |
| `underLabelColor` | string | Hex color |
| `unit` | string | e.g. `"events"`, `"ms"` |
| `unitPosition` | string | `before` \| `after` |
| `trendValue` | string | DSL expression for trend |
| `trendDisplay` | string | `absolute` \| `percent` \| `off` |
| `trendColor` | string | Hex color |
| `sparklineDisplay` | string | `before` \| `after` \| `below` \| `off` |
| `sparklineValues` | string | DSL expression for sparkline data |
| `numberPrecision` | number | Decimal places |
| `shouldUseThousandSeparators` | boolean | |

### Viz options — `splunk.events`

| Option | Type | Values |
|---|---|---|
| `pageCount` | number | Events per page |
| `showFieldSummary` | boolean | |

### Viz options — `splunk.markdown`

No datasource required. Content goes in `options.markdown`:

```json
{
  "type": "splunk.markdown",
  "options": {
    "markdown": "## Heading\n\nText with **bold** and token: $myToken$"
  }
}
```

---

### Input options — common

| Option | Type | Notes |
|---|---|---|
| `token` | string | Required — token name the input sets |
| `tokenNamespace` | string | Default `"default"` |
| `defaultValue` | string | Value when no selection made |

### Input options — `input.dropdown`

| Option | Type | Notes |
|---|---|---|
| `items` | array \| string | Static `[{label, value}]` or DSL string (see dynamic pattern below) |
| `selectFirstSearchResult` | boolean | Mutually exclusive with `defaultValue` |
| `isCascadingInput` | boolean | Resets to default when upstream token changes |

### Input options — `input.multiselect`

Same as dropdown plus:

| Option | Type | Notes |
|---|---|---|
| `defaultValue` | string \| array | Single string or array of strings |
| `clearDefaultOnSelection` | boolean | Default `true` |

### Input options — `input.text`

`token`, `tokenNamespace`, `defaultValue` only.

### Input options — `input.number`

| Option | Type | Notes |
|---|---|---|
| `min` | number | |
| `max` | number | |
| `step` | number | Default `1` |

### Input options — `input.timerange`

`token` and `defaultValue` as `"earliest,latest"` string (e.g. `"-24h@h,now"`).

Sets two tokens: `$<token>.earliest$` and `$<token>.latest$`.

### Input options — `input.button`

Uses `eventHandlers` rather than `token`. See `references/drilldown.md`.

---

### Dynamic input items DSL

When a classic `<input>` has `<fieldForLabel>` / `<fieldForValue>` with a `<search>`, the Studio equivalent uses DSL expressions in `context`. This pattern is sourced from the conversion script and verified working:

```json
{
  "type": "input.dropdown",
  "options": {
    "token": "my_token",
    "items": "> frame(label, value) | objects()"
  },
  "context": {
    "formattedConfig": { "number": { "prefix": "" } },
    "label": "> primary | seriesByName(\"<fieldForLabel>\") | renameSeries(\"label\") | formatByType(formattedConfig)",
    "value": "> primary | seriesByName(\"<fieldForValue>\") | renameSeries(\"value\") | formatByType(formattedConfig)"
  },
  "dataSources": { "primary": "ds_my_search" }
}
```

With mixed static choices (`<choice>` + search), add `statics` and `formattedStatics`:

```json
{
  "options": {
    "items": "> frame(label, value) | prepend(formattedStatics) | objects()"
  },
  "context": {
    "formattedConfig": { "number": { "prefix": "" } },
    "formattedStatics": "> statics | formatByType(formattedConfig)",
    "statics": [["Label A", "Label B"], ["value_a", "value_b"]],
    "label": "> primary | seriesByName(\"<fieldForLabel>\") | renameSeries(\"label\") | formatByType(formattedConfig)",
    "value": "> primary | seriesByName(\"<fieldForValue>\") | renameSeries(\"value\") | formatByType(formattedConfig)"
  }
}
```

`statics` is `[labelsArray, valuesArray]` — not an array of `{label, value}` objects.
