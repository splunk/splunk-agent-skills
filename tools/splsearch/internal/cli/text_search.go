package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/splunk/splunk-agent-skills/tools/splsearch/internal/splunk"
)

const defaultResultTextSearchLimit = 20

type resultTextSearchFlags struct {
	table string
	query string
	limit int
}

func newResultTextSearchCommand(e *env) *cobra.Command {
	var flags resultTextSearchFlags
	cmd := &cobra.Command{
		Use:   "result-text-search --table=<result_table> [query]",
		Short: "BM25-search a saved Splunk result table",
		Long: `Search one saved local result table with SQLite FTS5/BM25.

This command does not run a new Splunk search. It searches saved rows in
~/.config/splsearch/results.sqlite unless XDG_CONFIG_HOME changes the config
directory. Query input is plain text, not raw FTS syntax.

Use this for text recall: error strings, request IDs, symptoms, titles, and
phrases that may already be in a saved table. All query terms must match the
same saved row. Use result-search SQL for numeric predicates such as latency
thresholds, counts, comparisons, grouping, and joins.

Check match_scope before citing a hit. table_context and title-only matches are
leads to inspect, not row-body evidence by themselves.`,
		Example: `  splsearch result-text-search --table=app_logs --query='deploy failed'
  splsearch result-text-search --table=app_logs 'tenant token auth' --limit=20`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(flags.table) == "" {
				return writeTextSearchErrorAndFail(cmd, "missing --table=<result_table>")
			}
			query, err := textSearchQueryArg(flags.query, args)
			if err != nil {
				return writeTextSearchErrorAndFail(cmd, "%s", err.Error())
			}
			if flags.limit <= 0 {
				return writeTextSearchErrorAndFail(cmd, "--limit must be > 0")
			}
			result, err := splunk.NewResultStore(e.configDir).TextSearch(cmd.Context(), splunk.ResultTextSearchRequest{
				Table: flags.table,
				Query: query,
				Limit: flags.limit,
			})
			if err != nil {
				return writeTextSearchErrorAndFail(cmd, "%s", err.Error())
			}
			decorateTextSearchResult(&result, query)
			if err := writeSearchJSON(cmd.OutOrStdout(), result); err != nil {
				return fail(1, "%w", err)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&flags.table, "table", "", "saved result table returned by splsearch search")
	cmd.Flags().StringVar(&flags.query, "query", "", "plain-text BM25 query")
	cmd.Flags().IntVar(&flags.limit, "limit", defaultResultTextSearchLimit, "maximum hits printed to stdout")
	return cmd
}

func textSearchQueryArg(flagQuery string, args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("unexpected positional argument %q", args[1])
	}
	query := strings.TrimSpace(flagQuery)
	if len(args) == 1 {
		if query != "" {
			return "", fmt.Errorf("use either positional query or --query, not both")
		}
		query = strings.TrimSpace(args[0])
	}
	if query == "" {
		return "", fmt.Errorf("missing --query=<text>")
	}
	if err := textSearchPlaceholderQueryError(query); err != nil {
		return "", err
	}
	return query, nil
}

func textSearchPlaceholderQueryError(query string) error {
	switch query {
	case "<text>":
		return fmt.Errorf("replace <text> with a concrete search query")
	case "<row-specific-text>":
		return fmt.Errorf("replace <row-specific-text> with distinctive row text from sample_command output")
	default:
		return nil
	}
}

func decorateTextSearchResult(result *splunk.ResultTextSearchResult, query string) {
	for i := range result.Hits {
		hit := &result.Hits[i]
		textQuery := query
		if hit.MatchScope == "table_context" {
			hit.SampleCommand = resultSampleCommand(hit.Table)
			textQuery = "<row-specific-text>"
		} else {
			hit.SampleCommand = resultSampleRowCommand(hit.Table, hit.Row)
			rowTextQuery := strings.TrimSpace(hit.RowTextQuery)
			rowContentQuery := strings.TrimSpace(hit.RowContentQuery)
			if rowContentQuery != "" && (strings.Contains(hit.MatchScope, "title") || strings.Contains(hit.MatchScope, "table_context")) {
				if resultTextSearchStandaloneMetricQuery(rowContentQuery) {
					if rowTextQuery != "" && !strings.EqualFold(rowTextQuery, rowContentQuery) {
						textQuery = rowTextQuery
					} else {
						textQuery = "<row-specific-text>"
					}
				} else {
					textQuery = rowContentQuery
				}
			} else if strings.Contains(hit.MatchScope, "table_context") && rowTextQuery != "" {
				textQuery = rowTextQuery
			} else if strings.Contains(hit.MatchScope, "title") || strings.Contains(hit.MatchScope, "table_context") {
				textQuery = "<row-specific-text>"
			}
		}
		hit.TextSearchCommand = resultTextSearchCommandWithQuery(hit.Table, textQuery, result.Limit)
	}
}

func resultTextSearchStandaloneMetricQuery(query string) bool {
	tokens := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(tokens) != 1 {
		return false
	}
	switch tokens[0] {
	case "avg_ms", "count", "duration_ms", "durationms", "elapsed_ms", "elapsedms", "latency_ms", "max_ms", "median_ms", "min_ms", "p50_ms", "p75_ms", "p90_ms", "p95_ms", "p99_ms", "request_count", "rows", "slow_pct", "slow_requests", "total_requests":
		return true
	default:
		return false
	}
}

func resultSampleRowCommand(table string, row int) string {
	query := fmt.Sprintf("SELECT _row, _json FROM results WHERE _row = %d", row)
	return strings.Join([]string{
		"splsearch",
		"result-search",
		"--table=" + shellArg(table),
		"--query=" + shellArg(query),
		"--limit=1",
	}, " ")
}

func resultTextSearchCommand(table string) string {
	return resultTextSearchCommandWithQuery(table, "<text>", defaultResultTextSearchLimit)
}

func resultTextSearchCommandWithQuery(table, query string, limit int) string {
	if strings.TrimSpace(query) == "" {
		query = "<text>"
	}
	if limit <= 0 {
		limit = defaultResultTextSearchLimit
	}
	return strings.Join([]string{
		"splsearch",
		"result-text-search",
		"--table=" + shellArg(table),
		"--query=" + shellArg(query),
		fmt.Sprintf("--limit=%d", limit),
	}, " ")
}

func writeTextSearchErrorAndFail(cmd *cobra.Command, format string, args ...any) error {
	message := fmt.Sprintf(format, args...)
	_ = writeSearchJSON(cmd.OutOrStdout(), searchErrorResult{OK: false, Message: message})
	return failSilent(1, "%s", message)
}
