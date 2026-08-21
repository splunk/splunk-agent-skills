package splunk

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const resultTextIndexVersion = 1

var resultTextQueryTokenPattern = regexp.MustCompile(`[\p{L}\p{N}_]+(?:'[\p{L}\p{N}_]+)*`)
var resultTextFTSTokenPattern = regexp.MustCompile(`[\p{L}\p{N}]+`)
var resultTextCompactTicketTokenPattern = regexp.MustCompile(`^([A-Za-z]{2,8})([0-9]{3,})$`)
var resultTextCompactTicketPrefixes = map[string]bool{
	"cinc":   true,
	"config": true,
	"inc":    true,
	"msb":    true,
	"piinc":  true,
	"rm":     true,
	"scp":    true,
	"spl":    true,
	"to":     true,
	"vuln":   true,
}

func ensureResultTextSearch(ctx context.Context, db *sql.DB) error {
	exists, err := tableExists(ctx, db, "result_text_fts")
	if err != nil {
		return fmt.Errorf("inspect result text index: %w", err)
	}
	if exists {
		columns, err := tableColumnNames(ctx, db, "result_text_fts")
		if err != nil {
			return fmt.Errorf("inspect result text index columns: %w", err)
		}
		if !columns["context"] {
			if _, err := db.ExecContext(ctx, `DROP TABLE result_text_fts`); err != nil {
				return fmt.Errorf("upgrade result text index: %w", err)
			}
			if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS result_text_index`); err != nil {
				return fmt.Errorf("clear stale result text index metadata: %w", err)
			}
		}
	}
	if _, err := db.ExecContext(ctx, `CREATE VIRTUAL TABLE IF NOT EXISTS result_text_fts USING fts5(
		source UNINDEXED,
		kind UNINDEXED,
		table_name UNINDEXED,
		row_id UNINDEXED,
		title,
		body,
		metadata,
		context
	)`); err != nil {
		return fmt.Errorf("initialize result text index: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS result_text_index (
		table_name TEXT PRIMARY KEY,
		source TEXT NOT NULL,
		kind TEXT NOT NULL,
		rows INTEGER NOT NULL,
		index_version INTEGER NOT NULL,
		indexed_at INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("initialize result text index metadata: %w", err)
	}
	return nil
}

func (s *ResultStore) indexResultTextBestEffort(ctx context.Context, db *sql.DB, tableName string, progress io.Writer) {
	if err := s.rebuildResultTextIndex(ctx, db, tableName); err != nil && progress != nil {
		_, _ = fmt.Fprintf(progress, "warning: result text index update failed for %s: %v\n", tableName, err)
	}
}

func (s *ResultStore) TextSearch(ctx context.Context, request ResultTextSearchRequest) (ResultTextSearchResult, error) {
	query := strings.TrimSpace(request.Query)
	if query == "" {
		return ResultTextSearchResult{}, fmt.Errorf("missing --query=<text>")
	}
	if request.Limit <= 0 {
		return ResultTextSearchResult{}, fmt.Errorf("--limit must be > 0")
	}
	tableName := strings.TrimSpace(request.Table)
	if tableName == "" {
		return ResultTextSearchResult{}, fmt.Errorf("missing --table=<result_table>")
	}
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return ResultTextSearchResult{}, err
	}
	match, err := resultTextMatchQuery(query)
	if err != nil {
		return ResultTextSearchResult{}, err
	}
	if _, err := os.Stat(s.Path()); err != nil {
		if os.IsNotExist(err) {
			result := emptyResultTextSearchResult(s.Path(), tableName, query, request.Limit, "results database not found; no saved result tables")
			return result, nil
		}
		return ResultTextSearchResult{}, fmt.Errorf("stat results db: %w", err)
	}
	unlock, err := s.lockWrites(ctx)
	if err != nil {
		return ResultTextSearchResult{}, err
	}
	defer unlock()

	db, err := s.openExisting(ctx)
	if err != nil {
		return ResultTextSearchResult{}, err
	}
	defer db.Close()

	exists, err := s.resultTableExists(ctx, db, tableName)
	if err != nil {
		return ResultTextSearchResult{}, err
	}
	if !exists {
		return ResultTextSearchResult{}, fmt.Errorf("result table %q was not found in %s", tableName, s.Path())
	}
	record, err := s.readResultTableRecord(ctx, db, tableName)
	if err != nil {
		return ResultTextSearchResult{}, err
	}
	if record.Rows == 0 {
		return emptyResultTextSearchResult(s.Path(), tableName, query, request.Limit, fmt.Sprintf("result table %q has no saved rows", tableName)), nil
	}
	if err := s.ensureResultTextIndexForRecord(ctx, db, record, true); err != nil {
		return ResultTextSearchResult{}, err
	}
	return s.queryResultTextIndex(ctx, db, tableName, query, match, request.Limit)
}

func (s *ResultStore) queryResultTextIndex(ctx context.Context, db *sql.DB, tableName, plainQuery, matchQuery string, limit int) (ResultTextSearchResult, error) {
	rows, err := db.QueryContext(ctx, `SELECT source, kind, table_name, row_id, title,
		snippet(result_text_fts, 4, '', '', ' ... ', 24) AS title_snippet,
		snippet(result_text_fts, 5, '', '', ' ... ', 24) AS body_snippet,
		snippet(result_text_fts, 6, '', '', ' ... ', 24) AS metadata_snippet,
		snippet(result_text_fts, 7, '', '', ' ... ', 24) AS context_snippet,
		body AS body_text,
		metadata AS metadata_text,
		context AS context_text,
		bm25(result_text_fts, 1.0, 1.0, 1.0, 1.0, 4.0, 2.0, 1.0, 0.25) AS score
		FROM result_text_fts
		WHERE result_text_fts MATCH ? AND table_name = ?
		ORDER BY score ASC, CAST(row_id AS INTEGER) ASC
		LIMIT ?`, matchQuery, tableName, limit+1)
	if err != nil {
		return ResultTextSearchResult{}, fmt.Errorf("search result text index: %w", err)
	}
	defer rows.Close()

	hits := []ResultTextSearchHit{}
	for rows.Next() {
		var hit ResultTextSearchHit
		var rowID string
		var titleSnippet string
		var bodySnippet string
		var metadataSnippet string
		var contextSnippet string
		var bodyText string
		var metadataText string
		var contextText string
		if err := rows.Scan(&hit.Source, &hit.Kind, &hit.Table, &rowID, &hit.Title, &titleSnippet, &bodySnippet, &metadataSnippet, &contextSnippet, &bodyText, &metadataText, &contextText, &hit.Score); err != nil {
			return ResultTextSearchResult{}, fmt.Errorf("scan result text hit: %w", err)
		}
		hit.Row, _ = strconv.Atoi(rowID)
		hit.Snippet = chooseResultTextSnippet(plainQuery, titleSnippet, bodySnippet, metadataSnippet, contextSnippet)
		hit.bodyTextCoverage = resultTextSnippetCoverage(plainQuery, bodyText)
		hit.titleTextCoverage = resultTextSnippetCoverage(plainQuery, hit.Title)
		hit.metadataTextCoverage = resultTextSnippetCoverage(plainQuery, metadataText)
		hit.contextTextCoverage = resultTextSnippetCoverage(plainQuery, contextText)
		hit.contextTextOnly = hit.bodyTextCoverage == 0 && hit.titleTextCoverage == 0 && hit.metadataTextCoverage == 0 && hit.contextTextCoverage > 0
		hit.MatchScope, hit.contextAddsTerms = resultTextMatchScopeForText(plainQuery, hit.Title, bodyText, metadataText, contextText)
		hit.RowTextQuery = resultTextRowSpecificQuery(plainQuery, hit.Title, bodyText, metadataText)
		hit.RowContentQuery = resultTextRowSpecificQuery(plainQuery, "", bodyText, metadataText)
		if hit.contextTextOnly {
			hit.Title = resultTextTableContextTitle(hit)
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return ResultTextSearchResult{}, fmt.Errorf("read result text hits: %w", err)
	}
	truncated := len(hits) > limit
	if truncated {
		hits = hits[:limit]
	}
	for i := range hits {
		hits[i].Rank = i + 1
	}
	message := resultTextSearchMessage(plainQuery, limit, truncated, hits)
	return ResultTextSearchResult{
		OK:        true,
		DB:        s.Path(),
		Table:     tableName,
		Query:     plainQuery,
		Limit:     limit,
		Count:     len(hits),
		Truncated: truncated,
		Message:   message,
		Hits:      hits,
	}, nil
}

func resultTextSearchMessage(query string, limit int, truncated bool, hits []ResultTextSearchHit) string {
	message := ""
	if truncated {
		message = fmt.Sprintf("output truncated to --limit=%d hits; refine --query or increase --limit if more hits are needed", limit)
	} else if len(hits) == 0 {
		message = noResultTextSearchHitsMessage(query)
	}
	contextOnly := resultTextContextOnlyHitCount(hits)
	contextAssisted := resultTextContextAssistedHitCount(hits)
	titleOnly := resultTextTitleOnlyHitCount(hits)
	if contextOnly > 0 {
		if contextOnly == len(hits) {
			message = appendResultTextMessage(message, "all hits matched table-level context only; inspect rows or add row-specific terms before treating them as evidence")
		} else {
			message = appendResultTextMessage(message, "some hits matched table-level context only; inspect rows or add row-specific terms before treating them as evidence")
		}
	}
	if contextAssisted > 0 {
		if contextAssisted == len(hits) {
			message = appendResultTextMessage(message, "all hits used table-level context for part of the query; verify row snippets before treating them as row-specific matches")
		} else {
			message = appendResultTextMessage(message, "some hits used table-level context for part of the query; verify row snippets before treating them as row-specific matches")
		}
	}
	if titleOnly > 0 {
		if titleOnly == len(hits) {
			message = appendResultTextMessage(message, "all hits matched title fields only; inspect rows or add body terms before treating them as row-content evidence")
		} else {
			message = appendResultTextMessage(message, "some hits matched title fields only; verify snippets before treating them as row-content evidence")
		}
	}
	return message
}

func (s *ResultStore) ensureResultTextIndexForRecord(ctx context.Context, db *sql.DB, record ResultTableRecord, strictMissing bool) error {
	exists, err := tableExists(ctx, db, record.Table)
	if err != nil {
		return err
	}
	if !exists {
		if strictMissing {
			return fmt.Errorf("result table %q was not found in %s", record.Table, s.Path())
		}
		return nil
	}
	var indexedSource, indexedKind string
	var indexedRows, indexedVersion int
	err = db.QueryRowContext(ctx, `SELECT source, kind, rows, index_version FROM result_text_index WHERE table_name = ?`, record.Table).Scan(&indexedSource, &indexedKind, &indexedRows, &indexedVersion)
	if err == nil && indexedSource == ResultSourceSplunk && indexedKind == ResultKindSearch && indexedRows == record.Rows && indexedVersion == resultTextIndexVersion {
		var ftsRows int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM result_text_fts WHERE table_name = ?`, record.Table).Scan(&ftsRows); err != nil {
			return fmt.Errorf("read result text index row count: %w", err)
		}
		if ftsRows == record.Rows {
			return nil
		}
	}
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("read result text index metadata: %w", err)
	}
	return s.rebuildResultTextIndex(ctx, db, record.Table)
}

