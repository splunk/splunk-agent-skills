package splunk

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	_ "modernc.org/sqlite"
)

const (
	resultsDBName            = "results.sqlite"
	resultsDBLockName        = "results.sqlite.lock"
	resultsDBBusyTimeoutMS   = 15000
	resultWriteProgressRows  = 10000
	resultWriteProgressEvery = 2 * time.Second
	ResultSourceSplunk       = "splsearch"
	ResultKindSearch         = "search"
)

var resultTableNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,127}$`)
var unsafeSQLPredicatePattern = regexp.MustCompile(`(?i)\b(attach|alter|create|delete|detach|drop|insert|pragma|replace|update|vacuum)\b`)

type ResultStore struct {
	configDir string
	now       func() time.Time
}

type SaveOptions struct {
	Warnings           []string
	WarningDetails     []ResultWarning
	Progress           io.Writer
	StructuredProgress SearchProgressFunc
}

func NewResultStore(configDir string) *ResultStore {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	return &ResultStore{
		configDir: configDir,
		now:       time.Now,
	}
}

func (s *ResultStore) Path() string {
	return filepath.Join(s.configDir, resultsDBName)
}

func (s *ResultStore) lockPath() string {
	return filepath.Join(s.configDir, resultsDBLockName)
}

func (s *ResultStore) Save(ctx context.Context, result SearchResult, explicitTable string) (StoredSearchResult, error) {
	return s.SaveWithOptions(ctx, result, explicitTable, SaveOptions{})
}

func (s *ResultStore) SaveWithOptions(ctx context.Context, result SearchResult, explicitTable string, options SaveOptions) (StoredSearchResult, error) {
	unlock, err := s.lockWrites(ctx)
	if err != nil {
		return StoredSearchResult{}, err
	}
	defer unlock()

	db, err := s.open(ctx)
	if err != nil {
		return StoredSearchResult{}, err
	}
	defer db.Close()

	tableName, replace, err := s.resolveTableName(ctx, db, explicitTable)
	if err != nil {
		return StoredSearchResult{}, err
	}
	createdAt := s.now().Unix()
	warningDetails := normalizeWarningDetails(options.WarningDetails, options.Warnings)
	if err := s.writeResult(ctx, db, tableName, replace, createdAt, result, warningDetails, options.Progress, options.StructuredProgress); err != nil {
		return StoredSearchResult{}, err
	}
	s.indexResultTextBestEffort(ctx, db, tableName, options.Progress)
	rows := len(result.Results)
	_ = os.Chmod(s.Path(), 0o600)
	activeWarnings, acceptedWarnings := warningMessagesByStatus(warningDetails)
	return StoredSearchResult{
		OK:                   true,
		DB:                   s.Path(),
		Table:                tableName,
		URL:                  result.URL,
		App:                  result.App,
		SID:                  result.SID,
		Query:                result.Query,
		Earliest:             result.Earliest,
		Latest:               result.Latest,
		ResultCount:          result.ResultCount,
		Rows:                 rows,
		Offset:               result.Offset,
		HasMore:              result.HasMore,
		RunDuration:          result.RunDuration,
		CreatedAt:            createdAt,
		Warnings:             activeWarnings,
		WarningCount:         len(activeWarnings),
		AcceptedWarnings:     acceptedWarnings,
		AcceptedWarningCount: len(acceptedWarnings),
		WarningDetails:       warningDetails,
	}, nil
}

func (s *ResultStore) Query(ctx context.Context, request ResultQueryRequest) (ResultQueryResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return ResultQueryResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return ResultQueryResult{}, err
	}
	query, err := normalizeResultQuery(request.Query)
	if err != nil {
		return ResultQueryResult{}, err
	}
	if request.Limit <= 0 {
		return ResultQueryResult{}, fmt.Errorf("--limit must be > 0")
	}
	unlock, err := s.lockReads(ctx)
	if err != nil {
		return ResultQueryResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ResultQueryResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return ResultQueryResult{}, err
	}
	if !exists {
		return ResultQueryResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}
	if _, err := db.ExecContext(ctx, `CREATE TEMP VIEW `+quoteIdent("results")+` AS SELECT * FROM `+quoteIdent(tableName)); err != nil {
		return ResultQueryResult{}, fmt.Errorf("prepare result table alias: %w", err)
	}

	rows, err := db.QueryContext(ctx, `SELECT * FROM (`+query+`) LIMIT ?`, request.Limit+1)
	if err != nil {
		return ResultQueryResult{}, fmt.Errorf("query result table %s: %w", tableName, err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return ResultQueryResult{}, fmt.Errorf("read result query columns: %w", err)
	}
	resultRows, err := scanResultQueryRows(rows, columns)
	if err != nil {
		return ResultQueryResult{}, err
	}
	truncated := len(resultRows) > request.Limit
	if truncated {
		resultRows = resultRows[:request.Limit]
	}
	message := ""
	if truncated {
		message = fmt.Sprintf("output truncated to --limit=%d rows; refine --query or increase --limit if more rows are needed", request.Limit)
	}
	return ResultQueryResult{
		OK:        true,
		DB:        s.Path(),
		Table:     tableName,
		Limit:     request.Limit,
		Truncated: truncated,
		Message:   message,
		Columns:   columns,
		Rows:      resultRows,
	}, nil
}

func (s *ResultStore) Schema(ctx context.Context, request ResultSchemaRequest) (ResultSchemaResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return ResultSchemaResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return ResultSchemaResult{}, err
	}
	unlock, err := s.lockReads(ctx)
	if err != nil {
		return ResultSchemaResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ResultSchemaResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return ResultSchemaResult{}, err
	}
	if !exists {
		return ResultSchemaResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}

	var rowCount int
	if err := db.QueryRowContext(ctx, `SELECT rows FROM searches WHERE table_name = ?`, tableName).Scan(&rowCount); err != nil {
		return ResultSchemaResult{}, fmt.Errorf("read result table metadata: %w", err)
	}
	columns, err := readResultTableColumns(ctx, db, tableName)
	if err != nil {
		return ResultSchemaResult{}, err
	}
	return ResultSchemaResult{
		OK:          true,
		DB:          s.Path(),
		Table:       tableName,
		Rows:        rowCount,
		ColumnCount: len(columns),
		QueryTable:  "results",
		Columns:     columns,
	}, nil
}

func (s *ResultStore) ListTables(ctx context.Context, request ListResultTablesRequest) (ListResultTablesResult, error) {
	if request.Limit <= 0 {
		return ListResultTablesResult{}, fmt.Errorf("--limit must be > 0")
	}
	if _, err := os.Stat(s.Path()); err != nil {
		if os.IsNotExist(err) {
			return ListResultTablesResult{
				OK:      true,
				DB:      s.Path(),
				Limit:   request.Limit,
				Tables:  []ResultTableRecord{},
				Message: "results database not found; no saved result tables",
			}, nil
		}
		return ListResultTablesResult{}, fmt.Errorf("stat results db: %w", err)
	}
	unlock, err := s.lockReads(ctx)
	if err != nil {
		return ListResultTablesResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ListResultTablesResult{}, err
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT
		table_name, url, app, sid, query, earliest, latest, result_count, rows, offset, has_more, run_duration, created_at, warnings_json
		FROM searches
		ORDER BY created_at DESC, table_name
		LIMIT ?`, request.Limit+1)
	if err != nil {
		return ListResultTablesResult{}, fmt.Errorf("read result table list: %w", err)
	}
	defer rows.Close()

	records := []ResultTableRecord{}
	for rows.Next() {
		var record ResultTableRecord
		var hasMore int
		var warningsJSON string
		if err := rows.Scan(
			&record.Table,
			&record.URL,
			&record.App,
			&record.SID,
			&record.Query,
			&record.Earliest,
			&record.Latest,
			&record.ResultCount,
			&record.Rows,
			&record.Offset,
			&hasMore,
			&record.RunDuration,
			&record.CreatedAt,
			&warningsJSON,
		); err != nil {
			return ListResultTablesResult{}, fmt.Errorf("scan result table list: %w", err)
		}
		record.HasMore = hasMore != 0
		record.Source = ResultSourceSplunk
		record.Kind = ResultKindSearch
		record.CreatedAtUTC = time.Unix(record.CreatedAt, 0).UTC().Format(time.RFC3339)
		applyWarningDetails(&record, warningsJSON)
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return ListResultTablesResult{}, fmt.Errorf("read result table list: %w", err)
	}
	truncated := len(records) > request.Limit
	if truncated {
		records = records[:request.Limit]
	}
	message := ""
	if truncated {
		message = fmt.Sprintf("output truncated to --limit=%d tables; increase --limit if more table metadata is needed", request.Limit)
	}
	return ListResultTablesResult{
		OK:        true,
		DB:        s.Path(),
		Count:     len(records),
		Limit:     request.Limit,
		Truncated: truncated,
		Message:   message,
		Tables:    records,
	}, nil
}

