# Panel Visibility (`depends` / `rejects`)

Classic `depends="$tokenName$"` and `rejects="$tokenName$"` map to named conditions in `expressions.conditions` referenced by `containerOptions.visibility`.

## When to load

Load when the classic XML has `depends=` or `rejects=` attributes on a `<row>`, `<panel>`, or viz element.

## Supported conversions

- `depends="$token$"` → `expressions.conditions` entry + `containerOptions.visibility.showConditions`.
- `rejects="$token$"` → `expressions.conditions` entry + `containerOptions.visibility.hideConditions`.
- Both on the same element → emit both `showConditions` and `hideConditions`.
- Empty-string sentinel patterns → equality-check condition instead of `$isSet`.

## Unsupported — warn or ask

- None. All `depends`/`rejects` forms convert.

## Conversion rules

### `depends` → showConditions

1. Create a named condition using `$isSet($tokenName$)`:
```json
"expressions": {
  "conditions": {
    "<tokenName>_show": {
      "name": "<tokenName>_show",
      "value": "$isSet($<tokenName>$)"
    }
  }
}
```

2. Reference it in the viz's `containerOptions.visibility.showConditions`:
```json
"viz_<id>": {
  "type": "splunk.table",
  "containerOptions": {
    "visibility": {
      "showConditions": ["<tokenName>_show"]
    }
  }
}
```

### `rejects` → hideConditions

Use `hideConditions` with the same `$isSet(...)` expression:
```json
"<tokenName>_hide": { "name": "<tokenName>_hide", "value": "$isSet($<tokenName>$)" }
```
```json
"containerOptions": { "visibility": { "hideConditions": ["<tokenName>_hide"] } }
```

> **`$isSet()` — use `$...$` delimiters for reactivity**: write `$isSet($tokenName$)`, not the quoted `$isSet("tokenName")` form. Both forms *evaluate* correctly, but the delimiter form is what reliably re-evaluates the condition when the token changes. The quoted form (which the UDF docs show) is fine for a token that is already set at load, but in older Studio versions (verified on `splunk-dashboard-studio` 1.28.0) a condition written as `$isSet("tokenName")` does **not** react to live input changes — the panel stays hidden until reload. The dependency tracker wires reactivity off the `$tokenName$` delimiter, so always prefer it.

### Scope rules

- `depends`/`rejects` on `<row>` → replicate condition on **every** viz in that row
- `depends`/`rejects` on `<panel>` → apply to all vizzes inside that panel
- `depends`/`rejects` on the viz element itself → apply only to that viz
- Both `depends` and `rejects` on same element → emit both `showConditions` and `hideConditions`

### De-duplication

Multiple vizzes sharing the same token → reuse one condition entry in `expressions.conditions`. The same condition ID can appear in many vizzes' arrays.

### Key naming

- `depends="$foo$"` → key `foo_show`, name `foo_show`
- `rejects="$foo$"` → key `foo_hide`, name `foo_hide`

Strip `$` delimiters from the token name when building the key.

### Custom equality conditions

For empty-string sentinel patterns (where `depends` doesn't use a plain `$isSet` test), replace `$isSet($token$)` with an equality check:

```json
"sourcetype_show": { "name": "sourcetype_show", "value": "$sourcetype$ != 'none'" }
```

See `form-inputs.md` for the full empty-string sentinel conversion pattern.

## Examples

### `depends` on a viz

```json
"expressions": {
  "conditions": {
    "host_show": { "name": "host_show", "value": "$isSet($host$)" }
  }
},
"visualizations": {
  "viz_detail": {
    "type": "splunk.table",
    "containerOptions": { "visibility": { "showConditions": ["host_show"] } }
  }
}
```

### `rejects` on a viz

```json
"host_hide": { "name": "host_hide", "value": "$isSet($host$)" }
```
```json
"containerOptions": { "visibility": { "hideConditions": ["host_hide"] } }
```

## Report notes

- Note where a single condition was reused across multiple vizzes (de-duplication).
- Note any empty-string sentinel that was converted to an equality check rather than `$isSet`.
