package splunk

type AuthMethod string

const (
	MethodWeb        AuthMethod = "web"
	MethodSessionKey AuthMethod = "session_key"
)

type Cookie struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expires  float64 `json:"expires,omitempty"`
	HTTPOnly bool    `json:"http_only,omitempty"`
	Secure   bool    `json:"secure,omitempty"`
	SameSite string  `json:"same_site,omitempty"`
}

type AuthRecord struct {
	URL                   string     `json:"url"`
	Aliases               []string   `json:"aliases,omitempty"`
	APIBaseURL            string     `json:"api_base_url,omitempty"`
	WebBaseURL            string     `json:"web_base_url,omitempty"`
	Method                AuthMethod `json:"method"`
	Cookies               []Cookie   `json:"cookies,omitempty"`
	SessionKey            string     `json:"-"`
	TLSInsecureSkipVerify bool       `json:"tls_insecure_skip_verify,omitempty"`
	CreatedAt             int64      `json:"created_at"`
	UpdatedAt             int64      `json:"updated_at"`
	ExpiresAt             *int64     `json:"expires_at,omitempty"`
}

type LoginRequest struct {
	URL      string
	Insecure bool
}

type LoginResult struct {
	URL       string
	Validated bool
	Message   string
	ExpiresAt *int64
}

type StatusResult struct {
	URL            string
	LocalValid     bool
	RemoteCheck    bool
	RemoteValid    bool
	ErrorCode      string
	Operation      string
	Retryable      *bool
	DiagnosticHint string
	Message        string
	ExpiresAt      *int64
}

type LogoutResult struct {
	URL     string
	Removed bool
	Message string
}

type SearchRequest struct {
	URL      string
	Query    string
	Earliest string
	Latest   string
	App      string
	Limit    int
	Offset   int
	PageSize int
	Progress SearchProgressFunc `json:"-"`
}

type SearchProgressFunc func(SearchProgressEvent)

type SearchProgressEvent struct {
	Phase                 string  `json:"phase"`
	SID                   string  `json:"sid,omitempty"`
	State                 string  `json:"state,omitempty"`
	Table                 string  `json:"table,omitempty"`
	Percent               float64 `json:"percent,omitempty"`
	DoneProgress          float64 `json:"done_progress,omitempty"`
	ScanCount             int     `json:"scan_count,omitempty"`
	EventCount            int     `json:"event_count,omitempty"`
	ResultCount           int     `json:"result_count,omitempty"`
	ResultPreviewCount    int     `json:"result_preview_count,omitempty"`
	FetchedRows           int     `json:"fetched_rows,omitempty"`
	WrittenRows           int     `json:"written_rows,omitempty"`
	TotalRows             int     `json:"total_rows,omitempty"`
	PageSize              int     `json:"page_size,omitempty"`
	ElapsedSeconds        float64 `json:"elapsed_seconds,omitempty"`
	EstimatedTotalSeconds float64 `json:"estimated_total_seconds,omitempty"`
	ETASeconds            float64 `json:"eta_seconds,omitempty"`
}

type SearchResult struct {
	OK              bool             `json:"ok"`
	URL             string           `json:"url"`
	App             string           `json:"app"`
	SID             string           `json:"sid"`
	Query           string           `json:"query"`
	Earliest        string           `json:"earliest"`
	Latest          string           `json:"latest"`
	ResultCount     int              `json:"result_count"`
	ReturnedResults int              `json:"returned_results"`
	Offset          int              `json:"offset"`
	HasMore         bool             `json:"has_more"`
	RunDuration     float64          `json:"run_duration"`
	Results         []map[string]any `json:"results"`
}

