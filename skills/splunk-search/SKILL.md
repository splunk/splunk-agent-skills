---
name: splunk-search
description: Run bounded Splunk SPL searches through the splsearch CLI, save large result sets as local SQLite tables, and inspect those saved tables with focused summaries, text search, ordered events, or bounded SQL.
license: Apache-2.0
allowed-tools:
  - shell
metadata:
  splunk:
    domain: search
    products:
      - splunk-enterprise
      - splunk-cloud-platform
    entities:
      - SPL searches
      - saved result tables
      - local SQLite result stores
      - Splunk auth sessions
      - search events
    triggers:
      - Splunk Search
      - splsearch
      - run a Splunk search
      - query Splunk results
      - incident search evidence
    not-for:
      - production write actions
      - mutating Splunk data
      - secret handling
      - broad raw result dumps
    outcomes:
      - bounded Splunk search execution
      - saved result-table analysis
      - compact incident evidence
      - support-ready search summaries
---

# Splunk Search

Use Splunk Search when a user needs read-only Splunk evidence but the result
set may be too large for the agent context. The `splsearch` CLI writes search
results to a local SQLite table and returns compact JSON with the table name.
Treat the saved table as the artifact, then bring only focused summaries,
small samples, or exact evidence back into the conversation.

## Prerequisites

The `splsearch` CLI is installed and available on `PATH`, or the runtime
provides an equivalent `splsearch` binary for this skill. In this repository,
the packaged Go source for that CLI lives in `tools/splsearch`. From the
repository root, build the tool artifact with
`make -C tools/splsearch build`, then put `tools/splsearch/bin` on `PATH`
before using the skill locally.

The user has read-only access to the target Splunk Enterprise or Splunk Cloud
Platform environment and can provide the target URL, time window, and search
intent. Do not ask for passwords, cookies, tokens, or raw credential files.

## When to Use

Use this skill for Splunk log investigation, production or staging incident
triage, saved result-table analysis, and requests that mention `splsearch`,
Splunk search, SPL execution, or querying saved Splunk results.

Do not use this skill for changing Splunk configuration, editing knowledge
objects, deleting data, restarting services, creating alerts, modifying
indexes, or handling secrets. If the user wants SPL authoring help without
running a search or inspecting saved results, answer without invoking this
skill.

## Workflow Overview

1. Identify the target Splunk URL, time window, impact scope, and strongest
   available filters.
2. Check cached auth with `splsearch auth status --output=json` when the target
   is unclear.
3. Validate a specific target with
   `splsearch auth status --url=<splunk-url> --output=json` before deciding
   whether browser login is needed.
4. Run one bounded table-writing search with a useful time range and result
   table name.
5. Keep the returned `table` name and use it for every follow-up command.
6. Review any saved warnings before continuing. If a broad fetch was
   intentional, record that by accepting the warning.
7. Inspect schema or metadata before writing SQL against the saved table.
8. Use text search, summary, ordered events, or bounded SQL to reduce the saved
   results locally.
9. Bring only the compact answer set, evidence snippets, and command summary
   back into chat.
10. Keep the result table while the incident or handoff is active, then drop it
   when evidence has been captured elsewhere.

Do not shrink the Splunk search just because the agent context is small. Fetch
a relevant, bounded slice into SQLite, then reduce locally. Avoid `| head 10`,
tiny limits, or immediate output unless the task truly needs only a tiny sample.

Run table-writing searches serially. Parallel searches can contend for the
local result database if they complete at the same time.

## Commands

- `splsearch auth status --output=json` lists cached auth inventory.
- `splsearch auth status --url=<splunk-url> --output=json` validates one
  target without forcing a fresh login.
- `splsearch search --url=<splunk-url> --query='<SPL>' --earliest=-15m --result-table=<table>`
  runs a bounded Splunk search and saves results locally.
- `splsearch search --url=<splunk-url> --query='<SPL>' --earliest=-15m --result-table=<table> --progress=jsonl`
  writes parseable progress events to stderr while keeping stdout as the final
  JSON result.
- `splsearch results-list --limit=20` rediscovers saved tables and follow-up
  commands.
- `splsearch result-info --table=<table>` inspects metadata, original SPL,
  time bounds, row counts, and warnings.
- `splsearch result-warnings accept --table=<table> --code=<warning-code>`
  marks a reviewed local warning as intentional.
- `splsearch result-schema --table=<table>` inspects saved columns before SQL.
- `splsearch result-text-search --table=<table> --query='<text>' --limit=20`
  performs local BM25 text recall over saved rows.
- `splsearch result-summary --table=<table> --group-by=<field> --limit=20`
  builds first-pass aggregates.
- `splsearch result-events --table=<table> --field=<field> --value=<value>`
  returns compact ordered event sequences.
- `splsearch result-search --table=<table> --query='<SQL SELECT>' --limit=20`
  runs bounded SQL against the saved table as `results`.
- `splsearch results-drop --table=<table>` removes a local saved table after
  the evidence is no longer needed.

## Examples

Investigate recent API errors without dumping raw rows:

```sh
splsearch search --url=<splunk-url> --query='index=app_logs component=api severity=ERROR' --earliest=-30m --result-table=app_errors
splsearch result-info --table=app_errors
splsearch result-schema --table=app_errors
splsearch result-summary --table=app_errors --group-by=component --limit=20
splsearch result-events --table=app_errors --request-id=abc-123 --limit=100
splsearch result-search --table=app_errors --query='SELECT component, count(*) AS errors FROM results GROUP BY component ORDER BY errors DESC LIMIT 20' --limit=20
```

Find a text symptom inside a known saved table:

```sh
splsearch result-text-search --table=app_errors --query='timeout retry exhausted' --limit=20
```

Clean up after the handoff:

```sh
splsearch results-drop --table=app_errors
```

## Troubleshooting

If target validation fails with a transport error, inspect the structured
`error_code`, `operation`, `retryable`, `diagnostic_hint`, and `message`
fields. Do not treat DNS, network, or proxy failures as proof that credentials
need refreshing.

If browser auth fails, keep the compact stdout JSON in context and inspect
`error_code`, `operation`, `retryable`, `retryable_after_environment_change`,
`remediation_code`, `diagnostic_hint`, `launch_error_summary`, `message`,
`requested_channel`, `attempted_channel`, and `fallback_used`. Do not paste
diagnostic files or browser logs into chat unless the user explicitly asks.

If a long-running search appears stuck, use progress output rather than
guessing. `--progress=jsonl` keeps stdout machine-readable and puts events such
as dispatch, running state, fetch rows, write rows, ETA, and scan counts on
stderr. If progress is active but scan counts are huge, narrow the SPL or time
window instead of blindly extending `--timeout`.

If `search` returns `ok:false` or `table_created:false`, stop and report that
no saved table exists. Do not continue with invented table names.

If a saved table has active warnings, review them before using the result.
Broad or unlimited searches can be valid during an incident, but they must be
intentional and named in the evidence.

If a saved table disappears, explain that local result tables are ephemeral and
rerun the bounded Splunk search if the evidence is still needed.

## Safety

Do not print cookies, auth files, tokens, or `~/.config/splsearch/auth.json`.
Do not read `~/.config/splsearch/results.sqlite` directly unless the CLI cannot
answer the question. Do not paste broad raw result sets into chat.
