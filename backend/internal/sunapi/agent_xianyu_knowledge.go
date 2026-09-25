package sunapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	agentXianyuDefaultURL     = "https://seller.goofish.com/?site=COMM"
	agentXianyuDefaultSession = "闲鱼客服"
	// "全部" follows the virtual list until its real end. This ceiling only
	// protects a continuously changing list from running forever; it is not a
	// page-size limit and can be raised in the node settings up to this value.
	agentXianyuMaxConversations                 = 5000
	agentXianyuStateDir                         = "agent-xianyu-state"
	agentXianyuDefaultLoginWait                 = 180
	agentXianyuDefaultListWait                  = 8
	agentXianyuDefaultConversationWait          = 1
	agentXianyuDefaultHistoryRounds             = 6
	agentXianyuMaxConversationListScrolls       = 12
	agentXianyuConversationListStallLimit       = 6
	agentXianyuConversationListEndConfirmations = 2
	agentXianyuHistoryStableRounds              = 3
	agentXianyuMaxHistoryRounds                 = 240
	agentXianyuSummaryChunkRunes                = 12000
	agentXianyuHistoryWait                      = 180 * time.Millisecond
	agentXianyuDOMWait                          = 500 * time.Millisecond
	agentXianyuHistoryQuietPeriod               = 900 * time.Millisecond
	// A failed virtual-list fetch must not consume the whole blueprint timeout.
	// The transcript is archived only after it reaches a stable oldest boundary.
	agentXianyuHistoryMaxDuration = 20 * time.Second
	// Long conversations are summarized in bounded chunks and then reduced in
	// bounded batches. This keeps a single customer history from exceeding a
	// model gateway's context window while retaining every extracted message.
	agentXianyuSummaryMergeRunes      = 24000
	agentXianyuSummaryMaxReduceRounds = 12
	// Keep model calls low-concurrency to shorten total archive time without
	// provoking gateway 503s or anti-robot checks. The setting is capped at 2.
	agentXianyuDefaultSummaryConcurrency = 2
	// Model gateways used by this node have returned 502/503/524 responses after
	// roughly two minutes. Bound each summary request so one route cannot make
	// the whole 20-conversation run appear frozen.
	agentXianyuSummaryRequestTimeout = 150 * time.Second
	// Include the summary schema in the conversation hash so existing archived
	// conversations are rebuilt when privacy or style extraction rules change.
	agentXianyuKnowledgeFormatVersion = "public-reply-style-v2"
)

const agentXianyuDefaultSummaryInstruction = `逐条阅读已经完整加载的闲鱼客服历史消息，并结合页面明确展示的当前商品信息，仅依据已提供事实整理知识库。输出必须使用以下固定标题，缺少信息时写“未提及”，禁止猜测：
### 商品/服务信息
### 用户问题/场景
### 用户意图与约束
### 回复口吻与表达习惯
### 推荐客服回复
### 业务规则与例外
### 需要确认或转人工
### 禁止承诺
商品标题、规格、商品描述和页面状态必须与对应会话绑定，不得把其他会话的商品库存或状态混入；商品描述可作为后续客服回答的参考资料，页面状态只代表采集时刻。
“回复口吻与表达习惯”只分析卖家已发送消息的称呼、句长、语气词、分句方式和常用收尾，归纳可复用风格，不复制整段原话；“推荐客服回复”应沿用这种口吻，简短自然，避免通用 AI 客服套话。
删除姓名、电话、订单号、地址、账号等个人信息；删除曝光、浏览、想要、收藏、访客、点击、询单、成交、转化、成本、利润、底价、内部备注等卖家侧数据。不得在任何面向买家的示例中出现这些数据，也不保存原始对话；同一会话分段总结时保持事实一致。`

var agentXianyuSensitiveDigits = regexp.MustCompile(`(?:\d[\s-]?){7,20}`)
var agentXianyuEmail = regexp.MustCompile(`(?i)[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}`)
var agentXianyuLabeledPrivateValue = regexp.MustCompile(`(?i)(姓名|收货人|电话|手机号|联系方式|订单号|订单编号|地址|账号)\s*[:：]\s*[^\n，,；;。]+`)
var agentXianyuPrivateLine = regexp.MustCompile(`(?i)(来源网址|浏览器会话|会话归档标识|本次整理会话数|卖家工作台|内部备注|卖家备注|进货价|成本价|利润|底价|账号余额|提现金额|曝光(?:量|次数|人数|数据|\s*[:：])|浏览(?:量|次数|人数|数据|\s*[:：])|想要(?:数|人数|\s*[:：])|收藏(?:量|数|人数|\s*[:：])|访客(?:量|数|人数|\s*[:：])|点击率|转化率|询单人数|支付人数|支付订单数|成交金额|本周购买|本周消费|均单价|客单价|数据更新)`)
var agentXianyuSellerMetricValue = regexp.MustCompile(`(?i)(曝光|浏览|想要|收藏|访客|点击|询单|成交|转化|成本|利润|底价)[^\n，,。；;]{0,12}\d|\d[^\n，,。；;]{0,12}(曝光|浏览|人?想要|收藏|访客|点击|询单|成交|转化)`)

type agentXianyuConversation struct {
	Key        string                    `json:"-"`
	Title      string                    `json:"title"`
	Preview    string                    `json:"preview,omitempty"`
	Transcript string                    `json:"transcript"`
	URL        string                    `json:"url,omitempty"`
	Product    agentXianyuProductContext `json:"product,omitempty"`
}

type agentXianyuProductContext struct {
	Found        bool   `json:"found"`
	Title        string `json:"title,omitempty"`
	Status       string `json:"status,omitempty"`
	Availability string `json:"availability,omitempty"`
	Description  string `json:"description,omitempty"`
	Text         string `json:"text,omitempty"`
}

type agentXianyuConversationCandidate struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Preview string `json:"preview,omitempty"`
	Unread  bool   `json:"unread,omitempty"`
}

type agentXianyuCandidatePayload struct {
	URL   string                             `json:"url"`
	Title string                             `json:"title"`
	Items []agentXianyuConversationCandidate `json:"items"`
}

type agentXianyuTranscriptPayload struct {
	URL            string                         `json:"url"`
	Title          string                         `json:"title"`
	Text           string                         `json:"text"`
	Messages       []agentXianyuTranscriptMessage `json:"messages,omitempty"`
	MessageCount   int                            `json:"message_count"`
	ContainerFound bool                           `json:"container_found"`
}

type agentXianyuTranscriptMessage struct {
	Key       string `json:"key,omitempty"`
	Text      string `json:"text"`
	Direction string `json:"direction"`
}

func normalizeAgentXianyuMessageDirection(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "seller", "merchant", "shop", "self", "outgoing", "sent", "客服", "卖家", "商家", "我方":
		return "seller"
	case "customer", "buyer", "incoming", "received", "买家", "用户", "对方":
		return "customer"
	default:
		return "unknown"
	}
}

