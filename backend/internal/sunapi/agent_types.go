package sunapi

import "encoding/json"

// Agent 智能体
type Agent struct {
	ID               string `json:"id"`
	UserID           int64  `json:"user_id"`
	Name             string `json:"name"`
	Description      string `json:"description,omitempty"`
	Avatar           string `json:"avatar,omitempty"`
	Status           int    `json:"status"` // 1=active, 2=inactive, 3=archived
	CreatedTime      int64  `json:"created_time"`
	UpdatedTime      int64  `json:"updated_time"`
	LastExecutedTime int64  `json:"last_executed_time,omitempty"`
	BlueprintCount   int    `json:"blueprint_count"`
}

// AgentBlueprint 蓝图/任务
type AgentBlueprint struct {
	ID                  string          `json:"id"`
	AgentID             string          `json:"agent_id"`
	UserID              int64           `json:"user_id"`
	Name                string          `json:"name"`
	Description         string          `json:"description,omitempty"`
	Status              string          `json:"status"` // pending, running, completed, failed, paused
	TriggerConfig       json.RawMessage `json:"trigger"`
	ModelGroup          string          `json:"model_group"`
	ModelName           string          `json:"model_name"`
	Skills              json.RawMessage `json:"skills"` // []string
	InputVariables      json.RawMessage `json:"input_variables,omitempty"`
	ArchiveConfig       json.RawMessage `json:"archive_config,omitempty"`
	CreatedTime         int64           `json:"created_time"`
	UpdatedTime         int64           `json:"updated_time"`
	LastExecutedTime    int64           `json:"last_executed_time,omitempty"`
	NextExecutionTime   int64           `json:"next_execution_time,omitempty"`
	ExecutionCount      int             `json:"execution_count"`
	FailureReason       string          `json:"failure_reason,omitempty"`
	ExecutionDurationMs int64           `json:"execution_duration,omitempty"`
}

// AgentSkill Skill定义
type AgentSkill struct {
	ID          string          `json:"id"`
	UserID      int64           `json:"user_id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Version     string          `json:"version"`
	Source      string          `json:"source"` // builtin, marketplace, github, custom
	SourceURL   string          `json:"source_url,omitempty"`
	Enabled     bool            `json:"enabled"`
	Config      json.RawMessage `json:"config,omitempty"`
	Permissions json.RawMessage `json:"permissions,omitempty"` // []string
	CreatedTime int64           `json:"created_time"`
	UpdatedTime int64           `json:"updated_time"`
}

// AgentExecutionLog 执行日志
type AgentExecutionLog struct {
	ID          string          `json:"id"`
	BlueprintID string          `json:"blueprint_id"`
	AgentID     string          `json:"agent_id"`
	UserID      int64           `json:"user_id"`
	Status      string          `json:"status"`
	StartTime   int64           `json:"start_time"`
	EndTime     int64           `json:"end_time,omitempty"`
	DurationMs  int64           `json:"duration_ms,omitempty"`
	Output      json.RawMessage `json:"output,omitempty"`
	ErrorMsg    string          `json:"error_msg,omitempty"`
	ArchivePath string          `json:"archive_path,omitempty"` // 归档文件路径
}

// AgentFormData 创建/更新智能体的表单数据
type AgentFormData struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Avatar      string `json:"avatar,omitempty"`
}

// BlueprintFormData 创建/更新蓝图的表单数据
type BlueprintFormData struct {
	Name           string          `json:"name"`
	Description    string          `json:"description,omitempty"`
	TriggerConfig  json.RawMessage `json:"trigger"`
	ModelGroup     string          `json:"model_group"`
	ModelName      string          `json:"model_name"`
	Skills         json.RawMessage `json:"skills"`
	InputVariables json.RawMessage `json:"input_variables,omitempty"`
	ArchiveConfig  json.RawMessage `json:"archive_config,omitempty"`
}

// SkillFormData 创建Skill的表单数据
type SkillFormData struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	SourceType  string          `json:"source_type"` // builtin, marketplace, github, custom
	SourceURL   string          `json:"source_url,omitempty"`
	Config      json.RawMessage `json:"config,omitempty"`
}
