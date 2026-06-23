package sunapi

import "encoding/json"

type Settings struct {
	SystemName              string  `json:"system_name" yaml:"system_name"`
	ListenHost              string  `json:"listen_host" yaml:"listen_host"`
	ListenPort              int     `json:"listen_port" yaml:"listen_port"`
	DefaultGroup            string  `json:"default_group" yaml:"default_group"`
	DefaultInputPricePer1K  float64 `json:"default_input_price_per_1k" yaml:"default_input_price_per_1k"`
	DefaultOutputPricePer1K float64 `json:"default_output_price_per_1k" yaml:"default_output_price_per_1k"`
	CurrencySymbol          string  `json:"currency_symbol" yaml:"currency_symbol"`
	AutoOpenBrowser         bool    `json:"auto_open_browser" yaml:"auto_open_browser"`
	DefaultStartPage        string  `json:"default_start_page" yaml:"default_start_page"`
	ShowDashboard           bool    `json:"show_dashboard" yaml:"show_dashboard"`
	ShowAPIKeys             bool    `json:"show_api_keys" yaml:"show_api_keys"`
	ShowUsageLogs           bool    `json:"show_usage_logs" yaml:"show_usage_logs"`
	ShowPlayground          bool    `json:"show_playground" yaml:"show_playground"`
}

type Channel struct {
	ID                 int64       `json:"id"`
	Type               int         `json:"type"`
	Name               string      `json:"name"`
	Group              string      `json:"group"`
	BaseURL            string      `json:"base_url"`
	APIKey             string      `json:"api_key,omitempty"`
	Key                string      `json:"key"`
	Models             string      `json:"models"`
	Enabled            bool        `json:"enabled"`
	Status             int         `json:"status"`
	InputPricePer1K    float64     `json:"input_price_per_1k"`
	OutputPricePer1K   float64     `json:"output_price_per_1k"`
	UsedTokens         int64       `json:"used_tokens"`
	RequestCount       int64       `json:"request_count"`
	CreatedAt          int64       `json:"created_at"`
	UpdatedAt          int64       `json:"updated_at"`
	CreatedTime        int64       `json:"created_time"`
	TestTime           int64       `json:"test_time"`
	ResponseTime       int64       `json:"response_time"`
	Balance            float64     `json:"balance"`
	BalanceUpdatedTime int64       `json:"balance_updated_time"`
	UsedQuota          int64       `json:"used_quota"`
	Priority           int         `json:"priority"`
	Weight             int         `json:"weight"`
	TestModel          string      `json:"test_model,omitempty"`
	OpenAIOrganization string      `json:"openai_organization,omitempty"`
	ModelMapping       string      `json:"model_mapping,omitempty"`
	StatusCodeMapping  string      `json:"status_code_mapping,omitempty"`
	AutoBan            int         `json:"auto_ban,omitempty"`
	Tag                string      `json:"tag,omitempty"`
	Setting            string      `json:"setting,omitempty"`
	ParamOverride      string      `json:"param_override,omitempty"`
	HeaderOverride     string      `json:"header_override,omitempty"`
	Other              string      `json:"other"`
	OtherInfo          string      `json:"other_info"`
	Settings           string      `json:"settings"`
	MaxInputTokens     int64       `json:"max_input_tokens"`
	ChannelInfo        ChannelInfo `json:"channel_info"`
	Remark             string      `json:"remark,omitempty"`
}

type ChannelPayload struct {
	Type               int     `json:"type"`
	Name               string  `json:"name"`
	Group              string  `json:"group"`
	BaseURL            string  `json:"base_url"`
	APIKey             string  `json:"api_key"`
	Key                string  `json:"key"`
	Models             string  `json:"models"`
	Enabled            bool    `json:"enabled"`
	Status             int     `json:"status"`
	InputPricePer1K    float64 `json:"input_price_per_1k"`
	OutputPricePer1K   float64 `json:"output_price_per_1k"`
	Priority           int     `json:"priority"`
	Weight             int     `json:"weight"`
	TestModel          string  `json:"test_model"`
	OpenAIOrganization string  `json:"openai_organization"`
	ModelMapping       string  `json:"model_mapping"`
	StatusCodeMapping  string  `json:"status_code_mapping"`
	AutoBan            int     `json:"auto_ban"`
	Tag                string  `json:"tag"`
	Setting            string  `json:"setting"`
	ParamOverride      string  `json:"param_override"`
	HeaderOverride     string  `json:"header_override"`
	Other              string  `json:"other"`
	OtherInfo          string  `json:"other_info"`
	Settings           string  `json:"settings"`
	Remark             string  `json:"remark"`
}