func formatAgentXianyuTranscriptPayload(payload agentXianyuTranscriptPayload) string {
	if len(payload.Messages) == 0 {
		return strings.TrimSpace(payload.Text)
	}
	lines := make([]string, 0, len(payload.Messages))
	for _, message := range payload.Messages {
		text := strings.TrimSpace(message.Text)
		if text == "" {
			continue
		}
		label := "未知发言者"
		switch normalizeAgentXianyuMessageDirection(message.Direction) {
		case "seller":
			label = "卖家"
		case "customer":
			label = "买家"
		}
		lines = append(lines, label+"："+text)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

type agentXianyuScrollPayload struct {
	Height         int  `json:"height"`
	Top            int  `json:"top"`
	ClientHeight   int  `json:"client_height"`
	Moved          bool `json:"moved"`
	AtStart        bool `json:"at_start"`
	AtEnd          bool `json:"at_end"`
	ContainerFound bool `json:"container_found"`
}

type agentXianyuSyncState struct {
	Conversations map[string]string `json:"conversations"`
	UpdatedAt     int64             `json:"updated_at"`
}

type agentXianyuKnowledgeDocument struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	DocumentID      string `json:"document_id,omitempty"`
	Filename        string `json:"filename"`
	ChunkCount      int    `json:"chunk_count,omitempty"`
}

type agentXianyuSummaryResult struct {
	Index              int
	Item               agentXianyuConversation
	Summary            string
	ArchivedIdentities map[string]struct{}
	SkippedCount       int
	Err                error
}

func executeAgentXianyuKnowledgeGraphNode(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	inputVars map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return failAgentXianyuNode(result, "闲鱼客服归档无法执行，数据目录未配置。", "data directory is not configured")
	}
	if bp == nil {
		return failAgentXianyuNode(result, "闲鱼客服归档缺少当前智能体上下文。", "agent blueprint is required")
	}
	bases, baseIDs, err := resolveAgentXianyuKnowledgeBases(ctx, store, bp, inputVars)
	if err != nil {
		return failAgentXianyuNode(result, err.Error(), err.Error())
	}

	// Do not expose raw conversations in execution logs or persisted blueprint output.
	result.Input = map[string]interface{}{
		"knowledge_base_ids": baseIDs,
		"source":             "闲鱼客服后台",
	}

	reportAgentExecutionProgress(ctx, map[string]interface{}{
		"phase":      "collecting",
		"node_id":    node.ID,
		"node_kind":  node.Data.Kind,
		"node_title": node.Data.Title,
		"message":    "正在读取闲鱼会话列表和历史消息，请稍候…",
	})
	conversations, browserSession, sourceURL, err := collectAgentXianyuConversations(ctx, store.DataDir(), node.Data.Settings)
	if err != nil {
		return failAgentXianyuNodeWithOutput(result, "闲鱼客服对话采集失败。", err.Error(), map[string]interface{}{
			"error":           err.Error(),
			"source_url":      sourceURL,
			"browser_session": browserSession,
		})
	}

	statePath := agentXianyuStatePath(store.DataDir(), bp.ID, node.ID)
	state, err := loadAgentXianyuSyncState(statePath)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼客服增量状态读取失败。", err.Error())
	}
	syncAll := strings.Contains(firstNonEmpty(node.Data.Settings["同步范围"], "仅归档未归档或有新消息的会话"), "全部")
	changed, nextHashes := filterAgentXianyuChangedConversations(conversations, state, syncAll)
	unchanged := len(conversations) - len(changed)
	reportAgentExecutionProgress(ctx, map[string]interface{}{
		"phase":      "collected",
		"node_id":    node.ID,
		"node_kind":  node.Data.Kind,
		"node_title": node.Data.Title,
		"message":    fmt.Sprintf("已读取 %d 个会话，其中 %d 个需要总结，%d 个无变化。", len(conversations), len(changed), unchanged),
		"scanned":    len(conversations),
		"total":      len(changed),
		"completed":  0,
		"unchanged":  unchanged,
	})
	if len(changed) == 0 {
		result.Message = fmt.Sprintf("闲鱼客服同步完成，扫描 %d 个会话，暂无新增或更新内容。", len(conversations))
		result.Output = map[string]interface{}{
			"source_url":                   sourceURL,
			"browser_session":              browserSession,
			"scanned_conversation_count":   len(conversations),
			"archived_conversation_count":  0,
			"unchanged_conversation_count": unchanged,
			"knowledge_base_count":         len(bases),
			"sync_mode":                    firstNonEmpty(node.Data.Settings["同步范围"], "仅归档未归档或有新消息的会话"),
		}
		reportAgentExecutionProgress(ctx, map[string]interface{}{
			"phase":      "completed",
			"node_id":    node.ID,
			"node_kind":  node.Data.Kind,
			"node_title": node.Data.Title,
			"message":    result.Message,
			"total":      0,
			"completed":  0,
		})
		return result
	}

	summaryNode := node
	if strings.TrimSpace(summaryNode.Data.Description) == "" {
		summaryNode.Data.Description = agentXianyuDefaultSummaryInstruction
	} else {
		summaryNode.Data.Description = agentXianyuDefaultSummaryInstruction + "\n附加整理要求：" + strings.TrimSpace(summaryNode.Data.Description)
	}
	documents := make([]agentXianyuKnowledgeDocument, 0, len(changed)*len(bases))
	archivedConversations := make([]agentXianyuConversation, 0, len(changed))
	skippedCount := 0
	var firstSummaryError error
	concurrency := parseAgentPositiveSetting(firstNonEmpty(node.Data.Settings["摘要并发数"], fmt.Sprintf("%d", agentXianyuDefaultSummaryConcurrency)), agentXianyuDefaultSummaryConcurrency, agentXianyuDefaultSummaryConcurrency)
	reportAgentExecutionProgress(ctx, map[string]interface{}{
		"phase":       "summarizing",
		"node_id":     node.ID,
		"node_kind":   node.Data.Kind,
		"node_title":  node.Data.Title,
		"message":     fmt.Sprintf("开始总结 %d 个会话（并发 %d），完成后会自动写入知识库。", len(changed), concurrency),
		"total":       len(changed),
		"completed":   0,
		"concurrency": concurrency,
	})

	jobs := make(chan int)
	results := make(chan agentXianyuSummaryResult, len(changed))
	var workers sync.WaitGroup
	for worker := 0; worker < concurrency; worker++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for index := range jobs {
				item := changed[index]
				reportAgentExecutionProgress(ctx, map[string]interface{}{
					"phase":       "summarizing",
					"node_id":     node.ID,
					"node_kind":   node.Data.Kind,
					"node_title":  node.Data.Title,
					"message":     fmt.Sprintf("正在总结第 %d/%d 个会话…", index+1, len(changed)),
					"current":     index + 1,
					"total":       len(changed),
					"completed":   0,
					"concurrency": concurrency,
				})
				summary, archivedIdentities, itemSkipped, summaryErr := summarizeAgentXianyuConversationsWithStats(ctx, store, bp, summaryNode, []agentXianyuConversation{item}, sourceURL)
				results <- agentXianyuSummaryResult{Index: index, Item: item, Summary: summary, ArchivedIdentities: archivedIdentities, SkippedCount: itemSkipped, Err: summaryErr}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for index := range changed {
			select {
			case jobs <- index:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		workers.Wait()
		close(results)
	}()
	summaryResults := make([]agentXianyuSummaryResult, len(changed))
	for summaryResult := range results {
		summaryResults[summaryResult.Index] = summaryResult
	}
	if ctx.Err() != nil {
		return failAgentXianyuNodeWithOutput(result, "闲鱼客服对话总结已中断，已完成内容已保存。", ctx.Err().Error(), map[string]interface{}{
			"error":                       ctx.Err().Error(),
			"source_url":                  sourceURL,
			"browser_session":             browserSession,
			"scanned_conversation_count":  len(conversations),
			"archived_conversation_count": len(archivedConversations),
			"documents":                   documents,
		})
	}
	reportSummaryProgress := func(index int, message string) {
		reportAgentExecutionProgress(ctx, map[string]interface{}{
			"phase":      "persisting",
			"node_id":    node.ID,
			"node_kind":  node.Data.Kind,
			"node_title": node.Data.Title,
			"message":    message,
			"current":    index + 1,
			"total":      len(changed),
			"completed":  index + 1,
			"archived":   len(archivedConversations),
			"documents":  len(documents),
			"skipped":    skippedCount,
		})
	}
	for index, summaryResult := range summaryResults {
		item := summaryResult.Item
		identity := agentXianyuConversationIdentity(item.Key)
		summary := summaryResult.Summary
		archivedIdentities := summaryResult.ArchivedIdentities
		itemSkipped := summaryResult.SkippedCount
		summaryErr := summaryResult.Err
		if summaryErr != nil {
			if firstSummaryError == nil {
				firstSummaryError = summaryErr
			}
			if itemSkipped > 0 {
				skippedCount += itemSkipped
			} else {
				skippedCount++
			}
			log.WithFields(log.Fields{
				"conversation_index": index + 1,
				"error":              summaryErr.Error(),
			}).Warn("agent xianyu left conversation pending after summary failure")
			reportSummaryProgress(index, fmt.Sprintf("第 %d/%d 个会话总结失败，已跳过并保留待重试。", index+1, len(changed)))
			continue
		}
		if _, ok := archivedIdentities[identity]; !ok || strings.TrimSpace(summary) == "" {
			skippedCount++
			reportSummaryProgress(index, fmt.Sprintf("第 %d/%d 个会话没有可归档摘要，已跳过。", index+1, len(changed)))
			continue
		}
		committed, commitErr := commitAgentXianyuConversationKnowledge(
			ctx, store, bases, bp.ID, node.ID, statePath, &state, item,
			nextHashes[identity], sourceURL, browserSession, summary,
		)
		if commitErr != nil {
			return failAgentXianyuNodeWithOutput(result, "闲鱼客服知识增量写入失败，已完成内容已保存。", commitErr.Error(), map[string]interface{}{
				"error":                       commitErr.Error(),
				"source_url":                  sourceURL,
				"browser_session":             browserSession,
				"scanned_conversation_count":  len(conversations),
				"archived_conversation_count": len(archivedConversations),
				"documents":                   documents,
			})
		}
		documents = append(documents, committed...)
		archivedConversations = append(archivedConversations, item)
		reportSummaryProgress(index, fmt.Sprintf("已总结并写入第 %d/%d 个会话。", index+1, len(changed)))
	}
	if len(archivedConversations) == 0 && firstSummaryError != nil {
		return failAgentXianyuNodeWithOutput(result, "闲鱼客服对话总结失败。", firstSummaryError.Error(), map[string]interface{}{
			"error":                      firstSummaryError.Error(),
			"source_url":                 sourceURL,
			"browser_session":            browserSession,
			"scanned_conversation_count": len(conversations),
			"skipped_conversation_count": skippedCount,
		})
	}

	result.Message = fmt.Sprintf("闲鱼客服归档完成，已归档 %d 个新增或更新会话，生成或更新 %d 份知识文档。", len(archivedConversations), len(documents))
	if skippedCount > 0 {
		result.Message = fmt.Sprintf("闲鱼客服归档部分完成，已归档 %d 个会话，%d 个未完成会话保留待重试。", len(archivedConversations), skippedCount)
	}
	result.Output = map[string]interface{}{
		"source_url":                   sourceURL,
		"browser_session":              browserSession,
		"scanned_conversation_count":   len(conversations),
		"archived_conversation_count":  len(archivedConversations),
		"skipped_conversation_count":   skippedCount,
		"unchanged_conversation_count": unchanged,
		"knowledge_base_count":         len(bases),
		"documents":                    documents,
		"sync_mode":                    firstNonEmpty(node.Data.Settings["同步范围"], "仅归档未归档或有新消息的会话"),
		"raw_transcript_stored":        false,
	}
	reportAgentExecutionProgress(ctx, map[string]interface{}{
		"phase":      "completed",
		"node_id":    node.ID,
		"node_kind":  node.Data.Kind,
		"node_title": node.Data.Title,
		"message":    result.Message,
		"total":      len(changed),
		"completed":  len(changed),
		"archived":   len(archivedConversations),
		"documents":  len(documents),
		"skipped":    skippedCount,
	})
	return result
}

func summarizeAgentXianyuConversations(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode, conversations []agentXianyuConversation, sourceURL string) (string, error) {
	summary, _, _, err := summarizeAgentXianyuConversationsWithStats(ctx, store, bp, node, conversations, sourceURL)
	return summary, err
}

func summarizeAgentXianyuConversationsWithStats(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode, conversations []agentXianyuConversation, sourceURL string) (string, map[string]struct{}, int, error) {
	parts := make([]string, 0, len(conversations))
	archivedIdentities := make(map[string]struct{}, len(conversations))
	skippedCount := 0
	var firstLLMError error
	recordLLMError := func(err error) {
		if firstLLMError == nil {
			firstLLMError = err
		}
	}
	for index, item := range conversations {
		conversationName := "会话-" + agentXianyuShortIdentity(agentXianyuConversationIdentity(item.Key))
		chunks := splitAgentXianyuText(redactAgentXianyuText(item.Transcript), agentXianyuSummaryChunkRunes)
		if len(chunks) == 0 {
			archivedIdentities[agentXianyuConversationIdentity(item.Key)] = struct{}{}
			continue
		}
		chunkSummaries := make([]string, 0, len(chunks))
		conversationFailed := false
		for chunkIndex, chunk := range chunks {
			upstream := map[string]interface{}{
				"闲鱼客服对话": []map[string]string{{
					"会话":   conversationName,
					"对话内容": chunk,
				}},
				"当前商品信息": redactAgentXianyuProductContext(item.Product),
				"商品描述":   truncateRunes(strings.TrimSpace(item.Product.Description), 1600),
				"商品状态":   firstNonEmpty(item.Product.Status, item.Product.Availability, "未提及"),
				"来源网址":   sourceURL,
				"会话数量":   1,
				"当前分段":   fmt.Sprintf("%d/%d", chunkIndex+1, len(chunks)),
			}
			chunkSummary, err := callAgentXianyuSummaryLLM(ctx, store, bp, node, upstream)
			if err != nil {
				if ctx.Err() == nil {
					recordLLMError(err)
					skippedCount++
					conversationFailed = true
					log.WithFields(log.Fields{
						"conversation_index": index + 1,
						"chunk_index":        chunkIndex + 1,
						"error":              err.Error(),
					}).Warn("agent xianyu skipped conversation after llm failover exhausted")
					break
				}
				return "", archivedIdentities, skippedCount, fmt.Errorf("会话 %d 第 %d 段总结失败: %w", index+1, chunkIndex+1, err)
			}
			if strings.TrimSpace(chunkSummary) != "" {
				chunkSummaries = append(chunkSummaries, strings.TrimSpace(chunkSummary))
			}
		}
		if conversationFailed {
			continue
		}
		if len(chunkSummaries) == 0 {
			continue
		}
		summary := chunkSummaries[0]
		if len(chunkSummaries) > 1 {
			var err error
			summary, err = mergeAgentXianyuConversationSummaries(ctx, store, bp, node, conversationName, sourceURL, chunkSummaries)
			if err != nil {
				if ctx.Err() == nil {
					recordLLMError(err)
					skippedCount++
					log.WithFields(log.Fields{
						"conversation_index": index + 1,
						"error":              err.Error(),
					}).Warn("agent xianyu skipped conversation synthesis after llm failover exhausted")
					continue
				}
				return "", archivedIdentities, skippedCount, fmt.Errorf("会话 %d 分段摘要合并失败: %w", index+1, err)
			}
		}
		summary = strings.TrimSpace(summary)
		if summary != "" {
			archivedIdentities[agentXianyuConversationIdentity(item.Key)] = struct{}{}
			parts = append(parts, fmt.Sprintf("## %s\n\n%s", conversationName, summary))
		} else {
			skippedCount++
		}
	}
	if len(parts) == 0 {
		if firstLLMError != nil {
			return "", archivedIdentities, skippedCount, fmt.Errorf("闲鱼客服对话总结为空（已跳过 %d 个会话，模型调用失败）: %w", skippedCount, firstLLMError)
		}
		return "", archivedIdentities, skippedCount, errors.New("闲鱼客服对话总结为空")
	}
	return strings.Join(parts, "\n\n"), archivedIdentities, skippedCount, nil
}

func mergeAgentXianyuConversationSummaries(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	conversationName, sourceURL string,
	summaries []string,
) (string, error) {
	parts := normalizeAgentXianyuSummaryParts(summaries)
	for round := 0; len(parts) > 1 && round < agentXianyuSummaryMaxReduceRounds; round++ {
		batches := groupAgentXianyuSummaryParts(parts, agentXianyuSummaryMergeRunes)
		if len(batches) == 0 {
			return "", errors.New("闲鱼客服分段摘要为空")
		}
		next := make([]string, 0, len(batches))
		for batchIndex, batch := range batches {
			synthesisNode := node
			synthesisNode.Data.Description = strings.TrimSpace(firstNonEmpty(node.Data.Description, agentXianyuDefaultSummaryInstruction)) + "\n以下是同一会话不同历史分段的摘要。请合并为一份去重、完整、仅基于事实的会话总结，不要遗漏任何分段中的规则、例外或待确认事项；不要扩写，保持固定标题。"
			synthesisUpstream := map[string]interface{}{
				"闲鱼客服对话": []map[string]string{{
					"会话":   conversationName,
					"对话内容": strings.Join(batch, "\n\n--- 分段摘要 ---\n\n"),
				}},
				"来源网址": sourceURL,
				"会话数量": 1,
				"归并轮次": fmt.Sprintf("%d/%d", round+1, agentXianyuSummaryMaxReduceRounds),
				"归并批次": fmt.Sprintf("%d/%d", batchIndex+1, len(batches)),
			}
			merged, err := callAgentXianyuSummaryLLM(ctx, store, bp, synthesisNode, synthesisUpstream)
			if err != nil {
				return "", err
			}
			merged = strings.TrimSpace(merged)
			if merged == "" {
				return "", errors.New("闲鱼客服分段摘要合并为空")
			}
			next = append(next, merged)
		}
		parts = normalizeAgentXianyuSummaryParts(next)
	}
	if len(parts) != 1 {
		return "", fmt.Errorf("闲鱼客服分段摘要在 %d 轮内未能收敛", agentXianyuSummaryMaxReduceRounds)
	}
	return parts[0], nil
}

func normalizeAgentXianyuSummaryParts(values []string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		parts = append(parts, splitAgentXianyuText(value, agentXianyuSummaryChunkRunes)...)
	}
	return parts
}