func (s *ResultStore) Info(ctx context.Context, request ResultInfoRequest) (ResultInfoResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return ResultInfoResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return ResultInfoResult{}, err
	}
	unlock, err := s.lockReads(ctx)
	if err != nil {
		return ResultInfoResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ResultInfoResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return ResultInfoResult{}, err
	}
	if !exists {
		return ResultInfoResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}
	record, err := s.readResultTableRecord(ctx, db, tableName)
	if err != nil {
		return ResultInfoResult{}, err
	}
	return ResultInfoResult{
		OK:                true,
		DB:                s.Path(),
		ResultTableRecord: record,
	}, nil
}

func (s *ResultStore) AcceptWarning(ctx context.Context, request AcceptResultWarningRequest) (AcceptResultWarningResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return AcceptResultWarningResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return AcceptResultWarningResult{}, err
	}
	code := normalizeWarningCode(request.Code)
	if code == "" {
		return AcceptResultWarningResult{}, fmt.Errorf("missing --code=<warning_code>")
	}
	unlock, err := s.lockWrites(ctx)
	if err != nil {
		return AcceptResultWarningResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return AcceptResultWarningResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return AcceptResultWarningResult{}, err
	}
	if !exists {
		return AcceptResultWarningResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}

	var warningsJSON string
	if err := db.QueryRowContext(ctx, `SELECT warnings_json FROM searches WHERE table_name = ?`, tableName).Scan(&warningsJSON); err != nil {
		return AcceptResultWarningResult{}, fmt.Errorf("read result table warnings: %w", err)
	}
	details := decodeWarningDetails(warningsJSON)
	changed := false
	found := false
	now := s.now().Unix()
	for index := range details {
		if details[index].Code != code {
			continue
		}
		found = true
		if !details[index].Accepted {
			details[index].Accepted = true
			details[index].AcceptedAt = now
			details[index].AcceptedAtUTC = time.Unix(now, 0).UTC().Format(time.RFC3339)
			changed = true
		}
	}
	if !found {
		return AcceptResultWarningResult{}, fmt.Errorf("warning code %q was not found for result table %s", code, tableName)
	}
	if changed {
		encoded, err := encodeWarningDetails(details)
		if err != nil {
			return AcceptResultWarningResult{}, err
		}
		if _, err := db.ExecContext(ctx, `UPDATE searches SET warnings_json = ? WHERE table_name = ?`, encoded, tableName); err != nil {
			return AcceptResultWarningResult{}, fmt.Errorf("update result table warnings: %w", err)
		}
	}
	activeWarnings, acceptedWarnings := warningMessagesByStatus(details)
	message := fmt.Sprintf("accepted warning code %q for result table %s", code, tableName)
	if !changed {
		message = fmt.Sprintf("warning code %q was already accepted for result table %s", code, tableName)
	}
	return AcceptResultWarningResult{
		OK:                   true,
		DB:                   s.Path(),
		Table:                tableName,
		Code:                 code,
		Accepted:             changed,
		Message:              message,
		Warnings:             activeWarnings,
		WarningCount:         len(activeWarnings),
		AcceptedWarnings:     acceptedWarnings,
		AcceptedWarningCount: len(acceptedWarnings),
		WarningDetails:       details,
	}, nil
}