type StoredSearchResult struct {
	OK                   bool            `json:"ok"`
	DB                   string          `json:"db"`
	Table                string          `json:"table"`
	TextSearchCommand    string          `json:"text_search_command,omitempty"`
	URL                  string          `json:"url"`
	App                  string          `json:"app"`
	SID                  string          `json:"sid"`
	Query                string          `json:"query"`
	Earliest             string          `json:"earliest"`
	Latest               string          `json:"latest"`
	ResultCount          int             `json:"result_count"`
	Rows                 int             `json:"rows"`
	Offset               int             `json:"offset"`
	HasMore              bool            `json:"has_more"`
	RunDuration          float64         `json:"run_duration"`
	CreatedAt            int64           `json:"created_at"`
	Warnings             []string        `json:"warnings,omitempty"`
	WarningCount         int             `json:"warning_count"`
	AcceptedWarnings     []string        `json:"accepted_warnings,omitempty"`
	AcceptedWarningCount int             `json:"accepted_warning_count"`
	WarningDetails       []ResultWarning `json:"warning_details,omitempty"`
}

type ResultQueryRequest struct {
	Table string
	Query string
	Limit int
}

type ResultQueryResult struct {
	OK        bool             `json:"ok"`
	DB        string           `json:"db"`
	Table     string           `json:"table"`
	Limit     int              `json:"limit"`
	Truncated bool             `json:"truncated"`
	Message   string           `json:"message,omitempty"`
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
}

type ResultTextSearchRequest struct {
	Table string
	Query string
	Limit int
}

type ResultTextSearchHit struct {
	Rank              int     `json:"rank"`
	Score             float64 `json:"score"`
	Source            string  `json:"source"`
	Kind              string  `json:"kind"`
	Table             string  `json:"table"`
	Row               int     `json:"row"`
	Title             string  `json:"title,omitempty"`
	Snippet           string  `json:"snippet,omitempty"`
	MatchScope        string  `json:"match_scope,omitempty"`
	SampleCommand     string  `json:"sample_command,omitempty"`
	TextSearchCommand string  `json:"text_search_command,omitempty"`
	RowTextQuery      string  `json:"-"`
	RowContentQuery   string  `json:"-"`

	bodyTextCoverage     int
	titleTextCoverage    int
	metadataTextCoverage int
	contextTextCoverage  int
	contextAddsTerms     bool
	contextTextOnly      bool
}

type ResultTextSearchResult struct {
	OK                      bool                  `json:"ok"`
	DB                      string                `json:"db"`
	Table                   string                `json:"table"`
	Query                   string                `json:"query"`
	Limit                   int                   `json:"limit"`
	Count                   int                   `json:"count"`
	Truncated               bool                  `json:"truncated"`
	Message                 string                `json:"message,omitempty"`
	SuggestedQueries        []string              `json:"suggested_queries,omitempty"`
	SuggestedSearchCommands []string              `json:"suggested_search_commands,omitempty"`
	Hits                    []ResultTextSearchHit `json:"hits"`
}

type ResultSchemaRequest struct {
	Table string
}

type ResultColumn struct {
	Name       string `json:"name"`
	SQLiteType string `json:"sqlite_type"`
	PrimaryKey bool   `json:"primary_key"`
}

type ResultSchemaResult struct {
	OK          bool           `json:"ok"`
	DB          string         `json:"db"`
	Table       string         `json:"table"`
	Rows        int            `json:"rows"`
	ColumnCount int            `json:"column_count"`
	QueryTable  string         `json:"query_table"`
	Columns     []ResultColumn `json:"columns"`
}

type ListResultTablesRequest struct {
	Limit int
}

type ResultTableRecord struct {
	Source               string          `json:"source,omitempty"`
	Kind                 string          `json:"kind,omitempty"`
	Table                string          `json:"table"`
	URL                  string          `json:"url"`
	App                  string          `json:"app"`
	SID                  string          `json:"sid"`
	Query                string          `json:"query"`
	Earliest             string          `json:"earliest"`
	Latest               string          `json:"latest"`
	ResultCount          int             `json:"result_count"`
	Rows                 int             `json:"rows"`
	Offset               int             `json:"offset"`
	HasMore              bool            `json:"has_more"`
	RunDuration          float64         `json:"run_duration"`
	CreatedAt            int64           `json:"created_at"`
	CreatedAtUTC         string          `json:"created_at_utc"`
	Warnings             []string        `json:"warnings,omitempty"`
	WarningCount         int             `json:"warning_count"`
	AcceptedWarnings     []string        `json:"accepted_warnings,omitempty"`
	AcceptedWarningCount int             `json:"accepted_warning_count"`
	WarningDetails       []ResultWarning `json:"warning_details,omitempty"`
}