func groupAgentXianyuSummaryParts(parts []string, maxRunes int) [][]string {
	if maxRunes <= 0 {
		maxRunes = agentXianyuSummaryMergeRunes
	}
	groups := make([][]string, 0, len(parts))
	current := make([]string, 0, 2)
	currentRunes := 0
	for _, raw := range parts {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		partRunes := len([]rune(part))
		if len(current) > 0 && currentRunes+partRunes > maxRunes {
			groups = append(groups, current)
			current = make([]string, 0, 2)
			currentRunes = 0
		}
		current = append(current, part)
		currentRunes += partRunes
	}
	if len(current) > 0 {
		groups = append(groups, current)
	}
	return groups
}

func callAgentXianyuSummaryLLM(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode, upstream map[string]interface{}) (string, error) {
	summaryCtx, cancel := context.WithTimeout(ctx, agentXianyuSummaryRequestTimeout)
	defer cancel()
	return callLLMForAgentNodeWithPolicy(summaryCtx, store, bp, node, upstream, map[string]interface{}{"knowledge_disabled": true}, agentXianyuSummaryRequestTimeout, 2)
}

func splitAgentXianyuText(value string, maxRunes int) []string {
	value = strings.TrimSpace(value)
	if value == "" || maxRunes <= 0 {
		return nil
	}
	runes := []rune(value)
	chunks := make([]string, 0, (len(runes)+maxRunes-1)/maxRunes)
	for start := 0; start < len(runes); {
		end := start + maxRunes
		if end > len(runes) {
			end = len(runes)
		}
		if end < len(runes) {
			for split := end; split > start+maxRunes/2; split-- {
				if runes[split-1] == '\n' {
					end = split
					break
				}
			}
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		start = end
	}
	return chunks
}

func failAgentXianyuNode(result agentBlueprintNodeResult, message, errText string) agentBlueprintNodeResult {
	return failAgentXianyuNodeWithOutput(result, message, errText, map[string]interface{}{"error": errText})
}

func failAgentXianyuNodeWithOutput(result agentBlueprintNodeResult, message, errText string, output map[string]interface{}) agentBlueprintNodeResult {
	result.Status = "failed"
	result.Message = message
	result.Error = errText
	result.Output = output
	return result
}

func resolveAgentXianyuKnowledgeBases(ctx context.Context, store *Store, bp *AgentBlueprint, inputVars map[string]interface{}) ([]*KnowledgeBase, []string, error) {
	scope := applyKnowledgeInputScope(KnowledgeSearchScope{Module: "agent", AgentID: bp.AgentID, BlueprintID: bp.ID}, inputVars)
	if scope.Disabled {
		return nil, nil, errors.New("请先在当前智能体中选择并开启至少一个知识库")
	}
	ids := normalizeKnowledgeBaseIDs(scope.KnowledgeBaseIDs)
	if len(ids) == 0 {
		return nil, nil, errors.New("请先在当前智能体中选择并开启至少一个知识库")
	}
	bases := make([]*KnowledgeBase, 0, len(ids))
	for _, id := range ids {
		base, err := store.GetKnowledgeBase(ctx, id, bp.UserID)
		if err != nil {
			return nil, nil, err
		}
		if base == nil {
			return nil, nil, fmt.Errorf("知识库不存在或不属于当前用户：%s", id)
		}
		bases = append(bases, base)
	}
	return bases, ids, nil
}

func collectAgentXianyuConversations(ctx context.Context, dataDir string, settings map[string]string) ([]agentXianyuConversation, string, string, error) {
	targetURL, err := normalizeAgentXianyuURL(firstNonEmpty(settings["网址"], settings["URL"], agentXianyuDefaultURL))
	if err != nil {
		return nil, "", "", err
	}
	session := firstNonEmpty(settings["会话名称"], agentXianyuDefaultSession)
	paths := resolveAgentBrowserSessionPaths(dataDir, session)
	endpoint, err := ensureAgentBrowserDevtoolsForSession(ctx, paths)
	if err != nil {
		return nil, paths.Name, targetURL, err
	}
	target, _, err := openAgentBrowserTarget(ctx, endpoint, targetURL)
	if err != nil {
		return nil, paths.Name, targetURL, err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, paths.Name, targetURL, fmt.Errorf("连接浏览器调试通道失败: %w", err)
	}
	defer conn.Close()
	if _, err := agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return nil, paths.Name, targetURL, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return nil, paths.Name, targetURL, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return nil, paths.Name, targetURL, err
	}
	loginWait := parseAgentPositiveSetting(firstNonEmpty(settings["登录等待"], "180s"), agentXianyuDefaultLoginWait, 300)
	if loginWait > 0 && !strings.Contains(settings["登录态"], "无需登录") {
		if err := waitAgentBrowserLogin(ctx, conn, loginWait); err != nil {
			return nil, paths.Name, targetURL, err
		}
	}
	if err := waitAgentXianyu(ctx, settings["列表等待"], agentXianyuDefaultListWait); err != nil {
		return nil, paths.Name, targetURL, err
	}
	listSelector := firstNonEmpty(settings["对话列表选择器"], settings["会话列表选择器"])
	messageSelector := settings["消息区域选择器"]
	productSelector := settings["商品详情选择器"]
	// Reading and scrolling do not necessarily target the same DOM node on a
	// virtualized chat page. Keep them independently configurable and fall back
	// to the content selector for backwards compatibility.
	messageScrollSelector := firstNonEmpty(settings["消息滚动容器选择器"], messageSelector)
	reset, err := evaluateAgentXianyuConversationListReset(ctx, conn, 4, listSelector)
	if err != nil {
		return nil, paths.Name, targetURL, err
	}
	if reset.Moved {
		if err := waitAgentXianyuDOM(ctx); err != nil {
			return nil, paths.Name, targetURL, err
		}
	}
	conversationCeiling := parseAgentPositiveSetting(settings["最大会话保护上限"], agentXianyuMaxConversations, agentXianyuMaxConversations)
	candidates, err := evaluateAgentXianyuCandidates(ctx, conn, 10, listSelector, conversationCeiling)
	if err != nil {
		return nil, paths.Name, targetURL, err
	}
	if len(candidates.Items) == 0 {
		return nil, paths.Name, targetURL, errors.New("未找到闲鱼客服会话，请确认已登录并打开客服消息列表；必要时填写对话列表选择器")
	}

	limit, allConversations := parseAgentXianyuConversationLimitWithCeiling(settings["会话数量"], conversationCeiling)
	conversationWait := parseAgentPositiveSetting(firstNonEmpty(settings["单会话等待"], "1s"), agentXianyuDefaultConversationWait, 10)
	historyRounds := parseAgentPositiveSetting(firstNonEmpty(settings["历史加载轮数"], "6"), agentXianyuDefaultHistoryRounds, 12)
	log.WithFields(log.Fields{
		"conversation_limit":     limit,
		"conversation_limit_all": allConversations,
		"history_stable_rounds":  historyRounds,
		"history_max_rounds":     agentXianyuMaxHistoryRounds,
		"conversation_ceiling":   conversationCeiling,
	}).Debug("agent xianyu collection started")
	// Keep a stable key queue, but fill it lazily. Opening the first visible
	// conversation must not wait for the whole virtual list to be scanned.
	queueCapacity := len(candidates.Items)
	if !allConversations {
		queueCapacity = min(limit, queueCapacity)
	}
	queue := make([]agentXianyuConversationCandidate, 0, queueCapacity)
	queueSeen := make(map[string]struct{}, queueCapacity)
	appendCandidates := func(items []agentXianyuConversationCandidate) {
		for _, candidate := range items {
			key := agentXianyuCandidateKey(candidate)
			if key == "" {
				continue
			}
			candidate.Key = key
			if _, seen := queueSeen[key]; seen {
				continue
			}
			queueSeen[key] = struct{}{}
			queue = append(queue, candidate)
			if len(queue) >= limit {
				return
			}
		}
	}
	appendCandidates(candidates.Items)
	if len(queue) == 0 {
		return nil, paths.Name, targetURL, errors.New("已打开闲鱼客服列表，但没有读取到对话内容；请确认会话列表选择器和消息区域选择器")
	}
	conversations := make([]agentXianyuConversation, 0, len(queue))
	queueIndex := 0
	listTop := reset.Top
	// Do not infer the end from the initial viewport height. A virtualized list
	// can report no overflow until its first scroll/load event completes.
	listExhausted := false
	listNoProgress := 0
	listEndStreak := 0
	restoreList := func(baseID int) error {
		position, err := evaluateAgentXianyuConversationListRestore(ctx, conn, baseID, listSelector, listTop)
		if err != nil {
			return err
		}
		if position.Height > 0 {
			if position.Top > listTop {
				listTop = position.Top
				listNoProgress = 0
				listEndStreak = 0
			}
		}
		return nil
	}
	advanceList := func(baseID int) error {
		if listExhausted || len(queue) >= limit {
			return nil
		}
		var lastErr error
		if err := restoreList(baseID); err != nil {
			return err
		}
		// Read the current viewport before advancing. The queue is de-duplicated
		// by stable conversation key, so this is cheap and prevents a virtualized
		// list from being treated as a page counter.
		maxAdvanceRounds := agentXianyuMaxConversationListScrolls
		for round := 0; round < maxAdvanceRounds; round++ {
			before := len(queue)
			batch, err := evaluateAgentXianyuCandidates(ctx, conn, baseID+round*3, listSelector, conversationCeiling)
			if err != nil {
				lastErr = err
				if err := waitAgentXianyuDOM(ctx); err != nil {
					return err
				}
				continue
			}
			appendCandidates(batch.Items)
			if len(queue) > before {
				listNoProgress = 0
				listEndStreak = 0
				return nil
			}
			listScroll, err := evaluateAgentXianyuConversationListScroll(ctx, conn, baseID+1+round*3, listSelector)
			if err != nil {
				lastErr = err
				if err := waitAgentXianyuDOM(ctx); err != nil {
					return err
				}
				continue
			}
			if !listScroll.ContainerFound {
				listEndStreak = 0
				listNoProgress++
				if listNoProgress >= agentXianyuConversationListStallLimit {
					return errors.New("闲鱼客服会话列表容器暂时不可见，连续多次重试仍未恢复")
				}
				if err := waitAgentXianyuDOM(ctx); err != nil {
					return err
				}
				continue
			}
			if listScroll.Height <= 0 {
				listEndStreak++
				if listEndStreak >= agentXianyuConversationListEndConfirmations {
					listExhausted = true
					return nil
				}
				if err := waitAgentXianyuDOM(ctx); err != nil {
					return err
				}
				continue
			}
			if listScroll.Top > listTop {
				listTop = listScroll.Top
				listNoProgress = 0
				listEndStreak = 0
			} else if listScroll.Top < listTop {
				// The page remounted the list at an older offset. Keep the cursor
				// monotonic and restore it on the next iteration instead of treating
				// the backward jump as a new page.
				listNoProgress++
				if listNoProgress >= agentXianyuConversationListStallLimit {
					return fmt.Errorf("闲鱼客服会话列表发生回退（scrollTop=%d, savedTop=%d），连续多次恢复失败", listScroll.Top, listTop)
				}
			} else if listScroll.AtEnd {
				listNoProgress = 0
				listEndStreak++
				if listEndStreak >= agentXianyuConversationListEndConfirmations {
					listExhausted = true
					return nil
				}
			} else {
				listEndStreak = 0
				listNoProgress++
				if listNoProgress >= agentXianyuConversationListStallLimit {
					return fmt.Errorf("闲鱼客服会话列表连续 %d 次未向下移动，且未到末尾（scrollTop=%d, height=%d, clientHeight=%d）", listNoProgress, listScroll.Top, listScroll.Height, listScroll.ClientHeight)
				}
			}
			if err := waitAgentXianyuDOM(ctx); err != nil {
				return err
			}
		}
		if lastErr != nil {
			return lastErr
		}
		return nil
	}
	clickQueuedConversation := func(candidate agentXianyuConversationCandidate, baseID int) (bool, error) {
		var lastCandidateErr error
		tryClick := func(id int) (bool, error) {
			batch, err := evaluateAgentXianyuCandidates(ctx, conn, id, listSelector, conversationCeiling)
			if err != nil {
				lastCandidateErr = err
				return false, nil
			}
			if !agentXianyuCandidateBatchContains(batch.Items, candidate) {
				return false, nil
			}
			if err := clickAgentXianyuConversation(ctx, conn, id+1, listSelector, candidate); err != nil {
				return false, err
			}
			return true, nil
		}

		// Fast path: sequential rows normally remain in the current viewport.
		found, err := tryClick(baseID)
		if err != nil || found {
			return found, err
		}
		if err := restoreList(baseID + 2); err != nil {
			return false, err
		}

		// A newly arrived inquiry can push a queued row below the saved anchor.
		// Search only forward from that anchor; never reset the list or seek by
		// an absolute page number. The bounded scan prevents a removed row from
		// blocking the rest of the queue indefinitely.
		seekNoProgress := 0
		seekEndStreak := 0
		for seekRound := 0; seekRound < agentXianyuMaxConversationListScrolls; seekRound++ {
			found, err := tryClick(baseID + 10 + seekRound*4)
			if err != nil || found {
				return found, err
			}
			position, err := evaluateAgentXianyuConversationListScroll(ctx, conn, baseID+11+seekRound*4, listSelector)
			if err != nil {
				return false, err
			}
			if !position.ContainerFound {
				seekNoProgress++
				if seekNoProgress >= agentXianyuConversationListStallLimit {
					return false, errors.New("闲鱼客服会话列表容器暂时不可见，连续多次定位仍未恢复")
				}
				if err := waitAgentXianyuDOM(ctx); err != nil {
					return false, err
				}
				continue
			}
			if position.Height <= 0 {
				return false, nil
			}
			if position.AtEnd {
				seekEndStreak++
				if seekEndStreak >= agentXianyuConversationListEndConfirmations {
					return false, nil
				}
				if err := waitAgentXianyuDOM(ctx); err != nil {
					return false, err
				}
				continue
			}
			seekEndStreak = 0
			if position.Top > listTop {
				listTop = position.Top
				listNoProgress = 0
				seekNoProgress = 0
			} else if !position.Moved {
				seekNoProgress++
				if seekNoProgress >= agentXianyuConversationListStallLimit {
					return false, fmt.Errorf("闲鱼客服会话列表定位会话时连续 %d 次未向下移动，且未到末尾（scrollTop=%d, height=%d, clientHeight=%d）", seekNoProgress, position.Top, position.Height, position.ClientHeight)
				}
			}
			if err := waitAgentXianyuDOM(ctx); err != nil {
				return false, err
			}
		}
		return false, lastCandidateErr
	}
	for allConversations || queueIndex < limit {
		if queueIndex >= len(queue) {
			if err := advanceList(60 + queueIndex*10); err != nil {
				return nil, paths.Name, targetURL, err
			}
			if queueIndex >= len(queue) {
				break
			}
			continue
		}

		candidate := queue[queueIndex]
		queueIndex++
		found, err := clickQueuedConversation(candidate, 100+queueIndex*20)
		if err != nil {
			return nil, paths.Name, targetURL, err
		}
		if !found {
			// A new inquiry can move or remove a queued row. Skip only that row;
			// preserve listTop and continue with the next queued conversation.
			log.WithFields(log.Fields{
				"conversation_index": queueIndex,
				"list_top":           listTop,
				"conversation_key":   candidate.Key,
			}).Warn("agent xianyu skipped unavailable conversation")
			continue
		}
		if err := waitAgentXianyu(ctx, settings["单会话等待"], conversationWait); err != nil {
			return nil, paths.Name, targetURL, err
		}

		// Always start at the oldest visible message and wait for a stable oldest
		// boundary before using the transcript. One bounded read is sufficient:
		// retrying a stalled virtual list doubled the worst-case work per row and
		// could exhaust the blueprint's global timeout.
		text, conversationURL, historyErr := readAgentXianyuConversationHistory(ctx, conn, messageSelector, messageScrollSelector, historyRounds, 600+queueIndex*100)
		if historyErr != nil && ctx.Err() != nil {
			return nil, paths.Name, targetURL, fmt.Errorf("第 %d 个会话历史读取失败: %w", queueIndex, historyErr)
		}
		if historyErr != nil {
			// Never archive a partial transcript. A single malformed or stale row
			// should not prevent the cursor from continuing with later conversations.
			log.WithFields(log.Fields{
				"conversation_index": queueIndex,
				"error":              historyErr.Error(),
			}).Warn("agent xianyu skipped incomplete conversation history")
			continue
		}
		if text == "" {
			return nil, paths.Name, targetURL, fmt.Errorf("第 %d 个会话历史读取为空", queueIndex)
		}
		product, productErr := evaluateAgentXianyuProductContext(ctx, conn, 1000000+queueIndex, productSelector)
		if productErr != nil {
			log.WithFields(log.Fields{
				"conversation_index": queueIndex,
				"error":              productErr.Error(),
			}).Warn("agent xianyu could not read current product context")
		}
		log.WithFields(log.Fields{
			"conversation_index": queueIndex,
			"conversation_count": len(conversations) + 1,
			"transcript_runes":   len([]rune(text)),
			"product_found":      product.Found,
		}).Debug("agent xianyu conversation history read completely")
		conversations = append(conversations, agentXianyuConversation{
			Key:        candidate.Key,
			Title:      firstNonEmpty(strings.TrimSpace(candidate.Title), fmt.Sprintf("会话 %d", len(conversations)+1)),
			Preview:    truncateRunes(strings.TrimSpace(candidate.Preview), 240),
			Transcript: text,
			URL:        conversationURL,
			Product:    product,
		})
	}
	if len(conversations) == 0 {
		return nil, paths.Name, targetURL, errors.New("已打开闲鱼客服列表，但没有读取到对话内容；请确认会话列表选择器和消息区域选择器")
	}
	return conversations, paths.Name, targetURL, nil
}

