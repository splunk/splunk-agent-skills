# Public Governance Guidance

Use these public Splunk sources for documented expectations. Check that a page
still applies to the user's product and version. Put the citation beside the
action or behavior it supports; do not cite this reference as authority.

## Inventory and periodic review

- Use the applicable Splunk Web **Settings** pages to list and inspect
  knowledge objects. Available views and actions vary by object type and user
  permissions. [Manage knowledge objects through Settings pages](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/manage-knowledge-objects-through-settings-pages)
- Periodically review objects for ownership, permissions, naming, redundancy,
  and continued need before cleanup. [Monitor and organize knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/monitor-and-organize-knowledge-objects)
- Treat Settings, REST, metadata, and screenshots as observations of a
  deployment, not proof of general product behavior. Record the source and
  missing fields for every object-level conclusion.

## Naming and collision review

- Establish a naming convention that communicates the object's use and
  context. [Develop naming conventions for knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/develop-naming-conventions-for-knowledge-objects)
- Keep names unique within a knowledge-object type to reduce ambiguity and
  collision risk when scope or context changes. Diagnose an actual collision
  only after comparing the relevant source and target inventories. [Give knowledge objects of the same type unique names](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/give-knowledge-objects-of-the-same-type-unique-names)
- When policy is unresolved, recommend namespace, rename, or staging review;
  do not choose which object wins.

## Sharing and permissions

- Distinguish private, app-shared, and globally shared objects and record role
  read/write access when the evidence exposes it. Review permissions against
  least privilege and the workflow that must remain authorized. [Manage knowledge object permissions](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/manage-knowledge-object-permissions)
- App-level write access can allow a role to modify or delete knowledge
  objects in that app. Treat write-capable roles as an approval-sensitive
  exposure review, not an automatic finding that access is wrong. [Manage knowledge object permissions](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/manage-knowledge-object-permissions)
- For Splunk Enterprise app packaging, use `default.meta` for shipped defaults,
  `local.meta` for local overrides, and review system export deliberately when
  an object must be visible outside its app. Keep this as packaging guidance,
  not authority to edit metadata. [Set permissions for objects in an app](https://dev.splunk.com/enterprise/docs/developapps/manageknowledge/setpermissionsforobjects)

## Ownership and orphan handling

- Confirm orphan status from both object ownership and current user evidence;
  a missing or unfamiliar owner value alone is insufficient. Use the documented
  **Reassign Knowledge Objects** workflow when applicable, and require an
  authorized administrator, intended new owner, capability prerequisites, and
  review of scheduled content before any reassignment. [Manage orphaned knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/manage-orphaned-knowledge-objects)
- Orphaned scheduled searches do not run. Make the schedule impact explicit
  when evidence confirms that a scheduled search is orphaned. [Manage orphaned knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/manage-orphaned-knowledge-objects)
- For Splunk Cloud Platform, use the documented Splunk Web administration
  workflow. Do not instruct administrators to recover ownership by directly
  editing configuration files. [Manage orphaned knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/manage-orphaned-knowledge-objects)

## Lifecycle safety

- Check downstream use before disabling, moving, or deleting an object because
  searches, reports, dashboards, event types, and summary indexing can depend
  on knowledge objects. [Disable or delete knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/disable-or-delete-knowledge-objects)
- Prefer owner review, dependency checks, backup/export, and a reversible
  disable-first path before deletion. If usage or dependency evidence is
  missing, do not recommend deletion as the acceptance condition. [Disable or delete knowledge objects](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/disable-or-delete-knowledge-objects)

## REST and ACL surfaces

- REST knowledge-object endpoints can expose object and ACL state and can also
  support administration. In this skill, use only user-supplied or explicitly
  authorized read output; never invoke create, update, move, ACL-write, or
  delete operations. [Managing knowledge objects with the REST API](https://help.splunk.com/en/splunk-enterprise/leverage-rest-apis/rest-api-tutorials/10.4/rest-api-tutorials/managing-knowledge-objects)

## Evidence-only governance areas

The cited source set does not establish a universal lookup quota, retention
period, checksum scheme, or repair procedure. For lookups, assess only supplied
ownership, sharing, write path, provenance, size/storage, retention, and
monitoring facts. For search-head consistency, compare only supplied member
presence, metadata, owners, and checksum/version indicators. Route operational
diagnosis or repair to the platform-operations owner without inventing limits
or prescribing resync.
