package cli

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestResultsDropCommandDropsOneTable(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")
	seedCLIResultTable(t, e.configDir, "other_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"results-drop", "--table=identity_logs"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("results-drop output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || int(result["count"].(float64)) != 1 {
		t.Fatalf("unexpected drop result: %#v", result)
	}
	dropped := result["dropped"].([]any)
	if len(dropped) != 1 || dropped[0] != "identity_logs" {
		t.Fatalf("unexpected dropped tables: %#v", dropped)
	}

	db := openCLIResultDB(t, filepath.Join(e.configDir, "results.sqlite"))
	defer db.Close()
	if cliTableExists(t, db, "identity_logs") {
		t.Fatal("expected identity_logs to be dropped")
	}
	if !cliTableExists(t, db, "other_logs") {
		t.Fatal("expected other_logs to remain")
	}
}

func TestResultsDropCommandDropsAllTables(t *testing.T) {
	var stdout, stderr bytes.Buffer
	e := testEnv(t, &stdout, &stderr, nil, nil)
	seedCLIResultTable(t, e.configDir, "identity_logs")
	seedCLIResultTable(t, e.configDir, "other_logs")

	cmd := newRootCommand(e)
	cmd.SetArgs([]string{"results-drop", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("results-drop output is not JSON: %s", stdout.String())
	}
	if result["ok"] != true || int(result["count"].(float64)) != 2 {
		t.Fatalf("unexpected drop result: %#v", result)
	}

	db := openCLIResultDB(t, filepath.Join(e.configDir, "results.sqlite"))
	defer db.Close()
	if cliTableExists(t, db, "identity_logs") || cliTableExists(t, db, "other_logs") {
		t.Fatal("expected all result tables to be dropped")
	}
	if countSearchMetadataRows(t, db) != 0 {
		t.Fatal("expected all search metadata to be dropped")
	}
}

func TestResultsDropCommandPrintsJSONErrors(t *testing.T) {
	for _, args := range [][]string{
		{"results-drop"},
		{"results-drop", "--table=identity_logs", "--all"},
		{"results-drop", "--table=bad-name"},
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

func TestResultsDropHelpExplainsLocalOnlyDrop(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"results-drop", "--help"}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("expected code 0, got %d stderr=%q", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"Drop saved local result tables",
		"does not delete anything from Splunk",
		"--table=<result_table>",
		"--all",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("results-drop help missing %q:\n%s", want, output)
		}
	}
}

func countSearchMetadataRows(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM searches`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
