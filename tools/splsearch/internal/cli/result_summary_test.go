package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestResultSummaryCommandGroupsRows(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLISummaryTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-summary", "--table=incident_logs", "--group-by=service", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-summary output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["table"] != "incident_logs" || result["truncated"] != false {
		t.Fatalf("unexpected summary metadata: %#v", result)
	}
	columns := result["columns"].([]any)
	if strings.Join([]string{columns[0].(string), columns[1].(string)}, ",") != "service,rows" {
		t.Fatalf("unexpected columns: %#v", columns)
	}
	rows := result["rows"].([]any)
	first := rows[0].(map[string]any)
	if first["service"] != "identity" || int(first["rows"].(float64)) != 3 {
		t.Fatalf("unexpected summary rows: %#v", rows)
	}
}

func TestResultSummaryCommandMetricsAndErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLISummaryTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-summary", "--table=incident_logs", "--group-by=service", "--metric=duration_ms", "--thresholds=250,1000", "--error-where=level = 'ERROR'", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-summary output is not JSON: %s", stdout.String())
	}
	columns := result["columns"].([]any)
	columnText := ""
	for _, column := range columns {
		columnText += column.(string) + ","
	}
	for _, want := range []string{"avg_duration_ms", "max_duration_ms", "gte_250", "gte_1000", "error_count", "error_rate"} {
		if !strings.Contains(columnText, want) {
			t.Fatalf("columns missing %q: %#v", want, columns)
		}
	}
	first := result["rows"].([]any)[0].(map[string]any)
	if first["service"] != "identity" || int(first["gte_250"].(float64)) != 2 || int(first["gte_1000"].(float64)) != 1 || int(first["error_count"].(float64)) != 1 {
		t.Fatalf("unexpected metric summary row: %#v", first)
	}
}

func TestResultSummaryCommandFiltersExactTimeWindow(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLITimedSummaryTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-summary", "--table=incident_logs", "--group-by=service", "--metric=duration_ms", "--thresholds=250,1000", "--time-from=2026-04-28T10:00:00Z", "--time-to=2026-04-28T10:10:00Z", "--error-where=level = 'ERROR'", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-summary output is not JSON: %s", stdout.String())
	}
	rows := result["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("expected two time-window groups, got %#v", rows)
	}
	first := rows[0].(map[string]any)
	if first["service"] != "identity" || int(first["rows"].(float64)) != 2 || int(first["gte_1000"].(float64)) != 1 || int(first["error_count"].(float64)) != 1 {
		t.Fatalf("unexpected first time-window summary row: %#v", first)
	}
	second := rows[1].(map[string]any)
	if second["service"] != "billing" || int(second["rows"].(float64)) != 1 || int(second["gte_250"].(float64)) != 1 || int(second["error_count"].(float64)) != 1 {
		t.Fatalf("unexpected second time-window summary row: %#v", second)
	}
	query := result["query"].(string)
	for _, want := range []string{`"_time" >= '2026-04-28T10:00:00Z'`, `"_time" < '2026-04-28T10:10:00Z'`} {
		if !strings.Contains(query, want) {
			t.Fatalf("summary query missing %q: %s", want, query)
		}
	}
	if strings.Contains(query, "service = 'identity'") {
		t.Fatalf("summary query should not include a custom where predicate: %s", query)
	}
}

func TestResultSummaryCommandOrdersByThreshold(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLILatencyOrderingTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-summary", "--table=latency_logs", "--group-by=service", "--metric=duration_ms", "--thresholds=250,1000", "--order-by=gte_1000", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-summary output is not JSON: %s", stdout.String())
	}
	first := result["rows"].([]any)[0].(map[string]any)
	if first["service"] != "billing" || int(first["gte_1000"].(float64)) != 2 {
		t.Fatalf("unexpected threshold-ordered row: %#v", first)
	}
	if !strings.Contains(result["query"].(string), `ORDER BY "gte_1000" DESC`) {
		t.Fatalf("expected threshold order query, got %s", result["query"])
	}
}

func TestResultSummaryCommandLatencyPreset(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLILatencySLOTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-summary", "--table=latency_logs", "--group-by=service", "--metric=duration_ms", "--thresholds=250,1000", "--preset=latency", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-summary output is not JSON: %s", stdout.String())
	}
	first := result["rows"].([]any)[0].(map[string]any)
	if first["service"] != "api" || int(first["gte_250"].(float64)) != 4 {
		t.Fatalf("unexpected latency preset row: %#v", first)
	}
	if !strings.Contains(result["query"].(string), `ORDER BY "gte_250" DESC`) {
		t.Fatalf("expected latency preset order query, got %s", result["query"])
	}
}

