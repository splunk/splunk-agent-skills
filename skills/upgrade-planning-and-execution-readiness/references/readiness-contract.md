# Upgrade Readiness Contract

Use this reference for environment evidence, capability-specific decision
rules, and the report shape. Keep documentation facts and deployment
observations separate.

## Evidence ledger

Record one row per component or decision object. Preserve every supplied field,
even when another required field is absent.

| Object | Minimum useful fields | Provenance | Supported conclusion | Missing fields |
| --- | --- | --- | --- | --- |
| Upgrade scope | product/service, current version, target version, deployment type, topology | user-provided or document-backed | applicability envelope | unresolved fields |
| Platform | operating system, architecture, filesystem, capacity/privilege evidence | user-provided | documented prerequisite comparison | decisive missing fields |
| Premium product | name, current version, planned version, matrix evidence | user-provided plus public source | compatibility result for exact combination | missing versions/evidence |
| App/add-on | name, version, owner, lifecycle source, Splunkbase/AppInspect result | user-provided or supplied artifact | app-specific status if evidenced | inventory or result gap |
| Forwarder/component | role, current version, target relationship, documented compatibility | user-provided plus public source | supported relationship if exact | missing endpoint/version |
| Health/precheck | target/component, timestamp, result, applicable threshold/source | supplied artifact | observed pass/fail only for that check | stale or absent fields |
| Backup | data class, scope, timestamp, status, storage boundary | supplied artifact | backup observation | restore validation |
| Restore test | data class, target, timestamp, outcome, acceptance signal | supplied artifact | recovery evidence for tested scope | untested classes |
| Baseline/validation | metric or function, time window, expected range, postcheck result | supplied artifact | comparison for that signal | baseline or postcheck |
| Change control | window, freeze, approval, owner, dependency handoff | user-provided or supplied artifact | readiness of that prerequisite | missing owner/approval |

Use `document-backed` only for what a cited public source establishes. Use
`user-provided` for deployment facts and artifacts, even when they agree with
documentation. Use `unresolved` for absent or conflicting fields.

## Capability decision rules

### Scope intake

Output known current state, target state, topology, dependencies, constraints,
and missing inputs. If product/service, target release, or topology needed for
sequencing is missing, stop exact-path advice and request only those fields or
an authorized read-only inventory.

### Documented path

Bind every path, prerequisite, release note, system requirement, compatibility
claim, backup action, topology sequence, and postcheck to a current public page
for the specified product and release. A source can establish a documented
expectation; it cannot prove the environment is ready.

### Dependency compatibility

Group results under:

1. platform and infrastructure;
2. topology and Splunk platform components;
3. premium products;
4. apps and add-ons;
5. forwarders; and
6. other deployment dependencies.

For every row use `compatible`, `blocked`, or `unresolved`, followed by the
exact evidence. Use `compatible` only when a current applicable source or
supplied authoritative result establishes the exact combination. Missing app
inventory limits app readiness; it does not erase platform or premium-product
findings.

