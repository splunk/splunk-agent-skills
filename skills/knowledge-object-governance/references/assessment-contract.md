# Evidence and Assessment Contract

Use this reference for deployment-specific inventories and findings.

## Evidence handling

Normalize each supplied object into a separate record. Preserve supported
facts even when other fields are absent or contradictory.

| Field | Record when available | Never infer from |
| --- | --- | --- |
| identity | object type and exact name | a similar object type or display label |
| context | app, namespace, source/target context | the owner's default app |
| ownership | owner and user status, with separate sources | name shape or service-account appearance |
| sharing | private, app, or global | app location alone |
| access | role read/write lists and ACL source | sharing level alone |
| lifecycle | enabled state, last use, dependencies, backup, approval | age alone |
| lookup | provenance, size/storage indicator, update path, retention, monitoring | undocumented quota or product limit |
| cluster | member presence, metadata, owner, checksum/version indicator | a symptom reported on one member |

Attach an evidence label to each object-specific finding, such as a sanitized
artifact name plus row/object identifier, screenshot panel, REST endpoint and
field, cluster member, and observation timestamp. Do not include credentials,
tokens, cookies, raw customer payloads, or unnecessary broad exports.

## Partial-evidence gate

Apply this sequence to every object:

1. state every supported fact and its evidence;
2. state what those facts establish independently;
3. mark each absent or conflicting field `unknown`;
4. identify only the conclusion blocked by that field; and
5. request the smallest safe discriminator for that conclusion.

Example: an export that proves `owner=svc_search`, `sharing=app`, and object
type `saved search` still supports all three facts. Missing user status blocks
only the orphan/offboarding conclusion. Missing role ACLs blocks only the role
exposure conclusion. Neither omission erases the supported inventory.

## Capability gates

| Decision | Minimum useful evidence | If missing |
| --- | --- | --- |
| inventory completeness | relevant Settings export, REST/list output, app metadata, or screenshot fields | report present fields and list exact missing fields |
| orphan/offboarding status | object owner plus current user/account status; schedule state for impact | mark orphan status unconfirmed; request user inventory or Reassign Knowledge Objects/ACL evidence |
| ACL exposure | sharing plus role read/write ACL evidence | explain documented concepts; request bounded ACL or permission evidence |
| name collision | same-type source and target name lists plus intended scope/policy | give naming guidance; do not select collision disposition |
| stale/delete/move candidate | inventory plus recent usage and dependency evidence; owner/backup state for action planning | request bounded usage/dependency, owner validation, and backup/export evidence; do not require deletion |
| lookup risk | lookup file/list evidence plus available owner, ACL, size/storage, provenance, update path, retention, and monitoring facts | ask only for absent fields relevant to the stated risk; provide governance questions, not invented limits |
| cluster inconsistency | comparable object presence and metadata across relevant members, including owner and checksum/version indicators when available | request a bounded same-object member comparison; do not infer inconsistency from symptoms |
| operational repair | recurrence, impact, authority, rollback, and platform-operations confirmation | preserve comparison findings and route; do not recommend forced repair or resync |

## Report shape

Lead with findings. Use one row per object or exact comparison target:

| Object | Supported facts | Evidence | Unknowns | Governance finding | Priority | Next read-only step |
| --- | --- | --- | --- | --- | --- | --- |

Then include:

1. **Documented expectations** with point-of-use public citations.
2. **Prioritized review plan** that preserves authorized workflows and favors
   reversible precautions.
3. **Decision limits** naming conclusions that remain unconfirmed and the
   smallest safe missing evidence.
4. **Boundary route** only for execution, authoring, broken-content diagnosis,
   platform symptoms, or cluster repair outside this skill.

For ownership recovery, include prerequisites: confirmed orphan status,
intended authorized new owner, required administrative capability, scheduled
content impact review, dependencies, approval, and post-change validation.
For ACL review, include current workflow needs, read/write roles, least-
privilege proposal, approval owner, and validation plan. These are plans only;
this skill performs no mutation.

## Priority rules

Prioritize with explicit rationale, not a hidden score:

- higher: confirmed orphaned scheduled content, broad evidenced write exposure,
  or a dependency-backed risk to active workflows;
- medium: evidenced collision risk, unsupported ownership continuity, lookup
  write-path/retention gaps, or recurring cross-member differences with impact;
- lower/monitor: hygiene gaps without demonstrated impact or with key decision
  fields still unknown.

Do not upgrade an unknown into a risk finding. It can justify evidence
collection, not a deployment diagnosis.
