package splunk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResultStoreSaveCreatesTableAndMetadata(t *testing.T) {
	store := NewResultStore(t.TempDir())
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	saved, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs")
	if err != nil {
		t.Fatal(err)
	}
	if !saved.OK || saved.Table != "identity_logs" || saved.Rows != 2 || saved.CreatedAt != now.Unix() {
		t.Fatalf("unexpected saved result: %#v", saved)
	}

	db := openResultDB(t, store.Path())
	defer db.Close()

	var metadataRows int
	if err := db.QueryRow(`SELECT rows FROM searches WHERE table_name = ?`, "identity_logs").Scan(&metadataRows); err != nil {
		t.Fatal(err)
	}
	if metadataRows != 2 {
		t.Fatalf("expected metadata rows=2, got %d", metadataRows)
	}

	var service, nested, rawJSON string
	if err := db.QueryRow(`SELECT service, nested, _json FROM identity_logs WHERE _row = 1`).Scan(&service, &nested, &rawJSON); err != nil {
		t.Fatal(err)
	}
	if service != "identity" || nested != `{"a":1}` {
		t.Fatalf("unexpected stored values service=%q nested=%q", service, nested)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(rawJSON), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["event"] != "login" {
		t.Fatalf("unexpected raw json: %#v", decoded)
	}
}

func TestResultStoreSavePersistsWarnings(t *testing.T) {
	store := NewResultStore(t.TempDir())
	warnings := []string{"broad search warning", "second warning"}
	saved, err := store.SaveWithOptions(context.Background(), sampleSearchResult(), "identity_logs", SaveOptions{Warnings: warnings})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(saved.Warnings, ",") != strings.Join(warnings, ",") || saved.WarningCount != 2 || saved.AcceptedWarningCount != 0 {
		t.Fatalf("unexpected saved warnings: %#v", saved)
	}
	if len(saved.WarningDetails) != 2 || saved.WarningDetails[0].Code != ResultWarningCodeLegacy || saved.WarningDetails[0].Accepted {
		t.Fatalf("unexpected saved warning details: %#v", saved.WarningDetails)
	}

	info, err := store.Info(context.Background(), ResultInfoRequest{Table: "identity_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if info.WarningCount != 2 || info.AcceptedWarningCount != 0 || strings.Join(info.Warnings, ",") != strings.Join(warnings, ",") {
		t.Fatalf("unexpected info warnings: %#v", info)
	}
	if len(info.WarningDetails) != 2 || info.WarningDetails[0].Code != ResultWarningCodeLegacy {
		t.Fatalf("unexpected info warning details: %#v", info.WarningDetails)
	}

	list, err := store.ListTables(context.Background(), ListResultTablesRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Tables[0].WarningCount != 2 || list.Tables[0].AcceptedWarningCount != 0 || strings.Join(list.Tables[0].Warnings, ",") != strings.Join(warnings, ",") {
		t.Fatalf("unexpected listed warnings: %#v", list.Tables[0])
	}
}

func TestResultStoreAcceptWarningMarksFullFetchIntentional(t *testing.T) {
	store := NewResultStore(t.TempDir())
	now := time.Date(2026, 4, 28, 10, 30, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	warning := ResultWarning{
		Code:    ResultWarningCodeFullFetch,
		Message: "full fetch warning",
	}
	if _, err := store.SaveWithOptions(context.Background(), sampleSearchResult(), "identity_logs", SaveOptions{WarningDetails: []ResultWarning{warning}}); err != nil {
		t.Fatal(err)
	}

	accepted, err := store.AcceptWarning(context.Background(), AcceptResultWarningRequest{
		Table: "identity_logs",
		Code:  "full-fetch",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !accepted.OK || !accepted.Accepted || accepted.WarningCount != 0 || accepted.AcceptedWarningCount != 1 {
		t.Fatalf("unexpected accept result: %#v", accepted)
	}
	if len(accepted.WarningDetails) != 1 || !accepted.WarningDetails[0].Accepted || accepted.WarningDetails[0].AcceptedAt != now.Unix() {
		t.Fatalf("unexpected accepted warning details: %#v", accepted.WarningDetails)
	}

	info, err := store.Info(context.Background(), ResultInfoRequest{Table: "identity_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if info.WarningCount != 0 || info.AcceptedWarningCount != 1 || len(info.AcceptedWarnings) != 1 || info.AcceptedWarnings[0] != warning.Message {
		t.Fatalf("expected accepted warning in info, got %#v", info)
	}

	list, err := store.ListTables(context.Background(), ListResultTablesRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Tables[0].WarningCount != 0 || list.Tables[0].AcceptedWarningCount != 1 || len(list.Tables[0].WarningDetails) != 1 {
		t.Fatalf("expected accepted warning in list, got %#v", list.Tables[0])
	}

	again, err := store.AcceptWarning(context.Background(), AcceptResultWarningRequest{
		Table: "identity_logs",
		Code:  ResultWarningCodeFullFetch,
	})
	if err != nil {
		t.Fatal(err)
	}
	if again.Accepted || !strings.Contains(again.Message, "already accepted") {
		t.Fatalf("expected idempotent already-accepted result, got %#v", again)
	}
}

func TestResultStoreAcceptWarningRejectsMissingCode(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.SaveWithOptions(context.Background(), sampleSearchResult(), "identity_logs", SaveOptions{
		WarningDetails: []ResultWarning{{Code: ResultWarningCodeFullFetch, Message: "full fetch warning"}},
	}); err != nil {
		t.Fatal(err)
	}

	_, err := store.AcceptWarning(context.Background(), AcceptResultWarningRequest{
		Table: "identity_logs",
		Code:  "unknown",
	})
	if err == nil || !strings.Contains(err.Error(), `warning code "unknown" was not found`) {
		t.Fatalf("expected missing warning code error, got %v", err)
	}
}

func TestResultStoreUpgradesOldMetadataForWarnings(t *testing.T) {
	configDir := t.TempDir()
	store := NewResultStore(configDir)
	db := openResultDB(t, store.Path())
	if _, err := db.Exec(`CREATE TABLE searches (
		table_name TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		app TEXT NOT NULL,
		sid TEXT NOT NULL,
		query TEXT NOT NULL,
		earliest TEXT NOT NULL,
		latest TEXT NOT NULL,
		result_count INTEGER NOT NULL,
		rows INTEGER NOT NULL,
		offset INTEGER NOT NULL,
		has_more INTEGER NOT NULL,
		run_duration REAL NOT NULL,
		created_at INTEGER NOT NULL
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE identity_logs (_row INTEGER PRIMARY KEY, _json TEXT NOT NULL, service TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO searches (
		table_name, url, app, sid, query, earliest, latest, result_count, rows, offset, has_more, run_duration, created_at
	) VALUES ('identity_logs', 'https://splunk.example.com', 'search', 'sid-old', 'search index=test', '-1m', 'now', 1, 1, 0, 0, 0.1, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := store.Info(context.Background(), ResultInfoRequest{Table: "identity_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if info.WarningCount != 0 || len(info.Warnings) != 0 {
		t.Fatalf("expected empty upgraded warnings, got %#v", info)
	}

	db = openResultDB(t, store.Path())
	defer db.Close()
	columns, err := tableColumnNames(context.Background(), db, "searches")
	if err != nil {
		t.Fatal(err)
	}
	if !columns["warnings_json"] {
		t.Fatal("expected warnings_json column after metadata upgrade")
	}
}

func TestResultStoreMalformedWarningJSONDoesNotBreakList(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.SaveWithOptions(context.Background(), sampleSearchResult(), "identity_logs", SaveOptions{Warnings: []string{"warning"}}); err != nil {
		t.Fatal(err)
	}
	db := openResultDB(t, store.Path())
	if _, err := db.Exec(`UPDATE searches SET warnings_json = ? WHERE table_name = ?`, "not json", "identity_logs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	list, err := store.ListTables(context.Background(), ListResultTablesRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if list.Tables[0].WarningCount != 0 || len(list.Tables[0].Warnings) != 0 {
		t.Fatalf("expected malformed warnings to degrade to empty, got %#v", list.Tables[0])
	}
}

func TestResultStoreReadsLegacyWarningJSON(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	db := openResultDB(t, store.Path())
	if _, err := db.Exec(`UPDATE searches SET warnings_json = ? WHERE table_name = ?`, `["legacy warning"]`, "identity_logs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	info, err := store.Info(context.Background(), ResultInfoRequest{Table: "identity_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if info.WarningCount != 1 || len(info.WarningDetails) != 1 || info.WarningDetails[0].Code != ResultWarningCodeLegacy {
		t.Fatalf("expected legacy warning details, got %#v", info)
	}
}

func TestResultStoreSaveReplacesExplicitTable(t *testing.T) {
	store := NewResultStore(t.TempDir())
	first := sampleSearchResult()
	if _, err := store.Save(context.Background(), first, "identity_logs"); err != nil {
		t.Fatal(err)
	}

	second := sampleSearchResult()
	second.Results = second.Results[:1]
	second.ResultCount = 1
	second.ReturnedResults = 1
	second.Query = "search index=other"
	saved, err := store.Save(context.Background(), second, "identity_logs")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Rows != 1 || saved.Query != "search index=other" {
		t.Fatalf("unexpected replacement summary: %#v", saved)
	}

	db := openResultDB(t, store.Path())
	defer db.Close()

	var tableRows, metadataRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM identity_logs`).Scan(&tableRows); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT rows FROM searches WHERE table_name = ?`, "identity_logs").Scan(&metadataRows); err != nil {
		t.Fatal(err)
	}
	if tableRows != 1 || metadataRows != 1 {
		t.Fatalf("expected replacement row count 1, got table=%d metadata=%d", tableRows, metadataRows)
	}
}

func TestResultStoreRejectsInvalidExplicitTable(t *testing.T) {
	store := NewResultStore(t.TempDir())
	_, err := store.Save(context.Background(), sampleSearchResult(), "bad-name")
	if err == nil || !strings.Contains(err.Error(), "invalid --result-table") {
		t.Fatalf("expected invalid table error, got %v", err)
	}
}

func TestResultStoreSerializesConcurrentSaves(t *testing.T) {
	store := NewResultStore(t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	const writers = 24
	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result := sampleSearchResult()
			result.SID = fmt.Sprintf("sid-%d", index)
			result.Query = fmt.Sprintf("search index=test writer=%d", index)
			_, err := store.Save(ctx, result, fmt.Sprintf("identity_logs_%02d", index))
			errs <- err
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	db := openResultDB(t, store.Path())
	defer db.Close()

	var metadataRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM searches`).Scan(&metadataRows); err != nil {
		t.Fatal(err)
	}
	if metadataRows != writers {
		t.Fatalf("expected %d saved searches, got %d", writers, metadataRows)
	}
}

func TestResultStoreCleanupOlderThanDropsExpiredTables(t *testing.T) {
	store := NewResultStore(t.TempDir())
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	store.now = func() time.Time { return now.Add(-25 * time.Hour) }
	if _, err := store.Save(context.Background(), sampleSearchResult(), "old_results"); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	if _, err := store.Save(context.Background(), sampleSearchResult(), "fresh_results"); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanupOlderThan(context.Background(), 24*time.Hour); err != nil {
		t.Fatal(err)
	}

	db := openResultDB(t, store.Path())
	defer db.Close()

	if tableExistsInDB(t, db, "old_results") {
		t.Fatal("expected old_results to be dropped")
	}
	if !tableExistsInDB(t, db, "fresh_results") {
		t.Fatal("expected fresh_results to remain")
	}
	if searchMetadataExists(t, db, "old_results") {
		t.Fatal("expected old metadata to be deleted")
	}
	if !searchMetadataExists(t, db, "fresh_results") {
		t.Fatal("expected fresh metadata to remain")
	}
}

func TestResultStoreCleanupOlderThanCompactsAfterDroppingExpiredTables(t *testing.T) {
	store := NewResultStore(t.TempDir())
	now := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	store.now = func() time.Time { return now.Add(-25 * time.Hour) }
	if _, err := store.Save(context.Background(), largeSearchResult(), "old_large_results"); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return now }
	if _, err := store.Save(context.Background(), sampleSearchResult(), "fresh_results"); err != nil {
		t.Fatal(err)
	}
	before := resultDBFileSize(t, store.Path())

	if err := store.CleanupOlderThan(context.Background(), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	after := resultDBFileSize(t, store.Path())
	if after >= before {
		t.Fatalf("expected cleanup compaction to shrink db, before=%d after=%d", before, after)
	}
}

func TestResultStoreQuerySelectsSavedResults(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Query(context.Background(), ResultQueryRequest{
		Table: "identity_logs",
		Query: `SELECT _row, service, event FROM results WHERE service = 'identity' ORDER BY _row`,
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Table != "identity_logs" || result.Limit != 10 {
		t.Fatalf("unexpected query result metadata: %#v", result)
	}
	if result.Truncated || result.Message != "" {
		t.Fatalf("did not expect truncation: %#v", result)
	}
	if strings.Join(result.Columns, ",") != "_row,service,event" {
		t.Fatalf("unexpected columns: %#v", result.Columns)
	}
	if len(result.Rows) != 2 || result.Rows[0]["event"] != "login" || result.Rows[1]["event"] != "logout" {
		t.Fatalf("unexpected rows: %#v", result.Rows)
	}
}

func TestResultStoreQueryAppliesOuterLimit(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Query(context.Background(), ResultQueryRequest{
		Table: "identity_logs",
		Query: `SELECT _row, event FROM results ORDER BY _row LIMIT 1000`,
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected outer limit to cap rows to 1, got %#v", result.Rows)
	}
	if !result.Truncated || !strings.Contains(result.Message, "output truncated to --limit=1 rows") {
		t.Fatalf("expected explicit truncation marker, got %#v", result)
	}
}

func TestResultStoreQueryRejectsUnsafeSQL(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{
		`DELETE FROM results`,
		`SELECT * FROM results; SELECT * FROM searches`,
	} {
		_, err := store.Query(context.Background(), ResultQueryRequest{Table: "identity_logs", Query: query, Limit: 10})
		if err == nil {
			t.Fatalf("expected query %q to be rejected", query)
		}
	}
}

func TestResultStoreQueryRejectsInvalidTableWithTableFlagName(t *testing.T) {
	store := NewResultStore(t.TempDir())
	_, err := store.Query(context.Background(), ResultQueryRequest{Table: "bad-name", Query: `SELECT * FROM results`, Limit: 10})
	if err == nil || !strings.Contains(err.Error(), `invalid --table "bad-name"`) {
		t.Fatalf("expected invalid --table error, got %v", err)
	}
}

func TestResultStoreQueryMissingDBAndTable(t *testing.T) {
	store := NewResultStore(t.TempDir())
	_, err := store.Query(context.Background(), ResultQueryRequest{Table: "identity_logs", Query: `SELECT * FROM results`, Limit: 10})
	if err == nil || !strings.Contains(err.Error(), "results database not found") {
		t.Fatalf("expected missing db error, got %v", err)
	}

	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	_, err = store.Query(context.Background(), ResultQueryRequest{Table: "missing_table", Query: `SELECT * FROM results`, Limit: 10})
	if err == nil || !strings.Contains(err.Error(), `result table "missing_table" was not found`) {
		t.Fatalf("expected missing table error, got %v", err)
	}
}

func TestResultStoreSummaryCountsGroups(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), incidentSearchResult(), "incident_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:   "incident_logs",
		GroupBy: []string{"service"},
		Limit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Table != "incident_logs" || result.Truncated {
		t.Fatalf("unexpected summary metadata: %#v", result)
	}
	if strings.Join(result.Columns, ",") != "service,rows" {
		t.Fatalf("unexpected columns: %#v", result.Columns)
	}
	if len(result.Rows) != 2 || result.Rows[0]["service"] != "identity" || int64Value(result.Rows[0]["rows"]) != 3 {
		t.Fatalf("unexpected summary rows: %#v", result.Rows)
	}
}

func TestResultStoreSummaryMetricsAndErrors(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), incidentSearchResult(), "incident_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:      "incident_logs",
		GroupBy:    []string{"service"},
		Metric:     "duration_ms",
		Thresholds: []float64{250, 1000},
		ErrorWhere: `level = "ERROR"`,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	columns := strings.Join(result.Columns, ",")
	for _, want := range []string{"avg_duration_ms", "max_duration_ms", "gte_250", "gte_1000", "error_count", "error_rate"} {
		if !strings.Contains(columns, want) {
			t.Fatalf("summary columns missing %q: %#v", want, result.Columns)
		}
	}
	first := result.Rows[0]
	if first["service"] != "identity" || int64Value(first["rows"]) != 3 || int64Value(first["gte_250"]) != 2 || int64Value(first["gte_1000"]) != 1 || int64Value(first["error_count"]) != 1 {
		t.Fatalf("unexpected first summary row: %#v", first)
	}
}

func TestResultStoreSummaryFiltersExactTimeWindow(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), timedIncidentSearchResult(), "incident_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:      "incident_logs",
		GroupBy:    []string{"service"},
		Metric:     "duration_ms",
		Thresholds: []float64{250, 1000},
		TimeFrom:   "2026-04-28T10:00:00Z",
		TimeTo:     "2026-04-28T10:10:00Z",
		ErrorWhere: `level = "ERROR"`,
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected two time-window groups, got %#v", result.Rows)
	}
	first := result.Rows[0]
	if first["service"] != "identity" || int64Value(first["rows"]) != 2 || int64Value(first["gte_250"]) != 2 || int64Value(first["gte_1000"]) != 1 || int64Value(first["error_count"]) != 1 {
		t.Fatalf("unexpected first time-window summary row: %#v", first)
	}
	second := result.Rows[1]
	if second["service"] != "billing" || int64Value(second["rows"]) != 1 || int64Value(second["gte_250"]) != 1 || int64Value(second["gte_1000"]) != 0 || int64Value(second["error_count"]) != 1 {
		t.Fatalf("unexpected second time-window summary row: %#v", second)
	}
	for _, want := range []string{`"_time" >= '2026-04-28T10:00:00Z'`, `"_time" < '2026-04-28T10:10:00Z'`} {
		if !strings.Contains(result.Query, want) {
			t.Fatalf("summary query missing %q: %s", want, result.Query)
		}
	}
	if strings.Contains(result.Query, `service = "identity"`) {
		t.Fatalf("summary query should not contain a custom where predicate: %s", result.Query)
	}
}

func TestResultStoreSummaryOrdersByRowsAscending(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), incidentSearchResult(), "incident_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:   "incident_logs",
		GroupBy: []string{"service"},
		OrderBy: "rows",
		Order:   "asc",
		Limit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0]["service"] != "billing" || int64Value(result.Rows[0]["rows"]) != 1 {
		t.Fatalf("expected smallest row group first, got %#v", result.Rows)
	}
	if !strings.Contains(result.Query, `ORDER BY "rows" ASC`) {
		t.Fatalf("expected ascending row order in query, got %s", result.Query)
	}
}

func TestResultStoreSummaryOrdersByThresholdCount(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), latencyOrderingSearchResult(), "latency_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:      "latency_logs",
		GroupBy:    []string{"service"},
		Metric:     "duration_ms",
		Thresholds: []float64{250, 1000},
		OrderBy:    "gte_1000",
		Order:      "desc",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0]["service"] != "billing" || int64Value(result.Rows[0]["gte_1000"]) != 2 {
		t.Fatalf("expected highest threshold group first, got %#v", result.Rows)
	}
	if !strings.Contains(result.Query, `ORDER BY "gte_1000" DESC`) {
		t.Fatalf("expected threshold order in query, got %s", result.Query)
	}
}

func TestResultStoreSummaryLatencyPresetOrdersByLowestThreshold(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), latencySLOSearchResult(), "latency_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:      "latency_logs",
		GroupBy:    []string{"service"},
		Metric:     "duration_ms",
		Thresholds: []float64{250, 1000},
		Preset:     "latency",
		Limit:      10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0]["service"] != "api" || int64Value(result.Rows[0]["gte_250"]) != 4 {
		t.Fatalf("expected latency preset to order by lowest threshold, got %#v", result.Rows)
	}
	if !strings.Contains(result.Query, `ORDER BY "gte_250" DESC`) {
		t.Fatalf("expected latency preset threshold order in query, got %s", result.Query)
	}
}

