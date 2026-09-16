package sunapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
)

const (
	agentXianyuReplyStateDir       = "agent-xianyu-reply-state"
	agentXianyuReplyPollInterval   = 2 * time.Second
	agentXianyuReplyEventCheck     = 500 * time.Millisecond
	agentXianyuReplyRetryDelay     = 30 * time.Second
	agentXianyuReplyReconnectDelay = 5 * time.Second
	// A realtime reply should not hold the buyer for a full minute. When the
	// knowledge base has no match, the safe fallback is sent immediately below;
	// matched requests get a bounded model window before falling back.
	agentXianyuReplyRequestTimeout  = 25 * time.Second
	agentXianyuReplyDefaultBatch    = 20
	agentXianyuReplyDefaultMaxRunes = 300
)

const agentXianyuReplyInstruction = `根据知识库、当前商品信息和最近对话回答当前闲鱼买家的最新消息。
本节点是实时客服，覆盖通用组件的结构化输出要求；只输出可直接发送给买家的回复正文，不要输出分析、JSON、Markdown、标题或知识库来源。
只能使用检索到的知识库事实，不得编造价格、库存、时效、退款、售后或订单状态。
商品标题、规格、描述和状态以当前商品信息为准，不得套用其他商品的库存结论。商品描述是回答买家时的参考资料；若页面明确显示在售、在线、可拍或立即购买，则可回答当前有货/可以拍下；若显示下架、售罄、无货或不可拍，则必须如实说明。
只要节点仍在监听且最新消息来自买家，就必须回复。事实不足时发送一句简短澄清问题，不得静默或输出 SUNAPI_NO_REPLY。`

type agentXianyuReplyMessage struct {
	Key       string `json:"key,omitempty"`
	Text      string `json:"text"`
	Direction string `json:"direction"`
}

type agentXianyuReplyMessagePayload struct {
	URL            string                    `json:"url"`
	Title          string                    `json:"title"`
	Messages       []agentXianyuReplyMessage `json:"messages"`
	Latest         agentXianyuReplyMessage   `json:"latest"`
	ContainerFound bool                      `json:"container_found"`
}

type agentXianyuReplyObserverPayload struct {
	Installed bool `json:"installed"`
	Connected bool `json:"connected"`
	Dirty     bool `json:"dirty"`
}

type agentXianyuReplySendPayload struct {
	Sent   bool   `json:"sent"`
	Reason string `json:"reason,omitempty"`
}

type agentXianyuReplyConversationState struct {
	PreviewHash      string `json:"preview_hash,omitempty"`
	LastIncomingHash string `json:"last_incoming_hash,omitempty"`
	LastReplyHash    string `json:"last_reply_hash,omitempty"`
	Status           string `json:"status,omitempty"`
	RetryAfter       int64  `json:"retry_after,omitempty"`
	UpdatedAt        int64  `json:"updated_at"`
}

type agentXianyuReplyState struct {
	Conversations map[string]agentXianyuReplyConversationState `json:"conversations"`
	UpdatedAt     int64                                        `json:"updated_at"`
}

type agentXianyuReplyStats struct {
	Scans        int
	Detected     int
	Sent         int
	NoMatch      int
	Skipped      int
	ModelErrors  int
	SendErrors   int
	Reconnects   int
	LastActivity int64
}

func executeAgentXianyuReplyGraphNode(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	inputVars map[string]interface{},
	result agentBlueprintNodeResult,
) agentBlueprintNodeResult {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return failAgentXianyuNode(result, "闲鱼实时智能客服无法启动，数据目录未配置。", "data directory is not configured")
	}
	if bp == nil {
		return failAgentXianyuNode(result, "闲鱼实时智能客服缺少当前智能体上下文。", "agent blueprint is required")
	}
	_, baseIDs, err := resolveAgentXianyuKnowledgeBases(ctx, store, bp, inputVars)
	if err != nil {
		return failAgentXianyuNode(result, err.Error(), err.Error())
	}

	result.Input = map[string]interface{}{
		"knowledge_base_ids": baseIDs,
		"source":             "闲鱼卖家工作台已打开页面",
		"mode":               "page_observer_with_poll_fallback",
	}
	statePath := agentXianyuReplyStatePath(store.DataDir(), bp.ID, node.ID)
	state, err := loadAgentXianyuReplyState(statePath)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼实时智能客服状态读取失败。", err.Error())
	}
	stats := &agentXianyuReplyStats{}

	for {
		if ctx.Err() != nil {
			result.Message = "闲鱼实时智能客服已停止监听。"
			result.Output = agentXianyuReplyStatsOutput(stats, baseIDs)
			return result
		}
		err = runAgentXianyuReplySession(ctx, store, bp, node, baseIDs, statePath, &state, stats)
		if ctx.Err() != nil {
			result.Message = "闲鱼实时智能客服已停止监听。"
			result.Output = agentXianyuReplyStatsOutput(stats, baseIDs)
			return result
		}
		stats.Reconnects++
		log.WithError(err).WithFields(log.Fields{
			"blueprint_id": bp.ID,
			"node_id":      node.ID,
			"reconnects":   stats.Reconnects,
		}).Warn("agent xianyu auto reply browser session interrupted; reconnecting")
		if waitAgentXianyuDuration(ctx, agentXianyuReplyReconnectDelay) != nil {
			continue
		}
	}
}

