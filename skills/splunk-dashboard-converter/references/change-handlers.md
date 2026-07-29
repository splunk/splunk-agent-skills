# `<change>` Event Handler Conversion

Input `<change>` handlers convert to `expressions.conditions` + `expressions.eval` ternaries.

## When to load

Load when the classic XML has a `<change>` element inside an `<input>`.

## Supported conversions

- `<condition value="x">` equality matches → named condition + eval ternary.
- `<condition match="...">` SPL boolean expressions → JSONata condition + eval ternary.
- Nested `<condition>` elements → flattened with `and`.
- Catch-all `<condition>` (no `value`/`match`) → final else in the ternary chain.
- `<set>` and `<unset>` inside a condition → eval entry with `null` for the unset branch.

## Unsupported — warn or ask

- A `<change>` handler that sets tokens with logic that can't be expressed as condition + eval ternaries (SPL function calls, multi-step eval chains) — list in skipped features.

> If the only effect is setting a token another input's search depends on, use `isCascadingInput` instead — no explicit `<change>` conversion needed. See `form-inputs.md`.

## Conversion rules

1. Each `<condition value="x">` → named condition `$inputToken$ = 'x'`. Each `<set>` → eval ternary.
2. `<unset>` → `null` in the else branch.
3. Multiple `<set>` in one condition → one eval entry per token, same condition key.
4. Condition key naming: `<inputToken>_is_<value>` (e.g. `view_is_steps`).
5. For `<condition match="...">`, translate the SPL boolean to JSONata:
   - `$value$ == "x"` → `$<inputToken>$ = 'x'`
   - `$value$ != "x"` → `$<inputToken>$ != 'x'`
   - `$value$ == ""` → `$<inputToken>$ = ''`
   - `&quot;` → `'`, `&amp;` → `&`, `&lt;` → `<`, `&gt;` → `>`
6. Nested conditions → flatten with `and`: each ancestor condition's predicate is `and`-ed with its children's predicates.
7. Catch-all `<condition>` (no `value`/`match`) is always true — tokens it sets become the final else in the ternary chain (append `: $inputToken$`).

## Examples

### `<condition value="x">` — equality match

```xml
<input type="link" token="view">
  <change>
    <condition value="steps">
      <set token="showSteps">true</set>
      <unset token="showLogs"/>
    </condition>
    <condition value="logs">
      <set token="showLogs">true</set>
      <unset token="showSteps"/>
    </condition>
  </change>
</input>
```
→
```json
"conditions": {
  "view_is_steps": { "name": "view_is_steps", "value": "$view$ = 'steps'" },
  "view_is_logs":  { "name": "view_is_logs",  "value": "$view$ = 'logs'" }
},
"eval": {
  "showSteps": { "name": "showSteps", "value": "$condition:view_is_steps$ ? 'true' : null" },
  "showLogs":  { "name": "showLogs",  "value": "$condition:view_is_logs$ ? 'true' : null" }
}
```

### `<condition match="...">` — expression match

```xml
<condition match="$value$ == &quot;true&quot;">
  <set token="show_panel">true</set>
</condition>
```
→
```json
"html1_is_true": { "name": "html1_is_true", "value": "$html_summary1$ = 'true'" }
"show_panel": { "name": "show_panel", "value": "$condition:html1_is_true$ ? 'true' : null" }
```

### Nested conditions — flatten with `and`

```xml
<condition value="production">
  <condition field="status" value="critical">
    <set token="action">immediate</set>
  </condition>
</condition>
```
→
```json
"is_prod_critical": { "name": "is_prod_critical", "value": "$env$ = 'production' and $status$ = 'critical'" }
"action": { "name": "action", "value": "$condition:is_prod_critical$ ? 'immediate' : null" }
```

### Catch-all `<condition>` (no value/match)

```xml
<condition><set token="filter">$value$</set></condition>
```
→ append `: $inputToken$` as the final else.

## Report notes

- List every `<change>` handler that could not be expressed as condition + eval ternaries (SPL functions, multi-step eval chains) as a skipped feature.
- Note where a `<change>` handler was replaced by `isCascadingInput` rather than an explicit conversion.