func TestResultStoreSummaryLatencyPresetWithoutThresholdsOrdersByMaxMetric(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), latencyOrderingSearchResult(), "latency_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:   "latency_logs",
		GroupBy: []string{"service"},
		Metric:  "duration_ms",
		Preset:  "latency",
		Limit:   10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Rows[0]["service"] != "billing" || int64Value(result.Rows[0]["max_duration_ms"]) != 2500 {
		t.Fatalf("expected latency preset without thresholds to order by max metric, got %#v", result.Rows)
	}
	if !strings.Contains(result.Query, `ORDER BY "max_duration_ms" DESC`) {
		t.Fatalf("expected latency preset max order in query, got %s", result.Query)
	}
}

func TestResultStoreSummaryValidatesInputs(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), incidentSearchResult(), "incident_logs"); err != nil {
		t.Fatal(err)
	}
	for _, request := range []ResultSummaryRequest{
		{Table: "incident_logs", Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"missing"}, Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, Metric: "missing", Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, TimeFrom: "2026-04-28T10:00:00Z", Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, ErrorWhere: `level = "ERROR"; DROP TABLE incident_logs`, Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, Preset: "unknown", Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, Preset: "latency", Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, Order: "sideways", Limit: 10},
		{Table: "incident_logs", GroupBy: []string{"service"}, OrderBy: "missing", Limit: 10},
	} {
		if _, err := store.Summary(context.Background(), request); err == nil {
			t.Fatalf("expected summary request %#v to fail", request)
		}
	}
}

