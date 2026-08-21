# Read-Only Evidence and Handoff Method

Use this reference when the diagnosis needs stack observations or a support
handoff. It defines evidence handling, not product behavior or configuration.

## Evidence ladder

Collect the smallest available layer that can distinguish the current
hypotheses:

1. user-described impact, scope, exact time, and sanitized visible error
2. deployment/version and identity-provider family
3. identity-provider success/failure status or correlation identifier already
   available to the user, without assertions or tokens
4. sanitized Splunk-visible authentication, mapping, role, or capability state
5. a bounded, current-doc-supported log or state query through an existing-auth
   read-only `splsearch` session

Stop when the next layer cannot change the recommendation. Absence of access
to a layer is not evidence that the layer passed.

## Optional stack-read gate

Before a `splsearch` read, require:

- exact target URL supplied or confirmed by the user
- existing auth status valid for that target, with no login or setup
- a bounded time window and affected identity represented by a safe surrogate
  when possible
- a query purpose tied to one documented expectation
- a query reviewed as read-only, with no write, script, send, delete, collect,
  output, or remote side effect
- an output-reduction and cleanup plan

If any gate is missing, use public-doc-guided manual checks instead. Do not ask
the user to paste or provision credentials.

## Evidence record

For each read, retain only:

- timestamp and bounded time window
- target class, not an unnecessary hostname or customer identifier
- query purpose and a redacted query description
- row count, aggregate state, or the smallest sanitized excerpt
- limitations, missing fields, and conflicting observations
- local result-table cleanup status

Keep observed stack facts under their own heading. Never convert one stack's
state into a general product claim.

## Failure-phase worksheet

Use one row per phase:

| Phase | Current documented expectation | Observed fact | Gap | Next read-only discriminator | Owner |
| --- | --- | --- | --- | --- | --- |
| Endpoint or redirect | Citation required | Observation or unknown | Match/gap/unknown | Smallest check | Named boundary |
| Identity-provider authentication | Citation required | Observation or unknown | Match/gap/unknown | Smallest check | Named boundary |
| Assertion or directory exchange | Citation required | Observation or unknown | Match/gap/unknown | Smallest check | Named boundary |
| Identity and group extraction | Citation required | Observation or unknown | Match/gap/unknown | Smallest check | Named boundary |
| Role mapping | Citation required | Observation or unknown | Match/gap/unknown | Smallest check | Named boundary |
| Capability authorization | Citation required | Observation or unknown | Match/gap/unknown | Smallest check | Named boundary |

Omit irrelevant rows. Stop at the earliest evidenced gap and avoid collecting
later-phase data that will not change the next action.

## Sanitized support packet

Include:

- business and user impact; one-user, group, or all-user scope
- first-seen, last-known-good, timezone, recurrence, and current state
- Splunk product, deployment type, release/service version, and topology facts
  already known
- authentication scheme and identity-provider family, without tenant secrets
- exact sanitized visible symptom and the first failing phase
- documented expectation with direct current public links
- observed facts, read-only checks, timestamps, and explicit unknowns
- recent relevant changes and their owners
- correlation or request identifiers already available, never assertions,
  session values, keys, passwords, tokens, or cookies
- the specific question or customer-inaccessible evidence needed from Support

Do not attach broad user exports, raw directory records, full authentication
logs, configuration files containing secrets, SAML assertions, or credential
diagnostics. Do not claim a Support-owned observation was checked when it was
not available.
