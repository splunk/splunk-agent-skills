package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestResultInfoCommandReturnsMetadataWarningsAndCommands(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	_, err := splunk.NewResultStore(e.configDir).SaveWithOptions(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-info",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     2,
		ReturnedResults: 2,
		Results: []map[string]any{
			{"event": "login", "service": "identity"},
			{"event": "logout", "service": "identity"},
		},
	}, "identity_logs", splunk.SaveOptions{Warnings: []string{"broad search warning"}})
	if err != nil {
		t.Fatal(err)
	}

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-info", "--table=identity_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-info output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["table"] != "identity_logs" || int(result["warning_count"].(float64)) != 1 || int(result["accepted_warning_count"].(float64)) != 0 {
		t.Fatalf("unexpected info output: %#v", result)
	}
	warnings := result["warnings"].([]any)
	if len(warnings) != 1 || warnings[0] != "broad search warning" {
		t.Fatalf("unexpected warnings: %#v", result)
	}
	details := result["warning_details"].([]any)
	firstDetail := details[0].(map[string]any)
	if firstDetail["code"] != "legacy" || firstDetail["accepted"] != false {
		t.Fatalf("unexpected warning details: %#v", firstDetail)
	}
	for _, key := range []string{"schema_command", "count_command", "sample_command", "text_search_command", "summary_command", "latency_summary_command", "events_command", "drop_command"} {
		if !strings.Contains(result[key].(string), "identity_logs") {
			t.Fatalf("expected %s to reference table: %#v", key, result[key])
		}
	}
	if !strings.Contains(result["text_search_command"].(string), "result-text-search") {
		t.Fatalf("expected text search command, got %#v", result["text_search_command"])
	}
	if !strings.Contains(result["latency_summary_command"].(string), "--preset=latency") {
		t.Fatalf("expected latency summary command, got %#v", result["latency_summary_command"])
	}
	if !strings.Contains(result["events_command"].(string), "result-events") {
		t.Fatalf("expected events command, got %#v", result["events_command"])
	}
}

func TestResultInfoCommandPrintsJSONErrors(t *testing.T) {
	for _, args := range [][]string{
		{"result-info"},
		{"result-info", "--table=bad-name"},
		{"result-info", "--table=identity_logs", "extra"},
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

func TestResultInfoHelpExplainsWarnings(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-info", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Show metadata for one saved local result table",
		"does not read raw result rows",
		"active and accepted",
		"ready-to-run follow-up commands",
		"local BM25 text search",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-info help missing %q:\n%s", want, output)
		}
	}
}