func runAgentXianyuReplySession(
	ctx context.Context,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	baseIDs []string,
	statePath string,
	state *agentXianyuReplyState,
	stats *agentXianyuReplyStats,
) error {
	conn, sessionName, sourceURL, err := openAgentXianyuReplyPage(ctx, store.DataDir(), node.Data.Settings)
	if err != nil {
		return err
	}
	defer conn.Close()

	listSelector := firstNonEmpty(node.Data.Settings["对话列表选择器"], node.Data.Settings["会话列表选择器"])
	messageSelector := node.Data.Settings["消息区域选择器"]
	messageScrollSelector := firstNonEmpty(node.Data.Settings["消息滚动容器选择器"], messageSelector)
	productSelector := node.Data.Settings["商品详情选择器"]
	inputSelector := node.Data.Settings["消息输入框选择器"]
	sendSelector := node.Data.Settings["发送按钮选择器"]
	pollInterval := parseAgentXianyuReplyDuration(node.Data.Settings["消息轮询间隔"], agentXianyuReplyPollInterval, 2*time.Second, time.Minute)
	conversationWait := parseAgentXianyuReplyDuration(node.Data.Settings["单会话等待"], time.Second, 500*time.Millisecond, 10*time.Second)
	batchSize := parseAgentPositiveSetting(node.Data.Settings["每轮最大会话数"], agentXianyuReplyDefaultBatch, 100)
	maxReplyRunes := parseAgentPositiveSetting(node.Data.Settings["最大回复字数"], agentXianyuReplyDefaultMaxRunes, 1000)
	processUnreadOnStart := parseAgentXianyuReplyBool(firstNonEmpty(node.Data.Settings["启动时处理现有未读"], "是"), true)
	defaultReplyEnabled := parseAgentXianyuReplyBool(firstNonEmpty(node.Data.Settings["默认回复"], "开启"), true)

	callID := 100
	nextID := func() int {
		callID++
		return callID
	}
	if _, err := evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReplyObserverInstallExpression(listSelector), agentXianyuReplyObserverPayload{}); err != nil {
		return err
	}

	firstScan := true
	lastFallback := time.Time{}
	for {
		if err := waitAgentXianyuDuration(ctx, agentXianyuReplyEventCheck); err != nil {
			return err
		}
		observer, err := evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReplyObserverStatusExpression(), agentXianyuReplyObserverPayload{})
		if err != nil {
			return err
		}
		if !observer.Installed || !observer.Connected {
			observer, err = evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReplyObserverInstallExpression(listSelector), agentXianyuReplyObserverPayload{})
			if err != nil {
				return err
			}
		}
		fallbackDue := lastFallback.IsZero() || time.Since(lastFallback) >= pollInterval
		if !firstScan && !observer.Dirty && !fallbackDue {
			continue
		}
		lastFallback = time.Now()
		stats.Scans++
		batch, err := evaluateAgentXianyuCandidates(ctx, conn, nextID(), listSelector, batchSize)
		if err != nil {
			return err
		}
		if len(batch.Items) == 0 {
			return errors.New("闲鱼消息列表当前没有可识别的会话，请检查页面或配置对话列表选择器")
		}

		stateChanged := false
		for _, candidate := range batch.Items {
			identity := agentXianyuConversationIdentity(agentXianyuCandidateKey(candidate))
			previewHash := agentXianyuReplyHash(candidate.Key + "\n" + candidate.Preview)
			entry, exists := state.Conversations[identity]
			if !exists && firstScan && (!processUnreadOnStart || !candidate.Unread) {
				state.Conversations[identity] = agentXianyuReplyConversationState{
					PreviewHash: previewHash,
					Status:      "baseline",
					UpdatedAt:   time.Now().UnixMilli(),
				}
				stateChanged = true
				continue
			}
			if entry.RetryAfter > time.Now().UnixMilli() {
				continue
			}
			if exists && entry.PreviewHash == previewHash && !(defaultReplyEnabled && shouldResumeAgentXianyuReply(entry.Status)) {
				continue
			}
			stats.Detected++
			if err := processAgentXianyuReplyCandidate(ctx, conn, nextID, store, bp, node, baseIDs, statePath, state, stats, candidate, previewHash, listSelector, messageSelector, messageScrollSelector, productSelector, inputSelector, sendSelector, conversationWait, maxReplyRunes); err != nil {
				log.WithError(err).WithFields(log.Fields{
					"blueprint_id":          bp.ID,
					"node_id":               node.ID,
					"conversation_identity": agentXianyuShortIdentity(identity),
				}).Warn("agent xianyu auto reply skipped transient conversation failure")
			}
		}
		if stateChanged {
			state.UpdatedAt = time.Now().UnixMilli()
			if err := saveAgentXianyuReplyState(statePath, *state); err != nil {
				return err
			}
		}
		if firstScan {
			log.WithFields(log.Fields{
				"blueprint_id":          bp.ID,
				"node_id":               node.ID,
				"browser_session":       sessionName,
				"source_url":            sourceURL,
				"visible_conversations": len(batch.Items),
				"poll_interval":         pollInterval.String(),
			}).Info("agent xianyu auto reply page listener started")
		}
		firstScan = false
	}
}