const (
	ResultWarningCodeFullFetch = "full_fetch"
	ResultWarningCodeLegacy    = "legacy"
)

type ResultWarning struct {
	Code          string `json:"code"`
	Message       string `json:"message"`
	Accepted      bool   `json:"accepted"`
	AcceptedAt    int64  `json:"accepted_at,omitempty"`
	AcceptedAtUTC string `json:"accepted_at_utc,omitempty"`
}

type ResultInfoRequest struct {
	Table string
}

type ResultInfoResult struct {
	OK bool   `json:"ok"`
	DB string `json:"db"`
	ResultTableRecord
}

type AcceptResultWarningRequest struct {
	Table string
	Code  string
}

type AcceptResultWarningResult struct {
	OK                   bool            `json:"ok"`
	DB                   string          `json:"db"`
	Table                string          `json:"table"`
	Code                 string          `json:"code"`
	Accepted             bool            `json:"accepted"`
	Message              string          `json:"message,omitempty"`
	Warnings             []string        `json:"warnings,omitempty"`
	WarningCount         int             `json:"warning_count"`
	AcceptedWarnings     []string        `json:"accepted_warnings,omitempty"`
	AcceptedWarningCount int             `json:"accepted_warning_count"`
	WarningDetails       []ResultWarning `json:"warning_details,omitempty"`
}

type ResultSummaryRequest struct {
	Table      string
	GroupBy    []string
	Metric     string
	Thresholds []float64
	TimeFrom   string
	TimeTo     string
	ErrorWhere string
	Preset     string
	OrderBy    string
	Order      string
	Limit      int
}

type ResultSummaryResult struct {
	OK        bool             `json:"ok"`
	DB        string           `json:"db"`
	Table     string           `json:"table"`
	Query     string           `json:"query"`
	Limit     int              `json:"limit"`
	Truncated bool             `json:"truncated"`
	Message   string           `json:"message,omitempty"`
	Columns   []string         `json:"columns"`
	Rows      []map[string]any `json:"rows"`
}

type ResultEventsRequest struct {
	Table     string
	RequestID string
	Field     string
	JSONField string
	Value     string
	Limit     int
}

type ResultEventsResult struct {
	OK              bool             `json:"ok"`
	DB              string           `json:"db"`
	Table           string           `json:"table"`
	MatchMode       string           `json:"match_mode"`
	MatchedField    string           `json:"matched_field"`
	MatchedValue    string           `json:"matched_value"`
	MatchExpression string           `json:"match_expression"`
	Limit           int              `json:"limit"`
	Count           int              `json:"count"`
	Truncated       bool             `json:"truncated"`
	Message         string           `json:"message,omitempty"`
	Columns         []string         `json:"columns"`
	Rows            []map[string]any `json:"rows"`
}

type ListResultTablesResult struct {
	OK        bool                `json:"ok"`
	DB        string              `json:"db"`
	Count     int                 `json:"count"`
	Limit     int                 `json:"limit"`
	Truncated bool                `json:"truncated"`
	Message   string              `json:"message,omitempty"`
	Tables    []ResultTableRecord `json:"tables"`
}

type DropResultTablesRequest struct {
	Table string
	All   bool
}

type DropResultTablesResult struct {
	OK             bool     `json:"ok"`
	DB             string   `json:"db"`
	Dropped        []string `json:"dropped"`
	Count          int      `json:"count"`
	Compacted      bool     `json:"compacted"`
	BytesBefore    int64    `json:"bytes_before"`
	BytesAfter     int64    `json:"bytes_after"`
	BytesReclaimed int64    `json:"bytes_reclaimed"`
}
