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
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentXianyuDefaultURL              = "https://seller.goofish.com/?site=COMM"
	agentXianyuDefaultSession          = "闲鱼客服"
	agentXianyuMaxConversations        = 100
	agentXianyuMaxTranscriptRunes      = 12000
	agentXianyuMaxPromptRunes          = 60000
	agentXianyuStateDir                = "agent-xianyu-state"
	agentXianyuDefaultLoginWait        = 180
	agentXianyuDefaultListWait         = 8
	agentXianyuDefaultConversationWait = 1
	agentXianyuDefaultHistoryRounds    = 6
)

var agentXianyuSensitiveDigits = regexp.MustCompile(`\d{7,20}`)
var agentXianyuEmail = regexp.MustCompile(`(?i)[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}`)

type agentXianyuConversation struct {
	Key        string `json:"-"`
	Title      string `json:"title"`
	Preview    string `json:"preview,omitempty"`
	Transcript string `json:"transcript"`
	URL        string `json:"url,omitempty"`
}

type agentXianyuConversationCandidate struct {
	Key     string `json:"key"`
	Title   string `json:"title"`
	Preview string `json:"preview,omitempty"`
}

type agentXianyuCandidatePayload struct {
	URL   string                             `json:"url"`
	Title string                             `json:"title"`
	Items []agentXianyuConversationCandidate `json:"items"`
}

type agentXianyuTranscriptPayload struct {
	URL            string `json:"url"`
	Title          string `json:"title"`
	Text           string `json:"text"`
	ContainerFound bool   `json:"container_found"`
}

