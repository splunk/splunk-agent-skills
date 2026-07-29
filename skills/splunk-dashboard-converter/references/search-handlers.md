# Search Event Handlers (`<done>` / `<progress>`)

Classic `<done>` and `<progress>` handlers that set tokens from job metadata or result fields convert to Smart Sources + `expressions.eval`.

## When to load

Load when the classic XML has `<done>` or `<progress>` elements inside a `<search>`.

## Supported conversions

- `<done>` handlers that set tokens from `$job.*$` or `$result.*$`.
- `<progress>` handlers that set tokens from `$job.*$`.
- The job-token mappings listed under Conversion rules below.

## Unsupported — warn or ask

- `<error>`, `<finalized>`, `<cancelled>`, `<fail>`, `<preview>` — **no Studio equivalent**. List in skipped features.
- Any `$job.*$` token not in the mapping table — emit a warning.

## Conversion rules

1. Set `enableSmartSources: true` on every datasource with `<done>` / `<progress>` handlers.
2. For each `<set token="x">$job.*$</set>` or `<set token="x">$result.*$</set>`, create `expressions.eval` using the mapping table below.
3. Guard `<done>` result field reads with `$<dsName>:job.done$ ? ... : ""`.
4. For `<progress>` handlers, use `?? 0` / `?? ""` fallbacks.
5. Rewrite all downstream `$tokenName$` refs to `$eval:tokenName$`.

**`<dsName>`** = the datasource's `name` field (not the key like `ds_foo`).

### `<done>` / `<progress>` containing `<eval>`

When the handler computes a token with an `<eval>` (rather than a plain `$job.*$` / `$result.*$` passthrough), translate the eval per `references/eval-expressions.md` — its Step 1 classification applies unchanged (simple function → JSONata; conditional → ternary; complex nesting/`mvexpand`/`rex` → best-effort, flag for review). The only handler-specific additions are: enable `enableSmartSources: true`, and rewrite any `$job.*$` / `$result.*$` field reads inside the eval to their `$<dsName>:...$` Smart Source form (mapping table below). Do not treat a complex eval as unsupported — follow the eval ladder and flag low-confidence results in the report.

### Job token mapping

Direct mappings — classic SXML token to its Smart Source key (verified against UDF `dashboard-types/src/Token.ts` and the Splunk 10.2 job-properties doc):

| SXML source token | UDF target Smart Source token | Notes |
|---|---|---|
| `$job.sid$` | `$<dsName>:job.sid$` | Search ID |
| `$job.resultCount$` | `$<dsName>:job.resultCount$` | Result count |
| `$job.isDone$` | `$<dsName>:job.done$` | boolean |
| `$job.isFailed$` | `$<dsName>:job.failed$` | boolean |
| `$job.isPaused$` | `$<dsName>:job.pause$` | boolean (status key is `pause`, not `paused`) |
| `$job.doneProgress$` | `$<dsName>:job.percentComplete$` | **Scale differs** — classic `doneProgress` is `0`–`1.0`; Smart Source `percentComplete` is `0`–`100`. If downstream SPL/logic expects the 0–1.0 range, divide by 100: `$<dsName>:job.percentComplete$ / 100` |
| `$job.dispatchState$` | `$<dsName>:job.status$` | e.g. `"done"` |
| `$job.isRealTimeSearch$` | `$<dsName>:job.isRealTimeSearch$` | boolean |
| `$job.messages$` | `$<dsName>:job.messages$` | array of message strings |
| `$result.<field>$` | `$<dsName>:result.<field>$` | First result row field |

### No direct classic token

- **`job.hasResults`**: there is no classic `$job.hasResults$` token. Smart Sources expose `$<dsName>:job.hasResults$` natively (boolean) — use it directly, or derive it as `$<dsName>:job.resultCount$ > 0`.
- **`job.lastUpdated`**: Smart Source-only key (`$<dsName>:job.lastUpdated$`); no classic SXML equivalent. Only emit it if a target explicitly needs it, not as a translation of a classic token.

Other Smart Source keys with no classic counterpart: `job.startTime`, `job.fields`. The full status-boolean set follows the `JobStatus` values (`queued`, `parsing`, `running`, `pause`, `finalizing`, `failed`, `stopped`, `done`, `canceled`, `refreshing`).

## Examples

### `<done>` with `$result.*$`

```xml
<search id="base">
  <query>`my_macro("$stack$")`</query>
  <done><set token="column_filter">$result.filter$</set></done>
</search>
```
→
```json
"ds_base": {
  "type": "ds.search",
  "name": "base",
  "options": { "query": "`my_macro(\"$stack$\")`", "enableSmartSources": true }
},
"expressions": {
  "eval": {
    "column_filter": {
      "name": "column_filter",
      "value": "$base:job.done$ ? $base:result.filter$ : \"\""
    }
  }
}
```

### `<progress>` with `$job.*$`

```xml
<search id="search">
  <query>index=main | stats count by host</query>
  <progress>
    <set token="search_progress">$job.doneProgress$</set>
    <set token="result_count">$job.resultCount$</set>
  </progress>
</search>
```
→
```json
"ds_search": {
  "type": "ds.search",
  "name": "my_search",
  "options": { "query": "index=main | stats count by host", "enableSmartSources": true }
},
"expressions": {
  "eval": {
    "search_progress": {
      "name": "search_progress",
      "value": "$my_search:job.percentComplete$ ?? 0"
    },
    "result_count": {
      "name": "result_count",
      "value": "$my_search:job.done$ ? $my_search:job.resultCount$ : 0"
    }
  }
}
```

## Report notes

- List every `<error>` / `<finalized>` / `<cancelled>` / `<fail>` / `<preview>` handler as a skipped feature.
- For any `<done>` / `<progress>` handler containing an `<eval>`, report it per `eval-expressions.md` with its confidence level (complex evals translated best-effort flagged as low confidence for manual review).
- Note any `$job.*$` token that had no mapping and was warned/skipped.
- If `doneProgress` was converted, note whether the 0–1.0 → 0–100 scale change required a `/ 100` adjustment in downstream logic.
