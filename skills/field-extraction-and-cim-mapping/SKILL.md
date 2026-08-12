---
name: field-extraction-and-cim-mapping
description: Author, explain, diagnose, and validate Splunk search-time field extractions and mappings to Common Information Model (CIM) datasets from representative events, configuration, and search evidence. Use for automatic key-value extraction, regex or delimiter extraction, props.conf EXTRACT and REPORT/transforms.conf rules, SPL extraction commands, aliases, calculated fields, lookups, event types, tags, value normalization, CIM field mapping, and missing or incorrect normalization; do not use for deployment execution, ingestion transport, app installation, knowledge-object governance, data-model acceleration, or unrelated search/dashboard repair.
license: Apache-2.0
allowed-tools:
  - web
metadata:
  splunk:
    domain: data-normalization
    products:
      - splunk-cloud-platform
      - splunk-enterprise
    entities:
      - search-time field extractions
      - props.conf and transforms.conf
      - field aliases, calculated fields, and lookups
      - event types and tags
      - CIM datasets and normalized fields
    triggers:
      - extract fields from sample events
      - write EXTRACT or REPORT rules
      - map fields to CIM
      - validate CIM normalization
      - diagnose missing fields, aliases, lookups, tags, or event types
    not-for:
      - data-model acceleration or tstats tuning
      - HEC, transport, index routing, or source onboarding
      - app installation, Cloud changes, or cluster deployment
      - knowledge-object governance or lifecycle review
      - unrelated SPL, search, or dashboard troubleshooting
    outcomes:
      - cited extraction-mechanism guidance
      - evidence-bound extraction draft
      - semantic CIM mapping plan
      - unverified validation workflow or evidence-based result
      - bounded normalization-gap diagnosis
---

# Field Extraction and CIM Mapping

Produce documentation-backed, evidence-bound guidance for search-time field
extraction and CIM normalization. Draft text artifacts only. Never authenticate
to, modify, install on, or deploy to a Splunk environment.

## Prerequisites

Record or ask for only the missing items material to the requested decision:

- representative sanitized raw events, including meaningful edge cases;
- sourcetype and relevant current `props.conf`, `transforms.conf`, SPL, aliases,
  calculated fields, lookups, event types, and tags;
- desired source fields and their meanings;
- target CIM dataset and installed CIM version or model details;
- Splunk product/version, persistence scope, app context, and deployment
  topology when configuration or routing depends on them; and
- observed field/search/data-model validation output when diagnosing or
  claiming validation.

Never invent event structure or field meaning. With partial evidence, first
preserve and assess every supported object-level fact. Mark each absent field
or artifact `unknown`, and gate only the conclusion it affects; missing context
must not erase supplied evidence. Clearly separate supplied observations,
documented facts, assumptions, provisional conclusions, and unverified steps.

## When to Use

Use this skill when the primary outcome is documented extraction guidance, an
evidence-bound extraction draft, a semantic CIM mapping, validation steps, or a
normalization-gap diagnosis. Apply the boundaries below to adjacent work.

## Workflow Overview

### 1. Select the extraction mechanism

Read [public-guidance.md](references/public-guidance.md). Identify the documented
mechanism that fits the event shape: automatic key-value extraction, regex or
delimiter extraction, inline `EXTRACT`, reusable `REPORT` plus
`transforms.conf`, or an ad hoc SPL command. Separate persistent knowledge
objects from search-local SPL. Treat index-time extraction as a
performance-sensitive exception, not the default.

If source events or deployment context are absent, give only cited general
guidance. End the answer with a direct request for representative events,
sourcetype, product and version, and desired persistence scope before proposing
concrete configuration; merely listing these inputs as absent does not satisfy
the request.

### 2. Draft an evidence-bound extraction

Use the smallest approach that targets the supplied samples. Provide concrete
`props.conf`, `transforms.conf`, or SPL only when representative events and
desired fields are supplied. Explain why it matches the event shape and state
assumptions about sourcetype, app context, delimiters, cardinality, multivalue
behavior, and persistent search-time versus ad hoc scope.

Flag broad unnecessary key-value extraction, unbounded variable-key expansion,
duplicated calculated-field logic, brittle sample-only matching, and premature
index-time extraction. If evidence is insufficient, ask for the missing sample
and desired fields or give only a clearly marked, unvalidated template/checklist.