func processAgentXianyuReplyCandidate(
	ctx context.Context,
	conn *websocket.Conn,
	nextID func() int,
	store *Store,
	bp *AgentBlueprint,
	node agentBlueprintGraphNode,
	baseIDs []string,
	statePath string,
	state *agentXianyuReplyState,
	stats *agentXianyuReplyStats,
	candidate agentXianyuConversationCandidate,
	previewHash, listSelector, messageSelector, messageScrollSelector, productSelector, inputSelector, sendSelector string,
	conversationWait time.Duration,
	maxReplyRunes int,
) error {
	identity := agentXianyuConversationIdentity(agentXianyuCandidateKey(candidate))
	entry := state.Conversations[identity]
	defaultReplyEnabled := parseAgentXianyuReplyBool(firstNonEmpty(node.Data.Settings["默认回复"], "开启"), true)
	if err := clickAgentXianyuConversation(ctx, conn, nextID(), listSelector, candidate); err != nil {
		return err
	}
	if err := waitAgentXianyuDuration(ctx, conversationWait); err != nil {
		return err
	}
	if _, err := evaluateAgentXianyuScrollToLatest(ctx, conn, nextID(), messageScrollSelector); err != nil {
		return err
	}
	if err := waitAgentXianyuHistory(ctx); err != nil {
		return err
	}
	payload, err := evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReplyLatestMessageExpression(messageSelector), agentXianyuReplyMessagePayload{})
	if err != nil {
		return err
	}
	latestText := strings.TrimSpace(payload.Latest.Text)
	if !payload.ContainerFound || latestText == "" {
		return errors.New("闲鱼消息详情未找到最新文本消息")
	}
	incomingHash := agentXianyuReplyHash(strings.Join([]string{candidate.Key, payload.Latest.Key, latestText, payload.Latest.Direction}, "\n"))
	if entry.LastIncomingHash == incomingHash && !(defaultReplyEnabled && shouldResumeAgentXianyuReply(entry.Status)) {
		entry.PreviewHash = previewHash
		entry.UpdatedAt = time.Now().UnixMilli()
		state.Conversations[identity] = entry
		return saveAgentXianyuReplyState(statePath, *state)
	}
	latestDirection := payload.Latest.Direction
	if latestDirection == "unknown" && candidate.Unread {
		latestDirection = "customer"
	}
	if latestDirection == "unknown" && entry.LastReplyHash == agentXianyuReplyHash(latestText) {
		latestDirection = "self"
	}
	if latestDirection == "unknown" {
		// Xianyu occasionally omits direction attributes while a message row is
		// being mounted. For an enabled realtime responder, only an explicit
		// self-message signal is safe to suppress; unknown inbound rows must get
		// the default reply path instead of being silently dropped.
		latestDirection = "customer"
	}
	if latestDirection != "customer" {
		entry.PreviewHash = previewHash
		entry.LastIncomingHash = incomingHash
		entry.Status = "skipped_" + firstNonEmpty(latestDirection, "unknown_direction")
		entry.RetryAfter = 0
		entry.UpdatedAt = time.Now().UnixMilli()
		state.Conversations[identity] = entry
		state.UpdatedAt = entry.UpdatedAt
		stats.Skipped++
		return saveAgentXianyuReplyState(statePath, *state)
	}

	recordStarted := time.Now()
	record := AgentXianyuReplyRecord{
		BlueprintID:         bp.ID,
		AgentID:             bp.AgentID,
		NodeID:              node.ID,
		ConversationKey:     candidate.Key,
		ConversationTitle:   candidate.Title,
		ConversationPreview: candidate.Preview,
		IncomingMessage:     latestText,
		IncomingDirection:   latestDirection,
		Status:              "processing",
		StartedAt:           recordStarted.UnixMilli(),
		CreatedAt:           recordStarted.UnixMilli(),
	}
	for _, message := range payload.Messages {
		record.RecentMessages = append(record.RecentMessages, message)
	}
	defer func() {
		record.CompletedAt = time.Now().UnixMilli()
		record.DurationMs = record.CompletedAt - record.StartedAt
		if err := store.AppendAgentXianyuReplyRecord(context.Background(), record); err != nil {
			log.WithError(err).WithFields(log.Fields{
				"blueprint_id": bp.ID,
				"node_id":      node.ID,
			}).Warn("agent xianyu auto reply audit record could not be persisted")
		}
	}()

	product, productErr := evaluateAgentXianyuProductContext(ctx, conn, nextID(), productSelector)
	if productErr != nil {
		log.WithError(productErr).WithFields(log.Fields{
			"blueprint_id": bp.ID,
			"node_id":      node.ID,
		}).Warn("agent xianyu auto reply could not read current product context")
	}
	product = redactAgentXianyuProductContext(product)
	record.Product = product
	knowledgeQuery := buildAgentXianyuReplyKnowledgeQuery(latestText, candidate, product)
	record.KnowledgeQuery = knowledgeQuery
	scope := KnowledgeSearchScope{Module: "agent", AgentID: bp.AgentID, BlueprintID: bp.ID, KnowledgeBaseIDs: baseIDs}
	matches, knowledgeErr := store.SearchKnowledgeForScope(ctx, bp.UserID, knowledgeQuery, 6, scope)
	if knowledgeErr != nil {
		log.WithError(knowledgeErr).WithFields(log.Fields{
			"blueprint_id": bp.ID,
			"node_id":      node.ID,
		}).Warn("agent xianyu auto reply knowledge search failed; continuing with safe reply policy")
		record.Error = "knowledge_search: " + knowledgeErr.Error()
	}
	record.KnowledgeMatchCount = len(matches)
	if len(matches) == 0 {
		stats.NoMatch++
	}

	replyNode := node
	replyNode.Data.Settings = map[string]string{}
	if strings.TrimSpace(replyNode.Data.Description) == "" {
		replyNode.Data.Description = agentXianyuReplyInstruction
	} else {
		replyNode.Data.Description = agentXianyuReplyInstruction + "\n附加客服要求：" + strings.TrimSpace(replyNode.Data.Description)
	}
	recent := make([]map[string]string, 0, len(payload.Messages))
	for _, message := range payload.Messages {
		recent = append(recent, map[string]string{
			"speaker": firstNonEmpty(message.Direction, "unknown"),
			"text":    truncateRunes(strings.TrimSpace(message.Text), 500),
		})
	}
	usedFallback := false
	var modelErr error
	var reply string
	if len(matches) == 0 {
		// No knowledge hit is an expected state for a new product/question. Do
		// not spend up to a minute asking the model to invent an answer; send the
		// safe clarification immediately and let later knowledge updates improve
		// subsequent messages.
		reply = buildAgentXianyuFallbackReply(latestText, product, maxReplyRunes)
		usedFallback = true
		record.Status = "no_knowledge_match"
	} else {
		record.ModelStartedAt = time.Now().UnixMilli()
		reply, modelErr = callLLMForAgentNodeWithPolicy(ctx, store, bp, replyNode, map[string]interface{}{
			"conversation_preview":    truncateRunes(strings.TrimSpace(candidate.Preview), 300),
			"latest_customer_message": latestText,
			"recent_messages":         recent,
			"current_product":         product,
			"product_title":           product.Title,
			"product_description":     firstNonEmpty(product.Description, product.Text),
			"product_status":          firstNonEmpty(product.Status, product.Availability),
			"product_availability":    product.Availability,
			"knowledge_match_count":   len(matches),
		}, map[string]interface{}{
			"knowledge_base_ids": baseIDs,
			"knowledge_query":    knowledgeQuery,
		}, agentXianyuReplyRequestTimeout, 2)
		record.ModelCompletedAt = time.Now().UnixMilli()
		record.ModelDurationMs = record.ModelCompletedAt - record.ModelStartedAt
		record.ModelDraft = strings.TrimSpace(reply)
		if modelErr != nil {
			record.Status = "model_error"
			record.Error = modelErr.Error()
			stats.ModelErrors++
			log.WithError(modelErr).WithFields(log.Fields{
				"blueprint_id": bp.ID,
				"node_id":      node.ID,
			}).Warn("agent xianyu auto reply model request failed")
			// A model outage must not leave a buyer without any response. Fall
			// through to the same safe clarification used for a knowledge miss.
			reply = buildAgentXianyuFallbackReply(latestText, product, maxReplyRunes)
			usedFallback = true
		} else {
			reply = normalizeAgentXianyuReply(reply, maxReplyRunes)
			if reply == "" {
				reply = buildAgentXianyuFallbackReply(latestText, product, maxReplyRunes)
				usedFallback = true
			}
		}
	}
	if reply == "" {
		record.Status = "model_declined"
		entry.PreviewHash = previewHash
		entry.LastIncomingHash = incomingHash
		entry.Status = "model_declined"
		entry.RetryAfter = 0
		entry.UpdatedAt = time.Now().UnixMilli()
		state.Conversations[identity] = entry
		state.UpdatedAt = entry.UpdatedAt
		stats.Skipped++
		return saveAgentXianyuReplyState(statePath, *state)
	}
	record.FinalReply = reply
	record.Fallback = usedFallback

	sendStarted := time.Now()
	sendResult, err := evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReplySendExpression(inputSelector, sendSelector, reply), agentXianyuReplySendPayload{})
	record.SentAt = time.Now().UnixMilli()
	record.SendDurationMs = time.Since(sendStarted).Milliseconds()
	if err != nil || !sendResult.Sent {
		record.Status = "send_error"
		stats.SendErrors++
		if err != nil {
			record.Error = err.Error()
			return scheduleAgentXianyuReplyRetry(statePath, state, identity, entry, "send_error")
		}
		record.Error = firstNonEmpty(sendResult.Reason, "send_failed")
		log.WithFields(log.Fields{
			"blueprint_id": bp.ID,
			"node_id":      node.ID,
			"reason":       sendResult.Reason,
		}).Warn("agent xianyu auto reply was not sent")
		return scheduleAgentXianyuReplyRetry(statePath, state, identity, entry, "send_"+firstNonEmpty(sendResult.Reason, "failed"))
	}
	verified := false
	if waitAgentXianyuDuration(ctx, agentXianyuHistoryQuietPeriod) == nil {
		confirmation, confirmErr := evaluateAgentXianyuJSON(ctx, conn, nextID(), buildAgentXianyuReplyLatestMessageExpression(messageSelector), agentXianyuReplyMessagePayload{})
		verified = confirmErr == nil && confirmation.Latest.Direction == "self" && strings.TrimSpace(confirmation.Latest.Text) == strings.TrimSpace(reply)
	}

	entry.PreviewHash = previewHash
	entry.LastIncomingHash = incomingHash
	entry.LastReplyHash = agentXianyuReplyHash(reply)
	entry.Status = "sent"
	record.Status = "sent"
	if len(matches) == 0 {
		record.Status = "sent_no_match"
	}
	if usedFallback {
		entry.Status = "sent_fallback"
		record.Status = "sent_fallback"
		if len(matches) == 0 {
			record.Status = "sent_no_match_fallback"
		}
	}
	if !verified {
		entry.Status = "sent_unverified"
		record.Status = "sent_unverified"
		if usedFallback {
			entry.Status = "sent_fallback_unverified"
			record.Status = "sent_fallback_unverified"
			if len(matches) == 0 {
				record.Status = "sent_no_match_fallback_unverified"
			}
		}
		stats.SendErrors++
	}
	entry.RetryAfter = 0
	entry.UpdatedAt = time.Now().UnixMilli()
	state.Conversations[identity] = entry
	state.UpdatedAt = entry.UpdatedAt
	stats.Sent++
	stats.LastActivity = entry.UpdatedAt
	log.WithFields(log.Fields{
		"blueprint_id":          bp.ID,
		"node_id":               node.ID,
		"conversation_identity": agentXianyuShortIdentity(identity),
		"product_found":         product.Found,
		"fallback":              usedFallback,
		"reply_runes":           len([]rune(reply)),
	}).Info("agent xianyu auto reply sent")
	return saveAgentXianyuReplyState(statePath, *state)
}