type agentXianyuScrollPayload struct {
	Height int `json:"height"`
	Top    int `json:"top"`
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
		return result
	}

	promptConversations := make([]map[string]string, 0, len(changed))
	for index, item := range changed {
		promptConversations = append(promptConversations, map[string]string{
			"会话":   fmt.Sprintf("会话 %d", index+1),
			"对话内容": redactAgentXianyuText(item.Transcript),
		})
	}
	upstream := map[string]interface{}{
		"闲鱼客服对话": promptConversations,
		"来源网址":   sourceURL,
		"会话数量":   len(changed),
	}
	summaryNode := node
	if strings.TrimSpace(summaryNode.Data.Description) == "" {
		summaryNode.Data.Description = "逐条阅读闲鱼客服对话，仅依据事实总结常见问题、有效回复、规则边界、异常处理和待确认事项。删除个人信息，不保存原始对话，不编造结论。"
	}
	summary, err := callLLMForAgentNode(ctx, store, bp, summaryNode, upstream, map[string]interface{}{"knowledge_disabled": true})
	if err != nil {
		return failAgentXianyuNodeWithOutput(result, "闲鱼客服对话总结失败。", err.Error(), map[string]interface{}{
			"error":                      err.Error(),
			"source_url":                 sourceURL,
			"browser_session":            browserSession,
			"scanned_conversation_count": len(conversations),
		})
	}

	filename := fmt.Sprintf("闲鱼客服知识-%s.md", time.Now().Format("20060102-150405.000"))
	documentContent := buildAgentXianyuKnowledgeDocument(sourceURL, browserSession, changed, summary)
	documents, err := writeAgentXianyuKnowledgeDocuments(ctx, store, bases, filename, documentContent)
	if err != nil {
		return failAgentXianyuNodeWithOutput(result, "闲鱼客服知识库写入失败。", err.Error(), map[string]interface{}{
			"error":           err.Error(),
			"source_url":      sourceURL,
			"browser_session": browserSession,
		})
	}
	if err := saveAgentXianyuSyncState(statePath, agentXianyuSyncState{Conversations: nextHashes, UpdatedAt: time.Now().UnixMilli()}); err != nil {
		return failAgentXianyuNodeWithOutput(result, "闲鱼客服知识已写入，但同步状态保存失败。", err.Error(), map[string]interface{}{
			"error":      err.Error(),
			"documents":  documents,
			"source_url": sourceURL,
		})
	}

	result.Message = fmt.Sprintf("闲鱼客服归档完成，读取 %d 个新增或更新会话，并写入 %d 个知识库。", len(changed), len(documents))
	result.Output = map[string]interface{}{
		"source_url":                   sourceURL,
		"browser_session":              browserSession,
		"scanned_conversation_count":   len(conversations),
		"archived_conversation_count":  len(changed),
		"unchanged_conversation_count": unchanged,
		"knowledge_base_count":         len(bases),
		"documents":                    documents,
		"sync_mode":                    firstNonEmpty(node.Data.Settings["同步范围"], "仅归档未归档或有新消息的会话"),
		"raw_transcript_stored":        false,
	}
	return result
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
	candidates, err := evaluateAgentXianyuCandidates(ctx, conn, 10, listSelector, agentXianyuMaxConversations)
	if err != nil {
		return nil, paths.Name, targetURL, err
	}
	if len(candidates.Items) == 0 {
		return nil, paths.Name, targetURL, errors.New("未找到闲鱼客服会话，请确认已登录并打开客服消息列表；必要时填写对话列表选择器")
	}

	limit := parseAgentPositiveSetting(firstNonEmpty(settings["会话数量"], "20"), 20, agentXianyuMaxConversations)
	if limit < len(candidates.Items) {
		candidates.Items = candidates.Items[:limit]
	}
	conversationWait := parseAgentPositiveSetting(firstNonEmpty(settings["单会话等待"], "1s"), agentXianyuDefaultConversationWait, 10)
	historyRounds := parseAgentPositiveSetting(firstNonEmpty(settings["历史加载轮数"], "6"), agentXianyuDefaultHistoryRounds, 12)
	conversations := make([]agentXianyuConversation, 0, len(candidates.Items))
	for index, candidate := range candidates.Items {
		if err := clickAgentXianyuConversation(ctx, conn, 100+index, listSelector, candidate); err != nil {
			continue
		}
		if err := waitAgentXianyu(ctx, settings["单会话等待"], conversationWait); err != nil {
			return nil, paths.Name, targetURL, err
		}
		for round := 0; round < historyRounds; round++ {
			if _, err := evaluateAgentXianyuScroll(ctx, conn, 200+round, messageSelector); err != nil {
				break
			}
			if err := waitAgentXianyu(ctx, "", 1); err != nil {
				return nil, paths.Name, targetURL, err
			}
		}
		payload, err := evaluateAgentXianyuTranscript(ctx, conn, 300+index, messageSelector)
		if err != nil || strings.TrimSpace(payload.Text) == "" {
			continue
		}
		text := truncateRunes(strings.TrimSpace(payload.Text), agentXianyuMaxTranscriptRunes)
		conversations = append(conversations, agentXianyuConversation{
			Key:        candidate.Key,
			Title:      fmt.Sprintf("会话 %d", len(conversations)+1),
			Preview:    truncateRunes(strings.TrimSpace(candidate.Preview), 240),
			Transcript: text,
			URL:        strings.TrimSpace(payload.URL),
		})
		if total := agentXianyuConversationRunes(conversations); total >= agentXianyuMaxPromptRunes {
			break
		}
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

func evaluateAgentXianyuScroll(ctx context.Context, conn *websocket.Conn, id int, selector string) (agentXianyuScrollPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuScrollExpression(selector), agentXianyuScrollPayload{})
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
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s;
 const max = %d;
 const clean = value => String(value || '').replace(/\s+/g, ' ').trim();
 const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 80 && r.height > 24 && r.width < innerWidth * .82 && r.left < innerWidth * .72 && s.display !== 'none' && s.visibility !== 'hidden'; };
 const selectors = custom ? [custom] : ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[role="listitem"]','[role="option"]'];
 let nodes = [];
 for (const query of selectors) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} }
 if (nodes.length < 2 && !custom) nodes = [...document.querySelectorAll('li,button,[role="listitem"],[role="option"]')];
 const seen = new Set(); const items = [];
 nodes.forEach((el, index) => {
   if (!visible(el)) return;
   const text = clean(el.innerText || el.textContent); if (text.length < 2 || text.length > 360) return;
   if (/^(首页|工作台|消息|设置|帮助|客服中心)$/.test(text)) return;
   const key = clean(el.getAttribute('data-conversation-id') || el.getAttribute('data-session-id') || el.getAttribute('href') || text.slice(0, 180));
   if (!key || seen.has(key)) return; seen.add(key);
   el.setAttribute('data-sunapi-xianyu-key', key);
   items.push({ key, title: clean(text.split(/\n/)[0]).slice(0, 120), preview: text.slice(0, 240), score: (el.matches('button,[role="option"],a') ? 5 : 0) + (/(conversation|session|chat|contact)/i.test(el.className || '') ? 4 : 0), index });
 });
 items.sort((a,b) => b.score - a.score); return JSON.stringify({ url: location.href || '', title: document.title || '', items: items.slice(0, max).map(({key,title,preview}) => ({key,title,preview})) });
})()`, selectorJSON, limit)
}

func buildAgentXianyuClickExpression(selector string, candidate agentXianyuConversationCandidate) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	keyJSON := quoteAgentXianyuJS(candidate.Key)
	titleJSON := quoteAgentXianyuJS(candidate.Title)
	return fmt.Sprintf(`(() => {
 const key = %s, title = %s, custom = %s;
 const clean = value => String(value || '').replace(/\s+/g, ' ').trim();
 let el = [...document.querySelectorAll('[data-sunapi-xianyu-key]')].find(item => clean(item.getAttribute('data-sunapi-xianyu-key')) === key);
 if (!el) { const selectors = custom ? [custom] : ['[data-conversation-id]','[data-session-id]','[data-testid*="conversation"]','[class*="conversation-item"]','[class*="session-item"]','[class*="chat-item"]','[class*="contact-item"]','[role="listitem"]','[role="option"]']; let nodes=[]; for (const query of selectors) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} } el = nodes.find(item => clean(item.innerText || item.textContent).includes(title)); }
 if (!el) return JSON.stringify(false); el.scrollIntoView({block:'center'}); el.click(); return JSON.stringify(true);
})()`, keyJSON, titleJSON, selectorJSON)
}

func buildAgentXianyuTranscriptExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s; const clean = value => String(value || '').replace(/\n{3,}/g, '\n\n').replace(/[ \t]+/g, ' ').trim();
 const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 260 && r.height > 100 && s.display !== 'none' && s.visibility !== 'hidden'; };
 const queries = custom ? [custom] : ['[role="log"]','[data-testid*="message-list"]','[class*="message-list"]','[class*="messageList"]','[class*="chat-content"]','[class*="chatContent"]','[class*="conversation-content"]','[class*="conversationContent"]','[class*="im-message-list"]'];
 let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} }
 if (nodes.length === 0) nodes = [...document.querySelectorAll('main,section,article,div')];
 let best = null, bestScore = -1; for (const el of nodes) { if (!visible(el)) continue; const text = clean(el.innerText || el.textContent); if (text.length < 8) continue; const r = el.getBoundingClientRect(); const signal = /(message|chat|conversation|im-|msg)/i.test(el.className || '') ? 80 : 0; const score = signal + Math.min(text.length, 4000) / 20 + Math.min(r.height, 900) / 30 - (r.width > innerWidth * .92 ? 30 : 0); if (score > bestScore) { best = el; bestScore = score; } }
 const text = best ? clean(best.innerText || best.textContent) : ''; return JSON.stringify({url: location.href || '', title: document.title || '', text: text.slice(0, 20000), container_found: !!best});
})()`, selectorJSON)
}