func waitAgentXianyu(ctx context.Context, raw string, fallback int) error {
	seconds := parseAgentPositiveSetting(raw, fallback, 60)
	timer := time.NewTimer(time.Duration(seconds) * time.Second)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// parseAgentXianyuConversationLimit accepts an explicit numeric limit while
// allowing the node setting to request every conversation currently exposed by
// the seller console. The numeric ceiling is a safety guard for a list that is
// continuously receiving new inquiries; normal runs stop earlier at the real
// end of the virtual list.
func parseAgentXianyuConversationLimit(value string) (limit int, all bool) {
	return parseAgentXianyuConversationLimitWithCeiling(value, agentXianyuMaxConversations)
}

func parseAgentXianyuConversationLimitWithCeiling(value string, ceiling int) (limit int, all bool) {
	if ceiling <= 0 {
		ceiling = agentXianyuMaxConversations
	}
	raw := strings.ToLower(strings.TrimSpace(value))
	switch raw {
	case "", "0", "all", "unlimited", "全部", "不限", "所有", "全部会话", "所有会话":
		return ceiling, true
	default:
		return parseAgentPositiveSetting(raw, 20, ceiling), false
	}
}

func waitAgentXianyuHistory(ctx context.Context) error {
	return waitAgentXianyuDuration(ctx, agentXianyuHistoryWait)
}

func waitAgentXianyuDOM(ctx context.Context) error {
	return waitAgentXianyuDuration(ctx, agentXianyuDOMWait)
}

func waitAgentXianyuDuration(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func normalizeAgentXianyuURL(raw string) (string, error) {
	value, err := normalizeAgentBrowserURL(raw)
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	if host != "goofish.com" && !strings.HasSuffix(host, ".goofish.com") && host != "xianyu.com" && !strings.HasSuffix(host, ".xianyu.com") {
		return "", errors.New("闲鱼客服归档仅允许 seller.goofish.com 或 xianyu.com 域名")
	}
	return value, nil
}

func evaluateAgentXianyuCandidates(ctx context.Context, conn *websocket.Conn, id int, selector string, limit int) (agentXianyuCandidatePayload, error) {
	expression := buildAgentXianyuCandidatesExpression(selector, limit)
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuCandidatePayload{})
}

func evaluateAgentXianyuUnreadCandidates(ctx context.Context, conn *websocket.Conn, id int, selector string, limit int) (agentXianyuCandidatePayload, error) {
	expression := buildAgentXianyuCandidatesExpressionWithUnread(selector, limit, true)
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuCandidatePayload{})
}

