package splunk

import (
	"context"
	"strings"
	"testing"
)

func TestResultStoreTextSearchIndexesSavedRows(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), textSearchResult(), "text_logs"); err != nil {
		t.Fatal(err)
	}

	result, err := store.TextSearch(context.Background(), ResultTextSearchRequest{
		Table: "text_logs",
		Query: "request_remote_tok 401 Unauthorized",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Count != 1 || result.Truncated {
		t.Fatalf("unexpected text search result: %#v", result)
	}
	hit := result.Hits[0]
	if hit.Source != ResultSourceSplunk || hit.Kind != ResultKindSearch || hit.Table != "text_logs" || hit.Row != 2 {
		t.Fatalf("unexpected hit metadata: %#v", hit)
	}
	if !strings.Contains(hit.MatchScope, "body") || !strings.Contains(hit.Snippet, "request_remote_tok") {
		t.Fatalf("expected body snippet, got %#v", hit)
	}
}

func TestResultStoreTextSearchReplaceRemovesOldHits(t *testing.T) {
	store := NewResultStore(t.TempDir())
	first := textSearchResult()
	first.Results = []map[string]any{{"message": "oldneedle failed"}}
	if _, err := store.Save(context.Background(), first, "replace_logs"); err != nil {
		t.Fatal(err)
	}
	second := textSearchResult()
	second.Results = []map[string]any{{"message": "newneedle recovered"}}
	if _, err := store.Save(context.Background(), second, "replace_logs"); err != nil {
		t.Fatal(err)
	}

	oldHits, err := store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "replace_logs", Query: "oldneedle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if oldHits.Count != 0 {
		t.Fatalf("expected old FTS hits to be removed, got %#v", oldHits)
	}
	newHits, err := store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "replace_logs", Query: "newneedle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if newHits.Count != 1 || newHits.Hits[0].Row != 1 {
		t.Fatalf("expected replacement row hit, got %#v", newHits)
	}
}

func TestResultStoreTextSearchDropAndCleanupRemoveFTSRows(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), textSearchResult(), "drop_text_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Save(context.Background(), textSearchResult(), "keep_text_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.DropTables(context.Background(), DropResultTablesRequest{Table: "drop_text_logs"}); err != nil {
		t.Fatal(err)
	}
	db := openResultDB(t, store.Path())
	defer db.Close()
	var droppedFTSRows int
	if err := db.QueryRow(`SELECT COUNT(*) FROM result_text_fts WHERE table_name = ?`, "drop_text_logs").Scan(&droppedFTSRows); err != nil {
		t.Fatal(err)
	}
	if droppedFTSRows != 0 {
		t.Fatalf("expected dropped table FTS rows removed, got %d", droppedFTSRows)
	}
	kept, err := store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "keep_text_logs", Query: "request_remote_tok", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if kept.Count != 1 {
		t.Fatalf("expected kept table FTS rows to remain, got %#v", kept)
	}
}

func TestResultStoreTextSearchLazyRebuildsMissingIndex(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), textSearchResult(), "lazy_logs"); err != nil {
		t.Fatal(err)
	}
	db := openResultDB(t, store.Path())
	if _, err := db.Exec(`DELETE FROM result_text_fts WHERE table_name = ?`, "lazy_logs"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM result_text_index WHERE table_name = ?`, "lazy_logs"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	result, err := store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "lazy_logs", Query: "request_remote_tok", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 1 || result.Hits[0].Row != 2 {
		t.Fatalf("expected lazy rebuild hit, got %#v", result)
	}
}

func TestResultStoreTextSearchEscapesPlainTextQuerySyntax(t *testing.T) {
	store := NewResultStore(t.TempDir())
	result := textSearchResult()
	result.Results = []map[string]any{{
		"message": "NEAR operator text with quoted phrase and CINC-66363 incident",
	}}
	if _, err := store.Save(context.Background(), result, "syntax_logs"); err != nil {
		t.Fatal(err)
	}

	hits, err := store.TextSearch(context.Background(), ResultTextSearchRequest{
		Table: "syntax_logs",
		Query: `NEAR "quoted" CINC66363`,
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if hits.Count != 1 {
		t.Fatalf("expected escaped plain-text query to match, got %#v", hits)
	}
}

func TestResultStoreTextSearchTreatsSavedQueryAsTableContext(t *testing.T) {
	store := NewResultStore(t.TempDir())
	if _, err := store.Save(context.Background(), textSearchResult(), "context_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.TextSearch(context.Background(), ResultTextSearchRequest{
		Table: "context_logs",
		Query: "index textsearch",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count == 0 {
		t.Fatalf("expected table-context hits, got %#v", result)
	}
	if result.Hits[0].MatchScope != "table_context" || !strings.Contains(result.Message, "table-level context") {
		t.Fatalf("expected context-only hit, got %#v", result)
	}
}

func TestResultStoreTextSearchEmptyAndMissingTables(t *testing.T) {
	store := NewResultStore(t.TempDir())
	_, err := store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "missing_logs", Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("missing database should return empty JSON result, got %v", err)
	}
	empty := textSearchResult()
	empty.Results = nil
	empty.ResultCount = 0
	empty.ReturnedResults = 0
	if _, err := store.Save(context.Background(), empty, "empty_logs"); err != nil {
		t.Fatal(err)
	}
	result, err := store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "empty_logs", Query: "needle", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if result.Count != 0 || !strings.Contains(result.Message, "has no saved rows") {
		t.Fatalf("expected empty table message, got %#v", result)
	}
	_, err = store.TextSearch(context.Background(), ResultTextSearchRequest{Table: "missing_logs", Query: "needle", Limit: 10})
	if err == nil || !strings.Contains(err.Error(), `result table "missing_logs" was not found`) {
		t.Fatalf("expected missing table error, got %v", err)
	}
}

func textSearchResult() SearchResult {
	return SearchResult{
		OK:              true,
		URL:             "https://splunk.example.com",
		App:             "search",
		SID:             "sid-text",
		Query:           "search index=textsearch",
		Earliest:        "-15m",
		Latest:          "now",
		ResultCount:     2,
		ReturnedResults: 2,
		Results: []map[string]any{
			{"_time": "2026-05-18T12:00:00Z", "service": "identity", "message": "ordinary login"},
			{"_time": "2026-05-18T12:01:00Z", "service": "identity", "message": "request_remote_tok returned 401 Unauthorized"},
		},
	}
}
