package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type resultSchemaFlags struct {
	table string
}

type resultSchemaOutput struct {
	OK            bool                  `json:"ok"`
	DB            string                `json:"db"`
	Table         string                `json:"table"`
	Rows          int                   `json:"rows"`
	ColumnCount   int                   `json:"column_count"`
	QueryTable    string                `json:"query_table"`
	Columns       []splunk.ResultColumn `json:"columns"`
	CountCommand  string                `json:"count_command"`
	SampleCommand string                `json:"sample_command"`
}

func newResultSchemaCommand(e *env) *cobra.Command {
	var flags resultSchemaFlags
	cmd := &cobra.Command{
		Use:   "result-schema --table=<result_table>",
		Short: "Inspect saved result table columns",
		Long: `Inspect the schema of an existing local result table.

This command does not run a new Splunk search and does not read raw result rows.
It reads only local SQLite table metadata so AI callers can see available columns before writing result-search SQL.

Inside result-search SQL, refer to the selected table as results.`,
		Example: `  splsearch result-schema --table=identity_logs
  splsearch result-search --table=identity_logs --query='SELECT service, count(*) AS rows FROM results GROUP BY service' --limit=20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultSchemaErrorAndFail(cmd, "unexpected positional argument %q; use --table=<result_table>", args[0])
			}
			if strings.TrimSpace(flags.table) == "" {
				return writeResultSchemaErrorAndFail(cmd, "missing --table=<result_table>")
			}
			result, err := splunk.NewResultStore(e.configDir).Schema(cmd.Context(), splunk.ResultSchemaRequest{Table: flags.table})
			if err != nil {
				return writeResultSchemaErrorAndFail(cmd, "%s", err.Error())
			}
			output := resultSchemaOutput{
				OK:            result.OK,
				DB:            result.DB,
				Table:         result.Table,
				Rows:          result.Rows,
				ColumnCount:   result.ColumnCount,
				QueryTable:    result.QueryTable,
				Columns:       result.Columns,
				CountCommand:  resultCountCommand(result.Table),
				SampleCommand: resultSampleCommand(result.Table),
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), output); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	return cmd
}

func resultSchemaCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-schema",
		"--table=" + shellArg(table),
	}, " ")
}

func writeResultSchemaErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
