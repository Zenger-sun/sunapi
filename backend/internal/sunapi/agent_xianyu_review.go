package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentXianyuReviewStateDir     = "agent-xianyu-review-state"
	agentXianyuReviewDefaultText  = "沟通顺畅，交易愉快。"
	agentXianyuReviewDefaultStars = 5
)

type agentXianyuReviewState struct {
	Reviewed  map[string]int64 `json:"reviewed"`
	UpdatedAt int64            `json:"updated_at"`
}

type agentXianyuReviewStats struct {
	Scanned   int `json:"scanned"`
	Eligible  int `json:"eligible"`
	Submitted int `json:"submitted"`
	Skipped   int `json:"skipped"`
	Failed    int `json:"failed"`
}

func executeAgentXianyuReviewGraphNode(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode, inputVars map[string]interface{}, result agentBlueprintNodeResult) agentBlueprintNodeResult {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return failAgentXianyuNode(result, "闲鱼自动评价无法执行，数据目录未配置。", "data directory is not configured")
	}
	if bp == nil {
		return failAgentXianyuNode(result, "闲鱼自动评价缺少当前智能体上下文。", "agent blueprint is required")
	}
	settings := node.Data.Settings
	statePath := agentXianyuReviewStatePath(store.DataDir(), bp.ID, node.ID)
	state, err := loadAgentXianyuReviewState(statePath)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价状态读取失败。", err.Error())
	}
	result.Input = map[string]interface{}{"source": "闲鱼卖家工作台", "mode": "scan_all_conversations", "review_text": firstNonEmpty(settings["评价内容"], agentXianyuReviewDefaultText)}
	reportAgentExecutionProgress(ctx, map[string]interface{}{"phase": "collecting", "node_id": node.ID, "node_kind": node.Data.Kind, "node_title": node.Data.Title, "message": "正在扫描闲鱼全部会话和订单状态…"})

	targetURL, err := normalizeAgentXianyuURL(firstNonEmpty(settings["网址"], settings["URL"], agentXianyuDefaultURL))
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价网址无效。", err.Error())
	}
	session := firstNonEmpty(settings["会话名称"], agentXianyuDefaultSession)
	paths := resolveAgentBrowserSessionPaths(store.DataDir(), session)
	endpoint, err := ensureAgentBrowserDevtoolsForSession(ctx, paths)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法连接浏览器。", err.Error())
	}
	target, _, err := openAgentBrowserTarget(ctx, endpoint, targetURL)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法打开卖家工作台。", err.Error())
	}
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, target.WebSocketDebuggerURL, nil)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法连接浏览器调试通道。", err.Error())
	}
	defer conn.Close()
	if _, err = agentBrowserCDPCall(ctx, conn, 1, "Page.enable", nil); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价启用页面通道失败。", err.Error())
	}
	if _, err = agentBrowserCDPCall(ctx, conn, 2, "Runtime.enable", nil); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价启用脚本通道失败。", err.Error())
	}
	loginWait := parseAgentPositiveSetting(firstNonEmpty(settings["登录等待"], "180s"), agentXianyuDefaultLoginWait, 300)
	if loginWait > 0 && !strings.Contains(settings["登录态"], "无需登录") {
		if err = waitAgentBrowserLogin(ctx, conn, loginWait); err != nil {
			return failAgentXianyuNode(result, "闲鱼自动评价登录等待失败。", err.Error())
		}
	}
	if err = waitAgentXianyu(ctx, settings["列表等待"], agentXianyuDefaultListWait); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价等待页面失败。", err.Error())
	}

	listSelector := firstNonEmpty(settings["对话列表选择器"], settings["会话列表选择器"])
	orderSelector := settings["订单区域选择器"]
	entrySelector := settings["评价入口选择器"]
	inputSelector := settings["评价输入框选择器"]
	submitSelector := settings["评价提交按钮选择器"]
	limit, all := parseAgentXianyuConversationLimitWithCeiling(settings["会话数量"], agentXianyuMaxConversations)
	conversationWait := parseAgentPositiveSetting(firstNonEmpty(settings["单会话等待"], "1s"), agentXianyuDefaultConversationWait, 10)
	callID := 1000
	nextID := func() int { callID++; return callID }
	reset, err := evaluateAgentXianyuConversationListReset(ctx, conn, nextID(), listSelector)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法定位会话列表。", err.Error())
	}
	batch, err := evaluateAgentXianyuCandidates(ctx, conn, 10, listSelector, agentXianyuMaxConversations)
	if err != nil || len(batch.Items) == 0 {
		if err == nil {
			err = errors.New("未找到闲鱼客服会话")
		}
		return failAgentXianyuNode(result, "闲鱼自动评价未找到可扫描的会话。", err.Error())
	}
	queue := make([]agentXianyuConversationCandidate, 0, len(batch.Items))
	seen := map[string]struct{}{}
	appendCandidates := func(items []agentXianyuConversationCandidate) {
		for _, candidate := range items {
			candidate.Key = agentXianyuCandidateKey(candidate)
			if candidate.Key == "" {
				continue
			}
			if _, ok := seen[candidate.Key]; ok {
				continue
			}
			seen[candidate.Key] = struct{}{}
			queue = append(queue, candidate)
			if len(queue) >= limit {
				return
			}
		}
	}
	appendCandidates(batch.Items)
	listTop := reset.Top
	listExhausted := false
	listNoProgress := 0
	stats := agentXianyuReviewStats{}
	report := func(message string) {
		reportAgentExecutionProgress(ctx, map[string]interface{}{"phase": "reviewing", "node_id": node.ID, "node_kind": node.Data.Kind, "node_title": node.Data.Title, "message": message, "scanned": stats.Scanned, "eligible": stats.Eligible, "submitted": stats.Submitted, "skipped": stats.Skipped, "failed": stats.Failed})
	}
	for index := 0; all || index < limit; {
		if err := ctx.Err(); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价已中断。", err.Error(), stats)
		}
		if index >= len(queue) {
			if listExhausted {
				break
			}
			nextBatch, scrollErr := evaluateAgentXianyuCandidates(ctx, conn, nextID(), listSelector, agentXianyuMaxConversations)
			if scrollErr == nil {
				before := len(queue)
				appendCandidates(nextBatch.Items)
				if len(queue) > before {
					continue
				}
			}
			position, scrollErr := evaluateAgentXianyuConversationListScroll(ctx, conn, nextID(), listSelector)
			if scrollErr != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价滚动会话列表失败。", scrollErr.Error(), stats)
			}
			if position.AtEnd {
				listNoProgress++
				if listNoProgress >= agentXianyuConversationListEndConfirmations {
					listExhausted = true
					break
				}
			} else if position.Moved && position.Top > listTop {
				listTop = position.Top
				listNoProgress = 0
			} else {
				// A virtualized list may need another render tick before its scrollTop changes.
				listNoProgress++
				if listNoProgress >= agentXianyuConversationListStallLimit {
					return failAgentXianyuReviewWithStats(result, "闲鱼自动评价会话列表连续无法向后滚动。", "conversation list did not advance", stats)
				}
			}
			if err := waitAgentXianyuDOM(ctx); err != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待列表更新失败。", err.Error(), stats)
			}
			continue
		}
		candidate := queue[index]
		index++
		if _, err := evaluateAgentXianyuConversationListRestore(ctx, conn, nextID(), listSelector, listTop); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价恢复列表位置失败。", err.Error(), stats)
		}
		if err := clickAgentXianyuConversation(ctx, conn, nextID(), listSelector, candidate); err != nil {
			stats.Skipped++
			continue
		}
		if err := waitAgentXianyu(ctx, settings["单会话等待"], conversationWait); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待订单区域失败。", err.Error(), stats)
		}
		stats.Scanned++
		order, inspectErr := evaluateAgentXianyuReview(ctx, conn, nextID(), orderSelector, entrySelector, candidate.Key)
		if inspectErr != nil {
			stats.Failed++
			report(fmt.Sprintf("第 %d 个会话评价检查失败。", stats.Scanned))
			continue
		}
		if !order.Eligible || order.OrderKey == "" {
			stats.Skipped++
			report(fmt.Sprintf("第 %d 个会话没有明确可评价订单，已跳过。", stats.Scanned))
			continue
		}
		stats.Eligible++
		if _, ok := state.Reviewed[order.OrderKey]; ok {
			stats.Skipped++
			report(fmt.Sprintf("第 %d 个会话订单已评价，已跳过。", stats.Scanned))
			continue
		}
		text := firstNonEmpty(settings["评价内容"], agentXianyuReviewDefaultText)
		stars := parseAgentPositiveSetting(settings["评价星级"], agentXianyuReviewDefaultStars, 5)
		if err := evaluateAgentXianyuReviewSubmit(ctx, conn, nextID(), orderSelector, inputSelector, submitSelector, text, stars); err != nil {
			stats.Failed++
			report(fmt.Sprintf("第 %d 个会话评价提交失败。", stats.Scanned))
			continue
		}
		state.Reviewed[order.OrderKey] = time.Now().UnixMilli()
		state.UpdatedAt = time.Now().UnixMilli()
		if err := saveAgentXianyuReviewState(statePath, state); err != nil {
			stats.Failed++
			report(fmt.Sprintf("第 %d 个会话评价状态保存失败。", stats.Scanned))
			continue
		}
		stats.Submitted++
		report(fmt.Sprintf("已完成第 %d 个会话的订单评价。", stats.Scanned))
	}
	result.Message = fmt.Sprintf("闲鱼自动评价完成，扫描 %d 个会话，找到 %d 个可评价订单，成功评价 %d 个，跳过 %d 个，失败 %d 个。", stats.Scanned, stats.Eligible, stats.Submitted, stats.Skipped, stats.Failed)
	result.Output = map[string]interface{}{"scanned_conversation_count": stats.Scanned, "eligible_order_count": stats.Eligible, "submitted_review_count": stats.Submitted, "skipped_count": stats.Skipped, "failed_count": stats.Failed, "review_text": firstNonEmpty(settings["评价内容"], agentXianyuReviewDefaultText), "review_stars": parseAgentPositiveSetting(settings["评价星级"], agentXianyuReviewDefaultStars, 5)}
	return result
}