func TestResultStoreSummaryTruncates(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), incidentSearchResult(), "incident_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Summary(context.Background(), ResultSummaryRequest{
		Table:   "incident_logs",
		GroupBy: []string{"service"},
		Limit:   1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Truncated || len(result.Rows) != 1 || !strings.Contains(result.Message, "output truncated to --limit=1 groups") {
		t.Fatalf("expected truncated summary, got %#v", result)
	}
}

func TestResultStoreEventsMatchesColumnAndOrdersRows(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), eventSearchResult(), "event_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Events(context.Background(), ResultEventsRequest{
		Table: "event_logs",
		Field: "session_id",
		Value: "session-1",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.MatchMode != "field" || result.MatchedField != "session_id" || result.MatchedValue != "session-1" || result.Count != 3 || result.Truncated {
		t.Fatalf("unexpected event metadata: %#v", result)
	}
	if len(result.Rows) != 3 || result.Rows[0]["operation"] != "start" || result.Rows[2]["operation"] != "finish" {
		t.Fatalf("expected event rows ordered by _time, got %#v", result.Rows)
	}
	if strings.Join(result.Columns[:3], ",") != "matched_value,_row,_time" {
		t.Fatalf("unexpected event columns: %#v", result.Columns)
	}
}

func TestResultStoreEventsSupportsRequestIDShortcut(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), jsonEventSearchResult(), "event_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Events(context.Background(), ResultEventsRequest{
		Table:     "event_logs",
		RequestID: "req-json",
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MatchMode != "request_id" || result.MatchedField != "json:auto" || result.MatchedValue != "req-json" || result.Count != 2 {
		t.Fatalf("unexpected request-id shortcut event output: %#v", result)
	}
}

