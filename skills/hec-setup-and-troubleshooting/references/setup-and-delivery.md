# Documented HEC Setup and Delivery

Use only the branch matching the user's product and version. These links are
evidence anchors; verify current applicability when the user's version differs.

## Setup paths

### Splunk Cloud Platform

Use Splunk Web to create and manage HEC tokens. Confirm the target index already
exists, select only indexes the token may write, choose a default index, and set
source, sourcetype, and host metadata only when the integration requires fixed
values. Confirm deployment status before testing. Cloud HEC uses HTTPS, and
Cloud service constraints restrict global settings, output groups, and general
ACK use; verify the documented support for the specific sender rather than
assuming Enterprise behavior. See [Cloud HEC setup in Splunk
Web](https://help.splunk.com/en/splunk-cloud-platform/get-data-in/get-started-with-getting-data-in/10.4.2604/get-data-with-http-event-collector/set-up-and-use-http-event-collector-in-splunk-web)
and [Splunk Cloud Platform service
details](https://help.splunk.com/en/splunk-cloud-platform/get-started/service-terms-and-policies/9.3.2408/information-about-the-service/splunk-cloud-platform-service-details).

Do not redirect Cloud HEC setup to Enterprise CLI or configuration files. Route
an IP allowlist change to the Cloud administration owner; token and delivery
work remains here.

### Splunk Enterprise

Choose one documented administration surface:

- Splunk Web: enable HEC as needed, create the token, bind allowed/default
  indexes, and set metadata. See [Enterprise HEC setup in Splunk
  Web](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/set-up-and-use-http-event-collector-in-splunk-web).
- CLI: describe the documented commands for token creation, listing, update,
  enablement, disablement, deletion, and submission; do not run them. See
  [Enterprise HEC from the
  CLI](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/set-up-and-use-http-event-collector-from-the-cli).
- Configuration files: describe the relevant global or per-token
  `inputs.conf` settings and, for distributed output, applicable `outputs.conf`
  groups and persistent queues; do not edit files or restart Splunk. Cover SSL,
  ports, queues, allowed/default indexes, and output groups only when material.
  See [Enterprise HEC configuration
  files](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/set-up-and-use-http-event-collector-with-configuration-files).

For every path, state who owns the admin action, what prerequisites must already
exist, and the expected post-action state. A documented path is not proof that
the user's token is deployed or healthy.

## Endpoint and payload selection

- Use the receiver URL and port documented for the deployment. Splunk Cloud
  Platform HEC uses HTTPS on port 443; Splunk Enterprise commonly starts with
  the configurable HEC port 8088. Do not guess when a proxy or load balancer
  exposes a different approved endpoint. See [Cloud HEC setup in Splunk
  Web](https://help.splunk.com/en/splunk-cloud-platform/get-data-in/get-started-with-getting-data-in/10.4.2604/get-data-with-http-event-collector/set-up-and-use-http-event-collector-in-splunk-web)
  and [Enterprise HEC setup in Splunk
  Web](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/set-up-and-use-http-event-collector-in-splunk-web).
- Use `/services/collector/event` for a JSON envelope with an `event` value and
  optional `time`, `host`, `source`, `sourcetype`, `index`, and `fields`.
- Use `/services/collector/raw` for raw event data. Supply a request channel as
  documented when required, such as the `X-Splunk-Request-Channel` header or
  supported query parameter.
- Use `Authorization: Splunk $HEC_TOKEN`. Keep the token in an environment
  variable managed outside the transcript; never paste, echo, log, or embed it
  in a URL.

See [Format events for
HEC](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/format-events-for-http-event-collector)
for envelope, raw, metadata, multi-event, authorization, and channel behavior,
and [HEC REST API endpoint
overview](https://help.splunk.com/en/splunk-enterprise/get-data-in/collect-http-event-data/http-event-collector-rest-api-endpoints)
for endpoint families.

## Bounded event test

Have the user substitute a non-production receiver and approved test index. Do
not run the request without separate explicit authority.

```sh
HEC_URL='https://<hec-host>:<port>'
TEST_MARKER='hec-validation-<unique-id>'

printf 'header = "Authorization: Splunk %s"\n' "$HEC_TOKEN" | \
curl --silent --show-error --config - \
  --header 'Content-Type: application/json' \
  --data "{\"event\":{\"message\":\"$TEST_MARKER\"},\"index\":\"<approved-test-index>\",\"sourcetype\":\"<test-sourcetype>\",\"source\":\"hec-bounded-validation\"}" \
  --write-out '\nHTTP_STATUS=%{http_code}\n' \
  "$HEC_URL/services/collector/event"
```

Capture the HTTP status, response body, and request timestamp/timezone without
capturing the authorization header. The public cURL examples are documented in
[Use cURL with
HEC](https://help.splunk.com/en/splunk-enterprise/get-started/get-data-in/10.4/get-data-with-http-event-collector/use-curl-to-manage-http-event-collector-tokens-events-and-services).

Verify only the unique marker in the approved time window:

```spl
index="<approved-test-index>" sourcetype="<test-sourcetype>"
source="hec-bounded-validation" "hec-validation-<unique-id>"
| table _time host source sourcetype index _raw
```

The test is validated only when the response is successful and the matching
event is observed. A successful submission response by itself is not indexed-
event proof; a missing search result by itself does not identify the failing
phase.
