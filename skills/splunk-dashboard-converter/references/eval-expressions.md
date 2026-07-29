# `<eval>` Expression Conversion

Classic `<eval token="x">expression</eval>` computes a token from SPL eval. Studio equivalent: `expressions.eval`, evaluated reactively. Result available as `$eval:x$`.

## When to load

Load when the classic XML has an `<eval token=...>` element (standalone, inside `<search>`, or inside `<drilldown>`/`<change>`).

## Supported conversions

- **Passthrough** (`$row.field$`, `$click.value$`, bare token) → `drilldown.setToken` with `key`.
- **Simple function** (`strftime`, `tostring`, `upper`, etc.) → JSONata in `expressions.eval`.
- **Conditional** (`if`, `case`, `coalesce`, `isnotnull`) → JSONata ternary in `expressions.eval`.
- **Complex** (multi-level nesting, `mvexpand`, `rex`, multi-value) → best-effort translate, flag for review.

## Unsupported — warn or ask

- SPL functions with no JSONata equivalent: `searchmatch()`, `exact()`, `commands()`, `md5()`, `sha*()`, `sigfig()`, `tostring(x, "duration")`.
- `relative_time` **snap ops** (`@d`, `@h`) — warn, compute in SPL.

## Conversion rules

### Step 1 — classify

| Class | Pattern | Action |
|---|---|---|
| Passthrough | Body is just `$row.field$`, `$click.value$`, or bare token ref | Convert as `drilldown.setToken` with `key` |
| Simple function | Single SPL function: `strftime`, `tostring`, `upper`, etc. | Translate to JSONata → `expressions.eval` |
| Conditional | `if(...)`, `case(...)`, `coalesce(...)`, `isnotnull(...)` | Translate to JSONata ternary → `expressions.eval` |
| Complex | Multi-level nesting, `mvexpand`, `rex`, multi-value fields | Best-effort translate, flag for user review |

### Step 2 — passthrough as setToken

```xml
<eval token="selectedId">$row.id$</eval>
```
→
```json
{ "type": "drilldown.setToken", "options": { "tokens": [{ "token": "selectedId", "key": "row.id.value" }] } }
```

### Step 3 — SPL eval → JSONata translation table

| SPL | JSONata | Notes |
|---|---|---|
| `tostring(x)` | `$string(x)` | |
| `tonumber(x)` | `$number(x)` | |
| `upper(x)` | `$uppercase(x)` | |
| `lower(x)` | `$lowercase(x)` | |
| `len(x)` | `$length(x)` | |
| `substr(x, s, l)` | `$substring(x, s, l)` | |
| `strftime(t, fmt)` | `$fromMillis($number(t) * 1000, jsonataFmt)` | SPL = epoch seconds, JSONata = ms. Translate format string (see below) |
| `strptime(s, fmt)` | `$toMillis(s, jsonataFmt) / 1000` | Returns epoch seconds |
| `if(cond, a, b)` | `cond ? a : b` | |
| `case(c1, v1, c2, v2, d)` | `c1 ? v1 : c2 ? v2 : d` | Chain ternaries |
| `isnotnull(x)` | `$exists(x) and x != null` | |
| `isnull(x)` | `$not($exists(x)) or x = null` | |
| `coalesce(a, b, ...)` | `a ?? b ?? ...` | |
| `x + y` (numeric) | `$number(x) + $number(y)` | |
| `x + y` (string concat) | `x & y` | JSONata uses `&` for strings |
| `x == y` | `x = y` | **Critical**: `==` is a syntax error in JSONata |
| `mvcount(x)` | `$count(x)` | |
| `mvindex(x, i)` | `x[i]` | Zero-indexed |
| `tostring(x, "commas")` | `$formatNumber(x, "#,###")` | |
| `tostring(x, "hex")` | `$formatBase($number(x), 16)` | |
| `tostring(x, "duration")` | — | No equivalent; compute in SPL |
| `now()` | `$floor($millis() / 1000)` | epoch seconds |
| `trim(x)` | `$trim(x)` | |
| `ltrim(x)` / `rtrim(x)` | `$trim(x)` | JSONata strips both sides; note difference |
| `replace(x, pat, repl)` | `$replace(x, /pat/, repl)` | |
| `split(x, d)` | `$split(x, d)` | |
| `mvjoin(x, d)` | `$join(x, d)` | |
| `contains(x, y)` | `$contains(x, y)` | |
| `abs/ceil/floor/round/sqrt/power/random` | `$abs/$ceil/$floor/$round/$sqrt/$power/$random` | Direct mapping |
| `max(a, b, ...)` | `$max([a, b, ...])` | Wrap in array |
| `min(a, b, ...)` | `$min([a, b, ...])` | Wrap in array |
| `urlencode(x)` | `$encodeUrlComponent(x)` | |
| `urldecode(x)` | `$decodeUrlComponent(x)` | |
| `relative_time(t, offset)` | arithmetic only | Simple: `-7d` → `$millis() - (7*24*60*60*1000)`. **Snap ops** (`@d`, `@h`) → warn, compute in SPL |

