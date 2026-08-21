package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestResultSearchCommandReturnsJSONRows(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-search", "--table=identity_logs", "--query=SELECT event, service FROM results WHERE event = 'login'", "--limit=5"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["table"] != "identity_logs" || int(result["limit"].(float64)) != 5 {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	if result["truncated"] != false {
		t.Fatalf("did not expect truncation: %#v", result)
	}
	columns := result["columns"].([]any)
	if len(columns) != 2 || columns[0] != "event" || columns[1] != "service" {
		t.Fatalf("unexpected columns: %#v", columns)
	}
	rows := result["rows"].([]any)
	if len(rows) != 1 || rows[0].(map[string]any)["event"] != "login" {
		t.Fatalf("unexpected rows: %#v", rows)
	}
}

func TestResultSearchCommandReportsTruncation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-search", "--table=identity_logs", "--query=SELECT event FROM results ORDER BY _row", "--limit=1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-search output is not JSON: %s", stdout.String())
	}
	if result["truncated"] != true || !strings.Contains(result["message"].(string), "output truncated to --limit=1 rows") {
		t.Fatalf("expected explicit truncation result, got %#v", result)
	}
	rows := result["rows"].([]any)
	if len(rows) != 1 {
		t.Fatalf("expected one returned row, got %#v", rows)
	}
}

func TestResultSearchCommandDefaultsToSafeLimit(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-search", "--table=identity_logs", "--query=SELECT event FROM results"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-search output is not JSON: %s", stdout.String())
	}
	if int(result["limit"].(float64)) != defaultResultSearchLimit {
		t.Fatalf("expected default limit %d, got %#v", defaultResultSearchLimit, result)
	}
}

func TestResultSearchCommandPrintsJSONErrors(t *testing.T) {
	for _, args := range [][]string{
		{"result-search"},
		{"result-search", "--table=identity_logs"},
		{"result-search", "--table=identity_logs", "--query=SELECT * FROM results", "--limit=0"},
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

func TestResultSearchHelpWarnsAboutLargeOutput(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-search", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Search an existing local result table",
		"does not run a new Splunk search",
		"IMPORTANT FOR AI CALLERS",
		"can print a lot of data",
		"--limit=100 by default",
		`"truncated": true`,
		"result-schema --table=<result_table>",
		"json_extract(_raw, '$.customer')",
		"refer to the selected table as results",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-search help missing %q:\n%s", want, output)
		}
	}
}

func seedCLIResultTable(t *testing.T, configDir, tableName string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-result-search",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     2,
		ReturnedResults: 2,
		Results: []map[string]any{
			{"event": "login", "service": "identity"},
			{"event": "logout", "service": "identity"},
		},
	}, tableName)
	if err != nil {
		t.Fatal(err)
	}
}
