# Evidence and decision contract

Use this reference for case-specific work. It defines what each decision may
use, what success looks like, and the narrow gate when evidence is missing.

## Preserve partial evidence

Record each supplied object separately:

- search: authored SPL, intended semantics, owner context if relevant;
- job/SID: time range, runtime, optimized SPL, costs, counts, scan volume,
  buckets, peers, map/reduce strings, logs, and finalization state;
- invocation: dashboard refresh, schedule, concurrency, and workload pool;
- acceleration candidate: model/report, indexed fields, predicates,
  cardinality, qualification, summary range/coverage, and status;
- benchmark: query variant, time/data scope, result signature, runtime, counts,
  and resource impact; and
- platform snapshot: affected searches/peers, CPU, memory, disk, queue, limits,
  imbalance, and timestamp.

Attach source and timestamp when known. Preserve conflicting observations.
Assess every present field before gating the decision that needs a missing one.
Never replace a partially supported object with a generic “insufficient
evidence” result.

## Inspect search-job evidence

Evidence: authored SPL plus any Job Inspector, Job Details, `search.log`, SID,
or equivalent job detail.

Report:

1. the concrete artifacts and fields used;
2. authored SPL separately from optimized SPL;
3. supported costs, counts, scan volume, bucket/indexer timing, map/reduce
   behavior, and visible command or predicate changes;
4. the likely high-cost stages and confidence; and
5. the exact unknowns that prevent a stronger diagnosis.

If job evidence is incomplete, request only the missing subset among SPL, Job
Inspector/details, SID or sanitized log excerpt, time range, runtime,
scan/event/result counts, and visible indexer/workload context. Frame any
partial-evidence diagnosis as evidence-backed likelihood, never guaranteed root
cause.

## Recommend safe SPL changes

Require the current SPL, intended result semantics, time range, and baseline
metrics for a case-specific rewrite. Rank changes by expected leverage and
semantic risk. For each recommendation include:

- the exact SPL pattern or job signal;
- the smallest proposed change;
- why it might reduce retrieval, processing, movement, or centralized work;
- ordering, cardinality, field, aggregation, and result-completeness risks; and
- the benchmark that could demonstrate improvement.

If those inputs are absent, provide documented general patterns only and ask
for them. Never state that a rewrite is faster until equivalent before/after
evidence exists.

## Evaluate tstats and acceleration fit

Require the model or report context, indexed-field constraints, predicates and
cardinality hints, acceleration status, summary coverage/range, and comparable
baseline metrics for a case-specific recommendation.

Check:

- whether `tstats` can use indexed fields or an accelerated data model;
- whether broad model coverage or missing pruning increases work;
- whether high-cardinality grouping or predicates can negate the benefit;
- whether `summariesonly` and summary coverage preserve required completeness;
- whether report or model qualification and summary range match the search;
- storage and background-search load; and
- whether current and candidate searches can use equivalent time, data,
  permissions, and result semantics.

If evidence is absent, give only documented decision criteria. Do not assume
`tstats` or acceleration is faster, and do not enable or design an acceleration
structure within this skill.

## Diagnose workload and scheduling pressure

Use schedule or refresh cadence, concurrent search counts, workload pool,
Monitoring Console search activity, CPU/memory/disk indicators, and one-search
versus many-search scope. Explicitly flag expensive repeated invocation, such
as an overly frequent dashboard refresh, when supported.

Separate:

- query-owned: data access, command shape, movement, and centralization;
- invocation-owned: refresh or schedule cadence and clustering;
- workload-owned: queue, concurrency, pool limits, and policy; and
- platform-owned: resource pressure, peer imbalance, timeouts, limits, or disk.

Without deployment or Monitoring Console evidence, a slow job does not prove
workload or platform pressure. Request only the missing schedule, concurrency,
pool, resource, or affected-scope observations needed to choose among those
lanes.

## Define before-and-after validation

Capture the baseline before proposing a win. Hold time range, data scope,
permissions, workload conditions where practical, and intended results
equivalent. Record:

- runtime and queued/finalized state;
- scan, event, and result counts;
- bucket coverage and relevant peer timing;
- CPU and memory impact when available;
- concurrency or invocation context; and
- a result-equivalence check appropriate to the search.

Run or ask the user to run the candidate under the same envelope. A material
semantic difference invalidates the performance comparison. If metrics do not
improve, roll back the recommendation or retain it only for another documented
benefit. If execution is unavailable, provide this checklist and make no impact
claim.

## Route platform-level causes

Route only when evidence points beyond safe query tuning: capacity or health,
configuration, distributed-peer timeout, serialization or other limit, disk
exhaustion, workload imbalance, indexer imbalance, or a broad multi-search
incident.

Name the observed signal and explain why SPL-only tuning is insufficient.
Request or package only affected peers, limits hit, resource metrics, workload
policy, sanitized relevant logs, representative SIDs, scope, and timeline.
Route to a Splunk platform operations specialist, the platform operations owner,
or Splunk Support as appropriate. Do not prescribe an administrative change
without authority, evidence, and rollback context.