**No JSONata equivalent — warn**: `searchmatch()`, `exact()`, `commands()`, `md5()`, `sha*()`, `sigfig()`

#### `strftime` format code → JSONata picture string

| strftime | JSONata | Meaning |
|---|---|---|
| `%Y` | `[Y0001]` | 4-digit year |
| `%y` | `[Y01]` | 2-digit year |
| `%m` | `[M01]` | 2-digit month |
| `%d` | `[D01]` | 2-digit day |
| `%H` | `[H01]` | 24h hour |
| `%I` | `[h01]` | 12h hour |
| `%M` | `[m01]` | minute |
| `%S` | `[s01]` | second |
| `%p` | `[P]` | AM/PM |
| `%A` | `[FNn]` | Full weekday |
| `%a` | `[FNn,3-3]` | Abbreviated weekday |
| `%B` | `[MNn]` | Full month name |
| `%b` | `[MNn,3-3]` | Abbreviated month |

Example: `strftime(epoch, "%m/%d/%Y %H:%M:%S")` → `$fromMillis($number(epoch) * 1000, "[M01]/[D01]/[Y0001] [H01]:[m01]:[s01]")`

### Step 4 — emit `expressions.eval`

```json
"expressions": {
  "eval": {
    "<key>": {
      "name": "<tokenName>",
      "value": "<JSONata expression>"
    }
  }
}
```

Only **`name`** is load-bearing: UDF builds the env token as `eval:` + `name`, so the result is referenced as `$eval:<name>$` (see Step 5). The object key is an arbitrary dictionary id — it does **not** need an `eval_` prefix and does **not** need to match `name`. Use the token name (`"label"`) or follow UDF's editor convention of sequential ids (`eval_1`, `eval_2`); either works.

### Step 5 — rewrite downstream references

Replace every `$tokenName$` in the definition with `$eval:tokenName$`:
- SPL queries: `$eval:tokenName$`
- viz `title`: `$eval:tokenName$`
- `splunk.markdown` content: `$eval:tokenName$`
- Other `expressions.eval`/`conditions` values: `$eval:tokenName$`

## Examples

### strftime → `expressions.eval`

```xml
<eval token="label">strftime(earliest, "%m/%d/%Y %H:%M:%S")</eval>
```
→
```json
"label": {
  "name": "label",
  "value": "$fromMillis($number($earliest$) * 1000, \"[M01]/[D01]/[Y0001] [H01]:[m01]:[s01]\")"
}
```

## Report notes

Always list every converted `<eval>` in the report:
```
label: strftime(earliest, "%m/%d/%Y %H:%M:%S")
  → JSONata: $fromMillis($number($earliest$) * 1000, "[M01]/[D01]/[Y0001] [H01]:[m01]:[s01]")
  → Referenced as: $eval:label$
  → Confidence: high
```

Confidence levels: **high** (direct mapping), **medium** (structural translation), **low** (complex, manual review required).

**Important semantic difference** — classic `<eval>` inside `<drilldown>` is **click-triggered**; `expressions.eval` is **reactive** (re-evaluates when any referenced token changes). Note this in the report.