func openAgentXianyuReplyPage(ctx context.Context, dataDir string, settings map[string]string) (*websocket.Conn, string, string, error) {
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
	target, reused, err := openAgentBrowserTarget(ctx, endpoint, targetURL)
	if err != nil {
		return nil, paths.Name, targetURL, err
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return nil, paths.Name, targetURL, fmt.Errorf("连接浏览器调试通道失败: %w", err)
	}
	closeWithError := func(err error) (*websocket.Conn, string, string, error) {
		_ = conn.Close()
		return nil, paths.Name, targetURL, err
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return closeWithError(err)
	}
	if _, err := agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return closeWithError(err)
	}
	// Reuse the already-open seller page without refreshing it. Reloading here
	// loses the precise message-list position and can retrigger platform checks.
	if !reused {
		if _, err := agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
			return closeWithError(err)
		}
	}
	loginWait := parseAgentPositiveSetting(firstNonEmpty(settings["登录等待"], "180s"), agentXianyuDefaultLoginWait, 300)
	if loginWait > 0 && !strings.Contains(settings["登录态"], "无需登录") {
		if err := waitAgentBrowserLogin(ctx, conn, loginWait); err != nil {
			return closeWithError(err)
		}
	}
	if err := waitAgentXianyu(ctx, settings["列表等待"], agentXianyuDefaultListWait); err != nil {
		return closeWithError(err)
	}
	return conn, paths.Name, targetURL, nil
}

func buildAgentXianyuReplyObserverInstallExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s;
 try { if (window.__sunapiXianyuObserver) window.__sunapiXianyuObserver.disconnect(); } catch (_) {}
 let root = null;
 if (custom) { try { root = document.querySelector(custom); } catch (_) {} }
 if (!root) {
   const queries = ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[role="listitem"]','[role="option"]'];
   let rows = []; for (const query of queries) { try { rows.push(...document.querySelectorAll(query)); } catch (_) {} }
   rows = [...new Set(rows)].filter(el => { const r=el.getBoundingClientRect(); return r.width>80 && r.height>24 && r.right<innerWidth*.5; });
   if (rows.length) { root = rows[0].parentElement; while (root && root.parentElement && root.scrollHeight <= root.clientHeight + 20) root = root.parentElement; }
 }
 if (!root) root = document.body;
 if (!root) return JSON.stringify({installed:false,connected:false,dirty:true});
 window.__sunapiXianyuDirty = true;
 window.__sunapiXianyuObservedRoot = root;
 window.__sunapiXianyuObserver = new MutationObserver(() => { window.__sunapiXianyuDirty = true; });
 window.__sunapiXianyuObserver.observe(root,{subtree:true,childList:true,characterData:true,attributes:true,attributeFilter:['class','aria-label','data-unread','data-unread-count']});
 return JSON.stringify({installed:true,connected:root.isConnected,dirty:true});
})()`, selectorJSON)
}

func buildAgentXianyuReplyObserverStatusExpression() string {
	return `(() => {
 const installed = Boolean(window.__sunapiXianyuObserver);
 const connected = Boolean(window.__sunapiXianyuObservedRoot && window.__sunapiXianyuObservedRoot.isConnected);
 const dirty = Boolean(window.__sunapiXianyuDirty);
 window.__sunapiXianyuDirty = false;
 return JSON.stringify({installed,connected,dirty});
})()`
}

func buildAgentXianyuReplyLatestMessageExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s;
 const clean = value => String(value || '').replace(/\r/g,'').split(/\n+/).map(line => line.replace(/[ \t]+/g,' ').trim()).filter(Boolean).join('\n');
 const visible = el => { const r=el.getBoundingClientRect(),s=getComputedStyle(el); return r.width>24 && r.height>4 && s.display!=='none' && s.visibility!=='hidden' && s.opacity!=='0'; };
 const excluded = 'button,input,textarea,select,option,script,style,noscript,svg,[hidden],[aria-hidden="true"],[role="button"],[contenteditable="true"],[class*="toolbar"],[class*="quick-reply"],[class*="quickReply"],[class*="composer"],[class*="input-area"],[class*="inputArea"],[class*="send-box"],[class*="sendBox"]';
 const textOnly = root => { const parts=[],walker=document.createTreeWalker(root,NodeFilter.SHOW_TEXT); while(walker.nextNode()){const node=walker.currentNode,parent=node.parentElement;if(!parent)continue;const blocked=parent.closest(excluded);if(blocked&&root.contains(blocked))continue;const value=String(node.nodeValue||'').replace(/\s+/g,' ').trim();if(value)parts.push(value);} return clean(parts.join('\n')); };
 const rootQueries = custom ? [custom] : ['[role="log"]','[data-testid*="message-list"]','[class*="message-list"]','[class*="messageList"]','[class*="chat-content"]','[class*="chatContent"]','[class*="conversation-content"]','[class*="conversationContent"]','[class*="im-message-list"]'];
 let roots=[]; for(const query of rootQueries){try{roots.push(...document.querySelectorAll(query));}catch(_){}}
 let root=null,best=-1; for(const el of [...new Set(roots)]){if(!visible(el))continue;const r=el.getBoundingClientRect(),text=textOnly(el);if(!text)continue;const center=r.left>innerWidth*.16&&r.left<innerWidth*.75&&r.width<innerWidth*.8?80:0;const signal=/(message|chat|conversation|im-|msg)/i.test(String(el.className||'')+' '+String(el.getAttribute('data-testid')||''))?100:0;const score=center+signal+Math.min(text.length,4000)/25;if(score>best){root=el;best=score;}}
 if(!root)return JSON.stringify({url:location.href||'',title:document.title||'',messages:[],latest:{text:'',direction:'unknown'},container_found:false});
 const rowQueries=['[data-message-id]','[data-msg-id]','[data-testid*="message-item"]','[data-testid*="messageItem"]','[class*="message-item"]','[class*="messageItem"]','[class*="message-row"]','[class*="messageRow"]','[class*="msg-item"]','[class*="msgItem"]','[class*="chat-message"]','[class*="chatMessage"]','[class*="im-message"]','[class*="bubble"]','[class*="Bubble"]','[role="listitem"]'];
 let rows=[]; for(const query of rowQueries){try{rows.push(...root.querySelectorAll(query));}catch(_){}}
 const all=[...new Set(rows)].filter(visible); rows=all.filter(node=>!all.some(child=>child!==node&&node.contains(child)));
	 const directionOf = (row,text) => {
	   const isSelf=row.getAttribute('data-is-self'); if(isSelf==='true'||isSelf==='1')return 'self'; if(isSelf==='false'||isSelf==='0')return 'customer';
   const attrs=[row.getAttribute('data-direction'),row.getAttribute('data-side'),row.getAttribute('data-sender-type'),row.getAttribute('data-from'),row.getAttribute('data-is-self'),row.getAttribute('aria-label'),row.className].map(String).join(' ');
   if(/(outgoing|from-self|is-self|mine|my-message|seller-message|message-right|right-message|self\b)/i.test(attrs)||/data-is-self\s*true/i.test(attrs))return 'self';
   if(/(incoming|from-other|buyer-message|customer-message|message-left|left-message)/i.test(attrs)||/data-is-self\s*false/i.test(attrs))return 'customer';
   let bubble=row,bubbleArea=Infinity; for(const el of row.querySelectorAll('div,span,p')){if(!visible(el))continue;const value=textOnly(el);if(!value||(!value.includes(text)&&!text.includes(value)))continue;const r=el.getBoundingClientRect(),area=r.width*r.height;if(r.width<root.getBoundingClientRect().width*.82&&area<bubbleArea){bubble=el;bubbleArea=area;}}
   const rr=root.getBoundingClientRect(),br=bubble.getBoundingClientRect(),delta=(br.left+br.width/2)-(rr.left+rr.width/2);
   if(delta>rr.width*.08)return 'self'; if(delta<rr.width*-.08)return 'customer'; return 'unknown';
 };
 const messages=[]; rows.forEach((row,index)=>{const text=textOnly(row);if(!text||/^(\d{1,2}:\d{2}|\d{2,4}[-/.]\d{1,2}[-/.]\d{1,2}(\s+\d{1,2}:\d{2})?|已读|未读)$/.test(text))return;const direction=directionOf(row,text);const key=String(row.getAttribute('data-message-id')||row.getAttribute('data-msg-id')||row.getAttribute('data-id')||index);messages.push({key,text:text.slice(0,2000),direction});});
 const recent=messages.slice(-12); let latest=recent[recent.length-1]||{text:'',direction:'unknown'}; for(let i=recent.length-1;i>=0;i--){if(recent[i].direction==='customer'||recent[i].direction==='self'){latest=recent[i];break;}}
 return JSON.stringify({url:location.href||'',title:document.title||'',messages:recent,latest,container_found:true});
})()`, selectorJSON)
}