func (s *ResultStore) rebuildResultTextIndex(ctx context.Context, db *sql.DB, tableName string) error {
	if err := ValidateResultTableNameForFlag(tableName, "--table"); err != nil {
		return err
	}
	record, err := s.readResultTableRecord(ctx, db, tableName)
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
	if err := deleteResultTextIndexTx(ctx, tx, tableName); err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT _row, _json FROM `+quoteIdent(tableName)+` ORDER BY _row`)
	if err != nil {
		return fmt.Errorf("read result rows for text index: %w", err)
	}
	defer rows.Close()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO result_text_fts (source, kind, table_name, row_id, title, body, metadata, context) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare result text index insert: %w", err)
	}
	defer stmt.Close()

	indexedRows := 0
	for rows.Next() {
		var rowID int
		var rawJSON string
		if err := rows.Scan(&rowID, &rawJSON); err != nil {
			return fmt.Errorf("scan result row for text index: %w", err)
		}
		row := map[string]any{}
		if err := json.Unmarshal([]byte(rawJSON), &row); err != nil {
			row["_json"] = rawJSON
		}
		doc := buildResultTextDocument(tableName, row, resultTextRecordMetadata(record))
		if _, err := stmt.ExecContext(ctx, ResultSourceSplunk, ResultKindSearch, tableName, rowID, doc.title, doc.body, doc.metadata, doc.context); err != nil {
			return fmt.Errorf("insert result text row %d: %w", rowID, err)
		}
		indexedRows++
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read result rows for text index: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO result_text_index (table_name, source, kind, rows, index_version, indexed_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(table_name) DO UPDATE SET source = excluded.source, kind = excluded.kind, rows = excluded.rows, index_version = excluded.index_version, indexed_at = excluded.indexed_at`,
		tableName, ResultSourceSplunk, ResultKindSearch, indexedRows, resultTextIndexVersion, s.now().Unix()); err != nil {
		return fmt.Errorf("write result text index metadata: %w", err)
	}
	return tx.Commit()
}