For a renewed attempt after a regression in which an app or add-on is a
suspected dependency, apply one mandatory regression-retry protocol before
recommending rollout. Bind the exact current and proposed component, app/add-on,
and configuration combinations to current lifecycle and migration guidance;
when that guidance identifies breaking changes, require its migration steps and
non-production test (for example, the
[Splunk Add-on for Microsoft Windows listing](https://splunkbase.splunk.com/app/742)
flags a breaking 5.0.0 transition). Then return a staged validation matrix that
changes only one dependency dimension per stage and uses only combinations that
applicable documentation supports. For every stage state the cohort and exact
combination, baseline, postcheck, observable success threshold, soak period,
evidence source, accountable owner, stop condition, and recovery action. Keep
renewed rollout `decision-blocked` until each proposed combination has
compatibility evidence and every completed stage meets its threshold; treat the
suspected dependency as a hypothesis, not an established root cause.

### Execution readiness

Order the plan as:

1. confirm scope and target-release path;
2. close prerequisite and compatibility blockers;
3. capture current health and functional baselines;
4. verify configuration, indexed-data, and KV-store protection as applicable;
5. verify restore/recovery evidence and rollback boundary;
6. confirm topology-specific sequence and stop conditions;
7. confirm maintenance window, freeze, approvals, owners, communications, and
   dependency handoffs;
8. state explicit go/no-go criteria; and
9. define post-upgrade validation and escalation thresholds.

Return `go` only when every mandatory documented prerequisite and local
criterion has current supporting evidence. Return `no-go` for an evidenced
blocker. Return `decision-blocked` when decisive evidence is absent or
conflicting. Missing health, backup, owner, approval, or window evidence must
produce a template and evidence request, not a readiness assertion.

### Rollback and recovery

Report three separate questions:

- Does applicable documentation support in-place rollback?
- Which configuration, indexed-data, and KV-store protections are relevant and
  evidenced?
- Which restore procedures have actually been validated for this scope?

Never turn backup existence into restore proof or a recovery plan into a
supported in-place rollback promise. Without applicable backup and restore
evidence, use `recovery readiness unconfirmed`.

### Post-upgrade validation

Define the baseline, postcheck, success threshold, evidence source, and owner
only when a handoff is required for each applicable check:

- installed/running version and expected component availability;
- documented health checks;
- ingestion rate and continuity;
- search participation and representative functional searches;
- licensing state;
- premium-product and app smoke checks;
- resource utilization; and
- indexer/search-head cluster communications when applicable.

Separate documented procedures from telemetry-dependent results. Without both
baseline and post-upgrade observation, keep the comparison unresolved and do
not diagnose a regression.

## Smallest safe evidence route

Ask only for artifacts that can change the pending decision: a sanitized
product/version/topology inventory; named app and version list; exact public
compatibility record or supplied AppInspect result; bounded health/precheck
output with timestamp; backup scope/status and restore-test result; baseline
and matching postcheck; or maintenance window, freeze, approval, owner, and
dependency status.

Prefer sanitized summaries, relevant rows, counts, and status fields. Do not
request credentials, tokens, raw configurations, unbounded logs, broad customer
inventories, or private Support material. If read-only discovery is explicitly
authorized and available, bind it to an exact target and collect only the
fields above; otherwise ask the administrator to supply them.

## Mandatory coverage-first response protocol

Write the final answer in two passes. In the first pass, before optional
explanation, give every capability the request needs one compact, complete
result using the applicable item below. Do not begin the second pass until all
applicable first-pass items exist.

- **Decision and limits:** `go`, `no-go`, `decision-blocked`, or a narrower
  conclusion, including that execution is outside this skill.
- **Scope:** known current and target state, topology, dependencies and
  constraints with provenance, followed by unresolved fields.
- **Documented path:** exact-product/release requirements and point-of-use
  public citations, or the ambiguity that prevents exact advice.
- **Compatibility:** platform, topology, premium product, app/add-on,
  forwarder, and infrastructure rows, each `compatible`, `blocked`, or
  `unresolved` with its evidence.
- **Execution readiness:** ordered prechecks, backup/baseline prerequisites,
  evidenced sequence or topology gap, owners/window gaps, stop conditions, and
  explicit go/no-go criteria.
- **Rollback/recovery:** in-place rollback support, backup coverage, and actual
  restore evidence as three separate conclusions.
- **Validation:** baseline, postcheck, success threshold, evidence source, and
  required owner for each applicable validation category.
- **Next evidence or route:** the smallest decision-changing inputs and only
  the boundary handoffs actually required.

In the second pass, add detail only where it changes a decision, supplies a
requested checklist or template, or explains a cited requirement. Never defer
an applicable first-pass item until after research narration, background, or
optional detail. If response space is constrained, return the complete first
pass and omit the second pass.

### Hard concise final-answer budget

Start the final answer as soon as the minimum applicable public sources are
verified. Do not spend answer space narrating research, restating the request,
showing scratch work, or previewing the answer. The final answer has a hard
ceiling of 700 words: use at most 450 words for one requested capability and
use the remaining allowance only when the request spans multiple capabilities.

Allocate that ceiling in this order, completing each applicable item before
optional detail:

1. State the decisive action or conclusion, its evidence boundary, and the
   planning-only/no-execution limit.
2. Give every requested capability its compact first-pass result. For a
   documented-path question, this result must preserve the direct-versus-
   intermediate path, each required intermediate release or alternative, the
   first prerequisite areas, target-release README/release-notes and system-
   requirements sources, compatibility sources, and the distinction between a
   documented path and local readiness. For a compatibility request, preserve
   all six dependency groups. For a multi-capability readiness request, use one
   dense line or row per required result rather than dropping a capability.
3. Put one necessary public citation beside each decisive documented claim;
   reuse a citation for adjacent claims it supports and omit duplicative source
   lists.
4. End with only the smallest safe evidence request or boundary route that can
   change the decision.

Omit background, repeated caveats, examples, and a recap before shortening any
required path step, dependency group, capability result, evidence label, safe
route, or citation. Never exceed the ceiling in order to add optional detail.