func buildAgentXianyuReplySendExpression(inputSelector, sendSelector, reply string) string {
	inputJSON := quoteAgentXianyuJS(inputSelector)
	sendJSON := quoteAgentXianyuJS(sendSelector)
	replyJSON := quoteAgentXianyuJS(reply)
	return fmt.Sprintf(`(() => {
 const customInput=%s,customSend=%s,reply=%s;
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const visible=el=>{const r=el.getBoundingClientRect(),s=getComputedStyle(el);return r.width>80&&r.height>18&&r.bottom>0&&r.top<innerHeight&&s.display!=='none'&&s.visibility!=='hidden';};
 let inputs=[]; if(customInput){try{inputs=[...document.querySelectorAll(customInput)];}catch(_){}} else {inputs=[...document.querySelectorAll('textarea,[contenteditable="true"][role="textbox"],[contenteditable="true"],input[type="text"]')];}
 inputs=inputs.filter(visible).filter(el=>{const r=el.getBoundingClientRect();return r.left>innerWidth*.12&&r.left<innerWidth*.78&&r.top>innerHeight*.45;});
 inputs.sort((a,b)=>{const ar=a.getBoundingClientRect(),br=b.getBoundingClientRect();return (br.width+br.top)-(ar.width+ar.top);});
 const input=inputs[0]; if(!input)return JSON.stringify({sent:false,reason:'input_not_found'});
 const current=clean(input.isContentEditable?input.innerText:input.value); if(current)return JSON.stringify({sent:false,reason:'composer_not_empty'});
 input.focus();
 if(input.isContentEditable){const selection=getSelection(),range=document.createRange();range.selectNodeContents(input);selection.removeAllRanges();selection.addRange(range);document.execCommand('insertText',false,reply);if(clean(input.innerText)!==clean(reply))input.textContent=reply;}
 else {const proto=input.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;const setter=Object.getOwnPropertyDescriptor(proto,'value');if(setter&&setter.set)setter.set.call(input,reply);else input.value=reply;}
 input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:reply})); input.dispatchEvent(new Event('change',{bubbles:true}));
 let buttons=[]; if(customSend){try{buttons=[...document.querySelectorAll(customSend)];}catch(_){}} else {buttons=[...document.querySelectorAll('button,[role="button"]')].filter(el=>/^(发送|Send)$/i.test(clean(el.innerText||el.textContent||el.getAttribute('aria-label'))));}
 buttons=buttons.filter(visible).filter(el=>{const ir=input.getBoundingClientRect(),r=el.getBoundingClientRect();return Math.abs((r.top+r.height/2)-(ir.top+ir.height/2))<180&&r.left>ir.left;});
 const button=buttons[0]; if(button&&!button.disabled&&button.getAttribute('aria-disabled')!=='true'){button.click();return JSON.stringify({sent:true,reason:'button'});}
 for(const type of ['keydown','keypress','keyup'])input.dispatchEvent(new KeyboardEvent(type,{key:'Enter',code:'Enter',keyCode:13,which:13,bubbles:true,cancelable:true}));
 return JSON.stringify({sent:true,reason:'enter'});
})()`, inputJSON, sendJSON, replyJSON)
}