func buildAgentXianyuScrollExpression(selector string) string {
	selectorJSON := quoteAgentXianyuJS(selector)
	return fmt.Sprintf(`(() => {
 const custom = %s; const clean = value => String(value || '').replace(/\s+/g, ' ').trim(); const visible = el => { const r = el.getBoundingClientRect(); const s = getComputedStyle(el); return r.width > 260 && r.height > 100 && s.display !== 'none' && s.visibility !== 'hidden'; }; const queries = custom ? [custom] : ['[role="log"]','[data-testid*="message-list"]','[class*="message-list"]','[class*="messageList"]','[class*="chat-content"]','[class*="chatContent"]','[class*="conversation-content"]','[class*="conversationContent"]','[class*="im-message-list"]']; let nodes=[]; for (const query of queries) { try { nodes.push(...document.querySelectorAll(query)); } catch (_) {} } if (nodes.length === 0) nodes = [...document.querySelectorAll('main,section,article,div')]; let el=null,best=-1; for (const node of nodes) { if (!visible(node)) continue; const text=clean(node.innerText || node.textContent); if (text.length < 8) continue; const r=node.getBoundingClientRect(); const signal=/(message|chat|conversation|im-|msg)/i.test(node.className || '') ? 80 : 0; const score=signal + Math.min(text.length,4000)/20 + Math.min(r.height,900)/30 - (r.width > innerWidth*.92 ? 30 : 0); if (score > best) { el=node; best=score; } } if (!el) return JSON.stringify({height:0,top:0}); let scroll=el; while (scroll.parentElement && scroll.scrollHeight <= scroll.clientHeight + 20) scroll=scroll.parentElement; scroll.scrollTop=0; return JSON.stringify({height:scroll.scrollHeight || 0,top:scroll.scrollTop || 0});
})()`, selectorJSON)
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
	sum := sha256.Sum256([]byte(strings.Join([]string{item.Key, item.Transcript, item.URL}, "\n")))
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

func buildAgentXianyuKnowledgeDocument(sourceURL, browserSession string, conversations []agentXianyuConversation, summary string) string {
	var builder strings.Builder
	builder.WriteString("# 闲鱼客服知识摘要\n\n")
	builder.WriteString("本文件由只读浏览器会话生成，仅保留脱敏后的归纳结论，不包含原始客服对话。\n\n")
	builder.WriteString("- 来源：闲鱼客服后台\n")
	builder.WriteString("- 来源网址：" + sourceURL + "\n")
	builder.WriteString("- 浏览器会话：" + browserSession + "\n")
	builder.WriteString(fmt.Sprintf("- 本次整理会话数：%d\n\n", len(conversations)))
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
	value = agentXianyuEmail.ReplaceAllString(value, "[邮箱已脱敏]")
	value = agentXianyuSensitiveDigits.ReplaceAllString(value, "[数字信息已脱敏]")
	return strings.TrimSpace(value)
}
