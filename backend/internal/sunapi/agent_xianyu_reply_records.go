package sunapi

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const agentXianyuReplyRecordDir = "agent-xianyu-reply-records"

var agentXianyuReplyRecordMu sync.Mutex

// AgentXianyuReplyRecord is the operator-facing audit record for one incoming
// buyer message. It intentionally stores the compact message context used by
// the responder instead of the entire page DOM or browser snapshot.
type AgentXianyuReplyRecord struct {
	ID                  string                    `json:"id"`
	BlueprintID         string                    `json:"blueprint_id"`
	AgentID             string                    `json:"agent_id"`
	NodeID              string                    `json:"node_id"`
	ConversationKey     string                    `json:"conversation_key,omitempty"`
	ConversationTitle   string                    `json:"conversation_title,omitempty"`
	ConversationPreview string                    `json:"conversation_preview,omitempty"`
	IncomingMessage     string                    `json:"incoming_message"`
	IncomingDirection   string                    `json:"incoming_direction,omitempty"`
	RecentMessages      []agentXianyuReplyMessage `json:"recent_messages,omitempty"`
	Product             agentXianyuProductContext `json:"product,omitempty"`
	KnowledgeMatchCount int                       `json:"knowledge_match_count"`
	KnowledgeQuery      string                    `json:"knowledge_query,omitempty"`
	ModelDraft          string                    `json:"model_draft,omitempty"`
	FinalReply          string                    `json:"final_reply,omitempty"`
	Status              string                    `json:"status"`
	Fallback            bool                      `json:"fallback,omitempty"`
	Error               string                    `json:"error,omitempty"`
	StartedAt           int64                     `json:"started_at"`
	ModelStartedAt      int64                     `json:"model_started_at,omitempty"`
	ModelCompletedAt    int64                     `json:"model_completed_at,omitempty"`
	SentAt              int64                     `json:"sent_at,omitempty"`
	CompletedAt         int64                     `json:"completed_at"`
	ModelDurationMs     int64                     `json:"model_duration_ms,omitempty"`
	SendDurationMs      int64                     `json:"send_duration_ms,omitempty"`
	DurationMs          int64                     `json:"duration_ms"`
	CreatedAt           int64                     `json:"created_at"`
}

func agentXianyuReplyRecordPath(dataDir, blueprintID string) string {
	return filepath.Join(dataDir, agentXianyuReplyRecordDir, sanitizeAgentBrowserFilenamePart(blueprintID)+".jsonl")
}

func newAgentXianyuReplyRecordID(record AgentXianyuReplyRecord) string {
	seed := fmt.Sprintf("%s\n%s\n%s\n%d\n%d", record.BlueprintID, record.NodeID, record.ConversationKey, record.CreatedAt, time.Now().UnixNano())
	sum := sha256.Sum256([]byte(seed))
	return "reply-" + hex.EncodeToString(sum[:12])
}

// AppendAgentXianyuReplyRecord appends one compact audit record atomically.
// JSONL keeps writes cheap for the long-running page listener and avoids
// adding another database table to the browser automation path.
func (s *Store) AppendAgentXianyuReplyRecord(ctx context.Context, record AgentXianyuReplyRecord) error {
	if s == nil || strings.TrimSpace(s.DataDir()) == "" {
		return errors.New("data directory is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if record.StartedAt <= 0 {
		record.StartedAt = now
	}
	if record.CompletedAt <= 0 {
		record.CompletedAt = now
	}
	if record.CreatedAt <= 0 {
		record.CreatedAt = record.CompletedAt
	}
	if record.DurationMs <= 0 && record.CompletedAt >= record.StartedAt {
		record.DurationMs = record.CompletedAt - record.StartedAt
	}
	if strings.TrimSpace(record.ID) == "" {
		record.ID = newAgentXianyuReplyRecordID(record)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := agentXianyuReplyRecordPath(s.DataDir(), record.BlueprintID)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	agentXianyuReplyRecordMu.Lock()
	defer agentXianyuReplyRecordMu.Unlock()
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.Write(append(raw, '\n')); err != nil {
		return err
	}
	return file.Sync()
}

// ListAgentXianyuReplyRecords returns the newest records first. The scanner
// skips malformed lines so one interrupted append does not hide later audits.
func (s *Store) ListAgentXianyuReplyRecords(ctx context.Context, blueprintID string, limit int) ([]AgentXianyuReplyRecord, error) {
	if s == nil || strings.TrimSpace(s.DataDir()) == "" {
		return nil, errors.New("data directory is not configured")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	path := agentXianyuReplyRecordPath(s.DataDir(), blueprintID)
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return []AgentXianyuReplyRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Keep a bounded slice while scanning; files are append-only and can span
	// many days for a continuously running smart-customer-service blueprint.
	records := make([]AgentXianyuReplyRecord, 0, limit)
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 2*1024*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var record AgentXianyuReplyRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		records = append(records, record)
		if len(records) > limit {
			copy(records, records[len(records)-limit:])
			records = records[:limit]
		}
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	sort.SliceStable(records, func(i, j int) bool {
		if records[i].CreatedAt == records[j].CreatedAt {
			return records[i].ID > records[j].ID
		}
		return records[i].CreatedAt > records[j].CreatedAt
	})
	return records, nil
}