func agentXianyuCandidateKey(candidate agentXianyuConversationCandidate) string {
	key := strings.TrimSpace(candidate.Key)
	if key != "" {
		return key
	}
	return strings.TrimSpace(candidate.Title)
}

func agentXianyuCandidateBatchContains(items []agentXianyuConversationCandidate, target agentXianyuConversationCandidate) bool {
	targetKey := agentXianyuCandidateKey(target)
	targetTitle := strings.TrimSpace(target.Title)
	for _, item := range items {
		if targetKey != "" && agentXianyuCandidateKey(item) == targetKey {
			return true
		}
		if targetTitle != "" && strings.TrimSpace(item.Title) == targetTitle {
			return true
		}
	}
	return false
}

func clickAgentXianyuConversation(ctx context.Context, conn *websocket.Conn, id int, selector string, candidate agentXianyuConversationCandidate) error {
	expression := buildAgentXianyuClickExpression(selector, candidate)
	clicked, err := evaluateAgentXianyuJSON(ctx, conn, id, expression, false)
	if err != nil {
		return err
	}
	if !clicked {
		return errors.New("闲鱼客服会话不可点击")
	}
	return nil
}

func evaluateAgentXianyuTranscript(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuTranscriptPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuTranscriptExpression(selector), agentXianyuTranscriptPayload{})
}

func evaluateAgentXianyuProductContext(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuProductContext, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuProductContextExpression(selector), agentXianyuProductContext{})
}

func evaluateAgentXianyuScroll(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuScrollPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuScrollExpression(selector), agentXianyuScrollPayload{})
}

func evaluateAgentXianyuScrollToLatest(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuScrollPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuScrollToLatestExpression(selector), agentXianyuScrollPayload{})
}

func evaluateAgentXianyuConversationListScroll(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuScrollPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuConversationListScrollExpression(selector), agentXianyuScrollPayload{})
}

func evaluateAgentXianyuConversationListReset(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuScrollPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuConversationListResetExpression(selector), agentXianyuScrollPayload{})
}

func evaluateAgentXianyuConversationListRestore(ctx context.Context, conn *websocket.Conn, id int, selector string, top int) (agentXianyuScrollPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuConversationListRestoreExpression(selector, top), agentXianyuScrollPayload{})
}

// readAgentXianyuConversationHistory reads one opened conversation to a stable
// oldest boundary before allowing the caller to archive it. Xianyu commonly
// prepends older messages asynchronously when the message pane reaches the
// top, so a fixed number of scrolls can return a partial transcript.
func readAgentXianyuConversationHistory(ctx context.Context, conn *websocket.Conn, selector, scrollSelector string, configuredStableRounds, baseID int) (string, string, error) {
	if strings.TrimSpace(scrollSelector) == "" {
		scrollSelector = selector
	}
	stableRounds := configuredStableRounds
	if stableRounds < agentXianyuHistoryStableRounds {
		stableRounds = agentXianyuHistoryStableRounds
	}
	if stableRounds > agentXianyuMaxHistoryRounds {
		stableRounds = agentXianyuMaxHistoryRounds
	}

	snapshots := make([]string, 0, stableRounds+1)
	previousText := ""
	previousHeight := -1
	stableCount := 0
	stableSince := time.Time{}
	completed := false
	conversationURL := ""
	startedAt := time.Now()
	for round := 0; round < agentXianyuMaxHistoryRounds; round++ {
		if time.Since(startedAt) >= agentXianyuHistoryMaxDuration {
			return "", conversationURL, errors.New("闲鱼客服历史消息加载超时")
		}
		scroll, err := evaluateAgentXianyuScroll(ctx, conn, baseID+round*3, scrollSelector)
		if err != nil {
			return "", conversationURL, err
		}
		payload, err := evaluateAgentXianyuTranscript(ctx, conn, baseID+1+round*3, selector)
		if err != nil {
			return "", conversationURL, err
		}
		if strings.TrimSpace(payload.URL) != "" {
			conversationURL = strings.TrimSpace(payload.URL)
		}
		text := formatAgentXianyuTranscriptPayload(payload)
		if !payload.ContainerFound || text == "" {
			return "", conversationURL, errors.New("闲鱼客服消息区域未找到有效内容")
		}
		snapshots = append(snapshots, text)
		// A non-scrollable message pane already contains the complete history.
		// Return immediately instead of issuing unnecessary top/bottom scrolls.
		if round == 0 && scroll.Height <= scroll.ClientHeight+16 {
			return text, conversationURL, nil
		}

		atStart := scroll.AtStart || scroll.Top <= 12
		if atStart && scroll.Height == previousHeight && text == previousText {
			if stableSince.IsZero() {
				stableSince = time.Now()
			}
			stableCount++
		} else {
			stableCount = 0
			stableSince = time.Time{}
		}
		previousHeight = scroll.Height
		previousText = text
		if stableCount >= stableRounds && !stableSince.IsZero() && time.Since(stableSince) >= agentXianyuHistoryQuietPeriod {
			completed = true
			break
		}
		if err := waitAgentXianyuHistory(ctx); err != nil {
			return "", conversationURL, err
		}
	}
	if !completed {
		return "", conversationURL, errors.New("闲鱼客服历史消息未达到稳定状态")
	}

	// Restore the newest end once, then merge it with the progressively growing
	// oldest snapshots. This preserves both ends when the UI virtualizes rows.
	if _, err := evaluateAgentXianyuScrollToLatest(ctx, conn, baseID+agentXianyuMaxHistoryRounds*3+1, scrollSelector); err != nil {
		return "", conversationURL, err
	}
	if err := waitAgentXianyuHistory(ctx); err != nil {
		return "", conversationURL, err
	}
	latest, err := evaluateAgentXianyuTranscript(ctx, conn, baseID+agentXianyuMaxHistoryRounds*3+2, selector)
	if err != nil {
		return "", conversationURL, err
	}
	if strings.TrimSpace(latest.URL) != "" {
		conversationURL = strings.TrimSpace(latest.URL)
	}
	latestText := formatAgentXianyuTranscriptPayload(latest)
	if latest.ContainerFound && latestText != "" {
		snapshots = append(snapshots, latestText)
	}
	text := mergeAgentXianyuTranscriptSnapshots(snapshots)
	if text == "" {
		return "", conversationURL, errors.New("闲鱼客服对话内容为空")
	}
	return text, conversationURL, nil
}