func (s *ResultStore) Summary(ctx context.Context, request ResultSummaryRequest) (ResultSummaryResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return ResultSummaryResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return ResultSummaryResult{}, err
	}
	if len(request.GroupBy) == 0 {
		return ResultSummaryResult{}, fmt.Errorf("missing --group-by=<field[,field...]>")
	}
	if request.Limit <= 0 {
		return ResultSummaryResult{}, fmt.Errorf("--limit must be > 0")
	}
	preset, err := normalizeSummaryPreset(request.Preset)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	errorWhere, err := normalizeSummaryPredicate("--error-where", request.ErrorWhere)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	order, err := normalizeSummaryOrder(request.Order)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	metric := strings.TrimSpace(request.Metric)
	if preset == summaryPresetLatency && metric == "" {
		return ResultSummaryResult{}, fmt.Errorf("--preset=latency requires --metric=<numeric_field>")
	}
	unlock, err := s.lockReads(ctx)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	if !exists {
		return ResultSummaryResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}
	columns, err := readResultTableColumns(ctx, db, tableName)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	columnSet := resultColumnSet(columns)
	groupBy, err := validateSummaryFields(request.GroupBy, columnSet, "--group-by")
	if err != nil {
		return ResultSummaryResult{}, err
	}
	timeFrom := strings.TrimSpace(request.TimeFrom)
	timeTo := strings.TrimSpace(request.TimeTo)
	if (timeFrom != "" || timeTo != "") && !columnSet["_time"] {
		return ResultSummaryResult{}, fmt.Errorf("--time-from/--time-to require result table %s to contain _time", tableName)
	}
	if metric != "" {
		if _, ok := columnSet[metric]; !ok {
			return ResultSummaryResult{}, fmt.Errorf("--metric field %q was not found in result table %s", metric, tableName)
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE TEMP VIEW `+quoteIdent("results")+` AS SELECT * FROM `+quoteIdent(tableName)); err != nil {
		return ResultSummaryResult{}, fmt.Errorf("prepare result table alias: %w", err)
	}
	query, summaryColumns, err := buildSummaryQuery(summaryQueryOptions{
		GroupBy:    groupBy,
		Metric:     metric,
		Thresholds: request.Thresholds,
		TimeFrom:   timeFrom,
		TimeTo:     timeTo,
		ErrorWhere: errorWhere,
		Preset:     preset,
		OrderBy:    request.OrderBy,
		Order:      order,
	})
	if err != nil {
		return ResultSummaryResult{}, err
	}
	rows, err := db.QueryContext(ctx, query+` LIMIT ?`, request.Limit+1)
	if err != nil {
		return ResultSummaryResult{}, fmt.Errorf("summarize result table %s: %w", tableName, err)
	}
	defer rows.Close()
	resultColumns, err := rows.Columns()
	if err != nil {
		return ResultSummaryResult{}, fmt.Errorf("read result summary columns: %w", err)
	}
	resultRows, err := scanResultQueryRows(rows, resultColumns)
	if err != nil {
		return ResultSummaryResult{}, err
	}
	truncated := len(resultRows) > request.Limit
	if truncated {
		resultRows = resultRows[:request.Limit]
	}
	message := ""
	if truncated {
		message = fmt.Sprintf("output truncated to --limit=%d groups; refine --group-by or increase --limit if more groups are needed", request.Limit)
	}
	return ResultSummaryResult{
		OK:        true,
		DB:        s.Path(),
		Table:     tableName,
		Query:     query,
		Limit:     request.Limit,
		Truncated: truncated,
		Message:   message,
		Columns:   summaryColumns,
		Rows:      resultRows,
	}, nil
}

func (s *ResultStore) Events(ctx context.Context, request ResultEventsRequest) (ResultEventsResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return ResultEventsResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return ResultEventsResult{}, err
	}
	if request.Limit <= 0 {
		return ResultEventsResult{}, fmt.Errorf("--limit must be > 0")
	}
	unlock, err := s.lockReads(ctx)
	if err != nil {
		return ResultEventsResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ResultEventsResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return ResultEventsResult{}, err
	}
	if !exists {
		return ResultEventsResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}
	columns, err := readResultTableColumns(ctx, db, tableName)
	if err != nil {
		return ResultEventsResult{}, err
	}
	columnSet := resultColumnSet(columns)
	matcher, err := resolveEventMatcher(request, columnSet)
	if err != nil {
		return ResultEventsResult{}, err
	}
	if _, err := db.ExecContext(ctx, `CREATE TEMP VIEW `+quoteIdent("results")+` AS SELECT * FROM `+quoteIdent(tableName)); err != nil {
		return ResultEventsResult{}, fmt.Errorf("prepare result table alias: %w", err)
	}
	selects, outputColumns := eventSelectColumns(matcher, columnSet)
	orderBy := quoteIdent("_row")
	if columnSet["_time"] {
		orderBy = quoteIdent("_time") + ", " + quoteIdent("_row")
	}
	query := `SELECT ` + strings.Join(selects, ", ") + ` FROM ` + quoteIdent("results") + ` WHERE CAST(` + matcher.Expression + ` AS TEXT) = ? ORDER BY ` + orderBy + ` LIMIT ?`
	rows, err := db.QueryContext(ctx, query, matcher.Value, request.Limit+1)
	if err != nil {
		return ResultEventsResult{}, fmt.Errorf("match events in result table %s: %w", tableName, err)
	}
	defer rows.Close()
	resultColumns, err := rows.Columns()
	if err != nil {
		return ResultEventsResult{}, fmt.Errorf("read event match columns: %w", err)
	}
	resultRows, err := scanResultQueryRows(rows, resultColumns)
	if err != nil {
		return ResultEventsResult{}, err
	}
	truncated := len(resultRows) > request.Limit
	if truncated {
		resultRows = resultRows[:request.Limit]
	}
	message := ""
	if truncated {
		message = fmt.Sprintf("output truncated to --limit=%d events; increase --limit if more matching events are needed", request.Limit)
	}
	return ResultEventsResult{
		OK:              true,
		DB:              s.Path(),
		Table:           tableName,
		MatchMode:       matcher.Mode,
		MatchedField:    matcher.Field,
		MatchedValue:    matcher.Value,
		MatchExpression: matcher.Expression,
		Limit:           request.Limit,
		Count:           len(resultRows),
		Truncated:       truncated,
		Message:         message,
		Columns:         outputColumns,
		Rows:            resultRows,
	}, nil
}

func (s *ResultStore) DropTables(ctx context.Context, request DropResultTablesRequest) (DropResultTablesResult, error) {
	tableName := strings.TrimSpace(request.Table)
	if request.All && tableName != "" {
		return DropResultTablesResult{}, fmt.Errorf("use either --table=<result_table> or --all, not both")
	}
	if !request.All && tableName == "" {
		return DropResultTablesResult{}, fmt.Errorf("missing --table=<result_table> or --all")
	}
	if tableName != "" {
		if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
			return DropResultTablesResult{}, err
		}
	}
	unlock, err := s.lockWrites(ctx)
	if err != nil {
		return DropResultTablesResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return DropResultTablesResult{}, err
	}
	defer db.Close()
	bytesBefore, err := resultDBSize(s.Path())
	if err != nil {
		return DropResultTablesResult{}, err
	}

	var tableNames []string
	if request.All {
		tableNames, err = s.listResultTableNames(ctx, db)
		if err != nil {
			return DropResultTablesResult{}, err
		}
	} else {
		exists, err := s.resultTableExists(ctx, db, tableName)
		if err != nil {
			return DropResultTablesResult{}, err
		}
		if !exists {
			return DropResultTablesResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
		}
		tableNames = []string{tableName}
	}
	if tableNames == nil {
		tableNames = []string{}
	}

	if err := dropKnownTables(ctx, db, tableNames); err != nil {
		return DropResultTablesResult{}, err
	}
	if request.All {
		if err := deleteAllResultTextIndex(ctx, db); err != nil {
			return DropResultTablesResult{}, err
		}
	}
	if err := compactResultDB(ctx, db); err != nil {
		return DropResultTablesResult{}, err
	}
	bytesAfter, err := resultDBSize(s.Path())
	if err != nil {
		return DropResultTablesResult{}, err
	}
	bytesReclaimed := bytesBefore - bytesAfter
	if bytesReclaimed < 0 {
		bytesReclaimed = 0
	}
	return DropResultTablesResult{
		OK:             true,
		DB:             s.Path(),
		Dropped:        tableNames,
		Count:          len(tableNames),
		Compacted:      true,
		BytesBefore:    bytesBefore,
		BytesAfter:     bytesAfter,
		BytesReclaimed: bytesReclaimed,
	}, nil
}

func (s *ResultStore) CleanupOlderThan(ctx context.Context, age time.Duration) error {
	if _, err := os.Stat(s.Path()); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("stat results db: %w", err)
	}
	unlock, err := s.lockWrites(ctx)
	if err != nil {
		return err
	}
	defer unlock()

	db, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	tableNames, err := s.listResultTableNamesOlderThan(ctx, db, s.now().Add(-age).Unix())
	if err != nil {
		return err
	}
	if err := dropKnownTables(ctx, db, tableNames); err != nil {
		return err
	}
	if len(tableNames) == 0 {
		return nil
	}
	return compactResultDB(ctx, db)
}

func (s *ResultStore) listResultTableNames(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM searches ORDER BY created_at, table_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanValidTableNames(rows)
}

func (s *ResultStore) listResultTableNamesOlderThan(ctx context.Context, db *sql.DB, cutoff int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT table_name FROM searches WHERE created_at < ? ORDER BY created_at, table_name`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanValidTableNames(rows)
}

func (s *ResultStore) readResultTableRecord(ctx context.Context, db *sql.DB, tableName string) (ResultTableRecord, error) {
	row := db.QueryRowContext(ctx, `SELECT
		table_name, url, app, sid, query, earliest, latest, result_count, rows, offset, has_more, run_duration, created_at, warnings_json
		FROM searches
		WHERE table_name = ?`, tableName)
	var record ResultTableRecord
	var hasMore int
	var warningsJSON string
	if err := row.Scan(
		&record.Table,
		&record.URL,
		&record.App,
		&record.SID,
		&record.Query,
		&record.Earliest,
		&record.Latest,
		&record.ResultCount,
		&record.Rows,
		&record.Offset,
		&hasMore,
		&record.RunDuration,
		&record.CreatedAt,
		&warningsJSON,
	); err != nil {
		return ResultTableRecord{}, fmt.Errorf("read result table metadata: %w", err)
	}
	record.HasMore = hasMore != 0
	record.Source = ResultSourceSplunk
	record.Kind = ResultKindSearch
	record.CreatedAtUTC = time.Unix(record.CreatedAt, 0).UTC().Format(time.RFC3339)
	applyWarningDetails(&record, warningsJSON)
	return record, nil
}

func scanValidTableNames(rows *sql.Rows) ([]string, error) {
	var tableNames []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, err
		}
		if validResultTableName(tableName) {
			tableNames = append(tableNames, tableName)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return tableNames, nil
}

func dropKnownTables(ctx context.Context, db *sql.DB, tableNames []string) error {
	if len(tableNames) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	for _, tableName := range tableNames {
		if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+quoteIdent(tableName)); err != nil {
			return err
		}
		if err := deleteResultTextIndexTx(ctx, tx, tableName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM searches WHERE table_name = ?`, tableName); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func compactResultDB(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `VACUUM`); err != nil {
		return fmt.Errorf("compact results db: %w", err)
	}
	return nil
}

func resultDBSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("stat results db: %w", err)
	}
	return info.Size(), nil
}

func (s *ResultStore) lockReads(ctx context.Context) (func(), error) {
	return s.lockResults(ctx, syscall.LOCK_SH, "read")
}

func (s *ResultStore) lockWrites(ctx context.Context) (func(), error) {
	return s.lockResults(ctx, syscall.LOCK_EX, "write")
}

func (s *ResultStore) lockResults(ctx context.Context, lockType int, operation string) (func(), error) {
	if err := os.MkdirAll(s.configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	file, err := os.OpenFile(s.lockPath(), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open results db lock: %w", err)
	}
	_ = os.Chmod(s.lockPath(), 0o600)

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		err = syscall.Flock(int(file.Fd()), lockType|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if !isLockBusy(err) {
			_ = file.Close()
			return nil, fmt.Errorf("lock results db for %s: %w", operation, err)
		}
		select {
		case <-ctx.Done():
			_ = file.Close()
			return nil, fmt.Errorf("lock results db for %s: %w", operation, ctx.Err())
		case <-ticker.C:
		}
	}
}

func isLockBusy(err error) bool {
	return err == syscall.EWOULDBLOCK || err == syscall.EAGAIN
}

func (s *ResultStore) open(ctx context.Context) (*sql.DB, error) {
	return s.openDB(ctx, true)
}

func (s *ResultStore) openDB(ctx context.Context, ensureMetadata bool) (*sql.DB, error) {
	if err := os.MkdirAll(s.configDir, 0o700); err != nil {
		return nil, fmt.Errorf("create config dir: %w", err)
	}
	db, err := sql.Open("sqlite", s.Path())
	if err != nil {
		return nil, fmt.Errorf("open results db: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, fmt.Sprintf(`PRAGMA busy_timeout = %d`, resultsDBBusyTimeoutMS)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("configure results db: %w", err)
	}
	if ensureMetadata {
		if err := ensureResultsMetadata(ctx, db); err != nil {
			_ = db.Close()
			return nil, err
		}
	}
	_ = os.Chmod(s.Path(), 0o600)
	return db, nil
}

func (s *ResultStore) openExisting(ctx context.Context) (*sql.DB, error) {
	if _, err := os.Stat(s.Path()); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("results database not found at %s; run splsearch search first to create a result table", s.Path())
		}
		return nil, fmt.Errorf("stat results db: %w", err)
	}
	db, err := s.openDB(ctx, true)
	if err != nil {
		if db != nil {
			_ = db.Close()
		}
		return nil, err
	}
	return db, nil
}

func ensureResultsMetadata(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS searches (
		table_name TEXT PRIMARY KEY,
		url TEXT NOT NULL,
		app TEXT NOT NULL,
		sid TEXT NOT NULL,
		query TEXT NOT NULL,
		earliest TEXT NOT NULL,
		latest TEXT NOT NULL,
		result_count INTEGER NOT NULL,
		rows INTEGER NOT NULL,
		offset INTEGER NOT NULL,
		has_more INTEGER NOT NULL,
		run_duration REAL NOT NULL,
		created_at INTEGER NOT NULL,
		warnings_json TEXT NOT NULL DEFAULT '[]'
	)`)
	if err != nil {
		return fmt.Errorf("initialize results metadata: %w", err)
	}
	columns, err := tableColumnNames(ctx, db, "searches")
	if err != nil {
		return fmt.Errorf("inspect results metadata: %w", err)
	}
	if !columns["warnings_json"] {
		if _, err := db.ExecContext(ctx, `ALTER TABLE searches ADD COLUMN warnings_json TEXT NOT NULL DEFAULT '[]'`); err != nil {
			return fmt.Errorf("upgrade results metadata warnings: %w", err)
		}
	}
	if err := ensureResultTextSearch(ctx, db); err != nil {
		return err
	}
	return nil
}

func tableColumnNames(ctx context.Context, db *sql.DB, tableName string) (map[string]bool, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quoteIdent(tableName)+`)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name string
		var sqliteType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &sqliteType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (s *ResultStore) resolveTableName(ctx context.Context, db *sql.DB, explicitTable string) (string, bool, error) {
	explicitTable = strings.TrimSpace(explicitTable)
	if explicitTable != "" {
		if err := ValidateResultTableName(explicitTable); err != nil {
			return "", false, err
		}
		return explicitTable, true, nil
	}
	for i := 0; i < 10; i++ {
		tableName, err := s.generatedTableName()
		if err != nil {
			return "", false, err
		}
		exists, err := tableExists(ctx, db, tableName)
		if err != nil {
			return "", false, err
		}
		if !exists {
			return tableName, false, nil
		}
	}
	return "", false, fmt.Errorf("could not generate a unique result table name")
}

func (s *ResultStore) generatedTableName() (string, error) {
	var random [4]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generate result table suffix: %w", err)
	}
	return "splsearch_results_" + s.now().UTC().Format("20060102_150405") + "_" + hex.EncodeToString(random[:]), nil
}

func (s *ResultStore) writeResult(ctx context.Context, db *sql.DB, tableName string, replace bool, createdAt int64, result SearchResult, warningDetails []ResultWarning, progress io.Writer, structuredProgress SearchProgressFunc) error {
	fields := resultFields(result.Results)
	warningsJSON, err := encodeWarningDetails(warningDetails)
	if err != nil {
		return err
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if replace {
		if _, err := tx.ExecContext(ctx, `DROP TABLE IF EXISTS `+quoteIdent(tableName)); err != nil {
			return fmt.Errorf("replace result table: %w", err)
		}
		if err := deleteResultTextIndexTx(ctx, tx, tableName); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM searches WHERE table_name = ?`, tableName); err != nil {
			return fmt.Errorf("replace result metadata: %w", err)
		}
	}

	columns := []string{quoteIdent("_row") + " INTEGER PRIMARY KEY", quoteIdent("_json") + " TEXT NOT NULL"}
	for _, field := range fields {
		columns = append(columns, quoteIdent(field))
	}
	if _, err := tx.ExecContext(ctx, `CREATE TABLE `+quoteIdent(tableName)+` (`+strings.Join(columns, ", ")+`)`); err != nil {
		return fmt.Errorf("create result table %s: %w", tableName, err)
	}

	if err := insertResultRows(ctx, tx, tableName, fields, result.Results, progress, structuredProgress); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO searches (
		table_name, url, app, sid, query, earliest, latest, result_count, rows, offset, has_more, run_duration, created_at, warnings_json
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tableName,
		result.URL,
		result.App,
		result.SID,
		result.Query,
		result.Earliest,
		result.Latest,
		result.ResultCount,
		len(result.Results),
		result.Offset,
		boolToInt(result.HasMore),
		result.RunDuration,
		createdAt,
		warningsJSON,
	); err != nil {
		return fmt.Errorf("write result metadata: %w", err)
	}
	return tx.Commit()
}