func normalizeAgentXianyuReply(value string, maxRunes int) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "```text")
	value = strings.TrimPrefix(value, "```markdown")
	value = strings.TrimPrefix(value, "```")
	value = strings.TrimSuffix(value, "```")
	value = strings.Trim(strings.TrimSpace(value), "\"'“”")
	upper := strings.ToUpper(strings.TrimSpace(value))
	if upper == "SUNAPI_NO_REPLY" || strings.Contains(upper, "SUNAPI_NO_REPLY") {
		return ""
	}
	return truncateRunes(strings.TrimSpace(value), maxRunes)
}

func buildAgentXianyuReplyKnowledgeQuery(latestText string, candidate agentXianyuConversationCandidate, product agentXianyuProductContext) string {
	parts := make([]string, 0, 6)
	for _, value := range []string{product.Title, product.Status, product.Availability, product.Description, product.Text, latestText, candidate.Preview} {
		value = strings.TrimSpace(value)
		if value == "" || slicesContainAgentXianyuReplyString(parts, value) {
			continue
		}
		parts = append(parts, value)
	}
	return truncateRunes(strings.Join(parts, "\n"), 1200)
}

func slicesContainAgentXianyuReplyString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func shouldResumeAgentXianyuReply(status string) bool {
	switch strings.TrimSpace(status) {
	case "model_declined", "no_knowledge_match":
		return true
	default:
		return false
	}
}