func evaluateAgentXianyuJSON[T any](ctx context.Context, conn *websocket.Conn, id int, expression string, target T) (T, error) {
	raw, err := agentBrowserCDPCall(ctx, conn, id, "Runtime.evaluate", map[string]any{
		"expression":    expression,
		"returnByValue": true,
	})
	if err != nil {
		return target, err
	}
	var response struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
		ExceptionDetails json.RawMessage `json:"exceptionDetails"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return target, err
	}
	if len(response.ExceptionDetails) > 0 {
		return target, errors.New("浏览器执行闲鱼页面脚本失败")
	}
	if strings.TrimSpace(response.Result.Value) == "" {
		return target, errors.New("浏览器未返回闲鱼页面数据")
	}
	if err := json.Unmarshal([]byte(response.Result.Value), &target); err != nil {
		return target, err
	}
	return target, nil
}

func buildAgentXianyuCandidatesExpression(selector string, limit int) string {
	return buildAgentXianyuCandidatesExpressionWithUnread(selector, limit, false)
}

func buildAgentXianyuCandidatesExpressionWithUnread(selector string, limit int, unreadOnly bool) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s;
 const max = %d;
 const unreadOnly = %t;
 const clean = value => String(value || '').replace(/\s+/g, ' ').trim();
 const linesOf = value => String(value || '').replace(/\r/g, '').split(/\n/).map(line => line.replace(/[ \t]+/g, ' ').trim()).filter(Boolean);
	const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 80 && r.height > 24 && r.width < innerWidth * .82 && (custom || r.right < innerWidth * .55) && r.left < innerWidth * .78 && r.bottom > 0 && r.top < innerHeight && s.display !== 'none' && s.visibility !== 'hidden'; };
	const scope = custom ? (() => { try { return document.querySelector(custom) || document; } catch (_) { return document; } })() : document;
	// The configured selector names the conversation-list container. Keep row
	// discovery inside that container; also include the container itself so a
	// site-specific selector that points directly at a row remains compatible.
	const selectors = ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[class*="list-item"]','[class*="listItem"]','[class*="message-item"]','[class*="item-row"]','[class*="itemRow"]','[role="listitem"]','[role="option"]'];
	let nodes = [];
	for (const query of selectors) { try { nodes.push(...scope.querySelectorAll(query)); } catch (_) {} }
	try { nodes.push(...scope.querySelectorAll('li,button,[role="listitem"],[role="option"],[data-id],[class*="item"],[class*="Item"]')); } catch (_) {}
	if (custom && scope !== document) nodes.push(scope);
	const badgeNodes = [...scope.querySelectorAll('span,div,i,b,em,label,strong,small,mark,[class*="badge"],[class*="Badge"],[class*="count"],[class*="Count"],[class*="dot"],[class*="Dot"],[class*="unread"],[class*="Unread"]')];
	 const isUnreadBadge = item => {
		 const rect=item.getBoundingClientRect(),value=clean(item.textContent);
		 for(const pseudo of ['', '::before', '::after']){
		  const style=getComputedStyle(item,pseudo),content=String(style.content||'').replace(/[\"']/g,'').trim();
		  const width=pseudo?(parseFloat(style.width)||0):rect.width,height=pseudo?(parseFloat(style.height)||0):rect.height;
		  if(width<3||height<3||width>34||height>34)continue;
		  if(value!==''&&!/^\d{1,2}$/.test(value)&&(!pseudo||!/^\d{1,2}$/.test(content)))continue;
			if(pseudo&&(content==='none'||content==='normal'))continue;
		  const colors=(style.backgroundColor+' '+style.color).match(/\d+/g)||[];
		  for(let i=0;i+2<colors.length;i+=3){const r=Number(colors[i]),g=Number(colors[i+1]),b=Number(colors[i+2]);if(r>180&&g<150&&b<150)return true;}
		 }
		 return false;
	 };
	for(const badge of badgeNodes){
	 if(!isUnreadBadge(badge))continue;
	 let row=badge;
		 for(let depth=0;row&&depth<12;depth++,row=row.parentElement){
	  const rect=row.getBoundingClientRect(),text=clean(row.innerText||row.textContent),lines=linesOf(row.innerText||row.textContent);
	  if(rect.width>80&&rect.height>24&&rect.width<innerWidth*.82&&rect.right<innerWidth*.55&&rect.left<innerWidth*.78&&text.length>=2&&text.length<=360&&lines.length>=1){nodes.push(row);break;}
	 }
	}
	nodes = [...new Set(nodes)].sort((a,b) => { const position = a.compareDocumentPosition(b); return position & Node.DOCUMENT_POSITION_FOLLOWING ? -1 : position & Node.DOCUMENT_POSITION_PRECEDING ? 1 : 0; });
	const seen = new Set(); const items = [];
	nodes.forEach((el, index) => {
   if (!visible(el)) return;
   const lines = linesOf(el.innerText || el.textContent); const text = lines.join(' '); if (text.length < 2 || text.length > 360) return;
   if (/^(首页|工作台|消息|设置|帮助|客服中心)$/.test(text)) return;
   const firstLine = lines[0] || '';
   const stableID = clean(el.getAttribute('data-conversation-id') || el.getAttribute('data-session-id') || el.getAttribute('href'));
	 const unreadValue = clean(el.getAttribute('data-unread-count') || el.getAttribute('data-unread') || el.getAttribute('aria-label'));
	 const unreadClass = /(unread|un-read|new-message|newMessage)/i.test(String(el.className || ''));
	 const unreadChild = [...el.querySelectorAll('[data-unread-count],[data-unread],[class*="unread"],[class*="Unread"]')].some(item => {
	   const value = clean(item.getAttribute('data-unread-count') || item.getAttribute('data-unread') || item.textContent);
	   return value !== '0' && value !== 'false';
	 });
	 const badgeIsRed = (item,pseudo='') => {
	   const rect=item.getBoundingClientRect(),style=getComputedStyle(item,pseudo),value=clean(item.textContent),content=String(style.content||'').replace(/[\"']/g,'').trim();
	   if(rect.width<3||rect.height<3||rect.width>34||rect.height>34||(!/^\d{1,2}$/.test(value)&&value!==''&&!/^\d{1,2}$/.test(content)&&content!==''&&content!=='none'&&content!=='normal'))return false;
	   const colors=(style.backgroundColor+' '+style.color).match(/\d+/g)||[];
	   for(let i=0;i+2<colors.length;i+=3){const r=Number(colors[i]),g=Number(colors[i+1]),b=Number(colors[i+2]);if(r>180&&g<150&&b<150)return true;}
	   return false;
	 };
	 const unreadVisual = badgeNodes.some(item => (item===el||el.contains(item))&&isUnreadBadge(item));
	 const unread = unreadClass || unreadChild || unreadVisual || /(^|\D)[1-9]\d*\s*(条)?新消息/.test(unreadValue);
   // Preview text changes when a new message arrives; never use it as the
   // conversation identity when the page does not expose a stable id.
   if (unreadOnly && !unread) return;
   const key = stableID || firstLine;
   if (!key || seen.has(key)) return; seen.add(key);
   el.setAttribute('data-sunapi-xianyu-key', key);
	 items.push({ key, title: firstLine.slice(0, 120), preview: text.slice(0, 240), unread, index });
 });
	return JSON.stringify({ url: location.href || '', title: document.title || '', items: items.slice(0, max).map(({key,title,preview,unread}) => ({key,title,preview,unread})) });
})()`, selectorJSON, limit, unreadOnly)
}

func buildAgentXianyuClickExpression(selector string, candidate agentXianyuConversationCandidate) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	keyJSON := quoteAgentXianyuJS(candidate.Key)
	titleJSON := quoteAgentXianyuJS(candidate.Title)
	previewJSON := quoteAgentXianyuJS(candidate.Preview)
	return fmt.Sprintf(`(() => {
 const key = %s, title = %s, preview = %s, custom = %s;
 const clean = value => String(value || '').replace(/\s+/g, ' ').trim();
	const linesOf = value => String(value || '').replace(/\r/g, '').split(/\n/).map(line => line.replace(/[ \t]+/g, ' ').trim()).filter(Boolean);
	 const visible = item => { const box = item.getBoundingClientRect(); const style = getComputedStyle(item); return box.width > 80 && box.height > 24 && box.width < innerWidth * .82 && (custom || box.right < innerWidth * .55) && box.left < innerWidth * .78 && box.bottom > 0 && box.top < innerHeight && style.display !== 'none' && style.visibility !== 'hidden'; };
 let el = [...document.querySelectorAll('[data-sunapi-xianyu-key]')].find(item => visible(item) && clean(item.getAttribute('data-sunapi-xianyu-key')) === key);
	 if (!el) { let scope = document; if (custom) { try { scope = document.querySelector(custom) || document; } catch (_) {} } const selectors = ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[class*="list-item"]','[class*="listItem"]','[class*="message-item"]','[class*="item-row"]','[class*="itemRow"]','[role="listitem"]','[role="option"]']; let nodes=[]; for (const query of selectors) { try { nodes.push(...scope.querySelectorAll(query)); } catch (_) {} } try { nodes.push(...scope.querySelectorAll('li,button,[role="listitem"],[role="option"],[data-id],[class*="item"],[class*="Item"]')); } catch (_) {} if (custom && scope !== document) nodes.push(scope); nodes = [...new Set(nodes)].filter(visible); const matches = item => { const lines = linesOf(item.innerText || item.textContent); const itemTitle = lines[0] || ''; const text = lines.join(' '); return itemTitle === title && (!preview || text.includes(preview.slice(0, 80))); }; el = nodes.find(matches) || nodes.find(item => linesOf(item.innerText || item.textContent).join(' ').includes(title)); }
 if (!el) return JSON.stringify(false);
 el.scrollIntoView({block:'center',inline:'nearest'});
 const clickable = el.matches('button,a,[role="button"]') ? el : el.querySelector('button,a,[role="button"]') || el;
 try { clickable.focus({preventScroll:true}); } catch (_) {}
 if (window.PointerEvent) clickable.dispatchEvent(new PointerEvent('pointerdown',{bubbles:true,cancelable:true,view:window}));
 clickable.dispatchEvent(new MouseEvent('mousedown',{bubbles:true,cancelable:true,view:window}));
 if (window.PointerEvent) clickable.dispatchEvent(new PointerEvent('pointerup',{bubbles:true,cancelable:true,view:window}));
 clickable.dispatchEvent(new MouseEvent('mouseup',{bubbles:true,cancelable:true,view:window}));
 clickable.click(); return JSON.stringify(true);
	})()`, keyJSON, titleJSON, previewJSON, selectorJSON)
}

func buildAgentXianyuTranscriptExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s;
 const clean = value => String(value || '').replace(/\r/g, '').split(/\n+/).map(line => line.replace(/[ \t]+/g, ' ').trim()).filter(Boolean).join('\n');
 const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 260 && r.height > 100 && s.display !== 'none' && s.visibility !== 'hidden' && s.opacity !== '0'; };
 const visibleRow = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 24 && r.height > 4 && s.display !== 'none' && s.visibility !== 'hidden' && s.opacity !== '0'; };
 const excludedSelector = 'button,input,textarea,select,option,script,style,noscript,svg,[hidden],[aria-hidden="true"],[role="button"],[contenteditable="true"],[class*="toolbar"],[class*="quick-reply"],[class*="quickReply"],[class*="composer"],[class*="input-area"],[class*="inputArea"],[class*="send-box"],[class*="sendBox"]';
 const noiseLine = /^(发送|回复|快捷回复|请输入消息|输入消息)$/;
 const textOnly = root => {
  const parts = []; const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  while (walker.nextNode()) {
   const node = walker.currentNode; const parent = node.parentElement; if (!parent) continue;
   const excluded = parent.closest(excludedSelector); if (excluded && root.contains(excluded)) continue;
   const style = getComputedStyle(parent); if (style.display === 'none' || style.visibility === 'hidden' || style.opacity === '0') continue;
   const value = String(node.nodeValue || '').replace(/\s+/g, ' ').trim(); if (value && !noiseLine.test(value)) parts.push(value);
  }
  return clean(parts.join('\n'));
 };
 const queries = custom ? [custom] : ['[role="log"]','[data-testid*="message-list"]','[class*="message-list"]','[class*="messageList"]','[class*="chat-content"]','[class*="chatContent"]','[class*="conversation-content"]','[class*="conversationContent"]','[class*="im-message-list"]'];
 let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} }
 let best = null, bestScore = -1; for (const el of [...new Set(nodes)]) { if (!visible(el)) continue; const text = textOnly(el); if (text.length < 1) continue; const r = el.getBoundingClientRect(); const signal = /(message|chat|conversation|im-|msg)/i.test(String(el.className || '') + ' ' + String(el.getAttribute('data-testid') || '') + ' ' + String(el.getAttribute('role') || '')) ? 100 : 0; const center = r.left > innerWidth * .16 && r.left < innerWidth * .74 && r.width < innerWidth * .78 ? 80 : 0; const score = signal + center + Math.min(text.length, 4000) / 25 + Math.min(r.height, 900) / 30 - (r.width > innerWidth * .82 ? 120 : 0) - (r.left < innerWidth * .14 ? 80 : 0); if (score > bestScore) { best = el; bestScore = score; } }
 if (!best) return JSON.stringify({url: location.href || '', title: document.title || '', text: '', message_count: 0, container_found: false});
 const rowQueries = ['[data-message-id]','[data-msg-id]','[data-testid*="message-item"]','[data-testid*="messageItem"]','[class*="message-item"]','[class*="messageItem"]','[class*="message-row"]','[class*="messageRow"]','[class*="msg-item"]','[class*="msgItem"]','[class*="chat-message"]','[class*="chatMessage"]','[class*="im-message"]','[class*="bubble"]','[class*="Bubble"]','[role="listitem"]'];
 let rowNodes = []; for (const query of rowQueries) { try { rowNodes.push(...best.querySelectorAll(query)); } catch (_) {} }
 const rawRows = [...new Set(rowNodes)].filter(visibleRow);
 const rows = rawRows.filter(node => !rawRows.some(child => child !== node && node.contains(child)));
 const directionOf = row => {
  const parts = [];
  let current = row;
  for (let depth = 0; current && depth < 3; depth++, current = current.parentElement) {
   for (const name of ['data-direction','data-sender','data-role','data-message-direction','aria-label','class']) {
    const value = current.getAttribute && current.getAttribute(name); if (value) parts.push(String(value).toLowerCase());
   }
  }
  const value = parts.join(' ');
  if (/(seller|merchant|shop|客服|卖家|商家|self|mine|outgoing|sent|from[-_ ]?me)/i.test(value)) return 'seller';
  if (/(buyer|customer|买家|用户|incoming|received|from[-_ ]?other)/i.test(value)) return 'customer';
  return 'unknown';
 };
 const messages = rows.map((row,index) => {
  const text = textOnly(row); if (!text) return null;
  const key = row.getAttribute('data-message-id') || row.getAttribute('data-msg-id') || String(index);
  return {key, text, direction: directionOf(row)};
 }).filter(Boolean);
 const text = messages.length > 0 ? clean(messages.map(item => item.text).join('\n')) : textOnly(best);
 return JSON.stringify({url: location.href || '', title: document.title || '', text, messages, message_count: messages.length, container_found: true});
})()`, selectorJSON)
}

func buildAgentXianyuProductContextExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s;
 const clean = value => String(value || '').replace(/\r/g, '').split(/\n+/).map(line => line.replace(/[ \t]+/g, ' ').trim()).filter(Boolean).join('\n');
 const visible = el => { if (!el) return false; const r=el.getBoundingClientRect(),s=getComputedStyle(el); return r.width>24&&r.height>4&&s.display!=='none'&&s.visibility!=='hidden'&&s.opacity!=='0'; };
 const linesOf = root => { const lines=[],seen=new Set(),walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT); while(walker.nextNode()){const parent=walker.currentNode.parentElement;if(!parent||!visible(parent))continue;const value=clean(walker.currentNode.nodeValue);if(!value||seen.has(value))continue;seen.add(value);lines.push(value);} return lines; };
 let root=null;
 if(custom){try{root=document.querySelector(custom);}catch(_){}}
 if(!visible(root)){
  const tabs=[...document.querySelectorAll('[role="tab"]')].filter(visible);
  const tab=tabs.find(el=>/^(当前宝贝|当前商品|咨询的宝贝)$/.test(clean(el.innerText||el.textContent)));
  const panelID=tab&&tab.getAttribute('aria-controls');
  root=panelID?document.getElementById(panelID):null;
 }
 if(!visible(root))return JSON.stringify({found:false,title:'',status:'',text:''});
 const lines=linesOf(root).slice(0,40);
 const status=lines.find(line=>/^(已删除|已下架|已失效|售罄|无货|不可拍|交易关闭|在售|在线|可拍|已上架|出售中|有货|立即购买|马上购买|加入购物车|去购买)$/.test(line))||'';
 const privateLine=line=>/(来源网址|浏览器会话|会话归档标识|卖家工作台|内部备注|卖家备注|进货价|成本价|利润|底价|账号余额|提现金额|曝光(?:量|次数|人数|数据|\s*[:：])|浏览(?:量|次数|人数|数据|\s*[:：])|想要(?:数|人数|\s*[:：])|收藏(?:量|数|人数|\s*[:：])|访客(?:量|数|人数|\s*[:：])|点击率|转化率|询单人数|支付人数|支付订单数|成交金额|本周购买|本周消费|均单价|客单价|数据更新)/i.test(line);
 const publicLines=lines.filter(line=>!privateLine(line)&&line!=='描述不符');
 const titleCandidates=publicLines.filter(line=>{
  if(line===status||line.length<4)return false;
  if(/^(描述不符|曝光[:：]|浏览[:：]|想要[:：]|¥|￥|\d+(\.\d+)?元?$)/.test(line))return false;
  return !/^(全部|待付款|待发货|已发货|退款中|交易关闭|交易成功)$/.test(line);
 });
 titleCandidates.sort((a,b)=>b.length-a.length);
 const pageText=publicLines.join('\n').slice(0,1600);
 const unavailable=/^(已下架|已删除|已失效|售罄|无货|不可拍|交易关闭)$/.test(status);
 const available=!unavailable&&/^(在售|在线|可拍|已上架|出售中|有货|立即购买|马上购买|加入购物车|去购买)$/.test(status);
 return JSON.stringify({found:true,title:(titleCandidates[0]||'').slice(0,300),status:status.slice(0,80),availability:unavailable?'unavailable':available?'available':'unknown',description:pageText,text:pageText});
})()`, selectorJSON)
}

func buildAgentXianyuScrollExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s; const clean = value => String(value || '').replace(/\s+/g, ' ').trim(); const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 260 && r.height > 100 && s.display !== 'none' && s.visibility !== 'hidden'; }; const queries = custom ? [custom] : ['[role="log"]','[data-testid*="message-list"]','[class*="message-list"]','[class*="messageList"]','[class*="chat-content"]','[class*="chatContent"]','[class*="conversation-content"]','[class*="conversationContent"]','[class*="im-message-list"]']; let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} } if (nodes.length === 0) nodes = [...document.querySelectorAll('main,section,article,div')]; let el=null,best=-1; for (const node of nodes) { if (!visible(node)) continue; const text=clean(node.innerText || node.textContent); if (text.length < 8) continue; const r=node.getBoundingClientRect(); const signal=/(message|chat|conversation|im-|msg)/i.test(node.className || '') ? 80 : 0; const center=r.left > innerWidth*.18 && r.left < innerWidth*.72 && r.width < innerWidth*.8 ? 60 : 0; const score=signal + center + Math.min(text.length,4000)/20 + Math.min(r.height,900)/30 - (r.width > innerWidth*.82 ? 80 : 0) - (r.left < innerWidth*.18 ? 40 : 0); if (score > best) { el=node; best=score; } } if (!el) return JSON.stringify({height:0,top:0,client_height:0,moved:false,at_start:true,at_end:true}); let scroll=el; while (scroll.parentElement && scroll.scrollHeight <= scroll.clientHeight + 20) scroll=scroll.parentElement; const before=scroll.scrollTop || 0; scroll.scrollTop=0; scroll.dispatchEvent(new Event('scroll',{bubbles:true})); const top=scroll.scrollTop || 0; return JSON.stringify({height:scroll.scrollHeight || 0,top,client_height:scroll.clientHeight || 0,moved:top !== before,at_start:top <= 4,at_end:top + scroll.clientHeight >= scroll.scrollHeight - 4});
})()`, selectorJSON)
}

func buildAgentXianyuScrollToLatestExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s; const clean = value => String(value || '').replace(/\s+/g, ' ').trim(); const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 260 && r.height > 100 && s.display !== 'none' && s.visibility !== 'hidden'; }; const queries = custom ? [custom] : ['[role="log"]','[data-testid*="message-list"]','[class*="message-list"]','[class*="messageList"]','[class*="chat-content"]','[class*="chatContent"]','[class*="conversation-content"]','[class*="conversationContent"]','[class*="im-message-list"]']; let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} } if (nodes.length === 0) nodes = [...document.querySelectorAll('main,section,article,div')]; let el=null,best=-1; for (const node of nodes) { if (!visible(node)) continue; const text=clean(node.innerText || node.textContent); if (text.length < 8) continue; const r=node.getBoundingClientRect(); const signal=/(message|chat|conversation|im-|msg)/i.test(node.className || '') ? 80 : 0; const center=r.left > innerWidth*.18 && r.left < innerWidth*.72 && r.width < innerWidth*.8 ? 60 : 0; const score=signal + center + Math.min(text.length,4000)/20 + Math.min(r.height,900)/30 - (r.width > innerWidth*.82 ? 80 : 0) - (r.left < innerWidth*.18 ? 40 : 0); if (score > best) { el=node; best=score; } } if (!el) return JSON.stringify({height:0,top:0}); let scroll=el; while (scroll.parentElement && scroll.scrollHeight <= scroll.clientHeight + 20) scroll=scroll.parentElement; const before=scroll.scrollTop || 0; scroll.scrollTop=scroll.scrollHeight || 0; scroll.dispatchEvent(new Event('scroll',{bubbles:true})); return JSON.stringify({height:scroll.scrollHeight || 0,top:scroll.scrollTop || 0,client_height:scroll.clientHeight || 0,moved:(scroll.scrollTop || 0)!==before,at_end:true});
})()`, selectorJSON)
}

func buildAgentXianyuConversationListScrollExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s; const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 80 && r.height > 24 && s.display !== 'none' && s.visibility !== 'hidden'; }; const queries = custom ? [custom] : ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[role="listitem"]','[role="option"]']; let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} } if (nodes.length === 0) nodes = [...document.querySelectorAll('li,button,[role="listitem"],[role="option"]')]; let el=null,best=-1; for (const node of nodes) { if (!visible(node)) continue; let scroll=node; while (scroll.parentElement && scroll.scrollHeight <= scroll.clientHeight + 20) scroll=scroll.parentElement; if (scroll.scrollHeight <= scroll.clientHeight + 20) continue; const r=scroll.getBoundingClientRect(); if (!custom && (r.left > innerWidth * .6 || r.right > innerWidth * .48 || r.width > innerWidth * .85)) continue; const score=Math.min(scroll.scrollHeight,20000)/100 + Math.min(r.height,900)/20 - Math.abs(r.left)/100; if (score > best) { el=scroll; best=score; } } if (!el) return JSON.stringify({height:0,top:0,client_height:0,moved:false,at_end:true,container_found:false}); const before=el.scrollTop || 0; const maxTop=Math.max(0,(el.scrollHeight || 0)-(el.clientHeight || 0)); const step=Math.max(160, Math.floor((el.clientHeight || 0) * .8)); const target=Math.min(maxTop, before + step); el.scrollTop=target; el.dispatchEvent(new Event('scroll',{bubbles:true})); const top=el.scrollTop || 0; return JSON.stringify({height:el.scrollHeight || 0,top,client_height:el.clientHeight || 0,moved:top!==before,at_end:top+el.clientHeight >= el.scrollHeight-4,container_found:true});
})()`, selectorJSON)
}

func buildAgentXianyuConversationListResetExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s; const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 80 && r.height > 24 && s.display !== 'none' && s.visibility !== 'hidden'; }; const queries = custom ? [custom] : ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[role="listitem"]','[role="option"]']; let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} } if (nodes.length === 0) nodes = [...document.querySelectorAll('li,button,[role="listitem"],[role="option"]')]; let el=null,best=-1; for (const node of nodes) { if (!visible(node)) continue; let scroll=node; while (scroll.parentElement && scroll.scrollHeight <= scroll.clientHeight + 20) scroll=scroll.parentElement; if (scroll.scrollHeight <= scroll.clientHeight + 20) continue; const r=scroll.getBoundingClientRect(); if (!custom && (r.left > innerWidth * .6 || r.right > innerWidth * .48 || r.width > innerWidth * .85)) continue; const score=Math.min(scroll.scrollHeight,20000)/100 + Math.min(r.height,900)/20 - Math.abs(r.left)/100; if (score > best) { el=scroll; best=score; } } if (!el) return JSON.stringify({height:0,top:0,client_height:0,moved:false,at_start:true,at_end:true,container_found:false}); const before=el.scrollTop || 0; el.scrollTop=0; el.dispatchEvent(new Event('scroll',{bubbles:true})); const top=el.scrollTop || 0; return JSON.stringify({height:el.scrollHeight || 0,top,client_height:el.clientHeight || 0,moved:top!==before,at_start:top<=4,at_end:top+el.clientHeight>=el.scrollHeight-4,container_found:true});
})()`, selectorJSON)
}