func insertResultRows(ctx context.Context, tx *sql.Tx, tableName string, fields []string, rows []map[string]any, progress io.Writer, structuredProgress SearchProgressFunc) error {
	if len(rows) == 0 {
		return nil
	}
	columnNames := []string{quoteIdent("_row"), quoteIdent("_json")}
	for _, field := range fields {
		columnNames = append(columnNames, quoteIdent(field))
	}
	placeholders := make([]string, len(columnNames))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO `+quoteIdent(tableName)+` (`+strings.Join(columnNames, ", ")+`) VALUES (`+strings.Join(placeholders, ", ")+`)`)
	if err != nil {
		return fmt.Errorf("prepare result insert: %w", err)
	}
	defer stmt.Close()

	progressState := newResultWriteProgress(progress, structuredProgress, tableName, len(rows))
	progressState.start()
	for index, row := range rows {
		raw, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("encode result row: %w", err)
		}
		values := []any{index + 1, string(raw)}
		for _, field := range fields {
			values = append(values, sqliteValue(row[field]))
		}
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			return fmt.Errorf("insert result row %d: %w", index+1, err)
		}
		progressState.update(index + 1)
	}
	progressState.finish()
	return nil
}

func readResultTableColumns(ctx context.Context, db *sql.DB, tableName string) ([]ResultColumn, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA table_info(`+quoteIdent(tableName)+`)`)
	if err != nil {
		return nil, fmt.Errorf("read result table schema: %w", err)
	}
	defer rows.Close()

	columns := []ResultColumn{}
	for rows.Next() {
		var cid int
		var name string
		var sqliteType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &sqliteType, &notNull, &defaultValue, &primaryKey); err != nil {
			return nil, fmt.Errorf("scan result table schema: %w", err)
		}
		columns = append(columns, ResultColumn{
			Name:       name,
			SQLiteType: sqliteType,
			PrimaryKey: primaryKey != 0,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read result table schema: %w", err)
	}
	if len(columns) == 0 {
		return nil, fmt.Errorf("result table %q has no columns", tableName)
	}
	return columns, nil
}