func TestResultStoreEventsSupportsJSONField(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), jsonEventSearchResult(), "event_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.Events(context.Background(), ResultEventsRequest{
		Table:     "event_logs",
		JSONField: "$.sessionId",
		Value:     "json-session",
		Limit:     10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.MatchMode != "json_field" || result.MatchedField != "json:$.sessionId" || result.Count != 2 || result.Rows[0]["message"] != "json start" {
		t.Fatalf("unexpected JSON event output: %#v", result)
	}
}

func TestResultStoreEventsHandlesNoMatchesAndTruncation(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), eventSearchResult(), "event_logs"); err != nil {
		t.Fatal(err)
	}
	empty, err := store.Events(context.Background(), ResultEventsRequest{
		Table: "event_logs",
		Field: "session_id",
		Value: "missing",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if empty.Count != 0 || len(empty.Rows) != 0 || empty.Truncated {
		t.Fatalf("expected empty event result, got %#v", empty)
	}

	truncated, err := store.Events(context.Background(), ResultEventsRequest{
		Table: "event_logs",
		Field: "session_id",
		Value: "session-1",
		Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if truncated.Count != 1 || !truncated.Truncated || !strings.Contains(truncated.Message, "output truncated to --limit=1 events") {
		t.Fatalf("expected truncated event output, got %#v", truncated)
	}
}

func TestResultStoreEventsValidatesInputs(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	for _, request := range []ResultEventsRequest{
		{Table: "identity_logs", Field: "service", Value: "identity", Limit: 0},
		{Table: "identity_logs", Limit: 10},
		{Table: "bad-name", Field: "service", Value: "identity", Limit: 10},
		{Table: "identity_logs", Field: "missing", Value: "x", Limit: 10},
		{Table: "identity_logs", JSONField: "$.requestId", Value: "x", Limit: 10},
		{Table: "identity_logs", Field: "service", JSONField: "$.service", Value: "identity", Limit: 10},
		{Table: "identity_logs", RequestID: "req-1", Field: "service", Limit: 10},
		{Table: "identity_logs", RequestID: "req-1", Limit: 10},
	} {
		if _, err := store.Events(context.Background(), request); err == nil {
			t.Fatalf("expected event request %#v to fail", request)
		}
	}
}

func TestResultStoreSchemaReturnsColumns(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}

	result, err := store.Schema(context.Background(), ResultSchemaRequest{Table: "identity_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Table != "identity_logs" || result.Rows != 2 || result.QueryTable != "results" {
		t.Fatalf("unexpected schema metadata: %#v", result)
	}
	names := make([]string, 0, len(result.Columns))
	for _, column := range result.Columns {
		names = append(names, column.Name)
		switch column.Name {
		case "_row":
			if column.SQLiteType != "INTEGER" || !column.PrimaryKey {
				t.Fatalf("unexpected _row column: %#v", column)
			}
		case "_json":
			if column.SQLiteType != "TEXT" || column.PrimaryKey {
				t.Fatalf("unexpected _json column: %#v", column)
			}
		}
	}
	if got, want := strings.Join(names, ","), "_row,_json,count,event,nested,service"; got != want {
		t.Fatalf("unexpected columns %q, want %q", got, want)
	}
	if result.ColumnCount != len(result.Columns) {
		t.Fatalf("unexpected column count: %#v", result)
	}
}

func TestResultStoreSchemaMissingDBTableAndInvalidName(t *testing.T) {
	store := NewResultStore(t.TempDir())
	_, err := store.Schema(context.Background(), ResultSchemaRequest{Table: "identity_logs"})
	if err == nil || !strings.Contains(err.Error(), "results database not found") {
		t.Fatalf("expected missing db error, got %v", err)
	}
	_, err = store.Schema(context.Background(), ResultSchemaRequest{Table: "bad-name"})
	if err == nil || !strings.Contains(err.Error(), `invalid --table "bad-name"`) {
		t.Fatalf("expected invalid table error, got %v", err)
	}
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	_, err = store.Schema(context.Background(), ResultSchemaRequest{Table: "missing_table"})
	if err == nil || !strings.Contains(err.Error(), `result table "missing_table" was not found`) {
		t.Fatalf("expected missing table error, got %v", err)
	}
}

func TestResultStoreSharedLocksOverlapAndBlockWrites(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	firstUnlock, err := store.lockReads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer firstUnlock()
	secondUnlock, err := store.lockReads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer secondUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	unlockWrite, err := store.lockWrites(ctx)
	if err == nil {
		unlockWrite()
		t.Fatal("expected write lock to wait for readers and return context error")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected context deadline lock error, got %v", err)
	}
}

func TestResultStoreCanceledReadDoesNotLeaveStaleLock(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := store.Query(ctx, ResultQueryRequest{Table: "identity_logs", Query: `SELECT count(*) FROM results`, Limit: 1})
	if err == nil {
		t.Fatal("expected canceled query to fail")
	}

	if _, err := store.DropTables(context.Background(), DropResultTablesRequest{Table: "identity_logs"}); err != nil {
		t.Fatalf("expected later write to acquire lock after canceled query, got %v", err)
	}
}

func TestResultStoreListTablesReturnsMetadataNewestFirst(t *testing.T) {
	store := NewResultStore(t.TempDir())
	oldTime := time.Date(2026, 4, 27, 11, 0, 0, 0, time.UTC)
	newTime := time.Date(2026, 4, 27, 12, 0, 0, 0, time.UTC)

	store.now = func() time.Time { return oldTime }
	if _, err := store.Save(context.Background(), sampleSearchResult(), "old_logs"); err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return newTime }
	if _, err := store.Save(context.Background(), sampleSearchResult(), "new_logs"); err != nil {
		t.Fatal(err)
	}

	result, err := store.ListTables(context.Background(), ListResultTablesRequest{Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Count != 1 || result.Limit != 1 || !result.Truncated {
		t.Fatalf("unexpected list result: %#v", result)
	}
	if !strings.Contains(result.Message, "output truncated to --limit=1 tables") {
		t.Fatalf("expected truncation message, got %#v", result)
	}
	record := result.Tables[0]
	if record.Table != "new_logs" || record.CreatedAt != newTime.Unix() || record.CreatedAtUTC != "2026-04-27T12:00:00Z" {
		t.Fatalf("unexpected newest table record: %#v", record)
	}
	if record.URL != "https://splunk.example.com" || record.Query != "search index=test" || record.Earliest != "-1m" || record.Latest != "now" || record.Rows != 2 {
		t.Fatalf("unexpected table metadata: %#v", record)
	}
}

func TestResultStoreListTablesMissingDBReturnsEmpty(t *testing.T) {
	store := NewResultStore(t.TempDir())
	result, err := store.ListTables(context.Background(), ListResultTablesRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Count != 0 || len(result.Tables) != 0 || result.DB != store.Path() {
		t.Fatalf("unexpected missing-db result: %#v", result)
	}
	if !strings.Contains(result.Message, "no saved result tables") {
		t.Fatalf("expected missing-db message, got %#v", result)
	}
}

func TestResultStoreDropTablesDropsOneTable(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), sampleSearchResult(), "other_logs"); err != nil {
		t.Fatal(err)
	}

	result, err := store.DropTables(context.Background(), DropResultTablesRequest{Table: "identity_logs"})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Count != 1 || strings.Join(result.Dropped, ",") != "identity_logs" {
		t.Fatalf("unexpected drop result: %#v", result)
	}

	db := openResultDB(t, store.Path())
	defer db.Close()
	if tableExistsInDB(t, db, "identity_logs") || searchMetadataExists(t, db, "identity_logs") {
		t.Fatal("expected identity_logs to be dropped")
	}
	if !tableExistsInDB(t, db, "other_logs") || !searchMetadataExists(t, db, "other_logs") {
		t.Fatal("expected other_logs to remain")
	}
}

func TestResultStoreDropTablesDropsAllTables(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), sampleSearchResult(), "other_logs"); err != nil {
		t.Fatal(err)
	}

	result, err := store.DropTables(context.Background(), DropResultTablesRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Count != 2 || strings.Join(result.Dropped, ",") != "identity_logs,other_logs" {
		t.Fatalf("unexpected drop result: %#v", result)
	}

	db := openResultDB(t, store.Path())
	defer db.Close()
	if tableExistsInDB(t, db, "identity_logs") || tableExistsInDB(t, db, "other_logs") {
		t.Fatal("expected all result tables to be dropped")
	}
	var metadataRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM searches`).Scan(&metadataRows); err != nil {
		t.Fatal(err)
	}
	if metadataRows != 0 {
		t.Fatalf("expected no metadata rows, got %d", metadataRows)
	}
}

func TestResultStoreDropTablesCompactsWhenMetadataIsEmpty(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), largeSearchResult(), "large_logs"); err != nil {
		t.Fatal(err)
	}

	db := openResultDB(t, store.Path())
	if _, err := db.Exec(`DROP TABLE large_logs`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM searches WHERE table_name = ?`, "large_logs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	before := resultDBFileSize(t, store.Path())

	result, err := store.DropTables(context.Background(), DropResultTablesRequest{All: true})
	if err != nil {
		t.Fatal(err)
	}
	after := resultDBFileSize(t, store.Path())
	if !result.OK || result.Count != 0 || len(result.Dropped) != 0 || !result.Compacted {
		t.Fatalf("unexpected drop result: %#v", result)
	}
	if result.BytesBefore != before || result.BytesAfter != after || result.BytesReclaimed <= 0 {
		t.Fatalf("unexpected compaction sizes: result=%#v before=%d after=%d", result, before, after)
	}
	if after >= before {
		t.Fatalf("expected drop compaction to shrink db, before=%d after=%d", before, after)
	}
}

func TestResultStoreDropTablesValidatesRequest(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), sampleSearchResult(), "identity_logs"); err != nil {
		t.Fatal(err)
	}
	for _, request := range []DropResultTablesRequest{
		{},
		{All: true, Table: "identity_logs"},
		{Table: "bad-name"},
	} {
		if _, err := store.DropTables(context.Background(), request); err == nil {
			t.Fatalf("expected request %#v to fail", request)
		}
	}
}