// buildAgentXianyuConversationListRestoreExpression restores the last known
// pixel position after the detail pane remounts the virtualized list. It is
// intentionally derived from the reset expression so selector heuristics stay
// identical; unlike reset, it never jumps to page one.
func buildAgentXianyuConversationListRestoreExpression(selector string, top int) string {
	expression := buildAgentXianyuConversationListResetExpression(selector)
	old := "const before=el.scrollTop || 0; el.scrollTop=0;"
	newValue := fmt.Sprintf("const before=el.scrollTop || 0; const requestedTop=%d; const maxTop=Math.max(0,(el.scrollHeight || 0)-(el.clientHeight || 0)); const targetTop=Math.min(Math.max(0,requestedTop),maxTop); el.scrollTop=Math.max(before,targetTop);", top)
	return strings.Replace(expression, old, newValue, 1)
}

func mergeAgentXianyuTranscriptSnapshots(snapshots []string) string {
	merged := ""
	for _, raw := range snapshots {
		snapshot := strings.TrimSpace(raw)
		if snapshot == "" || strings.Contains(merged, snapshot) {
			continue
		}
		if merged == "" || strings.Contains(snapshot, merged) {
			merged = snapshot
			continue
		}
		if overlap := agentXianyuTextOverlap(merged, snapshot); overlap > 0 {
			merged += snapshot[overlap:]
			continue
		}
		if overlap := agentXianyuTextOverlap(snapshot, merged); overlap > 0 {
			merged = snapshot + merged[overlap:]
			continue
		}
		merged += "\n" + snapshot
	}
	return strings.TrimSpace(merged)
}

func agentXianyuTextOverlap(left, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	max := len(leftRunes)
	if len(rightRunes) < max {
		max = len(rightRunes)
	}
	if max > 2000 {
		max = 2000
	}
	for size := max; size >= 16; size-- {
		match := true
		for index := 0; index < size; index++ {
			if leftRunes[len(leftRunes)-size+index] != rightRunes[index] {
				match = false
				break
			}
		}
		if match {
			return len(string(rightRunes[:size]))
		}
	}
	return 0
}

func truncateAgentXianyuTranscript(value string, maxLength int) string {
	value = strings.TrimSpace(value)
	if maxLength <= 0 || len([]rune(value)) <= maxLength {
		return value
	}
	marker := "\n\n[中间历史已省略]\n\n"
	available := maxLength - len([]rune(marker))
	if available <= 0 {
		return truncateRunes(value, maxLength)
	}
	head := available * 2 / 5
	tail := available - head
	runes := []rune(value)
	return string(runes[:head]) + marker + string(runes[len(runes)-tail:])
}

func quoteAgentXianyuJS(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}

func agentXianyuConversationRunes(items []agentXianyuConversation) int {
	total := 0
	for _, item := range items {
		total += len([]rune(item.Transcript))
	}
	return total
}

func filterAgentXianyuChangedConversations(items []agentXianyuConversation, state agentXianyuSyncState, syncAll bool) ([]agentXianyuConversation, map[string]string) {
	if state.Conversations == nil {
		state.Conversations = map[string]string{}
	}
	next := make(map[string]string, len(state.Conversations)+len(items))
	for key, value := range state.Conversations {
		next[key] = value
	}
	changed := make([]agentXianyuConversation, 0, len(items))
	for _, item := range items {
		identity := agentXianyuConversationIdentity(item.Key)
		hash := agentXianyuConversationHash(item)
		if syncAll || state.Conversations[identity] != hash {
			changed = append(changed, item)
		}
		next[identity] = hash
	}
	return changed, next
}

func agentXianyuConversationIdentity(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])
}

func agentXianyuConversationHash(item agentXianyuConversation) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{agentXianyuKnowledgeFormatVersion, item.Key, item.Transcript, item.URL, item.Product.Title, item.Product.Status, item.Product.Availability, item.Product.Description, item.Product.Text}, "\n")))
	return hex.EncodeToString(sum[:])
}

func agentXianyuStatePath(dataDir, blueprintID, nodeID string) string {
	return filepath.Join(dataDir, agentXianyuStateDir, sanitizeAgentBrowserFilenamePart(blueprintID), sanitizeAgentBrowserFilenamePart(nodeID)+".json")
}

func loadAgentXianyuSyncState(path string) (agentXianyuSyncState, error) {
	state := agentXianyuSyncState{Conversations: map[string]string{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	if state.Conversations == nil {
		state.Conversations = map[string]string{}
	}
	return state, nil
}

func saveAgentXianyuSyncState(path string, state agentXianyuSyncState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func agentXianyuShortIdentity(identity string) string {
	identity = strings.TrimSpace(identity)
	if len(identity) > 16 {
		return identity[:16]
	}
	return identity
}

func agentXianyuKnowledgeDocumentID(blueprintID, nodeID, baseID, identity string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{blueprintID, nodeID, baseID, identity}, "\n")))
	return "doc-xianyu-" + hex.EncodeToString(sum[:16])
}

func agentXianyuKnowledgeFilename(identity string) string {
	return "闲鱼客服知识-" + agentXianyuShortIdentity(identity) + ".md"
}

func commitAgentXianyuConversationKnowledge(
	ctx context.Context,
	store *Store,
	bases []*KnowledgeBase,
	blueprintID, nodeID, statePath string,
	state *agentXianyuSyncState,
	conversation agentXianyuConversation,
	conversationHash, sourceURL, browserSession, summary string,
) ([]agentXianyuKnowledgeDocument, error) {
	if state == nil {
		return nil, errors.New("xianyu sync state is required")
	}
	identity := agentXianyuConversationIdentity(conversation.Key)
	if strings.TrimSpace(conversationHash) == "" {
		conversationHash = agentXianyuConversationHash(conversation)
	}
	filename := agentXianyuKnowledgeFilename(identity)
	content := buildAgentXianyuKnowledgeDocument(sourceURL, browserSession, []agentXianyuConversation{conversation}, summary)
	documents := make([]agentXianyuKnowledgeDocument, 0, len(bases))
	for _, base := range bases {
		documentID := agentXianyuKnowledgeDocumentID(blueprintID, nodeID, base.ID, identity)
		document, err := store.UpsertKnowledgeDocument(ctx, base.ID, documentID, base.UserID, filename, "text/markdown", int64(len([]byte(content))), content)
		if err != nil {
			return documents, err
		}
		documents = append(documents, agentXianyuKnowledgeDocument{
			KnowledgeBaseID: base.ID,
			DocumentID:      document.ID,
			Filename:        document.Filename,
			ChunkCount:      document.ChunkCount,
		})
	}
	if state.Conversations == nil {
		state.Conversations = map[string]string{}
	}
	state.Conversations[identity] = conversationHash
	state.UpdatedAt = time.Now().UnixMilli()
	if err := saveAgentXianyuSyncState(statePath, *state); err != nil {
		return documents, err
	}
	return documents, nil
}

func buildAgentXianyuKnowledgeDocument(sourceURL, browserSession string, conversations []agentXianyuConversation, summary string) string {
	var builder strings.Builder
	builder.WriteString("# 闲鱼客服知识摘要\n\n")
	builder.WriteString("仅保留可用于对外客服的脱敏结论和表达风格，不包含原始对话、内部标识或卖家侧经营数据。\n\n")
	if len(conversations) == 1 && conversations[0].Product.Found {
		product := redactAgentXianyuProductContext(conversations[0].Product)
		builder.WriteString("### 当前商品信息（采集时页面展示）\n")
		if product.Title != "" {
			builder.WriteString("- 商品标题：" + product.Title + "\n")
		}
		if product.Status != "" {
			builder.WriteString("- 页面状态：" + product.Status + "\n")
		}
		if product.Availability != "" {
			builder.WriteString("- 当前可售状态：" + product.Availability + "\n")
		}
		if product.Description != "" {
			builder.WriteString("- 商品描述（作为客服参考）：\n" + product.Description + "\n")
		}
		if product.Text != "" {
			builder.WriteString("- 页面商品字段：\n" + product.Text + "\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString(redactAgentXianyuText(strings.TrimSpace(summary)))
	builder.WriteString("\n")
	return builder.String()
}

func writeAgentXianyuKnowledgeDocuments(ctx context.Context, store *Store, bases []*KnowledgeBase, filename, content string) ([]agentXianyuKnowledgeDocument, error) {
	result := make([]agentXianyuKnowledgeDocument, 0, len(bases))
	for _, base := range bases {
		document, err := store.AddKnowledgeDocument(ctx, base.ID, base.UserID, filename, "text/markdown", int64(len([]byte(content))), content)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "already exists") {
				result = append(result, agentXianyuKnowledgeDocument{KnowledgeBaseID: base.ID, Filename: filename})
				continue
			}
			return nil, err
		}
		result = append(result, agentXianyuKnowledgeDocument{KnowledgeBaseID: base.ID, DocumentID: document.ID, Filename: document.Filename, ChunkCount: document.ChunkCount})
	}
	return result, nil
}

func redactAgentXianyuText(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	lines := strings.Split(value, "\n")
	redacted := make([]string, 0, len(lines))
	for _, line := range lines {
		if agentXianyuPrivateLine.MatchString(line) || agentXianyuSellerMetricValue.MatchString(line) {
			continue
		}
		line = agentXianyuLabeledPrivateValue.ReplaceAllString(line, "$1：[已脱敏]")
		line = agentXianyuEmail.ReplaceAllString(line, "[邮箱已脱敏]")
		line = agentXianyuSensitiveDigits.ReplaceAllString(line, "[数字信息已脱敏]")
		redacted = append(redacted, line)
	}
	return strings.TrimSpace(strings.Join(redacted, "\n"))
}

func redactAgentXianyuProductContext(product agentXianyuProductContext) agentXianyuProductContext {
	product.Title = truncateRunes(redactAgentXianyuText(product.Title), 300)
	product.Status = truncateRunes(redactAgentXianyuText(product.Status), 80)
	product.Availability = truncateRunes(redactAgentXianyuText(product.Availability), 40)
	product.Description = truncateRunes(redactAgentXianyuText(firstNonEmpty(product.Description, product.Text)), 1600)
	product.Text = truncateRunes(redactAgentXianyuText(product.Text), 1600)
	if product.Text == "" {
		product.Text = product.Description
	}
	return product
}
