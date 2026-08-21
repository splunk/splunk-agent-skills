package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type resultsDropFlags struct {
	table string
	all   bool
}

func newResultsDropCommand(e *env) *cobra.Command {
	var flags resultsDropFlags
	cmd := &cobra.Command{
		Use:   "results-drop (--table=<result_table> | --all)",
		Short: "Drop saved Splunk result tables",
		Long: `Drop saved local result tables created by splsearch search.

This command only modifies the local SQLite result database at
~/.config/splsearch/results.sqlite unless XDG_CONFIG_HOME changes the config
directory. It does not delete anything from Splunk.

Use --table=<result_table> to drop one table, or --all to drop every saved
result table known to the metadata table. Output is compact JSON listing the
dropped tables and database bytes reclaimed after compaction.`,
		Example: `  splsearch results-drop --table=app_errors
  splsearch results-drop --all`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultsDropErrorAndFail(cmd, "unexpected positional argument %q; use --table=<result_table> or --all", args[0])
			}
			if flags.all && strings.TrimSpace(flags.table) != "" {
				return writeResultsDropErrorAndFail(cmd, "use either --table=<result_table> or --all, not both")
			}
			if !flags.all && strings.TrimSpace(flags.table) == "" {
				return writeResultsDropErrorAndFail(cmd, "missing --table=<result_table> or --all")
			}
			result, err := splunk.NewResultStore(e.configDir).DropTables(cmd.Context(), splunk.DropResultTablesRequest{
				Table: flags.table,
				All:   flags.all,
			})
			if err != nil {
				return writeResultsDropErrorAndFail(cmd, "%s", err.Error())
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table to drop")
	cmd.Flags().BoolVar(&flags.all, "all", false, "drop all saved result tables")
	return cmd
}

func writeResultsDropErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
