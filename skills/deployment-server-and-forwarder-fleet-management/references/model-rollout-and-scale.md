# Model, Rollout, Scale, and Upgrade Guidance

Use this reference for public product guidance. Match every claim to the
user's exact version; these anchors cover Splunk Enterprise 10.4 unless stated
otherwise.

## Agent Management model

Splunk Enterprise 10.x calls the deployment-server capability **Agent
Management**, calls managed deployment clients **agents**, and retains legacy
names in `deploymentclient.conf`, `serverclass.conf`, CLI commands, and REST
endpoints. Explain both vocabularies when a user mixes them. Agent Management
groups agents into server classes and maps deployment apps to those classes;
an agent can belong to multiple classes. See [About Agent
Management](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/agent-management/about-agent-management)
and [Agent Management
architecture](https://help.splunk.com/splunk-enterprise/administer/update-your-deployment/10.4/agent-management/agent-management-architecture).

For the supported agent types, use the architecture page for the exact target
version. Do not imply that Agent Management is the bundle mechanism for an
indexer-cluster peer or search-head-cluster member; those clustered instances
use their cluster manager bundle or search-head deployer mechanisms. See
[About Agent
Management](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/agent-management/about-agent-management).

An agent is pointed at its management server through the documented CLI or
`deploymentclient.conf` path. Changing that target requires the documented
client restart; describe the action and its validation signal, but do not run
it. See [Specify the Agent Management
server](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/configure-agents/specify-the-agent-management-server).

## Rollout contract

For each proposed rollout, identify:

1. the deployment app and the content it packages;
2. the server class receiving it;
3. global, server-class, and app-level client filters that affect matching;
4. the post-delivery state and reload/restart settings; and
5. the expected server-side and client-side validation signals.

Deployment apps can package Splunk apps, configuration files, scripts, and
supporting content. See [Create deployment
apps](https://help.splunk.com/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/create-deployment-apps).
Use the [serverclass.conf
specification](https://help.splunk.com/en/data-management/splunk-enterprise-admin-manual/10.4/configuration-file-reference/10.4.0-configuration-file-reference/serverclass.conf)
and [agent filter
hierarchy](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/set-up-agent-filters)
for matching and app-level settings.

For post-delivery behavior, assess `stateOnClient`, `issueReload`,
`restartSplunkd`, `restartSplunkWeb`, and `restartIfNeeded` only where the
target version documents them. UI assignment changes trigger the documented
deployment/reload behavior; direct `serverclass.conf` or app-content changes
require the documented `splunk reload deploy-server` administrator action.
Do not run it. See [Use serverclass.conf to define server
classes](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/advanced-configuration/use-serverclass.conf-to-define-server-classes)
and [manage apps in Agent
Management](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/use-the-interface-to-manage-apps).

A canary is an environment-specific risk-control recommendation, not a product
guarantee: create or select a narrowly filtered class, validate its calculated
membership and app content, observe delivery and required reload/restart state
on representative agents, then broaden only under the user's change process.
Never report canary or fleet success without current server- and client-side
evidence.

## Scale and performance

Deployment duration depends on server capacity, number of agents, deployment-
app size, and phone-home interval. Increasing the interval can reduce request
load but increases propagation latency; decreasing it can improve response
time while increasing load. Treat this as a tradeoff, not a universal tuning
value. See [estimate Agent Management
performance](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/estimate-agent-management-performance)
and [change the phone-home
interval](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/troubleshooting/troubleshoot-performance-issues/change-the-phone-home-interval-on-every-agent).

For large fleets, include the application-matching cache and its freshness in
the assessment. See [application matching
cache](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/manage-the-agent-management/estimate-agent-management-performance/application-matching-cache).
Periodic Agent Management reload is a documented performance-troubleshooting
option, not a default remedy; require symptoms, timing, and current state before
recommending it. See [reload Agent Management
periodically](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/troubleshooting/troubleshoot-performance-issues/reload-agent-management-periodically).

Splunk Enterprise 10.4 documents Agent Management clusters of up to three
servers and a capacity of 25,000 agents per server, with shared storage and a
load balancer or DNS mechanism. Verify the current version's limits and
requirements before using those figures. See [deploy an Agent Management
cluster](https://help.splunk.com/en/splunk-enterprise/administer/update-your-deployment/10.4/configure-the-agent-management-system/implement-an-agent-management-cluster/deploy-an-agent-management-cluster).

## Remote Upgrader boundary

Deployment Server or Agent Management can deliver Splunk Remote Upgrader
content to targeted Linux Universal Forwarders, but the separately installed
Remote Upgrader performs the upgrade. Package assignment or delivery therefore
proves neither upgrader service health nor upgrade completion. See the [Remote
Upgrader quickstart](https://help.splunk.com/en/splunk-cloud-platform/forward-and-process-data/splunk-remote-upgrader-for-linux-universal-forwarders/10.4/quickstart-guide/quickstart-guide).

Before giving version-specific upgrade guidance, verify the operating platform,
Universal Forwarder version, and Remote Upgrader version against the applicable
public matrix. Do not extend Linux support to another platform or copy a matrix
from a different release. See [supported Universal Forwarder versions and
platforms](https://help.splunk.com/en/data-management/forward-data/splunk-remote-upgrader-for-linux-universal-forwarders/10.4/about-the-splunk-remote-upgrader-for-linux-universal-forwarders/supported-universal-forwarder-versions).

If content was delivered but execution is stuck, preserve the delivery finding
and route only service execution or platform-support diagnosis to the Remote
Upgrader owner with platform, versions, service state, and sanitized upgrader
logs. Do not treat the route as evidence of a particular cause.
