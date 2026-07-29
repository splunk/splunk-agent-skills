package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestResultEventsCommandReturnsOrderedSequenceForColumn(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIEventsTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-events", "--table=event_logs", "--field=session_id", "--value=session-1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-events output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["match_mode"] != "field" || result["matched_field"] != "session_id" || result["matched_value"] != "session-1" || int(result["count"].(float64)) != 3 || result["truncated"] != false {
		t.Fatalf("unexpected event metadata: %#v", result)
	}
	rows := result["rows"].([]any)
	if rows[0].(map[string]any)["operation"] != "start" || rows[2].(map[string]any)["operation"] != "finish" {
		t.Fatalf("expected rows ordered by _time, got %#v", rows)
	}
}

func TestResultEventsCommandSupportsJSONAndRequestIDShortcut(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIJSONEventsTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-events", "--table=event_logs", "--json-field=$.sessionId", "--value=json-session"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-events output is not JSON: %s", stdout.String())
	}
	if result["match_mode"] != "json_field" || result["matched_field"] != "json:$.sessionId" || int(result["count"].(float64)) != 2 {
		t.Fatalf("unexpected JSON event output: %#v", result)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"result-events", "--table=event_logs", "--request-id=req-json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var requestResult map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &requestResult); err != nil {
		t.Fatalf("result-events output is not JSON: %s", stdout.String())
	}
	if requestResult["match_mode"] != "request_id" || requestResult["matched_field"] != "json:auto" || int(requestResult["count"].(float64)) != 2 {
		t.Fatalf("unexpected request-id shortcut output: %#v", requestResult)
	}
}

func TestResultEventsCommandSupportsTruncation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIEventsTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-events", "--table=event_logs", "--field=trace_id", "--value=trace-1", "--limit=1"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-events output is not JSON: %s", stdout.String())
	}
	if result["matched_field"] != "trace_id" || int(result["count"].(float64)) != 1 || result["truncated"] != true {
		t.Fatalf("expected truncation, got %#v", result)
	}
}

func TestResultEventsCommandPrintsJSONErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIEventsTable(t, e.configDir)

	for _, args := range [][]string{
		{"result-events"},
		{"result-events", "--table=event_logs"},
		{"result-events", "--table=event_logs", "--field=session_id"},
		{"result-events", "--table=event_logs", "--json-field=$.sessionId"},
		{"result-events", "--table=event_logs", "--field=session_id", "--json-field=$.sessionId", "--value=session-1"},
		{"result-events", "--table=event_logs", "--request-id=req-1", "--field=session_id"},
		{"result-events", "--table=event_logs", "--field=session_id", "--value=session-1", "--limit=0"},
		{"result-events", "--table=bad-name", "--field=session_id", "--value=session-1"},
		{"result-events", "--table=event_logs", "--field=session_id", "--value=session-1", "extra"},
	} {
		stdout.Reset()
		stderr.Reset()
		cmd := newRootCommand(e)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for args %v", args)
		}
		if !strings.Contains(stdout.String(), `"ok":false`) {
			t.Fatalf("expected JSON error for %v, got stdout=%q stderr=%q", args, stdout.String(), stderr.String())
		}
	}
}

func TestResultEventsHelpExplainsMatchModes(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-events", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Find matching events",
		"--field",
		"--json-field",
		"--value",
		"--request-id",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-events help missing %q:\n%s", want, output)
		}
	}
}

func seedCLIEventsTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-events",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     4,
		ReturnedResults: 4,
		Results: []map[string]any{
			{"_time": "2026-04-28T10:02:00Z", "session_id": "session-1", "trace_id": "trace-1", "component": "api", "operation": "db", "severity": "INFO", "message": "db query"},
			{"_time": "2026-04-28T10:01:00Z", "session_id": "session-1", "trace_id": "trace-1", "component": "api", "operation": "start", "severity": "INFO", "message": "start"},
			{"_time": "2026-04-28T10:03:00Z", "session_id": "session-2", "trace_id": "trace-2", "component": "api", "operation": "start", "severity": "INFO", "message": "other"},
			{"_time": "2026-04-28T10:04:00Z", "session_id": "session-1", "trace_id": "trace-3", "component": "api", "operation": "finish", "severity": "ERROR", "message": "finish"},
		},
	}, "event_logs")
	if err != nil {
		t.Fatal(err)
	}
}

func seedCLIJSONEventsTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).Save(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-events",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     4,
		ReturnedResults: 4,
		Results: []map[string]any{
			{"_time": "2026-04-28T10:01:00Z", "_raw": `{"requestId":"req-json","sessionId":"json-session"}`, "component": "api", "message": "json start"},
			{"_time": "2026-04-28T10:02:00Z", "_raw": `{"requestId":"other","sessionId":"other"}`, "component": "api", "message": "json other"},
			{"_time": "2026-04-28T10:02:30Z", "_raw": `plain text`, "component": "api", "message": "plain"},
			{"_time": "2026-04-28T10:03:00Z", "_raw": `{"requestId":"req-json","sessionId":"json-session"}`, "component": "api", "message": "json finish"},
		},
	}, "event_logs")
	if err != nil {
		t.Fatal(err)
	}
}
