# Public guidance

Use these public Splunk sources at the point where they support a decisive
action. Version-qualify guidance and prefer the user's installed product and
CIM documentation when it differs from these routes.

## Field extraction choices

- Start with Splunk's field and extraction overview when choosing among
  automatic key-value, delimiter, regular-expression, multivalue, and
  structured-data extraction. Prefer search-time extraction; treat index-time
  extraction as a performance-sensitive exception:
  [Fields and field extractions](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/9.3/fields-and-field-extractions).
- Use an inline `EXTRACT-<class>` rule for a regex extraction scoped in
  `props.conf`; use `REPORT-<class>` plus `transforms.conf` when a reusable
  transform or advanced extraction is warranted:
  [Regular expressions with field extractions](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.0/fields-and-field-extractions/about-regular-expressions-with-field-extractions)
  and [advanced field transforms](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/9.0/use-the-configuration-files-to-configure-field-extractions/configure-advanced-extractions-with-field-transforms).
- Use search commands such as `rex`, `extract`/`kv`, `multikv`, `spath`,
  `xmlkv`, `xpath`, or `kvform` for ad hoc, search-local extraction:
  [Extract fields with search commands](https://help.splunk.com/en/splunk-enterprise/search/search-manual/10.0/evaluate-and-manipulate-fields/extract-fields-with-search-commands).
- Check operation order before combining extractions, aliases, lookups, and
  calculated fields:
  [Sequence of search-time operations](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/get-started-with-knowledge-objects/the-sequence-of-search-time-operations).

## Normalization mechanisms

- Use a field alias when an existing field already has the needed value and a
  normalized name is required:
  [Configure field aliases with props.conf](https://help.splunk.com/en/splunk-cloud-platform/manage-knowledge-objects/knowledge-management-manual/9.3.2408/field-aliases/configure-field-aliases-with-props.conf).
- Use a lookup for documented enrichment or value normalization use cases:
  [About lookups](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.2/use-lookups-in-splunk-web/about-lookups).
- Use a calculated field for a reusable derived value, while respecting
  search-time operation order:
  [Create calculated fields](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.0/calculated-fields/create-calculated-fields-with-splunk-web).
- Use event types and tags to classify the events selected by a CIM dataset:
  [Define event types](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.0/event-types/define-event-types-in-splunk-web).

## CIM mapping and validation

- Ground normalization in the event's meaning and CIM's common semantic model:
  [CIM overview](https://help.splunk.com/en/splunk-enterprise/common-information-model/6.1/introduction/overview-of-the-splunk-common-information-model)
  and [normalize data at search time](https://help.splunk.com/data-management/common-information-model/6.1/using-the-common-information-model/use-the-cim-to-normalize-data-at-search-time).
- Read the target dataset table for required tags, fields, types, descriptions,
  and expected values. The reference tables do not expose every inherited field
  or constraint; use the installed Data Model Editor or model JSON for the
  complete local definition:
  [Use the CIM data model reference tables](https://help.splunk.com/en/data-management/common-information-model/6.0/data-models/how-to-use-the-cim-data-model-reference-tables).
- Validate with field inspection and the appropriate CIM paths, including
  Pivot/Datasets, `datamodel`, `from datamodel`, `datamodelsimple`, and the CIM
  Validation data model's Missing Extractions and Untagged Events datasets:
  [Use the CIM to validate your data](https://help.splunk.com/en/data-management/common-information-model/6.1/using-the-common-information-model/use-the-cim-to-validate-your-data).

Installing or upgrading the CIM add-on is outside this skill. If installation
status itself blocks mapping, route that operational action and cite the
[CIM installation documentation](https://help.splunk.com/en/data-management/common-information-model/6.3/introduction/install-the-splunk-common-information-model-add-on).
