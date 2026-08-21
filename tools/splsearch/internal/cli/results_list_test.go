package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResultsListCommandPrintsTablesAndCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"results-list", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("results-list output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || int(result["count"].(float64)) != 1 || result["truncated"] != false {
		t.Fatalf("unexpected list result: %#v", result)
	}
	tables := result["tables"].([]any)
	table := tables[0].(map[string]any)
	if table["table"] != "identity_logs" || table["earliest"] != "-1m" || table["latest"] != "now" {
		t.Fatalf("unexpected table metadata: %#v", table)
	}
	searchCommand := table["search_command"].(string)
	for _, want := range []string{
		"splsearch search",
		"--url='https://splunk.example.com'",
		"--query='search index=test'",
		"--earliest='-1m'",
		"--latest='now'",
		"--limit=2",
		"--result-table='identity_logs'",
	} {
		if !strings.Contains(searchCommand, want) {
			t.Fatalf("search command missing %q:\n%s", want, searchCommand)
		}
	}
	if !strings.Contains(table["count_command"].(string), "SELECT count(*) AS rows FROM results") {
		t.Fatalf("unexpected count command: %s", table["count_command"])
	}
	if !strings.Contains(table["info_command"].(string), "result-info --table='identity_logs'") {
		t.Fatalf("unexpected info command: %s", table["info_command"])
	}
	if !strings.Contains(table["schema_command"].(string), "result-schema --table='identity_logs'") {
		t.Fatalf("unexpected schema command: %s", table["schema_command"])
	}
	if !strings.Contains(table["sample_command"].(string), "--limit=20") {
		t.Fatalf("unexpected sample command: %s", table["sample_command"])
	}
	if !strings.Contains(table["text_search_command"].(string), "result-text-search --table='identity_logs' --query='<text>'") {
		t.Fatalf("unexpected text search command: %s", table["text_search_command"])
	}
	if !strings.Contains(table["drop_command"].(string), "results-drop --table='identity_logs'") {
		t.Fatalf("unexpected drop command: %s", table["drop_command"])
	}
	if !strings.Contains(table["summary_command"].(string), "result-summary --table='identity_logs' --group-by=<field>") {
		t.Fatalf("unexpected summary command: %s", table["summary_command"])
	}
	if !strings.Contains(table["latency_summary_command"].(string), "result-summary --table='identity_logs' --group-by=<field> --metric=<numeric_field> --preset=latency") {
		t.Fatalf("unexpected latency summary command: %s", table["latency_summary_command"])
	}
	if !strings.Contains(table["events_command"].(string), "result-events --table='identity_logs' --field=<field> --value=<value>") {
		t.Fatalf("unexpected events command: %s", table["events_command"])
	}
	if int(table["warning_count"].(float64)) != 0 || int(table["accepted_warning_count"].(float64)) != 0 {
		t.Fatalf("unexpected warning count: %#v", table)
	}
}

func TestResultsListCommandTruncatesTableMetadata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")
	seedCLIResultTable(t, e.configDir, "other_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"results-list", "--limit=1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("results-list output is not JSON: %s", stdout.String())
	}
	if result["truncated"] != true || !strings.Contains(result["message"].(string), "output truncated to --limit=1 tables") {
		t.Fatalf("expected truncation marker, got %#v", result)
	}
	if len(result["tables"].([]any)) != 1 {
		t.Fatalf("expected one table due to --limit=1, got %#v", result["tables"])
	}
}

func TestResultsListCommandPrintsJSONErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"results-list", "--limit=0"}, &stdout, &stderr)
	if code != 1 {
		t.Fatalf("expected code 1, got %d", code)
	}
	if !strings.Contains(stdout.String(), `"ok":false`) || !strings.Contains(stdout.String(), "--limit must be > 0") {
		t.Fatalf("expected JSON error, got stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestResultsListHelpExplainsMetadataOnlyCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"results-list", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"does not read raw result rows",
		"search_command reruns the Splunk search with --earliest, --latest, and a bounded --limit",
		"info_command prints one table's metadata, active warnings, and accepted warnings",
		"schema_command lists saved-table columns before writing SQL",
		"sample_command reads a small saved-table sample with --limit=20",
		"text_search_command searches saved rows with local BM25/FTS",
		"summary_command is a template for common aggregate summaries",
		"latency_summary_command is a template for first-pass latency incident summaries",
		"events_command is a template for ordered event matching",
		"drop_command removes the saved local table",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("results-list help missing %q:\n%s", want, output)
		}
	}
}