type resultWriteProgress struct {
	writer      io.Writer
	reporter    SearchProgressFunc
	tableName   string
	totalRows   int
	started     time.Time
	lastPrinted time.Time
	enabled     bool
}

func newResultWriteProgress(writer io.Writer, reporter SearchProgressFunc, tableName string, totalRows int) *resultWriteProgress {
	return &resultWriteProgress{
		writer:    writer,
		reporter:  reporter,
		tableName: tableName,
		totalRows: totalRows,
		enabled:   (writer != nil || reporter != nil) && totalRows >= resultWriteProgressRows,
	}
}

func (p *resultWriteProgress) start() {
	if !p.enabled {
		return
	}
	p.started = time.Now()
	p.lastPrinted = p.started
	p.print(0, "starting")
}

func (p *resultWriteProgress) update(written int) {
	if !p.enabled || written >= p.totalRows {
		return
	}
	if time.Since(p.lastPrinted) >= resultWriteProgressEvery {
		p.print(written, "writing")
	}
}

func (p *resultWriteProgress) finish() {
	if !p.enabled {
		return
	}
	p.print(p.totalRows, "done")
}

func (p *resultWriteProgress) print(written int, state string) {
	now := time.Now()
	if state == "writing" && written < p.totalRows && now.Sub(p.lastPrinted) < resultWriteProgressEvery {
		return
	}
	p.lastPrinted = now
	elapsed := now.Sub(p.started).Round(time.Second)
	elapsedSeconds := now.Sub(p.started).Seconds()
	percent := 0.0
	if p.totalRows > 0 {
		percent = float64(written) * 100 / float64(p.totalRows)
	}
	eta := "unknown"
	if written > 0 && written < p.totalRows {
		rate := float64(written) / now.Sub(p.started).Seconds()
		if rate > 0 {
			eta = time.Duration(float64(p.totalRows-written) / rate).Round(time.Second).String()
		}
	} else if written >= p.totalRows {
		eta = "0s"
	}
	if p.writer != nil {
		_, _ = fmt.Fprintf(p.writer, "splsearch: %s table=%s rows=%d/%d percent=%.1f elapsed=%s eta=%s\n", state, p.tableName, written, p.totalRows, percent, elapsed, eta)
	}
	if p.reporter != nil {
		event := SearchProgressEvent{
			Phase:          searchProgressPhaseWrite,
			State:          state,
			Table:          p.tableName,
			WrittenRows:    written,
			TotalRows:      p.totalRows,
			Percent:        percent,
			ElapsedSeconds: elapsedSeconds,
		}
		if written > 0 && written < p.totalRows && elapsedSeconds > 0 {
			rate := float64(written) / elapsedSeconds
			if rate > 0 {
				event.ETASeconds = float64(p.totalRows-written) / rate
				event.EstimatedTotalSeconds = elapsedSeconds + event.ETASeconds
			}
		} else if written >= p.totalRows {
			event.EstimatedTotalSeconds = elapsedSeconds
		}
		p.reporter(event)
	}
}

