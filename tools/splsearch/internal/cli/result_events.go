package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

const defaultResultEventsLimit = 100

type resultEventsFlags struct {
	table     string
	requestID string
	field     string
	jsonField string
	value     string
	limit     int
}

func newResultEventsCommand(e *env) *cobra.Command {
	var flags resultEventsFlags
	cmd := &cobra.Command{
		Use:   "result-events --table=<result_table> (--field=<field> --value=<value> | --json-field=<path> --value=<value> | --request-id=<id>)",
		Short: "Find ordered events in a saved result table",
		Long: `Find matching events in an existing local result table.

This command does not run a new Splunk search. It finds matching saved rows,
orders them by _time and _row when _time exists, and returns a compact event
sequence.

Use --field=<column> --value=<value> for saved columns, --json-field=$.path
--value=<value> for JSON fields inside _raw, or --request-id=<id> as a shortcut
that auto-detects common request, trace, and correlation ID fields.`,
		Example: `  splsearch result-events --table=app_logs --field=session_id --value=abc-123
  splsearch result-events --table=app_logs --json-field=$.requestId --value=abc-123
  splsearch result-events --table=app_logs --request-id=abc-123 --limit=200`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultEventsErrorAndFail(cmd, "unexpected positional argument %q; use flags to select saved events", args[0])
			}
			if strings.TrimSpace(flags.table) == "" {
				return writeResultEventsErrorAndFail(cmd, "missing --table=<result_table>")
			}
			if flags.limit <= 0 {
				return writeResultEventsErrorAndFail(cmd, "--limit must be > 0")
			}
			result, err := splunk.NewResultStore(e.configDir).Events(cmd.Context(), splunk.ResultEventsRequest{
				Table:     flags.table,
				RequestID: flags.requestID,
				Field:     flags.field,
				JSONField: flags.jsonField,
				Value:     flags.value,
				Limit:     flags.limit,
			})
			if err != nil {
				return writeResultEventsErrorAndFail(cmd, "%s", err.Error())
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	cmd.Flags().StringVar(&flags.field, "field", "", "saved-table column to match")
	cmd.Flags().StringVar(&flags.jsonField, "json-field", "", "JSON path inside _raw to match, for example $.requestId")
	cmd.Flags().StringVar(&flags.value, "value", "", "value to match for --field or --json-field")
	cmd.Flags().StringVar(&flags.requestID, "request-id", "", "request ID shortcut that auto-detects common request, trace, and correlation ID fields")
	cmd.Flags().IntVar(&flags.limit, "limit", defaultResultEventsLimit, "maximum matching events printed to stdout")
	return cmd
}

func resultEventsCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-events",
		"--table=" + shellArg(table),
		"--field=<field>",
		"--value=<value>",
		"--limit=100",
	}, " ")
}

func writeResultEventsErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