type ChannelInfo struct {
	IsMultiKey             bool              `json:"is_multi_key"`
	MultiKeySize           int               `json:"multi_key_size"`
	MultiKeyStatusList     map[string]int    `json:"multi_key_status_list,omitempty"`
	MultiKeyDisabledReason map[string]string `json:"multi_key_disabled_reason,omitempty"`
	MultiKeyDisabledTime   map[string]int64  `json:"multi_key_disabled_time,omitempty"`
	MultiKeyPollingIndex   int               `json:"multi_key_polling_index"`
	MultiKeyMode           string            `json:"multi_key_mode"`
}

type AddChannelRequest struct {
	Mode                      string         `json:"mode"`
	MultiKeyMode              string         `json:"multi_key_mode"`
	BatchAddSetKeyPrefix2Name bool           `json:"batch_add_set_key_prefix_2_name"`
	Channel                   ChannelPayload `json:"channel"`
}

type ChannelListOptions struct {
	Page      int
	PageSize  int
	Keyword   string
	Model     string
	Group     string
	Status    string
	Type      int
	SortBy    string
	SortOrder string
	IDSort    bool
}

type ChannelMetadataPayload struct {
	Type    int    `json:"type"`
	BaseURL string `json:"base_url"`
	APIKey  string `json:"api_key"`
}

type ChannelMetadata struct {
	Models           []string `json:"models"`
	InputPricePer1K  float64  `json:"input_price_per_1k"`
	OutputPricePer1K float64  `json:"output_price_per_1k"`
	Source           string   `json:"source"`
}

type Group struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	Description     string  `json:"description,omitempty"`
	PriceMultiplier float64 `json:"price_multiplier"`
	Channels        int64   `json:"channels"`
	UsedTokens      int64   `json:"used_tokens"`
	RequestCount    int64   `json:"request_count"`
	Cost            float64 `json:"cost"`
	CreatedAt       int64   `json:"created_at"`
	UpdatedAt       int64   `json:"updated_at"`
}

type GroupPayload struct {
	Name            string  `json:"name"`
	Description     string  `json:"description"`
	PriceMultiplier float64 `json:"price_multiplier"`
}

type APIKey struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Key                string `json:"key"`
	Status             int    `json:"status"`
	RemainQuota        int64  `json:"remain_quota"`
	UsedQuota          int64  `json:"used_quota"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ExpiredTime        int64  `json:"expired_time"`
	CreatedTime        int64  `json:"created_time"`
	AccessedTime       int64  `json:"accessed_time"`
	Group              string `json:"group"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	AllowIPs           string `json:"allow_ips"`
}

type APIKeyPayload struct {
	ID                 int64  `json:"id,omitempty"`
	Name               string `json:"name"`
	Status             int    `json:"status,omitempty"`
	RemainQuota        int64  `json:"remain_quota"`
	ExpiredTime        int64  `json:"expired_time"`
	UnlimitedQuota     bool   `json:"unlimited_quota"`
	ModelLimitsEnabled bool   `json:"model_limits_enabled"`
	ModelLimits        string `json:"model_limits"`
	AllowIPs           string `json:"allow_ips"`
	Group              string `json:"group"`
	CrossGroupRetry    bool   `json:"cross_group_retry"`
}

type PlaygroundSession struct {
	ID           int64           `json:"id"`
	UserID       int64           `json:"user_id"`
	Title        string          `json:"title"`
	Summary      string          `json:"summary"`
	Model        string          `json:"model"`
	Group        string          `json:"group"`
	Pinned       bool            `json:"pinned"`
	Messages     json.RawMessage `json:"messages,omitempty"`
	MessageCount int             `json:"message_count"`
	Config       json.RawMessage `json:"config"`
	CreatedTime  int64           `json:"created_time"`
	UpdatedTime  int64           `json:"updated_time"`
}

type PlaygroundSessionPayload struct {
	ID       int64           `json:"id,omitempty"`
	Title    string          `json:"title"`
	Summary  string          `json:"summary"`
	Model    string          `json:"model"`
	Group    string          `json:"group"`
	Messages json.RawMessage `json:"messages"`
	Config   json.RawMessage `json:"config"`
}

type PlaygroundSessionMetaPayload struct {
	Title  *string `json:"title"`
	Pinned *bool   `json:"pinned"`
}

type PlaygroundAttachment struct {
	ID        string `json:"id"`
	FileID    string `json:"file_id"`
	UserID    int64  `json:"user_id,omitempty"`
	Type      string `json:"type"`
	URL       string `json:"url"`
	MediaType string `json:"media_type"`
	Filename  string `json:"filename"`
	Size      int64  `json:"size"`
	Data      []byte `json:"-"`
	CreatedAt int64  `json:"created_time"`
}

