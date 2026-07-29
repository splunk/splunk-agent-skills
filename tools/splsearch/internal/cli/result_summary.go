package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

const defaultResultSummaryLimit = 50
const defaultResultSummaryThresholds = "250,1000"
const defaultResultSummaryOrder = "desc"

type resultSummaryFlags struct {
	table      string
	groupBy    string
	metric     string
	thresholds string
	timeFrom   string
	timeTo     string
	errorWhere string
	preset     string
	orderBy    string
	order      string
	limit      int
}

func newResultSummaryCommand(e *env) *cobra.Command {
	var flags resultSummaryFlags
	cmd := &cobra.Command{
		Use:   "result-summary --table=<result_table> --group-by=<field[,field...]>",
		Short: "Summarize saved Splunk result tables",
		Long: `Summarize a saved local result table with common incident aggregates.

This command does not run a new Splunk search. It groups saved rows by explicit
fields and returns compact JSON for first-pass incident shape.

Use result-schema first to find available columns. Use --time-from/--time-to
for exact alert windows. Use result-search for custom SQL analysis that this
helper cannot express.
With --preset=latency and thresholds, the default order is the lowest threshold
column, such as gte_250 for --thresholds=250,1000.`,
		Example: `  splsearch result-summary --table=app_logs --group-by=component --limit=20
  splsearch result-summary --table=app_logs --group-by=component,operation --metric=latency_ms --thresholds=250,1000 --limit=20
  splsearch result-summary --table=app_logs --group-by=component --metric=latency_ms --thresholds=250,1000 --order-by=gte_1000 --limit=20
  splsearch result-summary --table=app_logs --group-by=component --metric=latency_ms --preset=latency --time-from=2026-04-28T10:00:00Z --time-to=2026-04-28T10:10:00Z --error-where='severity = "ERROR"' --limit=20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return writeResultSummaryErrorAndFail(cmd, "unexpected positional argument %q; use --table=<result_table> and --group-by=<field[,field...]>", args[0])
			}
			if strings.TrimSpace(flags.table) == "" {
				return writeResultSummaryErrorAndFail(cmd, "missing --table=<result_table>")
			}
			groupBy := splitCommaValues(flags.groupBy)
			if len(groupBy) == 0 {
				return writeResultSummaryErrorAndFail(cmd, "missing --group-by=<field[,field...]>")
			}
			if flags.limit <= 0 {
				return writeResultSummaryErrorAndFail(cmd, "--limit must be > 0")
			}
			if strings.TrimSpace(flags.thresholds) != "" && strings.TrimSpace(flags.metric) == "" && cmd.Flags().Changed("thresholds") {
				return writeResultSummaryErrorAndFail(cmd, "--thresholds requires --metric=<numeric_field>")
			}
			if strings.EqualFold(strings.TrimSpace(flags.preset), "latency") && strings.TrimSpace(flags.metric) == "" {
				return writeResultSummaryErrorAndFail(cmd, "--preset=latency requires --metric=<numeric_field>")
			}
			var thresholds []float64
			if strings.TrimSpace(flags.metric) != "" {
				var err error
				thresholds, err = parseSummaryThresholds(flags.thresholds)
				if err != nil {
					return writeResultSummaryErrorAndFail(cmd, "%s", err.Error())
				}
			}
			result, err := splunk.NewResultStore(e.configDir).Summary(cmd.Context(), splunk.ResultSummaryRequest{
				Table:      flags.table,
				GroupBy:    groupBy,
				Metric:     flags.metric,
				Thresholds: thresholds,
				TimeFrom:   flags.timeFrom,
				TimeTo:     flags.timeTo,
				ErrorWhere: flags.errorWhere,
				Preset:     flags.preset,
				OrderBy:    flags.orderBy,
				Order:      flags.order,
				Limit:      flags.limit,
			})
			if err != nil {
				return writeResultSummaryErrorAndFail(cmd, "%s", err.Error())
			}
			if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	cmd.Flags().StringVar(&flags.groupBy, "group-by", "", "comma-separated saved-table fields to group by")
	cmd.Flags().StringVar(&flags.metric, "metric", "", "optional numeric field for avg, max, and threshold counts")
	cmd.Flags().StringVar(&flags.thresholds, "thresholds", defaultResultSummaryThresholds, "comma-separated numeric thresholds used with --metric")
	cmd.Flags().StringVar(&flags.timeFrom, "time-from", "", "inclusive lower _time bound applied before grouping")
	cmd.Flags().StringVar(&flags.timeTo, "time-to", "", "exclusive upper _time bound applied before grouping")
	cmd.Flags().StringVar(&flags.errorWhere, "error-where", "", "optional read-only SQL predicate for error_count and error_rate")
	cmd.Flags().StringVar(&flags.preset, "preset", "", `optional preset; currently "latency"`)
	cmd.Flags().StringVar(&flags.orderBy, "order-by", "", "summary output column to order by; defaults to rows or the latency preset signal")
	cmd.Flags().StringVar(&flags.order, "order", defaultResultSummaryOrder, `sort direction: "desc" or "asc"`)
	cmd.Flags().IntVar(&flags.limit, "limit", defaultResultSummaryLimit, "maximum groups printed to stdout")
	return cmd
}

func splitCommaValues(value string) []string {
	var result []string
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func parseSummaryThresholds(value string) ([]float64, error) {
	parts := splitCommaValues(value)
	if len(parts) == 0 {
		return nil, nil
	}
	thresholds := make([]float64, 0, len(parts))
	for _, part := range parts {
		threshold, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid --thresholds value %q", part)
		}
		thresholds = append(thresholds, threshold)
	}
	return thresholds, nil
}

func resultSummaryCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-summary",
		"--table=" + shellArg(table),
		"--group-by=<field>",
		"--limit=50",
	}, " ")
}

func resultLatencySummaryCommand(table string) string {
	return strings.Join([]string{
		"splsearch",
		"result-summary",
		"--table=" + shellArg(table),
		"--group-by=<field>",
		"--metric=<numeric_field>",
		"--preset=latency",
		"--limit=50",
	}, " ")
}

func writeResultSummaryErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
