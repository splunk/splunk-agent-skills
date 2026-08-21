# splsearch

`splsearch` lets agents run bounded Splunk searches without placing large
result sets directly in model context. A normal search writes rows to a local
SQLite table and prints compact JSON containing the saved table name.

This directory is the repo-native Go source for the `splsearch` tool artifact
used by `skills/splunk-search`. It intentionally contains only the generic CLI
implementation. Experiment-only incident evaluation scripts, private
environment knowledge, service-specific fixtures, and local author paths are
not part of this packaged tool.

## Build

From this directory:

```sh
make build
bin/splsearch --help
```

From this repository's root:

```sh
make -C tools/splsearch build
tools/splsearch/bin/splsearch --help
```

## Authentication

Use browser auth for Splunk targets:

```sh
splsearch auth status
splsearch auth status --url=https://splunk.example.com --output=json
splsearch auth login --url=https://splunk.example.com --output=json
```

`auth status` without `--url` is a local credential inventory. JSON inventory
includes expired saved targets with per-server state, so agents can discover
stale Splunk targets without already knowing the URL. Use
`auth status --url=<splunk-url> --output=json` when a target-specific workflow
needs live validation.

On macOS, browser auth prefers installed stable Chrome when it is available,
then uses bundled Chromium as the fallback path. Set
`SPLSEARCH_BROWSER_CHANNEL=chrome` or `SPLSEARCH_BROWSER_CHANNEL=msedge` to
force a supported installed browser channel. Set `SPLSEARCH_BROWSER_CHANNEL=`
to force bundled Chromium.

## Search Flow

```sh
splsearch search --query='<SPL>' --result-table=<table>
splsearch search --query='index=_internal | stats count by sourcetype' --progress=jsonl
splsearch result-info --table=<table>
splsearch result-warnings accept --table=<table> --code=full_fetch
splsearch result-schema --table=<table>
splsearch result-text-search --table=<table> --query='<text>' --limit=20
splsearch result-summary --table=<table> --group-by=<field> --time-from=<from> --time-to=<to> --limit=20
splsearch result-events --table=<table> --field=<field> --value=<value> --limit=100
splsearch result-search --table=<table> --query='<SQL SELECT>' --limit=20
splsearch results-drop --table=<table>
```

`splsearch search` keeps stdout as the final JSON object and writes progress to
stderr by default. Use `--progress=jsonl` for parseable progress events or
`--progress=off` to suppress progress. Search timeouts return
`error_code:"search_timeout"` with `last_progress` when Splunk reported job
status before the timeout.

If `search` fails before creating a saved table, including during automatic
browser authentication, JSON output keeps the human `message` and adds
parseable fields including `error_code`, `operation`, `retryable`,
`table_created:false`, and `diagnostic_hint`. Browser-launch failures also add
`launch_error_summary`; macOS bootstrap permission failures additionally set
`retryable_after_environment_change:true` and
`remediation_code:"retry_from_unsandboxed_environment"`.

Useful saved-table queries:

```sh
splsearch result-text-search --table=<table> --query='request_remote_tok 401 Unauthorized' --limit=20
splsearch result-search --table=<table> --query="SELECT json_extract(_raw, '$.customer') AS customer, count(*) AS rows FROM results GROUP BY customer ORDER BY rows DESC" --limit=20
splsearch result-summary --table=<table> --group-by=component --metric=latency_ms --thresholds=250,1000 --order-by=gte_1000 --limit=20
splsearch result-events --table=<table> --request-id=<id> --limit=100
```

## Safety

- Do not print cookies, auth files, tokens, or `~/.config/splsearch/auth.json`.
- Do not read `~/.config/splsearch/results.sqlite` directly unless the CLI
  cannot answer the question.
- Do not paste broad raw result sets into chat.
- Result tables may be cleaned after 24 hours; rerun the bounded Splunk search
  if a table disappears.
