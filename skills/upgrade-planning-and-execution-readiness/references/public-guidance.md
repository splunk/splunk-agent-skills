# Public Upgrade Guidance

Use these public Splunk sources as starting points, then confirm that each page
still matches the user's product, target release, and topology. Cite the direct
page beside the action or conclusion it supports. This reference is not itself
product authority.

## Scope and release binding

- Select the upgrade documentation for the actual target release, inventory the
  deployment, back it up, capture a baseline, follow the applicable upgrade
  workflow, and validate afterward. The linked page is for Splunk Enterprise
  10.4; do not project it onto another release or Splunk Cloud Platform.
  [How to upgrade Splunk Enterprise](https://help.splunk.com/en/splunk-enterprise/get-started/install-and-upgrade/10.4/upgrade-or-migrate-splunk-enterprise/how-to-upgrade-splunk-enterprise)
- For Enterprise 10.4, `9.4.x -> 10.4.x` is not a direct upgrade. Supported
  latest-version routes are `9.4.x -> 10.0.x -> 10.4.x`,
  `9.4.x -> 10.2.x -> 10.4.x`, or
  `9.4.x -> 10.0.x -> 10.2.x -> 10.4.x`. This establishes only the documented
  version path; local readiness still depends on deployment evidence.
  [How to upgrade Splunk Enterprise](https://help.splunk.com/en/splunk-enterprise/get-started/install-and-upgrade/10.4/upgrade-or-migrate-splunk-enterprise/how-to-upgrade-splunk-enterprise)
- Review the target release's public release notes plus its prerequisite and
  breaking-change guidance before deciding readiness. This bundled starting
  point is specific to Enterprise 10.4; locate and cite the matching release
  notes for the user's actual target rather than assuming this page is enough.
  [About upgrading to 10.4: READ THIS FIRST](https://help.splunk.com/en/splunk-enterprise/get-started/install-and-upgrade/10.4/upgrade-or-migrate-splunk-enterprise/about-upgrading-to-10.4-read-this-first)
- Check the target release's supported operating systems, architectures,
  filesystems, and hardware guidance against the supplied infrastructure
  inventory.
  [System requirements for Splunk Enterprise on-premises](https://help.splunk.com/en/splunk-enterprise/administer/install-and-upgrade/10.4/plan-your-splunk-enterprise-installation/system-requirements-for-use-of-splunk-enterprise-on-premises)

## Compatibility and prechecks

- Check each named Splunk premium product against the public product-version
  matrix; a matrix result does not establish third-party app compatibility.
  [Splunk products version compatibility matrix](https://help.splunk.com/en/splunk-enterprise/release-notes-and-updates/compatibility-matrix/splunk-products-version-compatibility/splunk-products-version-compatibility-matrix)
- Do not prescribe the retired Upgrade Readiness App as a current universal
  precheck. Confirm its lifecycle and use current documented replacement paths.
  [Sunsetting of the Upgrade Readiness App](https://help.splunk.com/en/splunk-enterprise/administer/upgrade-readiness-app/9.4/get-started/sunsetting-of-the-upgrade-readiness-app)
- Use applicable Monitoring Console health checks as one documented health
  surface, while treating actual results as environment evidence.
  [Access and customize health check](https://help.splunk.com/en/data-management/monitor-and-troubleshoot/monitor-data-in-splunk-enterprise/10.4/configure-the-monitoring-console/access-and-customize-health-check)

## Backup and recovery

- Represent configuration backup as its own readiness item and require evidence
  that the relevant configuration was captured.
  [Back up configuration information](https://help.splunk.com/en/data-management/splunk-enterprise-admin-manual/10.4/administer-splunk-enterprise-with-configuration-files/back-up-configuration-information)
- Represent indexed-data protection separately when indexed data is in scope;
  select the strategy applicable to the actual deployment and release.
  [Back up and archive your indexes](https://help.splunk.com/en/data-management/manage-splunk-enterprise-indexers/10.0/back-up-and-archive-your-indexes)
- Represent KV-store backup and restore separately when apps or platform state
  depend on KV store, and require restore-validation evidence before declaring
  recovery ready.
  [Back up and restore KV store](https://help.splunk.com/en/data-management/splunk-enterprise-admin-manual/10.4/administer-the-app-key-value-store/back-up-and-restore-kv-store)
- For the cited Enterprise 10.4 context, plan recovery from verified backups
  rather than promise a supported in-place rollback to the earlier release.
  Recheck the exact target-release guidance and never infer rollback support
  merely because backups exist.
  [How to upgrade Splunk Enterprise](https://help.splunk.com/en/splunk-enterprise/get-started/install-and-upgrade/10.4/upgrade-or-migrate-splunk-enterprise/how-to-upgrade-splunk-enterprise)

## Topology-aware sequencing

- Use the target-release indexer-cluster procedure only after evidence confirms
  an indexer cluster and its current health and version meet the procedure's
  prerequisites.
  [Upgrade an indexer cluster](https://help.splunk.com/en/splunk-enterprise/administer/manage-indexers-and-indexer-clusters/10.4/deploy-the-indexer-cluster/upgrade-an-indexer-cluster)
- Treat automated rolling upgrade as conditional on the documented version,
  platform, cluster-health, and maintenance constraints, not as a default.
  [Perform an automated rolling upgrade of an indexer cluster](https://help.splunk.com/en/splunk-enterprise/administer/manage-indexers-and-indexer-clusters/10.4/deploy-the-indexer-cluster/perform-an-automated-rolling-upgrade-of-an-indexer-cluster)
- Use the target-release search-head-cluster procedure only when evidence
  confirms that topology, mode, and constraints.
  [Upgrade a search head cluster](https://help.splunk.com/en/splunk-enterprise/administer/distributed-search/10.4/deploy-search-head-clustering/upgrade-a-search-head-cluster)

## Cloud and documentation gaps

The bundled public sources above primarily establish Splunk Enterprise
guidance. They do not establish an exact Splunk Cloud Platform upgrade sequence,
maintenance promise, or customer execution authority. For a Cloud request,
preserve supplied service, maintenance, dependency, baseline, owner, and
support-plan facts; research current public Cloud guidance for the exact request
if available; and keep execution support-assisted. If exact public guidance is
not available, state the gap and request the smallest applicable support plan
or maintenance evidence instead of reusing Enterprise steps.
