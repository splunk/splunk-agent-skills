# Assignment and Delivery Evidence Contract

Use this reference for client-, app-, or server-class-specific findings. Public
documentation defines expected behavior; supplied deployment evidence shows
whether a particular environment currently matches it.

## Preserve object-level evidence

Normalize every relevant client, server class, deployment app, and delivery
attempt separately. Retain supported facts even when another field is missing
or two surfaces disagree.

| Object | Preserve when supplied | Do not infer |
| --- | --- | --- |
| client | client name, hostname, IP, GUID/identity, version, management-server target, last check-in, interval | identity equivalence from a similar hostname |
| server class | stanza, filters, matching result, app links, source/effective/in-memory observation | current membership from source text alone |
| deployment app | name, metadata, content/version indicator, client state, post-delivery settings | installation or activation from assignment alone |
| delivery attempt | time, handshake/status, bundle response, install result, reload/restart result, follow-up status | end-to-end success from one phase |
| state surface | file, `btool`, in-memory, UI, REST, client, `_ds*`, or log observation plus timestamp | freshness or agreement with another surface |

Apply the partial-evidence gate in this order:

1. state every supported fact and its evidence source;
2. state what each fact establishes independently;
3. mark absent or conflicting fields `unknown`;
4. identify only the conclusion blocked by each unknown; and
5. request the smallest safe discriminator for that conclusion.

Example: a UI row can prove a client was visible at its observation time, and
client-side `btool` can prove effective received configuration. Missing cache
timing may block a current assignment conclusion, but it does not erase either
observation.

## Effective assignment worksheet

Assess these surfaces separately and in order:

| Surface | Question | Smallest useful evidence |
| --- | --- | --- |
| source filesystem | What do redacted source stanzas and app metadata declare? | relevant `serverclass.conf` stanzas and app metadata only |
| effective configuration | Which merged settings and source paths win? | bounded `btool ... list --debug` output for relevant stanzas/configs |
| server memory/cache | What rules and cached matches is the server using now? | last reload, cache timing, configuration viewer, or bounded server observation |
| UI | What assignment and deployment state did the interface show, and when? | target client/app/class row with timestamp |
| REST | What client, server-class, or download state was visible? | bounded relevant endpoint fields and timestamp |
| client received state | What app/configuration exists and is effective on the agent? | app presence/version plus bounded client `btool` or status output |

