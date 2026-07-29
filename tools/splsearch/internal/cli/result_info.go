package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type resultInfoFlags struct {
	table string
}

type resultInfoOutput struct {
	OK                   bool                   `json:"ok"`
	DB                   string                 `json:"db"`
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
	SchemaCommand        string                 `json:"schema_command"`
	CountCommand         string                 `json:"count_command"`
	SampleCommand        string                 `json:"sample_command"`
	TextSearchCommand    string                 `json:"text_search_command"`
	SummaryCommand       string                 `json:"summary_command"`
	LatencyCommand       string                 `json:"latency_summary_command"`
	EventsCommand        string                 `json:"events_command"`
	DropCommand          string                 `json:"drop_command"`
}

func newResultInfoCommand(e *env) *cobra.Command {
	var flags resultInfoFlags
	cmd := &cobra.Command{
		Use:   "result-info --table=<result_table>",
		Short: "Show saved result table metadata",
		Long: `Show metadata for one saved local result table.

This command does not run a new Splunk search and does not read raw result rows.
It returns the original SPL, time bounds, row counts, active and accepted
warnings, and ready-to-run follow-up commands, including local BM25 text search.`,
		Example: `  splsearch result-info --table=app_logs
  splsearch result-schema --table=app_logs
  splsearch result-summary --table=app_logs --group-by=component --limit=20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultInfoErrorAndFail(cmd, "unexpected positional argument %q; use --table=<result_table>", args[0])
			}
			if strings.TrimSpace(flags.table) == "" {
				return writeResultInfoErrorAndFail(cmd, "missing --table=<result_table>")
			}
			result, err := splunk.NewResultStore(e.configDir).Info(cmd.Context(), splunk.ResultInfoRequest{Table: flags.table})
			if err != nil {
				return writeResultInfoErrorAndFail(cmd, "%s", err.Error())
			}
			output := resultInfoFromRecord(result.DB, result.ResultTableRecord)
			output.OK = result.OK
			if err := writeSearchJSON(cmd.OutOrStdout(), output); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	return cmd
}

func resultInfoFromRecord(db string, record splunk.ResultTableRecord) resultInfoOutput {
	return resultInfoOutput{
		OK:                   true,
		DB:                   db,
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
		SchemaCommand:        resultSchemaCommand(record.Table),
		CountCommand:         resultCountCommand(record.Table),
		SampleCommand:        resultSampleCommand(record.Table),
		TextSearchCommand:    resultTextSearchCommand(record.Table),
		SummaryCommand:       resultSummaryCommand(record.Table),
		LatencyCommand:       resultLatencySummaryCommand(record.Table),
		EventsCommand:        resultEventsCommand(record.Table),
		DropCommand:          resultDropCommand(record.Table),
	}
}

func resultInfoCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-info",
		"--table=" + shellArg(table),
	}, " ")
}

func writeResultInfoErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