func normalizeWarningDetails(details []ResultWarning, fallbackWarnings []string) []ResultWarning {
	if len(details) == 0 {
		details = warningDetailsFromMessages(fallbackWarnings)
	}
	clean := make([]ResultWarning, 0, len(details))
	for _, detail := range details {
		detail.Code = normalizeWarningCode(detail.Code)
		detail.Message = strings.TrimSpace(detail.Message)
		if detail.Message == "" {
			continue
		}
		if detail.Code == "" {
			detail.Code = ResultWarningCodeLegacy
		}
		if detail.Accepted && detail.AcceptedAt > 0 {
			detail.AcceptedAtUTC = time.Unix(detail.AcceptedAt, 0).UTC().Format(time.RFC3339)
		} else if !detail.Accepted {
			detail.AcceptedAt = 0
			detail.AcceptedAtUTC = ""
		}
		clean = append(clean, detail)
	}
	return clean
}

func normalizeWarningCode(code string) string {
	code = strings.TrimSpace(strings.ToLower(code))
	code = strings.ReplaceAll(code, "-", "_")
	return code
}

func warningDetailsFromMessages(warnings []string) []ResultWarning {
	details := make([]ResultWarning, 0, len(warnings))
	for _, warning := range warnings {
		warning = strings.TrimSpace(warning)
		if warning == "" {
			continue
		}
		details = append(details, ResultWarning{
			Code:    ResultWarningCodeLegacy,
			Message: warning,
		})
	}
	return details
}

func encodeWarningDetails(details []ResultWarning) (string, error) {
	details = normalizeWarningDetails(details, nil)
	if len(details) == 0 {
		return "[]", nil
	}
	raw, err := json.Marshal(details)
	if err != nil {
		return "", fmt.Errorf("encode result warnings: %w", err)
	}
	return string(raw), nil
}

func decodeWarningDetails(raw string) []ResultWarning {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []ResultWarning{}
	}
	var details []ResultWarning
	if err := json.Unmarshal([]byte(raw), &details); err == nil {
		return normalizeWarningDetails(details, nil)
	}
	var warnings []string
	if err := json.Unmarshal([]byte(raw), &warnings); err == nil {
		return normalizeWarningDetails(nil, warnings)
	}
	return []ResultWarning{}
}

func applyWarningDetails(record *ResultTableRecord, raw string) {
	details := decodeWarningDetails(raw)
	activeWarnings, acceptedWarnings := warningMessagesByStatus(details)
	record.Warnings = activeWarnings
	record.WarningCount = len(activeWarnings)
	record.AcceptedWarnings = acceptedWarnings
	record.AcceptedWarningCount = len(acceptedWarnings)
	record.WarningDetails = details
}

func warningMessagesByStatus(details []ResultWarning) ([]string, []string) {
	activeWarnings := []string{}
	acceptedWarnings := []string{}
	for _, detail := range normalizeWarningDetails(details, nil) {
		if detail.Accepted {
			acceptedWarnings = append(acceptedWarnings, detail.Message)
		} else {
			activeWarnings = append(activeWarnings, detail.Message)
		}
	}
	return activeWarnings, acceptedWarnings
}

type eventMatcher struct {
	Mode       string
	Field      string
	Value      string
	Expression string
}

var eventRequestIDCandidates = []string{
	"request_id",
	"requestId",
	"requestID",
	"req_id",
	"reqId",
	"trace_id",
	"traceId",
	"correlation_id",
	"correlationId",
}

var eventCompactColumns = []string{
	"_row",
	"_time",
	"level",
	"severity",
	"service",
	"host",
	"source",
	"sourcetype",
	"operation",
	"method",
	"path",
	"handler",
	"codeName",
	"status",
	"statusCode",
	"message",
	"event",
}

func resolveEventMatcher(request ResultEventsRequest, columnSet map[string]bool) (eventMatcher, error) {
	requestID := strings.TrimSpace(request.RequestID)
	field := strings.TrimSpace(request.Field)
	jsonField := strings.TrimSpace(request.JSONField)
	value := strings.TrimSpace(request.Value)
	if requestID != "" {
		if field != "" || jsonField != "" || value != "" {
			return eventMatcher{}, fmt.Errorf("use either --request-id=<id> or --field/--json-field with --value, not both")
		}
		matcher, err := resolveRequestIDEventMatcher(columnSet)
		if err != nil {
			return eventMatcher{}, err
		}
		matcher.Value = requestID
		return matcher, nil
	}
	if field == "" && jsonField == "" {
		return eventMatcher{}, fmt.Errorf("missing match selector; use --field=<column> --value=<value>, --json-field=$.path --value=<value>, or --request-id=<id>")
	}
	if field != "" && jsonField != "" {
		return eventMatcher{}, fmt.Errorf("use either --field=<column> or --json-field=$.path, not both")
	}
	if value == "" {
		return eventMatcher{}, fmt.Errorf("missing --value=<value>")
	}
	if field != "" {
		return resolveColumnEventMatcher(field, value, columnSet)
	}
	return resolveJSONEventMatcher(jsonField, value, columnSet)
}

