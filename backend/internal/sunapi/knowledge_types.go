package sunapi

type KnowledgeBase struct {
	ID            string             `json:"id"`
	UserID        int64              `json:"user_id"`
	Name          string             `json:"name"`
	Description   string             `json:"description,omitempty"`
	Enabled       bool               `json:"enabled"`
	DocumentCount int                `json:"document_count"`
	ChunkCount    int                `json:"chunk_count"`
	Bindings      []KnowledgeBinding `json:"bindings,omitempty"`
	CreatedTime   int64              `json:"created_time"`
	UpdatedTime   int64              `json:"updated_time"`
}

type KnowledgeBaseFormData struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type KnowledgeDocument struct {
	ID              string `json:"id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	UserID          int64  `json:"user_id"`
	Filename        string `json:"filename"`
	MediaType       string `json:"media_type,omitempty"`
	Size            int64  `json:"size"`
	ContentHash     string `json:"content_hash"`
	Content         string `json:"content,omitempty"`
	ChunkCount      int    `json:"chunk_count"`
	CreatedTime     int64  `json:"created_time"`
	UpdatedTime     int64  `json:"updated_time"`
}

type KnowledgeSearchMatch struct {
	KnowledgeBaseID   string  `json:"knowledge_base_id"`
	KnowledgeBaseName string  `json:"knowledge_base_name"`
	DocumentID        string  `json:"document_id"`
	Filename          string  `json:"filename"`
	ChunkIndex        int     `json:"chunk_index"`
	Content           string  `json:"content"`
	Score             float64 `json:"score"`
}

type KnowledgeBinding struct {
	ID              int64  `json:"id"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	UserID          int64  `json:"user_id"`
	TargetType      string `json:"target_type"`
	TargetID        string `json:"target_id"`
	CreatedTime     int64  `json:"created_time"`
}

type KnowledgeBindingFormData struct {
	TargetType string `json:"target_type"`
	TargetID   string `json:"target_id"`
}

type KnowledgeBindingPayload struct {
	Bindings []KnowledgeBindingFormData `json:"bindings"`
}

type KnowledgeSearchScope struct {
	Module           string
	AgentID          string
	BlueprintID      string
	ConversationID   int64
	KnowledgeBaseIDs []string
	Disabled         bool
}
