package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

type resultWarningsAcceptFlags struct {
	table string
	code  string
}

func newResultWarningsCommand(e *env) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "result-warnings",
		Short: "Manage saved result table warnings",
		Long: `Manage warning metadata for saved local result tables.

This command only updates local SQLite metadata in
~/.config/splsearch/results.sqlite unless XDG_CONFIG_HOME changes the config
directory. It does not run Splunk searches and does not change result rows.`,
	}
	cmd.AddCommand(newResultWarningsAcceptCommand(e))
	return cmd
}

func newResultWarningsAcceptCommand(e *env) *cobra.Command {
	var flags resultWarningsAcceptFlags
	cmd := &cobra.Command{
		Use:   "accept --table=<result_table> --code=<warning_code>",
		Short: "Accept a saved result table warning",
		Long: `Accept one warning code for a saved local result table.

Use this after reviewing a warning such as full_fetch and deciding it was
intentional for the incident. Accepted warnings remain visible in result-info
and results-list, but they no longer contribute to warning_count.`,
		Example: `  splsearch result-warnings accept --table=identity_logs --code=full_fetch
  splsearch result-info --table=identity_logs`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultWarningsErrorAndFail(cmd, "unexpected positional argument %q; use --table=<result_table> and --code=<warning_code>", args[0])
			}
			if strings.TrimSpace(flags.table) == "" {
				return writeResultWarningsErrorAndFail(cmd, "missing --table=<result_table>")
			}
			if strings.TrimSpace(flags.code) == "" {
				return writeResultWarningsErrorAndFail(cmd, "missing --code=<warning_code>")
			}
			result, err := splunk.NewResultStore(e.configDir).AcceptWarning(cmd.Context(), splunk.AcceptResultWarningRequest{
				Table: flags.table,
				Code:  flags.code,
			})
			if err != nil {
				return writeResultWarningsErrorAndFail(cmd, "%s", err.Error())
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	cmd.Flags().StringVar(&flags.code, "code", "", "warning code to accept, for example full_fetch")
	return cmd
}

func writeResultWarningsErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