### 3. Build a semantic CIM mapping plan

Name the target CIM dataset; if event meaning permits several candidates, ask
the user to choose and explain the candidates provisionally. Map by source-field
semantics, not name similarity. Where evidence permits, distinguish required
fields, recommended or expected fields, required tags, dataset constraints,
aliases, calculated fields, lookups, event types, enrichment, and value
normalization.

If the target dataset, source semantics, or installed model details are
missing, preserve any supported field facts, label candidate mappings
provisional, and request the smallest missing evidence. Direct the user to the
Data Model Editor or installed model JSON for complete constraints and inherited
fields; do not claim the public reference tables are complete.

### 4. Define or assess validation

Provide the relevant inspection path: normal search field inspection,
Pivot/Datasets, `datamodel` or `from datamodel`, `datamodelsimple`, or the CIM
Validation data model's Missing Extractions and Untagged Events datasets. State
the expected success evidence: correct values appear, required tags/event types
select the intended events, and edge cases preserve extraction behavior.

Call runtime validation `unverified` unless actual search or deployment results
are supplied. When results are absent, provide only commands and expected
observations. When results are present, assess only what they demonstrate.

Keep a validation-only answer bounded. Use one short status statement, at most
five focused validation steps, and one compact expected-evidence checklist.
Prefer one representative search per distinct validation purpose instead of
enumerating variants. Do not repeat prerequisites, boundaries, citations, or
the same caveat in multiple sections. A validation-only answer must be 900
words or fewer. If the user requests a longer runbook, first return a complete
validation answer within this limit; provide the longer runbook only afterward.

### 5. Diagnose normalization gaps

Connect every suspected missing or incorrect extraction, alias, lookup, tag,
event type, calculated field, or CIM mapping to a supplied raw event,
configuration fragment, field output, or data-model validation result. Preserve
confirmed facts even when other artifacts are absent.

Separate semantic/configuration explanations from possible deployment,
app-installation, cluster-bundle, managed Cloud change, or acceleration causes.
Provide a bounded semantic remediation plan and the exact evidence needed to
confirm it. If only symptoms are supplied, do not diagnose: ask for the smallest
safe subset of representative events, relevant current configuration, search
output, target dataset, product/version, and topology needed for the pending
decision.

## Boundaries

Keep extraction authoring and CIM-mapping semantics here. Route only work that
crosses the boundary:

- governance, ownership, naming, packaging policy, and lifecycle decisions to
  Knowledge Object Governance;
- source onboarding transport, HEC, tokens, and index routing to the relevant
  ingestion owner;
- acceleration design, `tstats` tuning, summaries, and acceleration failures to
  Data Model and Search Acceleration;
- unrelated search/dashboard remediation to its troubleshooting owner; and
- app installation, managed Cloud changes, cluster bundles, approvals, and
  production deployment to the appropriate Splunk operator.

Do not claim runtime verification, publication readiness, prevalence,
cross-system linkage, telemetry baselines, or rollback readiness without direct
evidence.

## Examples

- “Choose a persistent extraction for these sanitized events and fields.”
- “Map these existing fields to the Authentication CIM dataset.”
- “Assess these Missing Extractions results against the supplied config.”

## Troubleshooting

- No samples: provide cited mechanism guidance or an unvalidated template, then
  request representative events and desired fields.
- Partial artifacts: retain every supported fact, mark only missing facts
  unknown, and gate only the affected mapping or diagnosis.
- No runtime results: provide validation commands and expected observations;
  label validation unverified.
- Operational cause remains possible: separate it from the semantic finding and
  route only the operational action that crosses the boundary.

## Final-answer contract

Before returning, verify:

- Put a point-of-use public citation beside every decisive
  documentation-backed action or claim.
- Before evidence-dependent diagnosis, request the smallest safe evidence set;
  preserve every supported object-level fact and let absent fields limit only
  the affected conclusion.
- State assumptions, expected success evidence, and what remains provisional or
  unverified.
- Name an owner or route only when the answer crosses this skill's boundary;
  otherwise state that the answer remains within bounded field-extraction and
  CIM-mapping scope.
