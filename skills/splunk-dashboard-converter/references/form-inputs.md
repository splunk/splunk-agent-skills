# Form Input Translation

Classic `<input>` / `<fieldset>` elements convert to the Studio `inputs` block, with each input also listed in `layout.globalInputs`.

## When to load

Load when the classic XML has `<input>` or `<fieldset>` elements.

## Supported conversions

- All classic input types (`time`, `dropdown`, `text`, `multiselect`, `checkbox`, `radio`, `link`) → Studio `input.*` types.
- Static choice inputs → `options.items`.
- Dynamic (search-populated) inputs → `dataSources.primary` + `encoding`.
- Mixed static + dynamic dropdowns → `items` plus `dataSources` + `encoding`.
- Chained (cascading) inputs → `isCascadingInput: true`.
- Submit button → `layout.options.submitButton`.
- `text`/`dropdown` prefix/suffix → native `options.prefix`/`options.suffix`.

## Unsupported — warn or ask

- `input.multiselect` `delimiter`/`prefix`/`suffix` — silently ignored by Studio; values are always comma-joined with no wrapping. Skip with warning and list in the conversion report.
- `text`/`dropdown` prefix/suffix has a **double-prefix on hard reload** bug — no workaround; note in report.
- `searchWhenChanged="true"` on an upstream cascading input — no Studio equivalent; drop silently.

## Conversion rules

### Type mapping

| Classic input type | Studio input type |
|---|---|
| `time` | `input.timerange` |
| `dropdown` | `input.dropdown` |
| `text` | `input.text` |
| `multiselect` | `input.multiselect` |
| `checkbox` | `input.checkbox` |
| `radio` | `input.dropdown` |
| `link` | `input.dropdown` |

**Critical**: all input properties go inside `options`, not top-level. The `token` binding is required.

```json
"input_1": {
  "type": "input.text",
  "title": "<from classic <label>>",
  "options": {
    "token": "<classic token attribute>",
    "defaultValue": "<from classic <default>>"
  }
}
```

### Submit button

`<fieldset submitButton="true">` → `layout.options.submitButton: true`

Default in classic is `true`. Omit from Studio JSON (or set `false`) when the classic fieldset has `submitButton="false"`.

### Empty-string choice values

Studio silently drops any `items` entry with `value: ""`. Classic dashboards commonly use `<choice value="">-- None --</choice>` as a "nothing selected" sentinel paired with `depends="$token$"`. Convert by:

1. Replace `value: ""` with a real sentinel (e.g. `"none"`)
2. Set `defaultValue` to that sentinel
3. Replace any `$isSet($token$)` condition with an equality check: `$token$ != 'none'`

### Dynamic inputs (search-populated)

Wire a `dataSources.primary` and an `encoding` block that maps result fields to the dropdown's `label` and `value`. The `encoding` block is **required** — without it the dropdown has no fields to populate from and stays empty.

- **label = value** (most common): point both at the same field.
- **label ≠ value**: have the input's SPL return two fields and reference them separately, e.g. SPL `... | table display id` → `"encoding": { "label": "primary.display", "value": "primary.id" }`. The dropdown shows `display`, but submits `id`.

Reference fields by name (`primary.<fieldName>`), not by position (`primary[0]`) — name binding survives column reordering in the SPL.

### Mixed static + dynamic dropdowns

When a classic `<input>` has both `<choice>` elements AND a `<search>`, put the static choices in `options.items` and wire the search via `dataSources` + `encoding`. The static items render first, then the search results are appended. The static `items` entries persist in the list regardless of which option is selected — that is expected. When there are no static choices, omit `items` entirely.

### Text and dropdown prefix / suffix

`input.text` and `input.dropdown` natively support `prefix` and `suffix` in `options`.

> **Known caveat — double-prefix on hard reload**: Studio re-applies prefix/suffix to the stored token value on page reload, producing doubled output (e.g. `index=index=_internal`). No workaround. Note this in the conversion report.

### Multiselect delimiter / prefix / suffix

`input.multiselect` `delimiter`, `prefix`, and `suffix` are silently ignored by Studio — values are always comma-joined with no wrapping, regardless of what the classic XML specified. Do not attempt to replicate this behavior with SPL rewrites or `expressions.eval`. List it as unsupported in the conversion report.

### Chained (cascading) inputs

When one input's SPL references a `$token$` set by another input, add `isCascadingInput: true` to the receiving input's `options`. The input is still wired with `dataSources` + `encoding` like any other dynamic input. `searchWhenChanged="true"` on the upstream input has no Studio equivalent — Studio cascading inputs always re-evaluate on upstream token change. Drop the attribute silently.

### Time input

`defaultValue` is a comma-separated string: `"-12h,now"` (not an object). Reference the time token in `queryParameters` as `$token.earliest$` / `$token.latest$`.

### Token references in SPL

Leave `$tokenName$` as-is in queries. Do **not** rewrite to `$inputs.<id>.value$` syntax.

### Multiselect defaultValue

When classic `<default>` contains multiple comma-separated values, emit an array:
```json
"defaultValue": ["val1", "val2"]
```

## Examples

### Empty-string sentinel

```xml
<!-- Classic -->
<choice value="">-- None --</choice>
<default></default>
```
→
```json
{ "label": "-- None --", "value": "none" },
"defaultValue": "none"
```
```json
"token_show": { "name": "token_show", "value": "$token$ != 'none'" }
```

### Static choice input

```json
"input_3": {
  "type": "input.dropdown",
  "title": "Log Level",
  "options": {
    "token": "log_level",
    "defaultValue": "INFO",
    "items": [
      { "label": "INFO", "value": "INFO" },
      { "label": "WARN", "value": "WARN" }
    ]
  }
}
```

### Dynamic input

```json
"input_1": {
  "type": "input.dropdown",
  "title": "Sourcetype",
  "options": { "token": "field1" },
  "dataSources": { "primary": "ds_input_1" },
  "encoding": { "label": "primary.<fieldName>", "value": "primary.<fieldName>" }
}
```

### Mixed static + dynamic dropdown

```xml
<!-- Classic -->
<input type="dropdown" token="index">
  <choice value="*">All indexes</choice>
  <search>...</search>
</input>
```
→
```json
"input_index": {
  "type": "input.dropdown",
  "options": {
    "token": "index",
    "defaultValue": "*",
    "items": [{ "label": "All indexes", "value": "*" }]
  },
  "dataSources": { "primary": "ds_input_index" },
  "encoding": { "label": "primary.orig_index", "value": "primary.orig_index" }
}
```

### Prefix / suffix

```json
"options": {
  "token": "myToken",
  "prefix": "(",
  "suffix": ")"
}
```

### Cascading input

```json
"input_sourcetype": {
  "type": "input.dropdown",
  "options": { "token": "sourcetype", "isCascadingInput": true },
  "dataSources": { "primary": "ds_input_sourcetype" },
  "encoding": { "label": "primary.sourcetype", "value": "primary.sourcetype" }
}
```

## Report notes

- Note any `text`/`dropdown` prefix/suffix conversion (double-prefix-on-reload bug applies).
- For multiselect delimiter/prefix/suffix, note that these are unsupported and silently ignored — list them as skipped in the report.
- Note any `searchWhenChanged` attribute that was dropped.
- List any empty-string sentinel choices that were rewritten to a real sentinel value.
