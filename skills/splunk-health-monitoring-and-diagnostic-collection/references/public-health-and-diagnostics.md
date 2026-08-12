# Public Health and Diagnostic Guidance

Use these public Splunk sources for documented expectations. Confirm that the
page applies to the user's product and version. Put each citation beside the
action or behavior it supports; do not cite this reference as authority.

## Splunk Cloud Platform

- Use Cloud Monitoring Console (CMC) for Splunk Cloud deployment monitoring;
  the documented access prerequisite is the `sc_admin` role. CMC covers areas
  including ingestion, forwarders, HEC, indexing, licensing, search, user
  activity, and workload management. [Introduction to the Cloud Monitoring Console](https://help.splunk.com/en/splunk-cloud-platform/administer/admin-manual/10.5.2605/monitor-your-splunk-cloud-platform-deployment/introduction-to-the-cloud-monitoring-console)
- Use the CMC Health dashboard to inspect overall, collection, indexing,
  search, and security indicators. Report the exact indicator, validation
  criteria, state, suggested action, and observation time rather than treating
  an aggregate color as a diagnosis. [Use the Health dashboard](https://help.splunk.com/en/splunk-cloud-platform/administer/admin-manual/10.5.2605/monitor-your-splunk-cloud-platform-deployment/use-the-health-dashboard)
- For a persistent CMC dashboard error, retain the exact error and failed
  search ID for a Support handoff. [Troubleshoot the CMC dashboards](https://help.splunk.com/en/splunk-cloud-platform/administer/admin-manual/10.5.2605/monitor-your-splunk-cloud-platform-deployment/introduction-to-the-cloud-monitoring-console/troubleshoot-the-cmc-dashboards)

Do not substitute Splunk Enterprise Monitoring Console or local CLI guidance
for CMC behavior.

## Splunk Enterprise

- Distinguish the search-based Monitoring Console dashboards from the
  REST-based splunkd health report. Use the surface that matches the question
  and deployment role. [Monitoring Splunk Enterprise overview](https://help.splunk.com/en/splunk-enterprise/administer/monitor/10.4/introduction/monitoring-splunk-enterprise-overview)
- Use Monitoring Console dashboards for deployment-wide views such as search,
  indexing, resource usage, and distributed components, subject to documented
  setup and access. [What can the Monitoring Console do?](https://help.splunk.com/en/splunk-enterprise/administer/monitor/10.4/about-the-monitoring-console/what-can-the-monitoring-console-do)
- When reporting a Monitoring Console health check, retain the exact check and
  its documented `Error`, `Warning`, `Info`, `Success`, or `N/A` result.
  Relevant checks include indexing state, memory, licenses, forwarders, disk,
  and event-processing queue saturation. [Access and customize health check](https://help.splunk.com/en/splunk-enterprise/administer/monitor/10.4/configure-the-monitoring-console/access-and-customize-health-check)
- Investigate a splunkd feature health change through its health tree, related
  messages and details, `health.log`, or the documented
  `/services/server/health/splunkd` and details endpoints. Record the feature,
  emitted state, messages, node, and time; the state alone does not prove root
  cause. [Investigate feature health status changes](https://help.splunk.com/en/splunk-enterprise/administer/monitor/10.4/proactive-splunk-component-monitoring-with-the-splunkd-health-report/investigate-feature-health-status-changes)

## diag

- Explain that `diag` collects configuration, internal logs, platform and
  system information, and index metadata, not indexed data. Splunk documents
  collection through Splunk Web or CLI and supported remote collection.
  Confirm product/version, permission, node scope, time need, and output
  location before selecting a procedure. [Generate a diagnostic file](https://help.splunk.com/en/splunk-enterprise/administer/troubleshoot/10.4/contact-splunk-support/generate-a-diagnostic-file)
- Review every bundle before upload. Apply documented exclusions and
  search-string redaction where needed, and do not assume the tool guarantees
  compliance with local privacy or security policy. [Generate a diagnostic file](https://help.splunk.com/en/splunk-enterprise/administer/troubleshoot/10.4/contact-splunk-support/generate-a-diagnostic-file)
- When event samples are necessary, use the documented sample-anonymization
  workflow and review its result rather than placing raw customer data in the
  packet. [Anonymize data samples to send to Support](https://help.splunk.com/en/splunk-enterprise/administer/troubleshoot/10.4/contact-splunk-support/anonymize-data-samples-to-send-to-support)
- Upload only with explicit user authorization and an open Support case through
  the documented workflow. This skill may describe that option but does not
  upload. [Generate a diagnostic file](https://help.splunk.com/en/splunk-enterprise/administer/troubleshoot/10.4/contact-splunk-support/generate-a-diagnostic-file)

## RapidDiag

- Use RapidDiag only for its documented Splunk Enterprise scope. It is
  documented for Splunk Enterprise 8.1.1 and higher, Linux only, and requires
  the `get_diag` capability. Its CLI is local-only, and universal forwarders
  are unsupported. Confirm each constraint and the participating node scope
  before giving specific collection steps. [Using RapidDiag](https://help.splunk.com/en/splunk-enterprise/administer/troubleshoot/10.4/contact-splunk-support/using-rapiddiag)
- Use RapidDiag to coordinate targeted diagnostic collection with operating
  system and Splunk tools across supported Enterprise nodes. Treat the output
  as a privacy-reviewable artifact, not as proof of cause. [Using RapidDiag](https://help.splunk.com/en/splunk-enterprise/administer/troubleshoot/10.4/contact-splunk-support/using-rapiddiag)

## Evidence boundary

Documentation can establish a surface, prerequisite, status category,
endpoint, or collection workflow. It cannot establish that a user's deployment
is healthy, that an observed alert is accurate, or that a diagnostic bundle
proves root cause. Those conclusions require current, scoped user evidence.
