package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestResultSchemaCommandReturnsJSONColumns(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"result-schema", "--table=identity_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("result-schema output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || result["table"] != "identity_logs" || result["query_table"] != "results" {
		t.Fatalf("unexpected schema metadata: %#v", result)
	}
	if int(result["rows"].(float64)) != 2 || int(result["column_count"].(float64)) != 4 {
		t.Fatalf("unexpected schema counts: %#v", result)
	}
	columns := result["columns"].([]any)
	names := make([]string, 0, len(columns))
	for _, raw := range columns {
		names = append(names, raw.(map[string]any)["name"].(string))
	}
	if strings.Join(names, ",") != "_row,_json,event,service" {
		t.Fatalf("unexpected columns: %#v", columns)
	}
	if !strings.Contains(result["count_command"].(string), "SELECT count(*) AS rows FROM results") {
		t.Fatalf("unexpected count command: %#v", result["count_command"])
	}
	if !strings.Contains(result["sample_command"].(string), "--limit=20") {
		t.Fatalf("unexpected sample command: %#v", result["sample_command"])
	}
}

func TestResultSchemaCommandPrintsJSONErrors(t *testing.T) {
	for _, args := range [][]string{
		{"result-schema"},
		{"result-schema", "--table=bad-name"},
		{"result-schema", "--table=identity_logs", "extra"},
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

func TestResultSchemaHelpExplainsMetadataOnly(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"result-schema", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Inspect the schema of an existing local result table",
		"does not run a new Splunk search",
		"does not read raw result rows",
		"available columns before writing result-search SQL",
		"refer to the selected table as results",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("result-schema help missing %q:\n%s", want, output)
		}
	}
}
