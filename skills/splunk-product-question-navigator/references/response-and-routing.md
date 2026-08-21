# Answer and Routing Contract

Use this shape after live research. Adapt the length to the question, but keep
the evidence and applicability fields explicit.

## Product answer

**Answer:** Give the direct result first. Use conditional wording when the
result changes by deployment or version.

**Applies to:** Name the product, Cloud/Enterprise or other edition, exact
version/build, app or integration version, and any material provider, region,
experience, or topology. Identify assumptions and one missing fact that could
change the answer.

**Evidence checked:** Put direct citations beside the claims they support. For
each material source, make its role clear:

- `Official`: current/versioned Help, release note, policy, advisory, developer
  reference, or applicable Splunkbase structured compatibility/support field.
- `Supporting`: blog, Community/Answers, or Lantern context that does not
  establish support by itself.
- `Inference`: a narrow synthesis beyond direct source wording, including why
  it follows and what could falsify it.

Include `Checked: YYYY-MM-DD`. Preserve a source publication/update date when
shown. Do not cite this skill, its references, a search-results page, or a
generated summary as evidence.

**Boundary / next step:** State one of:

- the documented next public workflow;
- the focused applicability detail needed to finish the answer;
- `not publicly documented in the sources checked`, with the sources and scope
  searched;
- a concise unresolved official-source conflict; or
- the specialist, Splunk Support, account contact, or product contact that owns
  the next decision or action.

## Decision rules

| Customer intent | Handle here | Route |
| --- | --- | --- |
| What, where, edition, version, compatibility, support status, or documented change | Research and answer from current public sources | Route only the action that follows |
| Live health, crash, indexing, performance, restart, KV Store, or SmartStore symptom | Explain a cited product concept only when separately requested | A Splunk platform operations specialist for triage |
| Execute SPL or inspect private results | Explain a publicly documented capability only when separately requested | `splunk-search` |
| Cloud IP allowlist read/change | Explain public availability or prerequisites | `splunk-cloud-admin-copilot` for the approved operation |
| Other admin, configuration, ingestion, identity, app, upgrade, security, or dashboard question | If the user asks how, give the applicable cited public self-service procedure: prerequisites, ordered actions, expected result, and verification, without performing or validating it in the customer environment | Route only requested execution, private inspection, privileged mutation, or an undocumented or account-specific decision |
| Active outage, suspected defect, private environment, exception, or unpublished behavior | Summarize only applicable public evidence | Splunk Support or the responsible owner |
| Account entitlement, contract, order, tenant-specific enablement, or roadmap commitment | Explain only public policy or lifecycle language | Account/product contact or Support |

For a mixed request, do not withhold a well-supported explanatory answer merely
because the action is routed. Clearly separate `Documented answer` from
`Routed action`.

## Language guardrails

- Say `The public documentation states ...` only when the citation directly
  supports it.
- Say `not publicly documented in the sources checked` for silence. Do not say
  `unsupported`, `unavailable`, or `impossible` unless an applicable
  authoritative source says so.
- Say `Supporting source` for blogs, Community/Answers, and Lantern. A reported
  workaround or observed success is not a support statement.
- Preserve preview, controlled/limited availability, beta, and GA labels. Do
  not promise enablement or dates.
- Never imply that a current latest-version page applies to the user's older
  version without verifying it.
- Avoid unsupported root-cause claims, account decisions, and
  environment-specific security conclusions. Do not perform mutations or give
  unsupported or environment-specific mutation advice. A cited public
  procedure may describe customer actions while remaining explanatory.