func resolveRequestIDEventMatcher(columnSet map[string]bool) (eventMatcher, error) {
	for _, candidate := range eventRequestIDCandidates {
		if columnSet[candidate] {
			return eventMatcher{Mode: "request_id", Field: candidate, Expression: quoteIdent(candidate)}, nil
		}
	}
	if columnSet["_raw"] {
		var expressions []string
		for _, candidate := range eventRequestIDCandidates {
			path := "$." + candidate
			expressions = append(expressions, eventJSONExtractExpression(path))
		}
		return eventMatcher{
			Mode:       "request_id",
			Field:      "json:auto",
			Expression: `coalesce(` + strings.Join(expressions, ", ") + `)`,
		}, nil
	}
	return eventMatcher{}, fmt.Errorf("no request-id field was found; use --field=<column> --value=<value> or --json-field=$.requestId --value=<value>")
}

func resolveColumnEventMatcher(field, value string, columnSet map[string]bool) (eventMatcher, error) {
	if strings.ContainsRune(field, 0) {
		return eventMatcher{}, fmt.Errorf("invalid --field %q", field)
	}
	if !columnSet[field] {
		return eventMatcher{}, fmt.Errorf("--field column %q was not found in result table", field)
	}
	return eventMatcher{Mode: "field", Field: field, Value: value, Expression: quoteIdent(field)}, nil
}

func resolveJSONEventMatcher(path, value string, columnSet map[string]bool) (eventMatcher, error) {
	if strings.ContainsRune(path, 0) {
		return eventMatcher{}, fmt.Errorf("invalid --json-field %q", path)
	}
	if path == "" || !strings.HasPrefix(path, "$") {
		return eventMatcher{}, fmt.Errorf("--json-field must be a JSON path starting with $")
	}
	if !columnSet["_raw"] {
		return eventMatcher{}, fmt.Errorf("--json-field requires _raw in result table")
	}
	return eventMatcher{
		Mode:       "json_field",
		Field:      "json:" + path,
		Value:      value,
		Expression: eventJSONExtractExpression(path),
	}, nil
}

func eventSelectColumns(matcher eventMatcher, columnSet map[string]bool) ([]string, []string) {
	selects := []string{`CAST(` + matcher.Expression + ` AS TEXT) AS ` + quoteIdent("matched_value")}
	outputColumns := []string{"matched_value"}
	seen := map[string]bool{"matched_value": true}
	for _, column := range eventCompactColumns {
		if !columnSet[column] || seen[column] {
			continue
		}
		selects = append(selects, quoteIdent(column))
		outputColumns = append(outputColumns, column)
		seen[column] = true
	}
	return selects, outputColumns
}

func eventJSONExtractExpression(path string) string {
	raw := quoteIdent("_raw")
	return `CASE WHEN json_valid(` + raw + `) THEN json_extract(` + raw + `, ` + sqlStringLiteral(path) + `) ELSE NULL END`
}

func resultColumnSet(columns []ResultColumn) map[string]bool {
	columnSet := make(map[string]bool, len(columns))
	for _, column := range columns {
		columnSet[column.Name] = true
	}
	return columnSet
}

const summaryPresetLatency = "latency"

type summaryQueryOptions struct {
	GroupBy    []string
	Metric     string
	Thresholds []float64
	TimeFrom   string
	TimeTo     string
	ErrorWhere string
	Preset     string
	OrderBy    string
	Order      string
}

func validateSummaryFields(fields []string, columnSet map[string]bool, flagName string) ([]string, error) {
	result := make([]string, 0, len(fields))
	seen := map[string]bool{}
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		if !columnSet[field] {
			return nil, fmt.Errorf("%s field %q was not found in result table", flagName, field)
		}
		if !seen[field] {
			result = append(result, field)
			seen[field] = true
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("missing %s=<field[,field...]>", flagName)
	}
	return result, nil
}

func normalizeSummaryPreset(preset string) (string, error) {
	preset = strings.TrimSpace(strings.ToLower(preset))
	switch preset {
	case "", summaryPresetLatency:
		return preset, nil
	default:
		return "", fmt.Errorf(`unsupported --preset %q; use "latency"`, preset)
	}
}

func normalizeSummaryPredicate(flagName, predicate string) (string, error) {
	predicate = strings.TrimSpace(predicate)
	if predicate == "" {
		return "", nil
	}
	if strings.Contains(predicate, ";") {
		return "", fmt.Errorf("%s must be a single read-only SQL predicate", flagName)
	}
	if unsafeSQLPredicatePattern.MatchString(predicate) {
		return "", fmt.Errorf("%s must be read-only", flagName)
	}
	return predicate, nil
}

func normalizeSummaryOrder(order string) (string, error) {
	order = strings.TrimSpace(strings.ToLower(order))
	switch order {
	case "", "desc":
		return "DESC", nil
	case "asc":
		return "ASC", nil
	default:
		return "", fmt.Errorf(`invalid --order %q; use "desc" or "asc"`, order)
	}
}

