# Health Evidence and Packet Contract

Use this reference for evidence collection, evidence-bounded interpretation,
packet normalization, and handoff preparation.

## Evidence handling

Normalize each supplied object separately. An object can be a dashboard
indicator, health check, component or feature, node, log message, endpoint
response, diag/RapidDiag artifact, failed search, or Support case.

For each object, retain its exact label or identifier, source surface, product,
version, node and role, state and details, timestamp and timezone, related
impact, artifact provenance, redactions, and conflicts when supplied. Mark
absent values `missing`; never infer them from a neighboring object.

Apply this partial-evidence sequence before any gate:

1. State every supported object-level fact and its source.
2. State what those facts establish independently.
3. Mark absent, stale, or conflicting fields.
4. Identify only the conclusion blocked by each gap.
5. Request the smallest safe discriminator for that conclusion.

Example: a current `Disk` check with `Warning` on one named indexer still proves
that emitted observation and its time. A missing detail message blocks the
reason for the warning; a missing peer comparison blocks deployment-wide
scope. Neither gap erases the supplied observation.

## Core intake

For an evidence-dependent health decision, explicitly request every missing
core field:

- Splunk Cloud Platform or Splunk Enterprise
- deployment type/topology and exact version
- bounded time window and timezone
- affected component, feature, tier, or node role
- exact dashboard/check/endpoint/log source and observed state or alert
- user or data impact, recurrence, and recent maintenance or configuration
  change

Ask for sanitized excerpts or field-level exports, not credentials, full logs,
raw customer payloads, or an entire diag bundle in chat.

## Lane-specific minimums

Request only lanes relevant to the stated alert. Preserve any supplied lane
facts even if another field is absent.

| Lane | Smallest useful evidence |
| --- | --- |
| Indexing | exact check/indicator; affected indexer/index/sourcetype when known; indexing rate or lag; queue trend; time and impact |
| Search | exact check/indicator; failed search ID when supplied; scheduler or concurrency signal; bounded failure/latency count; time and impact |
| Forwarder | exact forwarder-related check; affected or missing population count; last-seen or connection state; receiving tier; time window |
| Disk | node and mount/path category; current utilization/free-space observation; exact check/details; trend or recurrence |
| License | exact pool/stack/check and state; usage versus entitlement observation; warning/violation time; affected capability |
| Memory | node/process/component; exact check/details; current and recent memory-pressure observation; OOM/restart evidence if supplied |
| Queues | node/component and queue name; fill/current/trend observation; duration; upstream/downstream symptom and impact |
| Security | exact CMC or health indicator and validation details; affected surface; time; sanitized related message; never request secrets |
| Workload management | exact CMC indicator or workload/pool class; saturation or delay observation; affected searches/workload; time and impact |

If the smallest discriminator is unavailable, make no lane-specific diagnosis.
Return the supported facts, exact gaps, and a collection checklist. Add a
support-handoff outline only when persistent impact, a documented Support-owned
step, or the user's existing case makes escalation likely.

## Interpretation rules

- Map a state only to the exact component, feature, or check that emitted it.
- Treat aggregate colors and Warning/Error/Critical/Yellow labels as observed
  states, not causes or deployment-wide conclusions.
- Label an explanation `inferred` unless the supplied evidence directly proves
  it. State what supports and what would falsify each hypothesis.
- Call a cause confirmed only when the supplied evidence directly establishes
  the causal link; otherwise recommend the next smallest check.
- Treat old evidence as historical. Ask for a current observation before a
  current-health decision.
- When evidence is absent, do not rank hypotheses. When it is partial, assess
  supported objects first and rank only hypotheses those facts can support.

Use this compact hypothesis shape when interpretation is warranted:

```text
Hypothesis [inferred]: <bounded explanation>
Supported by [observed]: <exact supplied facts>
Does not establish: <root cause, scope, or impact still unproved>
Next discriminator: <smallest safe check that can confirm or falsify it>
```

## Diagnostic packet

Produce a partial packet rather than filling gaps:

```text
Supported findings summary
- [observed] concise facts established by the supplied evidence
Missing:
- concise fields or artifacts still needed
[when root cause is unestablished] The root cause is not established.

Environment
- [observed|missing] product, deployment/topology, version, node roles

Timeline
- [observed|missing] start, end/current time, timezone, recurrence, changes

Observed health states
- [observed] one row per object: time, surface, component/check, node, state, details, impact
- [documented] applicable expectation with point-of-use public citation
- [inferred] bounded interpretation, if evidence supports one

Collected artifacts
- [observed] screenshots/exports, health.log excerpts, endpoint responses,
  diag/RapidDiag metadata, bundle IDs, failed search IDs, persistent errors

Evidence gaps
- [missing] field, decision it blocks, smallest safe evidence requested

Privacy notes
- [observed] redactions/review performed
- [missing] review, exclusions, anonymization, or authorization still needed

Recommended next recipient
- Keep within this skill when collection or bounded interpretation remains
- Name Splunk Support or a narrower specialist only when the next action crosses the boundary

Support details
- [observed] case number, requested decision, prior uploads/actions when supplied
```

Always lead a diagnostic packet with this concise summary. Use the `Missing:`
label even when the detailed Evidence gaps section follows, and include the
declarative sentence `The root cause is not established.` whenever the supplied
evidence does not establish a root cause. When the supplied evidence directly
establishes the causal link, state that cause and its supporting evidence
instead; do not include the unestablished-root-cause disclaimer.

Keep `observed`, `documented`, `inferred`, and `missing` distinct. Do not convert
a documented expectation into an observation or a missing impact field into an
assumed business effect.

## Boundary routes

- `splunk-search`: authenticated, bounded SPL evidence execution.
- `hec-setup-and-troubleshooting`: HEC protocol setup or delivery diagnosis.
- A broader Splunk platform operations specialist: multi-surface operational
  interpretation and planning.
- Cluster specialist: indexer-cluster or search-head-cluster deep diagnosis and
  remediation when that skill is available.
- Incident-diagnosis specialist: broader cross-component root-cause work when
  that skill is available.
- Splunk Support: persistent product-owned errors, diagnostic review, or
  private/account-specific remediation outside documented customer-safe checks.

Do not name a recipient for work still fully handled by this skill.