Check whitelist and blacklist behavior at global, server-class, and app levels
before concluding that a client should match. See [set up agent
filters](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/set-up-agent-filters)
and the [serverclass.conf
specification](https://help.splunk.com/en/data-management/splunk-enterprise-admin-manual/10.4/configuration-file-reference/10.4.0-configuration-file-reference/serverclass.conf).

The Agent Management server-class configuration viewer is an effective-
configuration surface; it is not interchangeable with source-file text or
client-received state. See [server-class configuration
viewer](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/view-server-class-configuration/access-the-server-class-configuration-viewer).
The forwarder configuration view reports installed/effective configuration on
forwarders; preserve it as its own observation. See [view configurations on
forwarders](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/view-configurations-installed-on-your-forwarders).
REST exposes documented deployment-client and server-class state, but a REST
observation still needs its endpoint, fields, and time. See the [Splunk
Enterprise REST endpoint
inventory](https://help.splunk.com/en/splunk-enterprise/rest-api-reference/10.4/introduction/endpoints-reference-list).

Before declaring a mismatch, determine whether the observed change came from
the UI or direct file/app-content editing and whether the documented reload
occurred. See [serverclass.conf reload
behavior](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/advanced-configuration/use-serverclass.conf-to-define-server-classes).
For large environments, also check matching-cache freshness. See [application
matching cache](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/estimate-agent-management-performance/application-matching-cache).

## Deployment-path diagnosis

Trace the first evidenced gap rather than starting with a remediation:

1. `deploymentclient.conf` or equivalent points to the intended management
   server;
2. DNS/network/TLS and relevant service state permit phone-home;
3. the client completes phone-home/handshake and appears with the expected
   identity and last-check-in time;
4. global, server-class, and app filters produce the expected match;
5. the server returns the expected bundle/app response;
6. the client receives and installs the app;
7. documented reload/restart behavior activates it; and
8. follow-up UI, REST, `_ds*`, client, and log status agree or expose the next
   gap.

The documented client-target setup path is in [Specify the Agent Management
server](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/configure-agents/specify-the-agent-management-server).
Use [manage apps in Agent
Management](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/use-the-interface-to-manage-apps)
for documented assignment, deployment status, content preview, and post-
delivery behavior.

Do not declare root cause while the observations still fit more than one of
these classes:

- wrong or stale configuration state;
- identity/filter mismatch;
- application-matching cache or missing reload;
- DNS, network, or certificate path;
- client or server service state;
- license-related behavior;
- UI, REST, or `_ds*` visibility/freshness; or
- successful delivery followed by client-side installation, reload, or
  restart failure.

## Mandatory earliest-gap evidence acquisition

For effective-assignment and phone-home requests, execute this protocol before
calling evidence unavailable or naming a remediation:

1. Identify the earliest unresolved phase or state surface in the applicable
   worksheet or deployment path.
2. Use every relevant authenticated read-only observation already available in
   the runtime for that phase. For example, query a permitted server-class
   endpoint for assignment or the permitted client endpoint for first
   registration. Record the endpoint, bounded fields, and observation time.
   Do not substitute a different client's row or an unrelated state surface.
   For an effective-assignment question, when both the server-class and client
   collections are available, query both before answering, even when the first
   result is empty. The server-class collection establishes only the classes
   visible from that endpoint at that time; the client collection independently
   establishes which clients and identity or assignment fields are visible.
   One empty collection does not prove client absence, non-membership, stale
   state, or current assignment. See the [deployment endpoint
   descriptions](https://help.splunk.com/en/splunk-enterprise/rest-api-reference/10.4/deployment-endpoints/deployment-endpoint-descriptions).
3. For a configuration phase, require both the effective value and its source
   path from bounded `btool ... list --debug` output or an equivalent effective-
   configuration view. Compare those with the redacted source stanza and the
   documented configuration precedence; a value without provenance is not a
   complete configuration discriminator. The server-class viewer similarly
   exposes resolved values and their source files but reflects disk, which can
   differ from loaded memory until reload. See [configuration file
   precedence](https://help.splunk.com/en/splunk-enterprise/administer/admin-manual/10.4/administer-splunk-enterprise-with-configuration-files/configuration-file-precedence)
   and [view server-class
   configuration](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/view-server-class-configuration).
4. If a read surface is unavailable, explicitly mark it unavailable and ask
   for the smallest bounded substitute. The final minimum-evidence list must
   include every still-missing field required by the applicable route below;
   prioritize the earliest discriminator, but do not silently omit later
   fields needed before remediation or an end-to-end verdict.
5. A configuration move or edit is only a conditional remediation after
   provenance proves the current file is ineffective or overridden. For
   `deploymentclient.conf`, the documented direct-edit location is
   `$SPLUNK_HOME/etc/system/local`; a change requires the documented agent
   restart and renewed registration/phone-home verification. Describe these
   actions but do not execute them. See [specify the Agent Management
   server](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/configure-agents/specify-the-agent-management-server).

In the answer, report what each acquired observation establishes, the surfaces
that were unavailable, the complete bounded request, competing hypotheses,
and the exact post-remediation verification signal. This protocol supplements
the no-root-cause and no-golden-remediation rules; it does not weaken them.

## Minimum missing-evidence routes

Ask only for absent fields that can change the pending decision.

For **effective assignment**, request the relevant redacted
`serverclass.conf` stanzas; app metadata and post-delivery settings; target
client name, hostname, IP, and identity fields used by filters; last reload and
matching-cache timing; and the smallest available UI, REST, `btool`, or client-
received observation. If unavailable, return supported facts plus a checklist,
not an assignment verdict.

For **phone-home or deployment delivery**, request the exact symptom and
time/timezone; product and versions; target client identity; intended
management-server target; last check-in and interval; smallest DNS/network/TLS
and certificate observations; relevant service state; expected class/app;
bounded UI/REST/`_ds*` observations; and the smallest relevant client/server
log excerpts. Prioritize the earliest missing discriminator, but do not hide
other required missing fields.

For **live performance**, request fleet size, app sizes, phone-home interval,
reload/cache timing, server CPU/memory/storage/network capacity, and observed
latency or load with timestamps. Without them, explain documented tradeoffs
only.

For **Remote Upgrader execution**, request platform, forwarder version,
upgrader version, upgrader service state, package-assignment/delivery evidence,
and sanitized upgrader logs. Preserve a proven package-delivery fact while
leaving execution and upgrade success unconfirmed.

Never request credentials, private keys, full diagnostics, raw customer data,
or unrelated configuration. Never prescribe a historical case resolution as a
golden fix without matching current evidence.

## Answer shape

Lead with one row per affected object or delivery phase:

| Object or phase | Supported facts | Evidence/time | Unknowns | Assessment limit | Next discriminator |
| --- | --- | --- | --- | --- | --- |

Then provide documented expectations with point-of-use public citations,
competing hypotheses when needed, the smallest safe next checks, and what was
not validated. Name another owner only for the exact work outside Deployment
Server and forwarder-fleet policy, assignment, phone-home, rollout, visibility,
scale, or Remote Upgrader delivery scope.
