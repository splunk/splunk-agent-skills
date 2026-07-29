# `<init>` Token Defaults

`<init>` blocks set default token values before the dashboard loads. Convert to `defaults.tokens.default`.

## When to load

Load when the classic XML has an `<init>` block.

## Supported conversions

- `<set token="x">value</set>` inside `<init>` → `defaults.tokens.default.<x>.value`.
- `prefix` + value + `suffix` → resolved into a single `value` string.

## Unsupported — warn or ask

- `<unset>` inside `<init>` → omit from defaults entirely.
- Nested `<condition>` inside `<init>` → flatten to the first matching condition's value, or omit if the conditional logic is too complex (warn).

## Conversion rules

```json
"defaults": {
  "tokens": {
    "default": {
      "<token_name>": {
        "value": "<prefix><default_value><suffix>"
      }
    }
  }
}
```

- Resolve `prefix + value + suffix` into a single string for `value`.
- The map key under `default` **is** the token name; reference it downstream as `$<token_name>$` (not `$default:...$`).
- Keep `$token$` references live in SPL — do **not** bake the resolved value into the query.

## Examples

```xml
<init>
  <set token="env">prod</set>
  <set token="time_range">-24h,now</set>
</init>
```
→
```json
"defaults": {
  "tokens": {
    "default": {
      "env":        { "value": "prod" },
      "time_range": { "value": "-24h,now" }
    }
  }
}
```

## Report notes

- Note any `<unset>` inside `<init>` that was omitted.
- Note any nested `<condition>` inside `<init>` that was flattened or skipped due to complexity.