func deleteResultTextIndexTx(ctx context.Context, tx *sql.Tx, tableName string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM result_text_fts WHERE table_name = ?`, tableName); err != nil {
		return fmt.Errorf("delete result text index rows: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM result_text_index WHERE table_name = ?`, tableName); err != nil {
		return fmt.Errorf("delete result text index metadata: %w", err)
	}
	return nil
}

func deleteAllResultTextIndex(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `DELETE FROM result_text_fts`); err != nil {
		return fmt.Errorf("delete all result text index rows: %w", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM result_text_index`); err != nil {
		return fmt.Errorf("delete all result text index metadata: %w", err)
	}
	return nil
}

type resultTextDocument struct {
	title    string
	body     string
	metadata string
	context  string
}

func buildResultTextDocument(tableName string, row map[string]any, metadata map[string]any) resultTextDocument {
	titleFields := []string{"_time", "host", "source", "sourcetype", "component", "service", "operation", "severity", "level"}
	bodyFields := []string{"_raw", "message", "error", "exception", "request_id", "requestId", "trace_id", "traceId"}
	used := map[string]bool{}
	title := strings.Join(collectResultTextFields(row, titleFields, used), " ")
	bodyParts := collectResultTextFields(row, bodyFields, used)
	bodyParts = append(bodyParts, collectRemainingResultText(row, used)...)
	contextParts := []string{resultTextTableContext(tableName)}
	if text := collectMapText(metadata, nil); text != "" {
		contextParts = append(contextParts, text)
	}
	if title == "" {
		title = resultTextFallbackTitle(row)
	}
	return resultTextDocument{
		title:    compactText(title, 600),
		body:     compactText(strings.Join(bodyParts, " "), 20000),
		metadata: "",
		context:  compactText(strings.Join(contextParts, " "), 6000),
	}
}

func resultTextRecordMetadata(record ResultTableRecord) map[string]any {
	return map[string]any{
		"url":            record.URL,
		"app":            record.App,
		"sid":            record.SID,
		"query":          record.Query,
		"earliest":       record.Earliest,
		"latest":         record.Latest,
		"result_count":   record.ResultCount,
		"offset":         record.Offset,
		"has_more":       record.HasMore,
		"run_duration":   record.RunDuration,
		"returned_rows":  record.Rows,
		"source_command": "splsearch search",
	}
}

func resultTextTableContext(tableName string) string {
	tableName = strings.TrimSpace(tableName)
	if tableName == "" {
		return ""
	}
	spaced := strings.NewReplacer("_", " ", "-", " ", ".", " ").Replace(tableName)
	split := splitResultTableNameText(tableName)
	parts := []string{"table", tableName}
	if spaced != tableName {
		parts = append(parts, spaced)
	}
	if split != "" && split != spaced {
		parts = append(parts, split)
	}
	return strings.Join(parts, " ")
}

func splitResultTableNameText(tableName string) string {
	var builder strings.Builder
	lastClass := byte(0)
	lastSpace := false
	for _, char := range tableName {
		class := byte(0)
		switch {
		case char >= 'A' && char <= 'Z', char >= 'a' && char <= 'z':
			class = 'a'
		case char >= '0' && char <= '9':
			class = '0'
		}
		if class == 0 {
			if builder.Len() > 0 && !lastSpace {
				builder.WriteByte(' ')
				lastSpace = true
			}
			lastClass = 0
			continue
		}
		if builder.Len() > 0 && lastClass != 0 && lastClass != class {
			builder.WriteByte(' ')
		}
		builder.WriteRune(char)
		lastClass = class
		lastSpace = false
	}
	return strings.Join(strings.Fields(builder.String()), " ")
}

func collectResultTextFields(row map[string]any, fields []string, used map[string]bool) []string {
	values := []string{}
	for _, field := range fields {
		value, ok := row[field]
		if !ok {
			continue
		}
		if text := scalarText(value); text != "" {
			values = append(values, text)
			used[field] = true
		}
	}
	return values
}

func collectRemainingResultText(row map[string]any, used map[string]bool) []string {
	return collectMapTextParts(row, used)
}

func collectMapText(values map[string]any, used map[string]bool) string {
	return strings.Join(collectMapTextParts(values, used), " ")
}

func collectMapTextParts(values map[string]any, used map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if used != nil && used[key] {
			continue
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := []string{}
	for _, key := range keys {
		if text := scalarText(values[key]); text != "" {
			parts = append(parts, key, text)
		}
	}
	return parts
}

func scalarText(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, json.Number:
		return strings.TrimSpace(fmt.Sprint(typed))
	default:
		return ""
	}
}

func chooseResultTextSnippet(query, titleSnippet, bodySnippet, metadataSnippet, contextSnippet string) string {
	tokens := resultTextFTSTokenPattern.FindAllString(strings.ToLower(query), -1)
	if snippet := bestResultTextSnippet(tokens, []string{bodySnippet}, true); snippet != "" {
		return snippet
	}
	if snippet := bestResultTextSnippet(tokens, []string{titleSnippet}, true); snippet != "" {
		return snippet
	}
	if snippet := bestResultTextSnippet(tokens, []string{metadataSnippet}, true); snippet != "" {
		return snippet
	}
	return bestResultTextSnippet(tokens, []string{contextSnippet}, false)
}

func bestResultTextSnippet(tokens []string, candidates []string, requireCoverage bool) string {
	bestSnippet := ""
	bestCoverage := 0
	for _, candidate := range candidates {
		snippet := strings.TrimSpace(candidate)
		if snippet == "" {
			continue
		}
		coverage := resultTextTokenCoverage(tokens, snippet)
		if coverage > bestCoverage {
			bestSnippet = snippet
			bestCoverage = coverage
		}
	}
	if requireCoverage && bestCoverage == 0 {
		return ""
	}
	return bestSnippet
}

func resultTextSnippetCoverage(query, snippet string) int {
	tokens := resultTextFTSTokenPattern.FindAllString(strings.ToLower(query), -1)
	return resultTextTokenCoverage(tokens, snippet)
}

func resultTextTokenCoverage(tokens []string, snippet string) int {
	return resultTextTokenSetCoverage(tokens, resultTextTokenSet(snippet))
}

func resultTextTokenSetCoverage(tokens []string, textTokens map[string]bool) int {
	coverage := 0
	seen := map[string]bool{}
	for _, token := range tokens {
		token = strings.ToLower(token)
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		if resultTextTokenCovered(token, textTokens) {
			coverage++
		}
	}
	return coverage
}

func resultTextTokenCovered(token string, textTokens map[string]bool) bool {
	token = strings.ToLower(strings.TrimSpace(token))
	if token == "" {
		return false
	}
	if textTokens[token] {
		return true
	}
	prefix, number, ok := splitResultTextCompactTicketToken(token)
	return ok && textTokens[strings.ToLower(prefix)] && textTokens[number]
}

func resultTextTokenSet(text string) map[string]bool {
	tokens := resultTextFTSTokenPattern.FindAllString(strings.ToLower(text), -1)
	set := make(map[string]bool, len(tokens))
	for _, token := range tokens {
		if token != "" {
			set[token] = true
		}
	}
	return set
}

func resultTextFallbackTitle(row map[string]any) string {
	for _, key := range []string{
		"title", "name", "id",
		"operationId", "operation_id", "operation",
		"request_id", "requestId", "trace_id", "traceId",
		"pod_name", "k8s_pod_name", "pod", "k8s_namespace", "namespace",
		"path", "method", "message", "error", "exception",
		"url", "query", "text", "_raw",
	} {
		if text := scalarText(row[key]); text != "" {
			return text
		}
	}
	return ResultSourceSplunk + " " + ResultKindSearch
}

func compactText(value string, max int) string {
	value = strings.Join(strings.Fields(value), " ")
	if max > 0 && len(value) > max {
		return value[:max]
	}
	return value
}

func resultTextMatchQuery(query string) (string, error) {
	tokens := resultTextQueryTokenPattern.FindAllString(query, -1)
	if len(tokens) == 0 {
		return "", fmt.Errorf("--query must include at least one searchable word or number")
	}
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		parts = append(parts, resultTextMatchQueryTerm(token))
	}
	if len(parts) == 0 {
		return "", fmt.Errorf("--query must include at least one searchable word or number")
	}
	return strings.Join(parts, " AND "), nil
}

func resultTextMatchQueryTerm(token string) string {
	prefix, number, ok := splitResultTextCompactTicketToken(token)
	if !ok {
		return strconv.Quote(token)
	}
	return fmt.Sprintf("(%s OR (%s AND %s))", strconv.Quote(token), strconv.Quote(prefix), strconv.Quote(number))
}

func splitResultTextCompactTicketToken(token string) (string, string, bool) {
	match := resultTextCompactTicketTokenPattern.FindStringSubmatch(strings.TrimSpace(token))
	if match == nil {
		return "", "", false
	}
	prefix := match[1]
	if !resultTextCompactTicketPrefixes[strings.ToLower(prefix)] {
		return "", "", false
	}
	return prefix, match[2], true
}

func resultTextMatchScopeForText(query, titleText, bodyText, metadataText, contextText string) (string, bool) {
	queryTokens := uniqueResultTextTokens(query)
	if len(queryTokens) == 0 {
		return "", false
	}
	fieldTokens := []struct {
		name   string
		tokens map[string]bool
	}{
		{name: "body", tokens: resultTextTokenSet(bodyText)},
		{name: "title", tokens: resultTextTokenSet(titleText)},
		{name: "row_metadata", tokens: resultTextTokenSet(metadataText)},
	}
	contextTokens := resultTextTokenSet(contextText)
	scopes := []string{}
	covered := map[string]bool{}
	addScope := func(name string, tokens map[string]bool, requireNew bool) bool {
		adds := false
		for _, token := range queryTokens {
			if !resultTextTokenCovered(token, tokens) {
				continue
			}
			if !covered[token] {
				adds = true
				covered[token] = true
			}
		}
		if adds || (!requireNew && resultTextTokenSetCoverage(queryTokens, tokens) > 0) {
			scopes = append(scopes, name)
			return adds
		}
		return false
	}
	for _, field := range fieldTokens {
		if addScope(field.name, field.tokens, false) {
			break
		}
	}
	for _, field := range fieldTokens {
		if len(scopes) > 0 && scopes[0] == field.name {
			continue
		}
		addScope(field.name, field.tokens, true)
	}
	contextAddsTerms := false
	if len(scopes) == 0 {
		if resultTextTokenSetCoverage(queryTokens, contextTokens) > 0 {
			scopes = append(scopes, "table_context")
		}
	} else {
		contextAddsTerms = addScope("table_context", contextTokens, true)
	}
	return strings.Join(scopes, "+"), contextAddsTerms
}

func resultTextRowSpecificQuery(query, titleText, bodyText, metadataText string) string {
	queryTokens := uniqueResultTextQueryTokens(query)
	if len(queryTokens) == 0 {
		return ""
	}
	rowTokenSet := resultTextTokenSet(strings.Join([]string{titleText, bodyText, metadataText}, " "))
	rowTokens := make([]string, 0, len(queryTokens))
	for _, token := range queryTokens {
		if resultTextQueryTokenInText(token, rowTokenSet) {
			rowTokens = append(rowTokens, token)
		}
	}
	if len(rowTokens) == 0 {
		return ""
	}
	if len(rowTokens) == 1 && !resultTextIsStrongSingleRowQueryToken(rowTokens[0]) {
		return ""
	}
	return strings.Join(rowTokens, " ")
}

func resultTextQueryTokenInText(token string, textTokens map[string]bool) bool {
	if resultTextTokenCovered(token, textTokens) {
		return true
	}
	parts := uniqueResultTextTokens(token)
	if len(parts) <= 1 {
		return false
	}
	for _, part := range parts {
		if !textTokens[part] {
			return false
		}
	}
	return true
}

func resultTextIsStrongSingleRowQueryToken(token string) bool {
	hasLetter := false
	hasDigit := false
	hasLower := false
	hasInteriorUpper := false
	for i, char := range token {
		switch {
		case char >= '0' && char <= '9':
			hasDigit = true
		case char >= 'a' && char <= 'z':
			hasLetter = true
			hasLower = true
		case char >= 'A' && char <= 'Z':
			hasLetter = true
			if i > 0 && hasLower {
				hasInteriorUpper = true
			}
		}
	}
	return strings.Contains(token, "_") || (hasLetter && hasDigit && len(token) >= 4) || hasInteriorUpper || len(token) >= 8
}

func uniqueResultTextTokens(text string) []string {
	tokens := resultTextFTSTokenPattern.FindAllString(strings.ToLower(text), -1)
	seen := map[string]bool{}
	unique := make([]string, 0, len(tokens))
	for _, token := range tokens {
		if token == "" || seen[token] {
			continue
		}
		seen[token] = true
		unique = append(unique, token)
	}
	return unique
}

func uniqueResultTextQueryTokens(query string) []string {
	return dedupeResultTextQueryTokens(resultTextQueryTokenPattern.FindAllString(query, -1))
}

func dedupeResultTextQueryTokens(tokens []string) []string {
	seen := map[string]bool{}
	unique := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		key := strings.ToLower(token)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, token)
	}
	return unique
}

func resultTextContextOnlyHitCount(hits []ResultTextSearchHit) int {
	count := 0
	for _, hit := range hits {
		if hit.contextTextOnly {
			count++
		}
	}
	return count
}

func resultTextContextAssistedHitCount(hits []ResultTextSearchHit) int {
	count := 0
	for _, hit := range hits {
		if hit.contextAddsTerms && !hit.contextTextOnly {
			count++
		}
	}
	return count
}

func resultTextTitleOnlyHitCount(hits []ResultTextSearchHit) int {
	count := 0
	for _, hit := range hits {
		if hit.MatchScope == "title" {
			count++
		}
	}
	return count
}

func resultTextTableContextTitle(hit ResultTextSearchHit) string {
	parts := []string{"table context", ResultSourceSplunk, ResultKindSearch}
	if hit.Table != "" {
		parts = append(parts, hit.Table)
	}
	return strings.Join(parts, " ")
}

func appendResultTextMessage(message, suffix string) string {
	message = strings.TrimSpace(message)
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return message
	}
	if message == "" {
		return suffix
	}
	return message + "; " + suffix
}

func noResultTextSearchHitsMessage(query string) string {
	message := "no saved rows matched the query"
	if len(resultTextQueryTokenPattern.FindAllString(query, -1)) > 1 {
		message += "; all query terms must match the same saved row, so try fewer or more distinctive terms"
	}
	return message
}

func emptyResultTextSearchResult(dbPath, tableName, query string, limit int, message string) ResultTextSearchResult {
	return ResultTextSearchResult{
		OK:      true,
		DB:      dbPath,
		Table:   tableName,
		Query:   strings.TrimSpace(query),
		Limit:   limit,
		Message: message,
		Hits:    []ResultTextSearchHit{},
	}
}
