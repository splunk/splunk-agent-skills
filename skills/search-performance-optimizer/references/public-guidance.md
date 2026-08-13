# Public optimization guidance

Use these public Splunk 10.4 sources as direct anchors, then verify that the
product, deployment, and version apply to the user's case. Cite the direct page
beside every decisive documentation-backed recommendation. Documentation
defines supported guidance; it does not prove a specific search improved.

## Search-job evidence

- Job Inspector and Job Details expose optimized search strings, execution
  costs, counts, and per-indexer timing: [View search job
  properties](https://help.splunk.com/en/splunk-enterprise/search/search-manual/10.4/manage-jobs/view-search-job-properties).
- Built-in optimization can change predicates, push work down, split or reorder
  processing, and remove projections; compare authored and optimized forms in
  Job Inspector: [Built-in
  optimization](https://help.splunk.com/en/splunk-enterprise/search/search-manual/10.4/optimize-searches/built-in-optimization).

## SPL optimization

- Retrieve only required data, minimize movement, preserve parallelism, and use
  an appropriate time window: [About search
  optimization](https://help.splunk.com/en/splunk-enterprise/search/search-manual/10.4/optimize-searches/about-search-optimization).
- Narrow time ranges, use specific indexed metadata, filter early, reduce
  fields, avoid unnecessary wildcards, and delay non-streaming commands when
  semantics permit: [Quick tips for
  optimization](https://help.splunk.com/en/splunk-enterprise/search/search-manual/10.4/optimize-searches/quick-tips-for-optimization).
- Consider command type, concurrency, and indexer-versus-search-head processing
  when explaining likely cost: [Write better
  searches](https://help.splunk.com/en/splunk-enterprise/search/search-manual/10.4/optimize-searches/write-better-searches).

Before applying any pattern, state the semantic condition under which it is
safe. An earlier filter, reduced field set, moved command, or metadata
constraint can change results. Rank a concrete rewrite only when the current
SPL and intended semantics are available.

## tstats and acceleration

- `tstats` searches indexed fields and accelerated data models, but memory and
  high-cardinality cases require measurement rather than an assumed win:
  [`tstats`](https://help.splunk.com/en/splunk-enterprise/spl-search-reference/10.4/search-commands/tstats).
- Data-model acceleration uses summaries; evaluate model coverage, pruning,
  summary coverage, storage and background load, and the completeness effect of
  `summariesonly`: [Accelerate data
  models](https://help.splunk.com/en/splunk-enterprise/manage-knowledge-objects/knowledge-management-manual/10.4/use-data-summaries-to-accelerate-searches/accelerate-data-models).
- Report acceleration applies to qualifying repeated searches and trades
  faster reuse against summary range, storage, and background-search cost:
  [Accelerate
  reports](https://help.splunk.com/en/splunk-enterprise/create-dashboards-and-reports/reporting-manual/10.4/report-management/accelerate-reports).

Require an equivalent benchmark for the original and accelerated variants.
Do not recommend enabling or maintaining acceleration as a standalone project;
that crosses this skill's boundary.

## Workload and platform evidence

- Search Activity dashboards provide deployment and instance views for
  runtime, CPU, memory, SID correlation, concurrency, and indexer imbalance:
  [Search:
  Search Activity](https://help.splunk.com/en/splunk-enterprise/administer/monitor/10.4/monitoring-console-dashboard-reference/search-search-activity).

Use these observations only when the user supplies them. A single slow search
does not establish concurrency, resource pressure, or indexer imbalance.
