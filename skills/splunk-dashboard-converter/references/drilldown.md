# Drilldown Conversion

Classic `<drilldown>` blocks convert to entries in a viz's `eventHandlers` array.

## When to load

Load when the classic XML has a `<drilldown>` element inside a viz (`<chart>`, `<table>`, `<single>`, `<event>`, `<map>`).

## Supported conversions

- `<set token>` → `drilldown.setToken`.
- `<unset token>` → `drilldown.unsetTokens`.
- `<condition field="...">` (field-scoped) → `drilldown.setToken` with `fields`.
- `<link>` navigation → `drilldown.linkToSearch` / `linkToReport` / `linkToDashboard` / `customUrl`.
- `<option name="drilldown">` (`cell` / `all` / `row`) → `options.drilldown` on the viz.

## Unsupported — warn or ask

- `<drilldown><condition value="...">` (value-based routing) — skip with warning.
- `<drilldown><eval>` (computed drilldown targets) — skip with warning.

## Conversion rules

### `<set token>` → `drilldown.setToken`

- `<set token="X">$row.Y$</set>` → `{ "token": "X", "key": "row.Y.value" }`
- `<set token="X">$click.value$</set>` → `{ "token": "X", "key": "value" }`
- `<set token="X">staticValue</set>` → `{ "token": "X", "value": "staticValue" }`
- `<option name="drilldown">cell</option>` / `all` / `row` → carry over as `options.drilldown` on the viz.

### `<unset token>` → `drilldown.unsetTokens`

- Multiple `<unset>` elements → collect all names into one `tokenNames` array.
- Same drilldown with both `<set>` and `<unset>` → emit both handler types in the `eventHandlers` array.

### `<condition field="...">` → field-scoped `setToken`

- Maps the `field` to the `fields` option on `drilldown.setToken`.
- Multiple `<condition field>` blocks → one `drilldown.setToken` per block.
- A `<condition>` with no `field` attribute (catch-all) → emit without `fields`.

### `<link>` → navigation handler

Parse the href to determine type:

| Href pattern | Studio handler type |
|---|---|
| `search?q=...` | `drilldown.linkToSearch` |
| `/app/<app>/report?s=...` | `drilldown.linkToReport` |
| `/app/<appName>/<dashboardName>` | `drilldown.linkToDashboard` |
| `http://` or `https://` | `drilldown.customUrl` |

- **`linkToSearch` `type: "auto"`** (default): derives search from the viz's datasource with click context. Use when the classic query contains `$row.*$` or other click tokens.
- **`linkToSearch` `type: "custom"`**: uses `query` verbatim. Only for fully static queries with no click tokens.
- **`linkToDashboard`/`linkToReport`** query-string token extraction — strip `$` delimiters from token values:
  - `?form.<token>=$click.value$` → `{ "token": "form.<token>", "value": "click.value" }`
  - `?form.<token>=$row.<field>.value$` → `{ "token": "form.<token>", "value": "row.<field>.value" }`
  - Static: `?form.<token>=someValue` → `{ "token": "form.<token>", "value": "someValue" }`
  - No query params → `"tokens": []`

## Examples

### `<set token>` — setToken

```json
"eventHandlers": [
  {
    "type": "drilldown.setToken",
    "options": {
      "tokens": [
        { "token": "<token_name>", "key": "row.<field>.value" }
      ]
    }
  }
]
```

### `<unset token>`

```json
{
  "type": "drilldown.unsetTokens",
  "options": { "tokenNames": ["X"] }
}
```

### `<condition field="...">` — field-scoped

```xml
<drilldown>
  <condition field="host">
    <set token="selected_host">$click.value$</set>
  </condition>
</drilldown>
```
→
```json
{
  "type": "drilldown.setToken",
  "options": {
    "fields": ["host"],
    "tokens": [{ "token": "selected_host", "key": "value" }]
  }
}
```

### `drilldown.linkToSearch`

```json
{
  "type": "drilldown.linkToSearch",
  "options": { "type": "auto", "newTab": true }
}
```

### `drilldown.linkToReport`

```json
{
  "type": "drilldown.linkToReport",
  "options": { "app": "<app>", "report": "<decoded report name>", "newTab": true }
}
```

### `drilldown.linkToDashboard`

```json
{
  "type": "drilldown.linkToDashboard",
  "options": {
    "app": "<appName>",
    "dashboard": "<dashboardName>",
    "newTab": true,
    "tokens": [{ "token": "<token>", "value": "<clickToken>" }]
  }
}
```

### `drilldown.customUrl`

```json
{
  "type": "drilldown.customUrl",
  "options": { "url": "<href verbatim>", "newTab": true }
}
```

## Report notes

- List every `<drilldown><condition value="...">` (value-based routing) and `<drilldown><eval>` (computed target) as a skipped feature.
- Note any `<link>` whose href pattern did not match a known handler type and was skipped or approximated.