func buildSummaryQuery(options summaryQueryOptions) (string, []string, error) {
	selectParts := make([]string, 0, len(options.GroupBy)+8)
	groupParts := make([]string, 0, len(options.GroupBy))
	outputColumns := make([]string, 0, len(options.GroupBy)+8)
	for _, field := range options.GroupBy {
		selectParts = append(selectParts, quoteIdent(field))
		groupParts = append(groupParts, quoteIdent(field))
		outputColumns = append(outputColumns, field)
	}
	selectParts = append(selectParts, `count(*) AS `+quoteIdent("rows"))
	outputColumns = append(outputColumns, "rows")
	if options.Metric != "" {
		metricExpr := `CAST(` + quoteIdent(options.Metric) + ` AS REAL)`
		avgAlias := "avg_" + summaryAlias(options.Metric)
		maxAlias := "max_" + summaryAlias(options.Metric)
		selectParts = append(selectParts, `avg(`+metricExpr+`) AS `+quoteIdent(avgAlias))
		selectParts = append(selectParts, `max(`+metricExpr+`) AS `+quoteIdent(maxAlias))
		outputColumns = append(outputColumns, avgAlias, maxAlias)
		for _, threshold := range options.Thresholds {
			alias := "gte_" + summaryThresholdAlias(threshold)
			selectParts = append(selectParts, `sum(CASE WHEN `+metricExpr+` >= `+summaryNumber(threshold)+` THEN 1 ELSE 0 END) AS `+quoteIdent(alias))
			outputColumns = append(outputColumns, alias)
		}
	}
	if options.ErrorWhere != "" {
		errorExpr := `CASE WHEN (` + options.ErrorWhere + `) THEN 1 ELSE 0 END`
		selectParts = append(selectParts, `sum(`+errorExpr+`) AS `+quoteIdent("error_count"))
		selectParts = append(selectParts, `(1.0 * sum(`+errorExpr+`) / count(*)) AS `+quoteIdent("error_rate"))
		outputColumns = append(outputColumns, "error_count", "error_rate")
	}
	orderBy, err := summaryOrderBy(options, outputColumns)
	if err != nil {
		return "", nil, err
	}
	filterParts := summaryFilterParts(options)
	query := `SELECT ` + strings.Join(selectParts, ", ") + ` FROM ` + quoteIdent("results")
	if len(filterParts) > 0 {
		query += ` WHERE ` + strings.Join(filterParts, " AND ")
	}
	query += ` GROUP BY ` + strings.Join(groupParts, ", ") + ` ORDER BY ` + quoteIdent(orderBy) + ` ` + options.Order
	return query, outputColumns, nil
}

func summaryFilterParts(options summaryQueryOptions) []string {
	var filters []string
	if options.TimeFrom != "" {
		filters = append(filters, quoteIdent("_time")+" >= "+sqlStringLiteral(options.TimeFrom))
	}
	if options.TimeTo != "" {
		filters = append(filters, quoteIdent("_time")+" < "+sqlStringLiteral(options.TimeTo))
	}
	return filters
}

func summaryOrderBy(options summaryQueryOptions, outputColumns []string) (string, error) {
	orderBy := strings.TrimSpace(options.OrderBy)
	if orderBy == "" {
		if options.Preset == summaryPresetLatency && options.Metric != "" {
			if len(options.Thresholds) > 0 {
				return "gte_" + summaryThresholdAlias(minSummaryThreshold(options.Thresholds)), nil
			}
			return "max_" + summaryAlias(options.Metric), nil
		}
		return "rows", nil
	}
	for _, column := range outputColumns {
		if orderBy == column {
			return orderBy, nil
		}
	}
	return "", fmt.Errorf("--order-by %q is not a result-summary output column; use one of: %s", orderBy, strings.Join(outputColumns, ", "))
}

func minSummaryThreshold(thresholds []float64) float64 {
	min := thresholds[0]
	for _, threshold := range thresholds[1:] {
		if threshold < min {
			min = threshold
		}
	}
	return min
}

func summaryAlias(value string) string {
	var builder strings.Builder
	for _, char := range strings.ToLower(value) {
		switch {
		case char >= 'a' && char <= 'z':
			builder.WriteRune(char)
		case char >= '0' && char <= '9':
			builder.WriteRune(char)
		default:
			builder.WriteByte('_')
		}
	}
	result := strings.Trim(builder.String(), "_")
	if result == "" {
		return "metric"
	}
	return result
}

func summaryThresholdAlias(value float64) string {
	return strings.ReplaceAll(strings.ReplaceAll(summaryNumber(value), ".", "_"), "-", "neg_")
}

func summaryNumber(value float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", value), "0"), ".")
}

func resultFields(rows []map[string]any) []string {
	seen := map[string]bool{
		"_row":  true,
		"_json": true,
	}
	fieldSet := map[string]string{}
	for _, row := range rows {
		for field := range row {
			if field == "" || strings.ContainsRune(field, 0) {
				continue
			}
			key := strings.ToLower(field)
			if seen[key] {
				continue
			}
			seen[key] = true
			fieldSet[field] = field
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for field := range fieldSet {
		fields = append(fields, field)
	}
	sort.Strings(fields)
	return fields
}

func normalizeResultQuery(query string) (string, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", fmt.Errorf("missing --query=<SQL SELECT>")
	}
	if strings.Count(query, ";") > 1 {
		return "", fmt.Errorf("--query must contain exactly one read-only SELECT statement")
	}
	if strings.HasSuffix(query, ";") {
		query = strings.TrimSpace(strings.TrimSuffix(query, ";"))
	}
	if strings.Contains(query, ";") {
		return "", fmt.Errorf("--query must contain exactly one read-only SELECT statement")
	}
	if !strings.HasPrefix(strings.ToLower(query), "select") {
		return "", fmt.Errorf("--query must start with SELECT and be read-only")
	}
	return query, nil
}

func scanResultQueryRows(rows *sql.Rows, columns []string) ([]map[string]any, error) {
	resultRows := []map[string]any{}
	for rows.Next() {
		values := make([]any, len(columns))
		dest := make([]any, len(columns))
		for i := range values {
			dest[i] = &values[i]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("scan result query row: %w", err)
		}
		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeSQLiteScanValue(values[i])
		}
		resultRows = append(resultRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read result query rows: %w", err)
	}
	return resultRows, nil
}

func normalizeSQLiteScanValue(value any) any {
	switch typed := value.(type) {
	case []byte:
		return string(typed)
	default:
		return typed
	}
}

func sqliteValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case string, bool, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return typed
	case json.Number:
		return typed.String()
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return fmt.Sprint(typed)
		}
		return string(raw)
	}
}

func tableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, tableName).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *ResultStore) resultTableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var metadataCount int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM searches WHERE table_name = ?`, tableName).Scan(&metadataCount); err != nil {
		return false, fmt.Errorf("read result table metadata: %w", err)
	}
	if metadataCount == 0 {
		return false, nil
	}
	exists, err := tableExists(ctx, db, tableName)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func validResultTableName(tableName string) bool {
	return resultTableNamePattern.MatchString(tableName)
}

func ValidateResultTableName(tableName string) error {
	return ValidateResultTableNameForFlag(tableName, "--result-table")
}

func ValidateResultTableNameForFlag(tableName, flagName string) error {
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return nil
	}
	if strings.TrimSpace(flagName) == "" {
		flagName = "--result-table"
	}
	if !validResultTableName(tableName) {
		return fmt.Errorf("invalid %s %q: use letters, numbers, and underscores; start with a letter or underscore; max 128 characters", flagName, tableName)
	}
	return nil
}

func quoteIdent(identifier string) string {
	return `"` + strings.ReplaceAll(identifier, `"`, `""`) + `"`
}

func sqlStringLiteral(value string) string {
	return `'` + strings.ReplaceAll(value, `'`, `''`) + `'`
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
