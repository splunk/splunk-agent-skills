# ACK, Troubleshooting, and Handoff

Use this reference for ACK questions, distributed receivers, evidence-based
diagnosis, and escalation. Do not turn documented behavior into a claim that a
specific deployment is healthy.

## Indexer acknowledgment and distributed receivers

When ACK is supported and enabled, the sender associates a channel identifier
with submissions, retains unacknowledged data, receives an `ackID`, polls the
acknowledgment endpoint, and retries according to the sender's bounded policy.
Account for the documented channel and ACK-capacity limits; do not recommend ACK
without the sender's delivery requirement, platform/version, and topology. See
[HEC indexer
acknowledgment](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/about-http-event-collector-indexer-acknowledgment).

For multiple HEC receivers, keep receiver configuration consistent, use
documented load-balancer health checks, and account for how submission and ACK
polling reach receivers. Explain those expectations, but do not design or
change a load balancer. See [Scale HEC with distributed
deployments](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/scale-http-event-collector-with-distributed-deployments).

Use `/services/collector/health` only as a documented HEC availability signal;
interpret its queue and acknowledgment-service status with the body and other
evidence. It does not prove end-to-end indexing. See [Input endpoint
descriptions](https://help.splunk.com/en/splunk-enterprise/leverage-rest-apis/rest-api-reference/10.4/input-endpoints/input-endpoint-descriptions).

For Splunk Cloud Platform, verify sender-specific ACK availability and Cloud
constraints before prescribing ACK; do not extrapolate Enterprise support. See
[Splunk Cloud Platform service
details](https://help.splunk.com/en/splunk-cloud-platform/get-started/service-terms-and-policies/9.3.2408/information-about-the-service/splunk-cloud-platform-service-details).

## Minimal diagnostic bundle

Ask only for missing items that can distinguish the next hypotheses:

- product, exact version, and receiver/load-balancer topology
- sender/integration and whether it uses `event`, `raw`, or ACK
- redacted endpoint host and port
- HTTP status and complete sanitized response body, or exact DNS/TLS error
- request timestamp/timezone and, for ACK, channel state, `ackID`, and sanitized
  poll result
- token enabled/deployed state, allowed/default index settings, and target index
  existence as reported by an authorized administrator; never the token value
- bounded verification SPL and result
- smallest relevant excerpts from `splunkd.log`,
  `http_event_collector_metrics.log`, Monitoring Console or Cloud Monitoring
  Console/introspection, queue, and throughput evidence

If these are unavailable, say root cause cannot yet be determined and provide
the next collection step. Absence of evidence is not evidence that a phase
passed.

## Evidence sequence and HTTP classes

Check the first failing phase: DNS/endpoint and port, TLS chain, HEC health,
token state, index authorization, request format, ACK/channel, receiver queues,
then bounded verification search. Use the response body and documentation to
interpret the status. The [HEC troubleshooting
guide](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/troubleshoot-http-event-collector)
documents HEC responses and the relevant logs, metrics, dashboards, queues,
throughput, token/index, and data-channel signals.

| HTTP class | Safe next check; not a root-cause claim |
| --- | --- |
| 400 | Match the sanitized response body to malformed envelope/raw input, missing required data or channel, invalid metadata, and endpoint/payload mismatch. |
| 401 | Confirm the request reached the intended receiver and have an authorized admin verify that the referenced token is enabled and deployed; do not request the token. |
| 403 | Use the response body to check authorization and target-index constraints, including whether the token may write the requested existing index. |
| 429 | Check the documented busy/throttling meaning, bounded retry behavior, HEC metrics, receiver queues, and throughput before changing sender rate or capacity. |
| 500 | Preserve timestamp and response, inspect the smallest relevant receiver logs, and hand off if customer-visible evidence cannot isolate the internal failure. |
| 503 | Check HEC health, enablement/availability, queue or acknowledgment-service health, and receiver reachability before treating the service as unavailable. |

Rank hypotheses by supplied evidence. For each, include supporting and
conflicting observations plus one read-only discriminator. Never infer a
service defect, scanner cause, token-propagation fault, indexer-health problem,
or successful fix from the status code alone.

## Sanitized escalation handoff

Use this skeleton when documented customer-safe checks stop at a service or
specialist boundary:

```text
Impact and scope:
Product/version/topology:
Sender, endpoint family, and ACK setting:
First seen / last known good / timezone:
Redacted endpoint and port:
Bounded test performed (no token/header):
HTTP status and sanitized body:
ACK/channel observations:
Verification search, time window, and result:
Relevant sanitized health/log/metric/queue observations:
Documented facts and point-of-use public links:
Evidence-backed hypotheses:
Ruled-out hypotheses and evidence:
Unknowns and missing evidence:
Exact boundary question for the named owner:
```

Do not attach tokens, authorization headers, customer identifiers, broad logs,
or configuration files containing secrets. Name
a Splunk platform operations specialist or Splunk Support only for downstream
indexing health, service-side token propagation, load-balancer/indexer repair,
or production remediation. Otherwise state that the result remains inside this
skill's HEC scope.