type PlaygroundImageGeneration struct {
	ID              string          `json:"id"`
	UserID          int64           `json:"user_id,omitempty"`
	Prompt          string          `json:"prompt"`
	NegativePrompt  string          `json:"negativePrompt,omitempty"`
	Params          json.RawMessage `json:"params"`
	Model           string          `json:"model"`
	Group           string          `json:"group"`
	ReferenceImages json.RawMessage `json:"referenceImages,omitempty"`
	URLs            []string        `json:"urls"`
	Status          string          `json:"status"`
	ErrorMessage    string          `json:"errorMessage,omitempty"`
	DurationMS      int64           `json:"durationMs,omitempty"`
	CreatedAt       int64           `json:"createdAt"`
	UpdatedAt       int64           `json:"updatedAt,omitempty"`
}

type UsageLog struct {
	ID               int64   `json:"id"`
	CreatedAt        int64   `json:"created_at"`
	ChannelID        int64   `json:"channel_id"`
	ChannelName      string  `json:"channel_name"`
	Username         string  `json:"username,omitempty"`
	Group            string  `json:"group"`
	Model            string  `json:"model"`
	Endpoint         string  `json:"endpoint"`
	PromptTokens     int64   `json:"prompt_tokens"`
	CompletionTokens int64   `json:"completion_tokens"`
	TotalTokens      int64   `json:"total_tokens"`
	Cost             float64 `json:"cost"`
	DurationMS       int64   `json:"duration_ms"`
	StatusCode       int     `json:"status_code"`
	Error            string  `json:"error,omitempty"`
}

type Dashboard struct {
	TotalCost        float64      `json:"total_cost"`
	TotalTokens      int64        `json:"total_tokens"`
	PromptTokens     int64        `json:"prompt_tokens"`
	CompletionTokens int64        `json:"completion_tokens"`
	TotalRequests    int64        `json:"total_requests"`
	Last24hCost      float64      `json:"last_24h_cost"`
	Last24hTokens    int64        `json:"last_24h_tokens"`
	Last24hRequests  int64        `json:"last_24h_requests"`
	EnabledChannels  int64        `json:"enabled_channels"`
	TotalChannels    int64        `json:"total_channels"`
	Groups           int64        `json:"groups"`
	AverageRPM       float64      `json:"average_rpm"`
	AverageTPM       float64      `json:"average_tpm"`
	Chart            []ChartPoint `json:"chart"`
	TopChannels      []TopChannel `json:"top_channels"`
	Settings         Settings     `json:"settings"`
}

type ChartPoint struct {
	Date     string  `json:"date"`
	Cost     float64 `json:"cost"`
	Tokens   int64   `json:"tokens"`
	Requests int64   `json:"requests"`
}

type TopChannel struct {
	ID       int64   `json:"id"`
	Name     string  `json:"name"`
	Cost     float64 `json:"cost"`
	Tokens   int64   `json:"tokens"`
	Requests int64   `json:"requests"`
}

type QuotaData struct {
	ID        int64  `json:"id,omitempty"`
	UserID    int    `json:"user_id,omitempty"`
	Username  string `json:"username,omitempty"`
	ModelName string `json:"model_name,omitempty"`
	Group     string `json:"group,omitempty"`
	CreatedAt int64  `json:"created_at"`
	TokenUsed int64  `json:"token_used,omitempty"`
	Count     int64  `json:"count,omitempty"`
	Quota     int64  `json:"quota,omitempty"`
}

type RepairUnknownQuotaDataGroupsResult struct {
	TotalUnknown int64 `json:"total_unknown"`
	Fixed        int64 `json:"fixed"`
	Skipped      int64 `json:"skipped"`
}

type RecalculateUsageCostsResult struct {
	Total   int64 `json:"total"`
	Updated int64 `json:"updated"`
	Skipped int64 `json:"skipped"`
}

type PerfModelSummary struct {
	ModelName    string  `json:"model_name"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTPS       float64 `json:"avg_tps"`
	RequestCount int64   `json:"request_count,omitempty"`
}

type PerformanceSeriesPoint struct {
	Timestamp    int64   `json:"ts"`
	AvgTTFTMS    float64 `json:"avg_ttft_ms"`
	AvgLatencyMS float64 `json:"avg_latency_ms"`
	SuccessRate  float64 `json:"success_rate"`
	AvgTPS       float64 `json:"avg_tps"`
}

type PerformanceGroup struct {
	Group        string                   `json:"group"`
	AvgTTFTMS    float64                  `json:"avg_ttft_ms"`
	AvgLatencyMS float64                  `json:"avg_latency_ms"`
	SuccessRate  float64                  `json:"success_rate"`
	AvgTPS       float64                  `json:"avg_tps"`
	Series       []PerformanceSeriesPoint `json:"series"`
}

type PerformanceMetrics struct {
	ModelName    string             `json:"model_name"`
	SeriesSchema string             `json:"series_schema,omitempty"`
	Groups       []PerformanceGroup `json:"groups"`
}
