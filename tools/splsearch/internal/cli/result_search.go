package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

const defaultResultSearchLimit = 100

type resultSearchFlags struct {
	table string
	query string
	limit int
}

func newResultSearchCommand(e *env) *cobra.Command {
	var flags resultSearchFlags
	cmd := &cobra.Command{
		Use:   "result-search --table=<result_table> --query=<SQL SELECT>",
		Short: "Search a saved Splunk result table",
		Long: `Search an existing local result table created by splsearch search.

This command does not run a new Splunk search. It queries the saved SQLite
result database at ~/.config/splsearch/results.sqlite unless XDG_CONFIG_HOME
changes the config directory.

IMPORTANT FOR AI CALLERS: result-search can print a lot of data. Always keep
queries small. Prefer selected columns, WHERE filters, aggregate queries, and
small LIMIT values. The command applies --limit=100 by default and caps stdout
with that limit even when the SQL query has a larger LIMIT. JSON output includes
"truncated": true and a message when more rows matched than were returned.

Run ` + "`splsearch result-schema --table=<result_table>`" + ` first when you need to see
available columns before writing SQL.

Inside --query, refer to the selected table as results.`,
		Example: `  splsearch result-search --table=splsearch_results_20260427_120000_ab12cd34 --query='SELECT _time, severity, component FROM results WHERE severity = "ERROR" LIMIT 20'
  splsearch result-search --table=app_logs --query='SELECT component, count(*) AS errors FROM results GROUP BY component ORDER BY errors DESC' --limit=25
  splsearch result-search --table=app_logs --query="SELECT json_extract(_raw, '$.customer') AS customer, count(*) AS rows FROM results GROUP BY customer ORDER BY rows DESC" --limit=20
  splsearch result-search --table=app_logs --query='SELECT _json FROM results WHERE component = "api" LIMIT 5' --limit=5`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultSearchErrorAndFail(cmd, "unexpected positional argument %q; use --table=<result_table> and --query=<SQL SELECT>", args[0])
			}
			if strings.TrimSpace(flags.table) == "" {
				return writeResultSearchErrorAndFail(cmd, "missing --table=<result_table>")
			}
			if strings.TrimSpace(flags.query) == "" {
				return writeResultSearchErrorAndFail(cmd, "missing --query=<SQL SELECT>")
			}
			if flags.limit <= 0 {
				return writeResultSearchErrorAndFail(cmd, "--limit must be > 0")
			}
			result, err := splunk.NewResultStore(e.configDir).Query(cmd.Context(), splunk.ResultQueryRequest{
				Table: flags.table,
				Query: flags.query,
				Limit: flags.limit,
			})
			if err != nil {
				return writeResultSearchErrorAndFail(cmd, "%s", err.Error())
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	cmd.Flags().StringVar(&flags.query, "query", "", "read-only SQL SELECT; refer to the selected result table as results")
	cmd.Flags().IntVar(&flags.limit, "limit", defaultResultSearchLimit, "maximum rows printed to stdout; keep small for AI context safety")
	return cmd
}

func writeResultSearchErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
