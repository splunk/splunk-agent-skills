package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

func TestResultWarningsAcceptCommandMarksWarningAccepted(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIWarningTable(t, e.configDir)

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-warnings", "accept", "--table=wide_logs", "--code=full-fetch"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-warnings output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["accepted"] != true || int(result["warning_count"].(float64)) != 0 || int(result["accepted_warning_count"].(float64)) != 1 {
		t.Fatalf("unexpected accept output: %#v", result)
	}
	details := result["warning_details"].([]any)
	firstDetail := details[0].(map[string]any)
	if firstDetail["code"] != "full_fetch" || firstDetail["accepted"] != true {
		t.Fatalf("unexpected warning details: %#v", firstDetail)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"result-info", "--table=wide_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var info map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &info); err != nil {
		t.Fatalf("result-info output is not JSON: %s", stdout.String())
	}
	if int(info["warning_count"].(float64)) != 0 || int(info["accepted_warning_count"].(float64)) != 1 {
		t.Fatalf("expected accepted warning in result-info, got %#v", info)
	}

	stdout.Reset()
	cmd = newRootCommand(e)
	cmd.SetArgs([]string{"result-warnings", "accept", "--table=wide_logs", "--code=full_fetch"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var again map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &again); err != nil {
		t.Fatalf("second result-warnings output is not JSON: %s", stdout.String())
	}
	if again["accepted"] != false || !strings.Contains(again["message"].(string), "already accepted") {
		t.Fatalf("expected already-accepted result, got %#v", again)
	}
}

func TestResultWarningsAcceptCommandPrintsJSONErrors(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIWarningTable(t, e.configDir)

	for _, args := range [][]string{
		{"result-warnings", "accept"},
		{"result-warnings", "accept", "--table=bad-name", "--code=full_fetch"},
		{"result-warnings", "accept", "--table=wide_logs", "--code=full_fetch", "extra"},
		{"result-warnings", "accept", "--table=wide_logs", "--code=unknown"},
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

func TestResultWarningsAcceptHelpExplainsLocalMetadata(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-warnings", "accept", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Accept one warning code",
		"full_fetch",
		"no longer contribute to warning_count",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-warnings accept help missing %q:\n%s", want, output)
		}
	}
}

func seedCLIWarningTable(t *testing.T, configDir string) {
	t.Helper()
	_, err := splunk.NewResultStore(configDir).SaveWithOptions(context.Background(), splunk.SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-wide",
		Query:           "search index=test",
		Earliest:        "-1m",
		Latest:          "now",
		ResultCount:     10001,
		ReturnedResults: 0,
		Results:         []map[string]any{},
	}, "wide_logs", splunk.SaveOptions{WarningDetails: []splunk.ResultWarning{{
		Code:    splunk.ResultWarningCodeFullFetch,
		Message: "search used --limit=0 and fetched all 10001 rows",
	}}})
	if err != nil {
		t.Fatal(err)
	}
}
