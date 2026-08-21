package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

const defaultResultsListLimit = 100

type resultsListFlags struct {
	limit int
}

type resultsListOutput struct {
	OK        bool                     `json:"ok"`
	DB        string                   `json:"db"`
	Count     int                      `json:"count"`
	Limit     int                      `json:"limit"`
	Truncated bool                     `json:"truncated"`
	Message   string                   `json:"message,omitempty"`
	Tables    []resultTableListCommand `json:"tables"`
}

type resultTableListCommand struct {
	Table                string                 `json:"table"`
	CreatedAt            int64                  `json:"created_at"`
	CreatedAtUTC         string                 `json:"created_at_utc"`
	URL                  string                 `json:"url"`
	App                  string                 `json:"app"`
	SID                  string                 `json:"sid"`
	Query                string                 `json:"query"`
	Earliest             string                 `json:"earliest"`
	Latest               string                 `json:"latest"`
	ResultCount          int                    `json:"result_count"`
	Rows                 int                    `json:"rows"`
	Offset               int                    `json:"offset"`
	HasMore              bool                   `json:"has_more"`
	RunDuration          float64                `json:"run_duration"`
	Warnings             []string               `json:"warnings,omitempty"`
	WarningCount         int                    `json:"warning_count"`
	AcceptedWarnings     []string               `json:"accepted_warnings,omitempty"`
	AcceptedWarningCount int                    `json:"accepted_warning_count"`
	WarningDetails       []splunk.ResultWarning `json:"warning_details,omitempty"`
	SearchCommand        string                 `json:"search_command"`
	InfoCommand          string                 `json:"info_command"`
	SchemaCommand        string                 `json:"schema_command"`
	CountCommand         string                 `json:"count_command"`
	SampleCommand        string                 `json:"sample_command"`
	TextSearchCommand    string                 `json:"text_search_command"`
	SummaryCommand       string                 `json:"summary_command"`
	LatencyCommand       string                 `json:"latency_summary_command"`
	EventsCommand        string                 `json:"events_command"`
	DropCommand          string                 `json:"drop_command"`
}

func newResultsListCommand(e *env) *cobra.Command {
	var flags resultsListFlags
	cmd := &cobra.Command{
		Use:   "results-list",
		Short: "List saved Splunk result tables",
		Long: `List saved local result tables created by splsearch search.

This command reads only local SQLite metadata from
~/.config/splsearch/results.sqlite unless XDG_CONFIG_HOME changes the config
directory. It does not run Splunk searches and does not read raw result rows.

Output is compact JSON for AI/tool use. Each table includes created time,
Splunk time bounds, row counts, and ready-to-run commands:
- search_command reruns the Splunk search with --earliest, --latest, and a bounded --limit.
- info_command prints one table's metadata, active warnings, and accepted warnings.
- schema_command lists saved-table columns before writing SQL.
- count_command counts rows in the saved local table.
- sample_command reads a small saved-table sample with --limit=20.
- text_search_command searches saved rows with local BM25/FTS.
- summary_command is a template for common aggregate summaries.
- latency_summary_command is a template for first-pass latency incident summaries.
- events_command is a template for ordered event matching.
- drop_command removes the saved local table when it is no longer needed.`,
		Example: `  splsearch results-list
  splsearch results-list --limit=20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultsListErrorAndFail(cmd, "unexpected positional argument %q; use results-list with optional --limit=<n>", args[0])
			}
			if flags.limit <= 0 {
				return writeResultsListErrorAndFail(cmd, "--limit must be > 0")
			}
			result, err := splunk.NewResultStore(e.configDir).ListTables(cmd.Context(), splunk.ListResultTablesRequest{Limit: flags.limit})
			if err != nil {
				return writeResultsListErrorAndFail(cmd, "%s", err.Error())
			}
			output := resultsListOutput{
				OK:        result.OK,
				DB:        result.DB,
				Count:     result.Count,
				Limit:     result.Limit,
				Truncated: result.Truncated,
				Message:   result.Message,
				Tables:    resultListCommands(result.Tables),
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), output); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&flags.limit, "limit", defaultResultsListLimit, "maximum result table metadata records to print")
	return cmd
}

func resultListCommands(records []splunk.ResultTableRecord) []resultTableListCommand {
	items := make([]resultTableListCommand, 0, len(records))
	for _, record := range records {
		item := resultTableListCommand{
			Table:                record.Table,
			CreatedAt:            record.CreatedAt,
			CreatedAtUTC:         record.CreatedAtUTC,
			URL:                  record.URL,
			App:                  record.App,
			SID:                  record.SID,
			Query:                record.Query,
			Earliest:             record.Earliest,
			Latest:               record.Latest,
			ResultCount:          record.ResultCount,
			Rows:                 record.Rows,
			Offset:               record.Offset,
			HasMore:              record.HasMore,
			RunDuration:          record.RunDuration,
			Warnings:             record.Warnings,
			WarningCount:         record.WarningCount,
			AcceptedWarnings:     record.AcceptedWarnings,
			AcceptedWarningCount: record.AcceptedWarningCount,
			WarningDetails:       record.WarningDetails,
			SearchCommand:        resultSearchCommand(record),
			InfoCommand:          resultInfoCommand(record.Table),
			SchemaCommand:        resultSchemaCommand(record.Table),
			CountCommand:         resultCountCommand(record.Table),
			SampleCommand:        resultSampleCommand(record.Table),
			TextSearchCommand:    resultTextSearchCommand(record.Table),
			SummaryCommand:       resultSummaryCommand(record.Table),
			LatencyCommand:       resultLatencySummaryCommand(record.Table),
			EventsCommand:        resultEventsCommand(record.Table),
			DropCommand:          resultDropCommand(record.Table),
		}
		items = append(items, item)
	}
	return items
}

func resultSearchCommand(record splunk.ResultTableRecord) string {
	limit := record.Rows
	if limit <= 0 {
		limit = 1
	}
	parts := []string{
		"splsearch",
		"search",
		"--url=" + shellArg(record.URL),
		"--query=" + shellArg(record.Query),
		"--earliest=" + shellArg(record.Earliest),
		"--latest=" + shellArg(record.Latest),
		"--app=" + shellArg(record.App),
		fmt.Sprintf("--limit=%d", limit),
	}
	if record.Offset > 0 {
		parts = append(parts, fmt.Sprintf("--offset=%d", record.Offset))
	}
	parts = append(parts, "--result-table="+shellArg(record.Table))
	return strings.Join(parts, " ")
}

func resultCountCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-search",
		"--table=" + shellArg(table),
		"--query=" + shellArg("SELECT count(*) AS rows FROM results"),
		"--limit=1",
	}, " ")
}

func resultSampleCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-search",
		"--table=" + shellArg(table),
		"--query=" + shellArg("SELECT _row, _json FROM results LIMIT 20"),
		"--limit=20",
	}, " ")
}

func resultDropCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"results-drop",
		"--table=" + shellArg(table),
	}, " ")
}

func shellArg(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func writeResultsListErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
