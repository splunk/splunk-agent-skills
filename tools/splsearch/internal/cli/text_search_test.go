package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestResultTextSearchCommandReturnsJSONHitsAndCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLITextResultTable(t, e.configDir, "text_cli_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-text-search", "--table=text_cli_logs", "--query=request_remote_tok 401 Unauthorized", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-text-search output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["table"] != "text_cli_logs" || int(result["count"].(float64)) != 1 {
		t.Fatalf("unexpected result metadata: %#v", result)
	}
	hits := result["hits"].([]any)
	hit := hits[0].(map[string]any)
	if hit["source"] != splunk.ResultSourceSplunk || hit["kind"] != splunk.ResultKindSearch || hit["match_scope"] != "body" {
		t.Fatalf("unexpected hit metadata: %#v", hit)
	}
	if !strings.Contains(hit["sample_command"].(string), "SELECT _row, _json FROM results WHERE _row = 2") {
		t.Fatalf("unexpected sample command: %s", hit["sample_command"])
	}
	if !strings.Contains(hit["text_search_command"].(string), "result-text-search --table='text_cli_logs' --query='request_remote_tok 401 Unauthorized' --limit=10") {
		t.Fatalf("unexpected text search command: %s", hit["text_search_command"])
	}
}

func TestResultTextSearchCommandMarksContextOnlyHits(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLITextResultTable(t, e.configDir, "context_cli_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-text-search", "--table=context_cli_logs", "--query=index textsearch", "--limit=10"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-text-search output is not JSON: %s", stdout.String())
	}
	hits := result["hits"].([]any)
	if len(hits) == 0 {
		t.Fatalf("expected context-only hits, got %#v", result)
	}
	hit := hits[0].(map[string]any)
	if hit["match_scope"] != "table_context" {
		t.Fatalf("expected table_context hit, got %#v", hit)
	}
	if !strings.Contains(hit["sample_command"].(string), "SELECT _row, _json FROM results LIMIT 20") {
		t.Fatalf("expected table sample command, got %s", hit["sample_command"])
	}
	if !strings.Contains(hit["text_search_command"].(string), "--query='<row-specific-text>'") {
		t.Fatalf("expected row-specific placeholder, got %s", hit["text_search_command"])
	}
}

func TestResultTextSearchCommandPrintsJSONErrors(t *testing.T) {
	for _, args := range [][]string{
		{"result-text-search"},
		{"result-text-search", "--table=text_cli_logs"},
		{"result-text-search", "--table=text_cli_logs", "--query=needle", "--limit=0"},
		{"result-text-search", "--table=text_cli_logs", "--query=needle", "other"},
		{"result-text-search", "--table=text_cli_logs", "--query=<text>"},
		{"result-text-search", "--table=text_cli_logs", "--query=<row-specific-text>"},
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

func TestResultTextSearchHelpExplainsBM25AndMatchScope(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-text-search", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"SQLite FTS5/BM25",
		"does not run a new Splunk search",
		"Query input is plain text",
		"All query terms must match the",
		"same saved row",
		"Use result-search SQL for numeric predicates",
		"Check match_scope",
		"table_context and title-only matches are",
		"leads to inspect",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-text-search help missing %q:\n%s", want, output)
		}
	}
}

func seedCLITextResultTable(t *testing.T, configDir, tableName string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-text-cli",
		Query:           "search index=textsearch",
		Earliest:        "-15m",
		Latest:          "now",
		ResultCount:     2,
		ReturnedResults: 2,
		Results: []map[string]any{
			{"_time": "2026-05-18T12:00:00Z", "service": "identity", "message": "ordinary login"},
			{"_time": "2026-05-18T12:01:00Z", "service": "identity", "message": "request_remote_tok returned 401 Unauthorized"},
		},
	}, tableName)
	if err != nil {
		t.Fatal(err)
	}
}