func TestResultStoreLargeSavePrintsProgress(t *testing.T) {
	store := NewResultStore(t.TempDir())
	var progress strings.Builder
	if _, err := store.SaveWithOptions(context.Background(), largeProgressSearchResult(), "large_progress", SaveOptions{Progress: &progress}); err != nil {
		t.Fatal(err)
	}
	output := progress.String()
	if !strings.Contains(output, "splsearch: starting table=large_progress") || !strings.Contains(output, "splsearch: done table=large_progress") {
		t.Fatalf("expected progress output, got %q", output)
	}
}

func TestResultStoreLargeSaveEmitsStructuredProgress(t *testing.T) {
	store := NewResultStore(t.TempDir())
	var events []SearchProgressEvent
	if _, err := store.SaveWithOptions(context.Background(), largeProgressSearchResult(), "large_progress", SaveOptions{
		StructuredProgress: func(event SearchProgressEvent) {
			events = append(events, event)
		},
	}); err != nil {
		t.Fatal(err)
	}
	if len(events) < 2 {
		t.Fatalf("expected structured write progress, got %#v", events)
	}
	first := events[0]
	last := events[len(events)-1]
	if first.Phase != searchProgressPhaseWrite || first.State != "starting" || first.Table != "large_progress" || first.WrittenRows != 0 {
		t.Fatalf("unexpected first progress event: %#v", first)
	}
	if last.Phase != searchProgressPhaseWrite || last.State != "done" || last.WrittenRows != resultWriteProgressRows || last.TotalRows != resultWriteProgressRows || last.Percent != 100 {
		t.Fatalf("unexpected final progress event: %#v", last)
	}
}

func largeSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 1
	result.ReturnedResults = 1
	result.Results = []map[string]any{{
		"event":   "large",
		"service": "identity",
		"payload": strings.Repeat("x", 4*1024*1024),
	}}
	return result
}

func sampleSearchResult() SearchResult {
	return SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-123",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     2,
		ReturnedResults: 2,
		RunDuration:     0.3,
		Results: []map[string]any{
			{"event": "login", "service": "identity", "nested": map[string]any{"a": float64(1)}},
			{"event": "logout", "service": "identity", "count": float64(2), "_json": "reserved field"},
		},
	}
}

func incidentSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 4
	result.ReturnedResults = 4
	result.Results = []map[string]any{
		{"service": "identity", "level": "INFO", "duration_ms": float64(120), "operation": "read"},
		{"service": "identity", "level": "ERROR", "duration_ms": float64(300), "operation": "write"},
		{"service": "identity", "level": "INFO", "duration_ms": float64(1200), "operation": "write"},
		{"service": "billing", "level": "ERROR", "duration_ms": float64(50), "operation": "read"},
	}
	return result
}

func timedIncidentSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 5
	result.ReturnedResults = 5
	result.Results = []map[string]any{
		{"_time": "2026-04-28T09:59:00Z", "service": "identity", "level": "INFO", "duration_ms": float64(120), "operation": "read"},
		{"_time": "2026-04-28T10:01:00Z", "service": "identity", "level": "ERROR", "duration_ms": float64(300), "operation": "write"},
		{"_time": "2026-04-28T10:05:00Z", "service": "identity", "level": "INFO", "duration_ms": float64(1200), "operation": "write"},
		{"_time": "2026-04-28T10:06:00Z", "service": "billing", "level": "ERROR", "duration_ms": float64(500), "operation": "write"},
		{"_time": "2026-04-28T10:12:00Z", "service": "billing", "level": "ERROR", "duration_ms": float64(50), "operation": "write"},
	}
	return result
}

func eventSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 4
	result.ReturnedResults = 4
	result.Results = []map[string]any{
		{"_time": "2026-04-28T10:02:00Z", "session_id": "session-1", "trace_id": "trace-1", "component": "api", "operation": "db", "severity": "INFO", "message": "db query"},
		{"_time": "2026-04-28T10:01:00Z", "session_id": "session-1", "trace_id": "trace-1", "component": "api", "operation": "start", "severity": "INFO", "message": "start"},
		{"_time": "2026-04-28T10:03:00Z", "session_id": "session-2", "trace_id": "trace-2", "component": "api", "operation": "start", "severity": "INFO", "message": "other"},
		{"_time": "2026-04-28T10:04:00Z", "session_id": "session-1", "trace_id": "trace-3", "component": "api", "operation": "finish", "severity": "ERROR", "message": "finish"},
	}
	return result
}

func jsonEventSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 4
	result.ReturnedResults = 4
	result.Results = []map[string]any{
		{"_time": "2026-04-28T10:01:00Z", "_raw": `{"requestId":"req-json","sessionId":"json-session"}`, "component": "api", "message": "json start"},
		{"_time": "2026-04-28T10:02:00Z", "_raw": `{"requestId":"other","sessionId":"other"}`, "component": "api", "message": "json other"},
		{"_time": "2026-04-28T10:02:30Z", "_raw": `plain text`, "component": "api", "message": "plain"},
		{"_time": "2026-04-28T10:03:00Z", "_raw": `{"requestId":"req-json","sessionId":"json-session"}`, "component": "api", "message": "json finish"},
	}
	return result
}

func latencyOrderingSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 5
	result.ReturnedResults = 5
	result.Results = []map[string]any{
		{"service": "identity", "duration_ms": float64(100), "level": "INFO"},
		{"service": "identity", "duration_ms": float64(200), "level": "INFO"},
		{"service": "identity", "duration_ms": float64(1200), "level": "ERROR"},
		{"service": "billing", "duration_ms": float64(1500), "level": "ERROR"},
		{"service": "billing", "duration_ms": float64(2500), "level": "ERROR"},
	}
	return result
}

func latencySLOSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = 5
	result.ReturnedResults = 5
	result.Results = []map[string]any{
		{"service": "api", "duration_ms": float64(300), "level": "INFO"},
		{"service": "api", "duration_ms": float64(350), "level": "INFO"},
		{"service": "api", "duration_ms": float64(400), "level": "INFO"},
		{"service": "api", "duration_ms": float64(450), "level": "INFO"},
		{"service": "db", "duration_ms": float64(1200), "level": "ERROR"},
	}
	return result
}

func largeProgressSearchResult() SearchResult {
	result := sampleSearchResult()
	result.ResultCount = resultWriteProgressRows
	result.ReturnedResults = resultWriteProgressRows
	rows := make([]map[string]any, 0, resultWriteProgressRows)
	for i := 0; i < resultWriteProgressRows; i++ {
		rows = append(rows, map[string]any{"event": "progress", "n": i})
	}
	result.Results = rows
	return result
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	default:
		return 0
	}
}

func openResultDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func tableExistsInDB(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count > 0
}

func searchMetadataExists(t *testing.T, db *sql.DB, tableName string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM searches WHERE table_name = ?`, tableName).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count > 0
}

func resultDBFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}