type agentXianyuReviewOrder struct {
	Eligible        bool   `json:"eligible"`
	AlreadyReviewed bool   `json:"already_reviewed"`
	OrderKey        string `json:"order_key"`
}

func evaluateAgentXianyuReview(ctx context.Context, conn *websocket.Conn, id int, orderSelector, entrySelector, fallbackKey string) (agentXianyuReviewOrder, error) {
	raw, err := agentBrowserCDPCall(ctx, conn, id, "Runtime.evaluate", map[string]any{"expression": buildAgentXianyuReviewInspectExpression(orderSelector, entrySelector), "returnByValue": true})
	if err != nil {
		return agentXianyuReviewOrder{}, err
	}
	var response struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return agentXianyuReviewOrder{}, err
	}
	var result agentXianyuReviewOrder
	if err := json.Unmarshal([]byte(response.Result.Value), &result); err != nil {
		return agentXianyuReviewOrder{}, err
	}
	if result.OrderKey == "" {
		result.OrderKey = strings.TrimSpace(fallbackKey)
	}
	return result, nil
}

func evaluateAgentXianyuReviewSubmit(ctx context.Context, conn *websocket.Conn, id int, orderSelector, inputSelector, submitSelector, text string, stars int) error {
	raw, err := agentBrowserCDPCall(ctx, conn, id, "Runtime.evaluate", map[string]any{"expression": buildAgentXianyuReviewSubmitExpression(orderSelector, inputSelector, submitSelector, text, stars), "returnByValue": true})
	if err != nil {
		return err
	}
	var response struct {
		Result struct {
			Value string `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return err
	}
	var payload struct {
		Submitted bool   `json:"submitted"`
		Reason    string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(response.Result.Value), &payload); err != nil {
		return err
	}
	if !payload.Submitted {
		return errors.New(firstNonEmpty(payload.Reason, "评价提交按钮未找到"))
	}
	return nil
}

func buildAgentXianyuReviewInspectExpression(orderSelector, entrySelector string) string {
	orderJSON, entryJSON := quoteAgentXianyuJS(orderSelector), quoteAgentXianyuJS(entrySelector)
	return fmt.Sprintf(`(() => { const custom=%s,entryCustom=%s; const clean=v=>String(v||'').replace(/\s+/g,' ').trim(); const visible=e=>{if(!e)return false;const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>20&&r.height>12&&r.right>innerWidth*.45&&r.bottom>0&&r.top<innerHeight&&s.display!=='none'&&s.visibility!=='hidden';}; let root=null;if(custom){try{root=document.querySelector(custom)}catch(_){}} if(!root){const all=[...document.querySelectorAll('body *')].filter(visible);root=all.sort((a,b)=>{const ar=a.getBoundingClientRect(),br=b.getBoundingClientRect();return (br.left+br.width*br.height)-(ar.left+ar.width*ar.height)})[0]||document.body;} const text=clean(root?.innerText||''); const orderMatch=text.match(/(?:订单编号|订单号)\s*[:：]?\s*([A-Za-z0-9_-]{6,})/); let entries=[];if(entryCustom){try{entries=[...document.querySelectorAll(entryCustom)]}catch(_){} } if(!entries.length){entries=[...document.querySelectorAll('button,[role="button"],a')].filter(visible).filter(e=>/^(去评价|立即评价|待评价|评价商品|评价)$/i.test(clean(e.innerText||e.textContent||e.getAttribute('aria-label'))));} const available=entries.find(e=>/^(去评价|立即评价|待评价|评价商品)$/i.test(clean(e.innerText||e.textContent||e.getAttribute('aria-label')))); const already=/(已评价|评价完成|已追评)/.test(text); return JSON.stringify({eligible:Boolean(available)&&!already,already_reviewed:already,order_key:orderMatch?orderMatch[1]:''}); })()`, orderJSON, entryJSON)
}

func buildAgentXianyuReviewSubmitExpression(orderSelector, inputSelector, submitSelector, text string, stars int) string {
	orderJSON, inputJSON, submitJSON, textJSON := quoteAgentXianyuJS(orderSelector), quoteAgentXianyuJS(inputSelector), quoteAgentXianyuJS(submitSelector), quoteAgentXianyuJS(text)
	return fmt.Sprintf(`(() => { const customRoot=%s,customInput=%s,customSubmit=%s,review=%s,stars=%d; const clean=v=>String(v||'').replace(/\s+/g,' ').trim(); const visible=e=>{if(!e)return false;const r=e.getBoundingClientRect(),s=getComputedStyle(e);return r.width>80&&r.height>18&&r.bottom>0&&r.top<innerHeight&&s.display!=='none'&&s.visibility!=='hidden';}; let root=null;if(customRoot){try{root=document.querySelector(customRoot)}catch(_){}} const scope=root||document; let inputs=[];if(customInput){try{inputs=[...scope.querySelectorAll(customInput)]}catch(_){} }if(!inputs.length)inputs=[...scope.querySelectorAll('textarea,[contenteditable="true"],[placeholder*="评价"],[placeholder*="评论"]')];inputs=inputs.filter(visible);const input=inputs[0];if(!input)return JSON.stringify({submitted:false,reason:'review_input_not_found'});input.focus();if(input.isContentEditable){input.textContent=review;input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:review}));}else{const proto=input.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;const setter=Object.getOwnPropertyDescriptor(proto,'value');if(setter&&setter.set)setter.set.call(input,review);else input.value=review;input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:review}));input.dispatchEvent(new Event('change',{bubbles:true}));} const starNodes=[...scope.querySelectorAll('[aria-label*="星"],[title*="星"],[data-score],[data-rating],[class*="star"],[class*="Star"]')].filter(visible);if(starNodes.length&&stars>0){const target=starNodes[Math.min(starNodes.length,stars)-1];try{target.click()}catch(_){} }let buttons=[];if(customSubmit){try{buttons=[...scope.querySelectorAll(customSubmit)]}catch(_){} }if(!buttons.length)buttons=[...scope.querySelectorAll('button,[role="button"],a')].filter(visible).filter(e=>/^(提交评价|发布评价|发表评价|提交)$/i.test(clean(e.innerText||e.textContent||e.getAttribute('aria-label'))));const button=buttons[0];if(!button)return JSON.stringify({submitted:false,reason:'review_submit_not_found'});button.click();return JSON.stringify({submitted:true}); })()`, orderJSON, inputJSON, submitJSON, textJSON, stars)
}

func failAgentXianyuReviewWithStats(result agentBlueprintNodeResult, message, errText string, stats agentXianyuReviewStats) agentBlueprintNodeResult {
	result.Status = "failed"
	result.Message = message
	result.Error = errText
	result.Output = map[string]interface{}{"scanned_conversation_count": stats.Scanned, "eligible_order_count": stats.Eligible, "submitted_review_count": stats.Submitted, "skipped_count": stats.Skipped, "failed_count": stats.Failed, "error": errText}
	return result
}

func agentXianyuReviewStatePath(dataDir, blueprintID, nodeID string) string {
	return filepath.Join(dataDir, agentXianyuReviewStateDir, sanitizeAgentBrowserFilenamePart(blueprintID), sanitizeAgentBrowserFilenamePart(nodeID)+".json")
}
func loadAgentXianyuReviewState(path string) (agentXianyuReviewState, error) {
	state := agentXianyuReviewState{Reviewed: map[string]int64{}}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err = json.Unmarshal(raw, &state); err != nil {
		return state, err
	}
	if state.Reviewed == nil {
		state.Reviewed = map[string]int64{}
	}
	return state, nil
}
func saveAgentXianyuReviewState(path string, state agentXianyuReviewState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	if err = os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
