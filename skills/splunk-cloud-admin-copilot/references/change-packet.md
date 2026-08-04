# Splunk Cloud ACS Allowlist Operation Record

Use this shape for the one-CIDR allowlist operation or a read-only readiness
assessment. Populate only fields supported by observed evidence.

## Decision

- Status: `blocked`, `no-op`, `approved`, `executed`, `rolled-back`,
  `verification-incomplete`, `ready`, or `not-ready`
- Rationale
- Routed dependencies and owners

## Exact Target and Provider

- Product: Splunk Cloud Platform
- Exact deployment alias
- Environment: `development` or `production`
- Exact ACS host
- Current-stack readback and timestamp
- Requester, operator, and approval owner

## Current and Desired State

- ACS CLI and stack compatibility evidence
- Feature: `acs`, `search-api`, `hec`, `s2s`, `search-ui`, `idm-api`, or
  `idm-ui`
- Action: `add` or `remove`
- One canonical IPv4 CIDR
- Fresh sanitized complete allowlist or digest
- Desired state and one-resource minimal diff
- Idempotency decision

## Safety Preflight

- Required capability evidence
- Current documentation and applicable limits checked
- Deployment-wide blast radius
- Maintenance and change-freeze conflicts
- Out-of-band recovery evidence when lockout is possible
- Exact inverse rollback and trigger
- Reasons the operation is blocked, if any

## Approval Gate

Record explicit human approval of the exact deployment, environment, host,
action, feature, one CIDR, before state, impact, and inverse rollback. Record the
approval reference and timestamp. Approval of a plan, a broader change, another
target, or multiple CIDRs does not open the write gate.

## Execution Evidence

- Exact allowed command shape used, with sensitive values sanitized
- Submission timestamp and actor
- ACS response category, not raw authentication-bearing output
- Bounded status-poll result and timestamp
- Full same-feature readback or digest
- Minimal-diff comparison

Never mark execution complete from intent or a submission message alone.

## Functional Verification

- Affected-feature data-plane check and evidence reference
- Existing-path regression check and evidence reference
- Control-plane/data-plane agreement
- Verification timestamp
- Routed specialist, when the functional probe belongs to another domain

## Rollback

- Whether rollback was pre-authorized
- Trigger evidence
- Exact inverse action against the same feature and CIDR
- Status and same-feature readback
- Functional recheck

## Read-only Readiness

- Current stack status
- Supplied maintenance schedule and change-freeze evidence
- Dependencies and owners
- Decision: `ready`, `not-ready`, or `blocked`
- Routed maintenance, upgrade, or restart action

Do not record a schedule, freeze preference, restart, or remediation as executed
by this skill.

## Sanitized Receipt

- Timestamp and actor
- Deployment, environment, host, operation, feature, and CIDR
- Before-state digest or sanitized snapshot
- Approval reference
- Submitted result versus converged result
- Control-plane verification
- Data-plane verification
- Rollback state
- Open issues and routed owner

Exclude credentials, tokens, cookies, authorization headers, complete CLI
configuration, and unnecessary customer data.