func buildAgentXianyuFallbackReply(latestText string, product agentXianyuProductContext, maxRunes int) string {
	message := strings.ToLower(strings.TrimSpace(latestText))
	productText := strings.ToLower(strings.TrimSpace(product.Status + "\n" + product.Availability + "\n" + product.Description + "\n" + product.Text))
	stockQuestion := strings.Contains(message, "有货") || strings.Contains(message, "还有吗") || strings.Contains(message, "能拍") || strings.Contains(message, "可以拍") || strings.Contains(message, "在吗")
	unavailable := strings.Contains(productText, "已删除") || strings.Contains(productText, "已下架") || strings.Contains(productText, "已失效") || strings.Contains(productText, "售罄") || strings.Contains(productText, "无货")
	available := product.Availability == "available" || strings.Contains(productText, "在售") || strings.Contains(productText, "在线") || strings.Contains(productText, "可拍") || strings.Contains(productText, "立即购买")

	reply := "您好，收到您的消息了。请告诉我需要咨询的商品或具体问题，我马上为您处理。"
	if stockQuestion {
		switch {
		case product.Found && unavailable:
			reply = "您好，当前商品页面显示已下架或暂不可拍。请告诉我需要的规格，我帮您确认可用选项。"
		case product.Found && available:
			reply = "您好，当前商品页面显示在售，有货，可以直接拍下。若有具体规格或数量要求，请告诉我。"
		case product.Found:
			reply = "您好，我已看到当前商品信息。请告诉我需要的规格或数量，我马上为您确认。"
		default:
			reply = "您好，请告诉我您咨询的商品名称和规格，我马上为您确认是否可拍。"
		}
	} else if product.Found && strings.TrimSpace(firstNonEmpty(product.Description, product.Text)) != "" {
		description := truncateRunes(strings.TrimSpace(firstNonEmpty(product.Description, product.Text)), 180)
		title := truncateRunes(strings.TrimSpace(product.Title), 80)
		if title != "" {
			reply = fmt.Sprintf("您好，您咨询的是「%s」。当前商品描述：%s 如需确认规格、价格或库存，请告诉我具体需求。", title, description)
		} else {
			reply = fmt.Sprintf("您好，当前商品描述显示：%s 如需确认规格、价格或库存，请告诉我具体需求。", description)
		}
	}
	return truncateRunes(reply, maxReplyRunesOrDefault(maxRunes))
}

func maxReplyRunesOrDefault(value int) int {
	if value > 0 {
		return value
	}
	return agentXianyuReplyDefaultMaxRunes
}

func scheduleAgentXianyuReplyRetry(path string, state *agentXianyuReplyState, identity string, entry agentXianyuReplyConversationState, status string) error {
	entry.Status = status
	entry.RetryAfter = time.Now().Add(agentXianyuReplyRetryDelay).UnixMilli()
	entry.UpdatedAt = time.Now().UnixMilli()
	state.Conversations[identity] = entry
	state.UpdatedAt = entry.UpdatedAt
	return saveAgentXianyuReplyState(path, *state)
}

func agentXianyuReplyStatsOutput(stats *agentXianyuReplyStats, baseIDs []string) map[string]interface{} {
	return map[string]interface{}{
		"mode":               "page_observer_with_poll_fallback",
		"knowledge_base_ids": baseIDs,
		"scan_count":         stats.Scans,
		"detected_count":     stats.Detected,
		"sent_count":         stats.Sent,
		"no_match_count":     stats.NoMatch,
		"skipped_count":      stats.Skipped,
		"model_error_count":  stats.ModelErrors,
		"send_error_count":   stats.SendErrors,
		"reconnect_count":    stats.Reconnects,
		"last_activity_time": stats.LastActivity,
	}
}

func agentXianyuReplyStatePath(dataDir, blueprintID, nodeID string) string {
	return filepath.Join(dataDir, agentXianyuReplyStateDir, sanitizeAgentBrowserFilenamePart(blueprintID), sanitizeAgentBrowserFilenamePart(nodeID)+".json")
}

func loadAgentXianyuReplyState(path string) (agentXianyuReplyState, error) {
	state := agentXianyuReplyState{Conversations: map[string]agentXianyuReplyConversationState{}}
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
		state.Conversations = map[string]agentXianyuReplyConversationState{}
	}
	return state, nil
}

func saveAgentXianyuReplyState(path string, state agentXianyuReplyState) error {
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

func agentXianyuReplyHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])
}

func parseAgentXianyuReplyDuration(value string, fallback, minimum, maximum time.Duration) time.Duration {
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration <= 0 {
		duration = fallback
	}
	if duration < minimum {
		return minimum
	}
	if duration > maximum {
		return maximum
	}
	return duration
}

func parseAgentXianyuReplyBool(value string, fallback bool) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "是", "开启", "处理":
		return true
	case "0", "false", "no", "off", "否", "关闭", "不处理":
		return false
	default:
		return fallback
	}
}