func TestResultSummaryCommandPrintsJSONErrors(t *testing.T) {
	for _, args := range [][]string{
		{"result-summary"},
		{"result-summary", "--table=incident_logs"},
		{"result-summary", "--table=incident_logs", "--group-by=service", "--limit=0"},
		{"result-summary", "--table=incident_logs", "--group-by=service", "--thresholds=250"},
		{"result-summary", "--table=incident_logs", "--group-by=service", "--metric=duration_ms", "--thresholds=bad"},
		{"result-summary", "--table=incident_logs", "--group-by=service", "--preset=latency"},
		{"result-summary", "--table=incident_logs", "--group-by=service", "--order=sideways"},
	} {
		var stdout, stderr bytes.Buffer
		code := Main(args, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("expected code 1 for %v, got %d", args, code)
		}
		if !strings.Contains(stdout.String(), `"ok":false`) {
			t.Fatalf("expected JSON error for %v, got stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

func TestResultSummaryCommandRejectsInvalidOrderBy(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLISummaryTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-summary", "--table=incident_logs", "--group-by=service", "--order-by=missing"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid order-by error")
	}
	if !strings.Contains(stdout.String(), `"ok":false`) || !strings.Contains(stdout.String(), "--order-by") {
		t.Fatalf("expected JSON order-by error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestResultSummaryHelpExplainsExplicitFields(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-summary", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Summarize a saved local result table",
		"groups saved rows by explicit",
		"Use result-schema first",
		"--metric",
		"--time-from",
		"--time-to",
		"--error-where",
		"--preset",
		"gte_250",
		"--order-by",
		"--order",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-summary help missing %q:\n%s", want, output)
		}
	}
}

func seedCLISummaryTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-summary",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     4,
		ReturnedResults: 4,
		Results: []map[string]any{
			{"service": "identity", "level": "INFO", "duration_ms": float64(120), "operation": "read"},
			{"service": "identity", "level": "ERROR", "duration_ms": float64(300), "operation": "write"},
			{"service": "identity", "level": "INFO", "duration_ms": float64(1200), "operation": "write"},
			{"service": "billing", "level": "ERROR", "duration_ms": float64(50), "operation": "read"},
		},
	}, "incident_logs")
	if err != nil {
		t.Fatal(err)
	}
}

func seedCLITimedSummaryTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-summary",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     5,
		ReturnedResults: 5,
		Results: []map[string]any{
			{"_time": "2026-04-28T09:59:00Z", "service": "identity", "level": "INFO", "duration_ms": float64(120), "operation": "read"},
			{"_time": "2026-04-28T10:01:00Z", "service": "identity", "level": "ERROR", "duration_ms": float64(300), "operation": "write"},
			{"_time": "2026-04-28T10:05:00Z", "service": "identity", "level": "INFO", "duration_ms": float64(1200), "operation": "write"},
			{"_time": "2026-04-28T10:06:00Z", "service": "billing", "level": "ERROR", "duration_ms": float64(500), "operation": "write"},
			{"_time": "2026-04-28T10:12:00Z", "service": "billing", "level": "ERROR", "duration_ms": float64(50), "operation": "write"},
		},
	}, "incident_logs")
	if err != nil {
		t.Fatal(err)
	}
}

func seedCLILatencyOrderingTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-latency",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     5,
		ReturnedResults: 5,
		Results: []map[string]any{
			{"service": "identity", "level": "INFO", "duration_ms": float64(100)},
			{"service": "identity", "level": "INFO", "duration_ms": float64(200)},
			{"service": "identity", "level": "ERROR", "duration_ms": float64(1200)},
			{"service": "billing", "level": "ERROR", "duration_ms": float64(1500)},
			{"service": "billing", "level": "ERROR", "duration_ms": float64(2500)},
		},
	}, "latency_logs")
	if err != nil {
		t.Fatal(err)
	}
}

func seedCLILatencySLOTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-latency",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     5,
		ReturnedResults: 5,
		Results: []map[string]any{
			{"service": "api", "level": "INFO", "duration_ms": float64(300)},
			{"service": "api", "level": "INFO", "duration_ms": float64(350)},
			{"service": "api", "level": "INFO", "duration_ms": float64(400)},
			{"service": "api", "level": "INFO", "duration_ms": float64(450)},
			{"service": "db", "level": "ERROR", "duration_ms": float64(1200)},
		},
	}, "latency_logs")
	if err != nil {
		t.Fatal(err)
	}
}
