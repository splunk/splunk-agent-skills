# Splunk Agent Skills

> [!WARNING]
> These skills are experimental. They are not covered by existing Splunk
> support contracts. Review [Support](SUPPORT.md) before using them.

This repository contains reusable skills for AI agents working with
Splunk.

## Skills

| Skill | Purpose |
| --- | --- |
| [`splunk-search`](skills/splunk-search/SKILL.md) | Run bounded Splunk SPL searches through the `splsearch` CLI, save large result sets as local SQLite tables, and inspect them with focused summaries, text search, ordered events, or bounded SQL. |
| [`splunk-dashboard-converter`](skills/splunk-dashboard-converter/SKILL.md) | Convert classic Splunk Simple XML dashboards (version 1) into Dashboard Studio (version 2), preserve every SPL query verbatim, and return the Studio JSON definition to the caller. |
| [`custom-visualization-builder`](skills/custom-visualization-builder/SKILL.md) | Scaffold, build, package, and install a custom visualization into Splunk using the `dashboard-studio-extension` framework. |


Each skill is self-contained under `skills/`. The lowercase `tools/splsearch`
directory contains the Go source for the helper CLI used by `splunk-search`.

## Install and use

Copy the desired directory from `skills/` into the skills directory configured
for an AI coding agent that supports `SKILL.md` files. Read the selected
`SKILL.md` before use; it defines the prerequisites, workflow, and safety
boundaries for that skill.

The `splunk-search` skill also requires the `splsearch` CLI on `PATH`. With Go
1.21 or later installed, build it from the repository root:

```sh
make -C tools/splsearch build
export PATH="$PWD/tools/splsearch/bin:$PATH"
splsearch --help
```

Then use the installed skill through your agent according to its normal skill
invocation workflow.

## Policies

- [Support](SUPPORT.md)
- [Security](SECURITY.md)
- [Apache License 2.0](LICENSE)
