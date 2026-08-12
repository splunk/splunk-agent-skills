# Evidence and Decision Contract

Use this reference for any item-specific classification, plan, migration, or
removal assessment. It defines evidence handling and output rules, not product
truth.

## Per-item evidence record

Preserve each supplied fact with its source, applicable version, and date when
available.

| Field | Record when supplied | Never infer from |
| --- | --- | --- |
| identity | app/add-on name, app ID, installed version, source | a similar package name |
| platform | Cloud or Enterprise, current/target Splunk version, topology | product-family compatibility |
| distribution | Splunkbase, supported add-on, vendor, or private app; release notes | package location alone |
| validation | AppInspect/vetting version, date, checks, result, exceptions | upload or install success alone |
| placement | current/intended tier, cluster role, deployment method, target | generic install guidance |
| operation | active inputs, checkpoints, routing, concurrent collectors | app enabled state alone |
| dependencies | dependent apps, knowledge objects, users, scripts, integrations | package manifest alone |
| lifecycle | documented support/deprecation/EOL state, dates, migration path | age or lack of updates |
| removal | retained indexed data, user directories, restart/bundle effects, cleanup guidance | uninstall availability alone |

Never retain credentials, tokens, cookies, private tenant identifiers, raw
customer payloads, or unnecessary broad exports.

## Partial-evidence gate

For each app or add-on:

1. state every supported fact and its evidence;
2. assess what those facts establish independently;
3. retain contradictions and mark only absent fields `unknown`;
4. name only the decision blocked by each unknown; and
5. request the smallest safe artifact or field that can resolve it.

Example: an AppInspect report can establish its observed result and check set
even when topology is unknown. Missing topology blocks placement readiness,
not the report finding. An installed-version inventory and active-input list
remain valid observations when dependency evidence is missing; only safe
removal stays undecided.

## Compatibility classification

Return exactly one label per item. Apply the first supported rule below; never
combine labels.

| Label | Evidence rule |
| --- | --- |
| `Compatible` | Applicable authoritative app-specific evidence explicitly supports the installed app/add-on version with the target Splunk version and stated deployment context. |
| `Update available` | Applicable authoritative app-specific evidence explicitly establishes that the installed version does not meet target readiness and identifies a different version as tested compatible or required for that target. |
| `Incompatible` | Applicable authoritative app-specific evidence explicitly rules out the installed/target combination and the supplied evidence establishes no compatible update or migration path. |
| `Needs review` | Evidence is missing, unknown, stale, adjacent-version, product-family-only, conflicting, or insufficiently applicable. |

Use the applicable Splunkbase compatibility record, app/add-on release notes,
supported-add-on documentation, or authoritative vendor/app documentation.
The Splunk product compatibility matrix can direct the research but cannot
prove a non-premium app/add-on combination. Recommend a target app/add-on
version only when an authoritative record explicitly supports it.

For every label, include installed and target versions, platform/topology,
evidence title and date or freshness, what it establishes, conflicts or
unknowns, and the next action. Documentation silence is `Needs review`, not
`Incompatible`.

## Capability evidence gates

| Decision | Smallest useful evidence | If missing |
| --- | --- | --- |
| specific environment readiness | name/version, current and target Splunk versions, platform/topology, source/status, applicable release notes, AppInspect/vetting result when relevant | preserve supplied facts; return `Needs review` and request only absent fields |
| install/upgrade/validation plan | platform, topology, app source/version, target Splunk version, intended location, active inputs, available validation results | give cited generic guidance only; mark readiness `Needs review` |
| deprecation/EOL/migration | applicable release notes or vendor/app lifecycle record, Splunkbase record, active inputs/checkpoints/routing, migration guidance | do not assert status, dates, safety, or replacement readiness |
| safe removal | app/version, platform/topology/location, app-specific removal guidance, dependencies/knowledge objects, active inputs/checkpoints, retained data, user directories, restart/bundle effects | return `Needs review`; give cited generic disable/uninstall guidance only |

Request only fields relevant to the user's pending decision. If the user asks
whether a specific item is safe or ready and no narrower gate applies, request
app/add-on name and version, current and target Splunk version, platform,
topology, Splunkbase/private-app status, applicable release notes, and
AppInspect/vetting result.

## Advisory plan contracts

### Install, upgrade, or validation

Produce ordered, non-mutating steps covering:

1. applicability, compatibility, app source, permissions, prerequisites, and
   backup or rollback inputs;
2. topology, tier placement, clustering, targeted installation, and deployment
   method where documented;
3. package structure, dependencies, AppInspect, and Cloud vetting or approval;
4. active-input ownership, checkpoint continuity, and duplicate/concurrent
   ingestion precautions;
5. the authorized administrator's install or upgrade action, described but not
   performed; and
6. version confirmation, AppInspect/vetting disposition, data-ingestion checks,
   input/checkpoint checks, errors, dashboards/searches, and focused smoke tests.

Identify documented support-assisted or unavailable self-service steps. Do not
claim readiness or success without the corresponding supplied evidence.

### Deprecation or migration

State only evidenced status, dates, support expiry, maintenance consequences,
and distribution consequences. List documented or supplied replacement options
and explicit next actions, then check prerequisites, version compatibility,
configuration mapping, checkpoint transfer or reset behavior, concurrent-input
overlap, routing and sourcetype/index changes, validation, rollback, and
data-loss risk. Unknown checkpoint or routing behavior blocks migration safety,
not the evidenced lifecycle status.

### Removal

Separate the documented disable/uninstall path from the environment decision.
Review dependent apps and knowledge objects, active inputs and checkpoint
ownership, external integrations, retained indexed data, user-directory and
other app-specific cleanup, cluster/deployment-server or bundle propagation,
restart requirements, rollback inputs, and post-disable/post-uninstall checks.
State whether the applicable Cloud or Enterprise path is self-service or
support-assisted. Never use uninstall availability as proof that removal is
safe.

## Compact output shape

Use one row per item when classification is requested:

| Item | Supported facts and evidence | Unknowns/conflicts | Decision | Rationale | Smallest next action |
| --- | --- | --- | --- | --- | --- |

Follow with documented expectations and point-of-use public citations. For a
plan, replace the table with applicability, prerequisites, ordered advisory
steps, validation signals, decision limits, and only the boundary owners that
are actually needed.
