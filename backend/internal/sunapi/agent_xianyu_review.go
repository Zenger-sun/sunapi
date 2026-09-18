package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	agentXianyuReviewStateDir         = "agent-xianyu-review-state"
	agentXianyuReviewDefaultURL       = "https://seller.goofish.com/?site=COMMONPRO#/seller-trade/evaluation-manage"
	agentXianyuReviewRoute            = "/seller-trade/evaluation-manage"
	agentXianyuReviewDefaultText      = "沟通顺利，交易愉快"
	agentXianyuReviewDefaultWait      = 1
	agentXianyuReviewDefaultMaxOrders = 2000
	agentXianyuReviewMaxOrders        = 5000
	agentXianyuReviewConfirmAttempts  = 6
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

type agentXianyuReviewPendingTabPayload struct {
	Found   bool   `json:"found"`
	Clicked bool   `json:"clicked"`
	Reason  string `json:"reason"`
}

type agentXianyuReviewCandidatePayload struct {
	Found             bool     `json:"found"`
	OrderKey          string   `json:"order_key"`
	VisibleCount      int      `json:"visible_count"`
	Signature         string   `json:"signature"`
	ReviewedOrderKeys []string `json:"reviewed_order_keys"`
	Reason            string   `json:"reason"`
}

type agentXianyuReviewActionPayload struct {
	Submitted bool   `json:"submitted"`
	Reason    string `json:"reason"`
}

type agentXianyuReviewConfirmationPayload struct {
	Confirmed bool   `json:"confirmed"`
	Failed    bool   `json:"failed"`
	Reason    string `json:"reason"`
}

type agentXianyuReviewNextPagePayload struct {
	Clicked bool   `json:"clicked"`
	AtEnd   bool   `json:"at_end"`
	Reason  string `json:"reason"`
}

func executeAgentXianyuReviewGraphNode(ctx context.Context, store *Store, bp *AgentBlueprint, node agentBlueprintGraphNode, inputVars map[string]interface{}, result agentBlueprintNodeResult) agentBlueprintNodeResult {
	if store == nil || strings.TrimSpace(store.DataDir()) == "" {
		return failAgentXianyuNode(result, "闲鱼自动评价无法执行，数据目录未配置。", "data directory is not configured")
	}
	if bp == nil {
		return failAgentXianyuNode(result, "闲鱼自动评价缺少当前智能体上下文。", "agent blueprint is required")
	}

	settings := node.Data.Settings
	reviewText := normalizeAgentXianyuReviewText(settings["评价内容"])
	statePath := agentXianyuReviewStatePath(store.DataDir(), bp.ID, node.ID)
	state, err := loadAgentXianyuReviewState(statePath)
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价状态读取失败。", err.Error())
	}
	result.Input = map[string]interface{}{
		"source":      "闲鱼评价管理",
		"mode":        "pending_seller_reviews",
		"review_text": reviewText,
	}
	reportAgentExecutionProgress(ctx, map[string]interface{}{
		"phase":      "collecting",
		"node_id":    node.ID,
		"node_kind":  node.Data.Kind,
		"node_title": node.Data.Title,
		"message":    "正在打开闲鱼评价管理并读取待卖家评价订单…",
	})

	targetURL, err := normalizeAgentXianyuReviewURL(firstNonEmpty(settings["网址"], settings["URL"], agentXianyuReviewDefaultURL))
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
		return failAgentXianyuNode(result, "闲鱼自动评价无法打开评价管理。", err.Error())
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
	// A reusable seller-console target may currently show the chat page. Always
	// navigate it to evaluation management before looking for pending reviews.
	if _, err = agentBrowserCDPCall(ctx, conn, 3, "Page.navigate", map[string]any{"url": targetURL}); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法进入评价管理。", err.Error())
	}
	loginWait := parseAgentPositiveSetting(firstNonEmpty(settings["登录等待"], "180s"), agentXianyuDefaultLoginWait, 300)
	if loginWait > 0 && !strings.Contains(settings["登录态"], "无需登录") {
		if err = waitAgentBrowserLogin(ctx, conn, loginWait); err != nil {
			return failAgentXianyuNode(result, "闲鱼自动评价登录等待失败。", err.Error())
		}
	}
	if err = waitAgentXianyu(ctx, settings["列表等待"], agentXianyuDefaultListWait); err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价等待评价管理页面失败。", err.Error())
	}

	callID := 1000
	nextID := func() int { callID++; return callID }
	pendingTab, err := evaluateAgentXianyuReviewPendingTab(ctx, conn, nextID())
	if err != nil {
		return failAgentXianyuNode(result, "闲鱼自动评价无法读取评价管理页签。", err.Error())
	}
	if !pendingTab.Found {
		return failAgentXianyuNode(result, "闲鱼自动评价未找到“待卖家评价”页签。", firstNonEmpty(pendingTab.Reason, "pending_seller_review_tab_not_found"))
	}
	if pendingTab.Clicked {
		if err = waitAgentXianyuDOM(ctx); err != nil {
			return failAgentXianyuNode(result, "闲鱼自动评价等待待评价列表失败。", err.Error())
		}
	}

	listSelector := settings["订单列表选择器"]
	actionSelector := settings["评价按钮选择器"]
	dialogSelector := settings["评价弹窗选择器"]
	inputSelector := settings["评价输入框选择器"]
	ratingSelector := settings["好评选项选择器"]
	submitSelector := settings["评价提交按钮选择器"]
	ceiling := parseAgentPositiveSetting(settings["最大评价保护上限"], agentXianyuReviewDefaultMaxOrders, agentXianyuReviewMaxOrders)
	limit, _ := parseAgentXianyuReviewLimit(settings["评价数量"], ceiling)
	reviewWait := parseAgentPositiveSetting(settings["单次评价等待"], agentXianyuReviewDefaultWait, 10)

	stats := agentXianyuReviewStats{}
	attempted := map[string]struct{}{}
	seenSkipped := map[string]struct{}{}
	navigatedSignatures := map[string]struct{}{}
	report := func(message string) {
		reportAgentExecutionProgress(ctx, map[string]interface{}{
			"phase":      "reviewing",
			"node_id":    node.ID,
			"node_kind":  node.Data.Kind,
			"node_title": node.Data.Title,
			"message":    message,
			"scanned":    stats.Scanned,
			"eligible":   stats.Eligible,
			"submitted":  stats.Submitted,
			"skipped":    stats.Skipped,
			"failed":     stats.Failed,
		})
	}

	for stats.Scanned < limit {
		if err := ctx.Err(); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价已中断。", err.Error(), stats)
		}
		candidate, inspectErr := evaluateAgentXianyuReviewOpenNext(ctx, conn, nextID(), listSelector, actionSelector, state.Reviewed, attempted)
		if inspectErr != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价读取待评价订单失败。", inspectErr.Error(), stats)
		}
		for _, orderKey := range candidate.ReviewedOrderKeys {
			if _, seen := seenSkipped[orderKey]; seen {
				continue
			}
			seenSkipped[orderKey] = struct{}{}
			stats.Skipped++
		}
		if !candidate.Found {
			signature := strings.TrimSpace(candidate.Signature)
			if signature == "" {
				signature = fmt.Sprintf("empty:%d:%d", stats.Scanned, candidate.VisibleCount)
			}
			if _, alreadyNavigated := navigatedSignatures[signature]; alreadyNavigated {
				break
			}
			navigatedSignatures[signature] = struct{}{}
			nextPage, pageErr := evaluateAgentXianyuReviewNextPage(ctx, conn, nextID())
			if pageErr != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价翻页失败。", pageErr.Error(), stats)
			}
			if !nextPage.Clicked || nextPage.AtEnd {
				break
			}
			if err := waitAgentXianyuDOM(ctx); err != nil {
				return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待下一页失败。", err.Error(), stats)
			}
			continue
		}

		orderKey := strings.TrimSpace(candidate.OrderKey)
		stats.Scanned++
		if orderKey == "" {
			stats.Failed++
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价无法确认待评价订单编号。", "review_order_key_not_found", stats)
		}
		attempted[orderKey] = struct{}{}
		stats.Eligible++
		report(fmt.Sprintf("正在评价订单 %s。", orderKey))

		if err := waitAgentXianyu(ctx, fmt.Sprintf("%ds", reviewWait), reviewWait); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼自动评价等待评价表单失败。", err.Error(), stats)
		}
		submit, submitErr := evaluateAgentXianyuReviewSubmit(ctx, conn, nextID(), dialogSelector, inputSelector, ratingSelector, submitSelector, reviewText)
		if submitErr != nil || !submit.Submitted {
			stats.Failed++
			reason := submit.Reason
			if submitErr != nil {
				reason = submitErr.Error()
			}
			report(fmt.Sprintf("订单 %s 评价提交失败：%s。", orderKey, firstNonEmpty(reason, "unknown_error")))
			continue
		}
		confirmation, confirmErr := confirmAgentXianyuReviewSubmission(ctx, conn, nextID, inputSelector, orderKey, reviewWait)
		if confirmErr != nil || !confirmation.Confirmed {
			stats.Failed++
			reason := confirmation.Reason
			if confirmErr != nil {
				reason = confirmErr.Error()
			}
			report(fmt.Sprintf("订单 %s 未确认评价成功：%s。", orderKey, firstNonEmpty(reason, "review_confirmation_timeout")))
			continue
		}

		now := time.Now().UnixMilli()
		state.Reviewed[orderKey] = now
		state.UpdatedAt = now
		seenSkipped[orderKey] = struct{}{}
		stats.Submitted++
		if err := saveAgentXianyuReviewState(statePath, state); err != nil {
			return failAgentXianyuReviewWithStats(result, "闲鱼评价已提交，但本地防重复状态保存失败。", err.Error(), stats)
		}
		report(fmt.Sprintf("订单 %s 已完成评价。", orderKey))
	}

	result.Message = fmt.Sprintf("闲鱼自动评价完成，从评价管理扫描 %d 个待卖家评价订单，成功评价 %d 个，跳过 %d 个，失败 %d 个。", stats.Scanned, stats.Submitted, stats.Skipped, stats.Failed)
	result.Output = map[string]interface{}{
		"source":                 "闲鱼评价管理",
		"scanned_order_count":    stats.Scanned,
		"eligible_order_count":   stats.Eligible,
		"submitted_review_count": stats.Submitted,
		"skipped_count":          stats.Skipped,
		"failed_count":           stats.Failed,
		"review_text":            reviewText,
		"review_rating":          "好评",
	}
	return result
}

func normalizeAgentXianyuReviewURL(raw string) (string, error) {
	value, err := normalizeAgentXianyuURL(firstNonEmpty(raw, agentXianyuReviewDefaultURL))
	if err != nil {
		return "", err
	}
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	query := parsed.Query()
	query.Set("site", "COMMONPRO")
	parsed.RawQuery = query.Encode()
	parsed.Fragment = agentXianyuReviewRoute
	return parsed.String(), nil
}

func parseAgentXianyuReviewLimit(value string, ceiling int) (limit int, all bool) {
	if ceiling <= 0 {
		ceiling = agentXianyuReviewDefaultMaxOrders
	}
	raw := strings.ToLower(strings.TrimSpace(value))
	switch raw {
	case "", "0", "all", "unlimited", "全部", "不限", "所有", "全部评价", "所有评价":
		return ceiling, true
	default:
		return parseAgentPositiveSetting(raw, 20, ceiling), false
	}
}

func normalizeAgentXianyuReviewText(_ string) string {
	return agentXianyuReviewDefaultText
}

func evaluateAgentXianyuReviewPendingTab(ctx context.Context, conn *websocket.Conn, id int) (agentXianyuReviewPendingTabPayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewPendingTabExpression(), agentXianyuReviewPendingTabPayload{})
}

func evaluateAgentXianyuReviewOpenNext(ctx context.Context, conn *websocket.Conn, id int, listSelector, actionSelector string, reviewed map[string]int64, attempted map[string]struct{}) (agentXianyuReviewCandidatePayload, error) {
	expression := buildAgentXianyuReviewOpenNextExpression(listSelector, actionSelector, agentXianyuReviewMapKeys(reviewed), agentXianyuReviewSetKeys(attempted))
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewCandidatePayload{})
}

func evaluateAgentXianyuReviewSubmit(ctx context.Context, conn *websocket.Conn, id int, dialogSelector, inputSelector, ratingSelector, submitSelector, text string) (agentXianyuReviewActionPayload, error) {
	expression := buildAgentXianyuReviewSubmitExpression(dialogSelector, inputSelector, ratingSelector, submitSelector, text)
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewActionPayload{})
}

func evaluateAgentXianyuReviewConfirmation(ctx context.Context, conn *websocket.Conn, id int, inputSelector, orderKey string) (agentXianyuReviewConfirmationPayload, error) {
	expression := buildAgentXianyuReviewConfirmationExpression(inputSelector, orderKey)
	return evaluateAgentXianyuJSON(ctx, conn, id, expression, agentXianyuReviewConfirmationPayload{})
}

func confirmAgentXianyuReviewSubmission(ctx context.Context, conn *websocket.Conn, nextID func() int, inputSelector, orderKey string, waitSeconds int) (agentXianyuReviewConfirmationPayload, error) {
	waitDuration := agentXianyuDOMWait
	if waitSeconds > 0 {
		waitDuration = time.Duration(waitSeconds) * time.Second
	}
	var last agentXianyuReviewConfirmationPayload
	for attempt := 0; attempt < agentXianyuReviewConfirmAttempts; attempt++ {
		if err := waitAgentXianyuDuration(ctx, waitDuration); err != nil {
			return last, err
		}
		payload, err := evaluateAgentXianyuReviewConfirmation(ctx, conn, nextID(), inputSelector, orderKey)
		if err != nil {
			return last, err
		}
		last = payload
		if payload.Confirmed || payload.Failed {
			return payload, nil
		}
		waitDuration = agentXianyuDOMWait
	}
	return last, nil
}

func evaluateAgentXianyuReviewNextPage(ctx context.Context, conn *websocket.Conn, id int) (agentXianyuReviewNextPagePayload, error) {
	return evaluateAgentXianyuJSON(ctx, conn, id, buildAgentXianyuReviewNextPageExpression(), agentXianyuReviewNextPagePayload{})
}

func buildAgentXianyuReviewPendingTabExpression() string {
	return `(() => {
 const clean = value => String(value || '').replace(/\s+/g, ' ').trim();
 const displayed = element => { if (!element) return false; const rect = element.getBoundingClientRect(); const style = getComputedStyle(element); return rect.width > 0 && rect.height > 0 && style.display !== 'none' && style.visibility !== 'hidden'; };
 const elements = [...document.querySelectorAll('[role="tab"],button,a,[role="button"],div,span')].filter(displayed);
 const label = elements.find(element => /^待卖家评价\s*\d*$/.test(clean(element.innerText || element.textContent)));
 if (!label) return JSON.stringify({found:false,clicked:false,reason:'pending_seller_review_tab_not_found'});
 const target = label.closest('[role="tab"],button,a,[role="button"]') || label;
 target.scrollIntoView({block:'center',inline:'nearest'});
 target.click();
 return JSON.stringify({found:true,clicked:true,reason:''});
})()`
}

func buildAgentXianyuReviewOpenNextExpression(listSelector, actionSelector string, reviewedKeys, attemptedKeys []string) string {
	listJSON := quoteAgentXianyuJS(listSelector)
	actionJSON := quoteAgentXianyuJS(actionSelector)
	reviewedJSON, _ := json.Marshal(reviewedKeys)
	attemptedJSON, _ := json.Marshal(attemptedKeys)
	return fmt.Sprintf(`(() => {
 const customList=%s,customAction=%s,reviewed=new Set(%s),attempted=new Set(%s);
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
 const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|\s)(disabled|is-disabled)(\s|$)/i.test(String(element.className||''));
 const extractOrder=text=>{const match=clean(text).match(/(?:订单编号|订单号)\s*[:：]?\s*([A-Za-z0-9_-]{8,})/);return match?match[1]:'';};
 let scope=document;
 if(customList){try{scope=document.querySelector(customList)||document;}catch(_){return JSON.stringify({found:false,visible_count:0,signature:'',reviewed_order_keys:[],reason:'invalid_order_list_selector'});}}
 let rawActions=[];
 if(customAction){try{rawActions=[...scope.querySelectorAll(customAction)];}catch(_){return JSON.stringify({found:false,visible_count:0,signature:'',reviewed_order_keys:[],reason:'invalid_review_action_selector'});}}
 if(!rawActions.length){
   rawActions=[...scope.querySelectorAll('button,a,[role="button"]')].filter(element=>/^(去评价|立即评价|评价商品|评价)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label'))));
   const textActions=[...scope.querySelectorAll('span,div')].filter(element=>element.childElementCount===0&&/^(去评价|立即评价|评价商品|评价)$/.test(clean(element.textContent)));
   rawActions.push(...textActions.map(element=>element.closest('button,a,[role="button"]')||element));
 }
 const actions=[...new Set(rawActions)].filter(element=>displayed(element)&&!disabled(element));
 const markerCandidates=[...scope.querySelectorAll('div,span,p')].filter(element=>{const text=clean(element.innerText||element.textContent);return displayed(element)&&text.length<160&&/(?:订单编号|订单号)\s*[:：]?\s*[A-Za-z0-9_-]{8,}/.test(text);});
 const orderForAction=action=>{
   let current=action;
   for(let depth=0;current&&depth<12;depth+=1,current=current.parentElement){
     const text=clean(current.innerText||current.textContent);
     const matches=[...text.matchAll(/(?:订单编号|订单号)\s*[:：]?\s*([A-Za-z0-9_-]{8,})/g)].map(match=>match[1]);
     const unique=[...new Set(matches)];
     if(unique.length===1&&text.length<8000)return unique[0];
     if(current===scope)break;
   }
   const actionRect=action.getBoundingClientRect();
   const nearby=markerCandidates.map(element=>({key:extractOrder(element.innerText||element.textContent),rect:element.getBoundingClientRect()})).filter(item=>item.key&&item.rect.top<=actionRect.bottom+40).sort((left,right)=>Math.abs(actionRect.top-left.rect.top)-Math.abs(actionRect.top-right.rect.top));
   return nearby.length?nearby[0].key:'';
 };
 const items=[];
 for(const action of actions){const key=orderForAction(action);if(key&&!items.some(item=>item.key===key))items.push({key,action});}
 const keys=items.map(item=>item.key);
 const reviewedOnPage=keys.filter(key=>reviewed.has(key));
 const candidate=items.find(item=>!reviewed.has(item.key)&&!attempted.has(item.key));
 const signature=keys.join('|');
 if(!candidate)return JSON.stringify({found:false,order_key:'',visible_count:items.length,signature,reviewed_order_keys:reviewedOnPage,reason:items.length?'all_visible_orders_already_processed':'no_pending_review_action'});
 candidate.action.scrollIntoView({block:'center',inline:'nearest'});
 candidate.action.click();
 return JSON.stringify({found:true,order_key:candidate.key,visible_count:items.length,signature,reviewed_order_keys:reviewedOnPage,reason:''});
})()`, listJSON, actionJSON, string(reviewedJSON), string(attemptedJSON))
}

func buildAgentXianyuReviewSubmitExpression(dialogSelector, inputSelector, ratingSelector, submitSelector, text string) string {
	dialogJSON := quoteAgentXianyuJS(dialogSelector)
	inputJSON := quoteAgentXianyuJS(inputSelector)
	ratingJSON := quoteAgentXianyuJS(ratingSelector)
	submitJSON := quoteAgentXianyuJS(submitSelector)
	textJSON := quoteAgentXianyuJS(text)
	return fmt.Sprintf(`(() => {
 const customDialog=%s,customInput=%s,customRating=%s,customSubmit=%s,review=%s;
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
 const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|\s)(disabled|is-disabled)(\s|$)/i.test(String(element.className||''));
 let root=null;
 if(customDialog){try{root=[...document.querySelectorAll(customDialog)].filter(displayed).pop()||null;}catch(_){return JSON.stringify({submitted:false,reason:'invalid_review_dialog_selector'});}}
 if(!root){const dialogs=[...document.querySelectorAll('[role="dialog"],[aria-modal="true"],[class*="modal" i],[class*="dialog" i],[class*="drawer" i]')].filter(displayed);root=dialogs.pop()||document;}
 let inputs=[];
 if(customInput){try{inputs=[...root.querySelectorAll(customInput)];if(!inputs.length)inputs=[...document.querySelectorAll(customInput)];}catch(_){return JSON.stringify({submitted:false,reason:'invalid_review_input_selector'});}}
 if(!inputs.length)inputs=[...root.querySelectorAll('textarea,[contenteditable="true"],input[placeholder*="评价"],input[placeholder*="感受"],textarea[placeholder*="感受"]')];
 const input=inputs.find(displayed);
 if(!input)return JSON.stringify({submitted:false,reason:'review_input_not_found'});
 let rating=null;
 if(customRating){try{rating=[...root.querySelectorAll(customRating)].find(displayed)||[...document.querySelectorAll(customRating)].find(displayed)||null;}catch(_){return JSON.stringify({submitted:false,reason:'invalid_positive_rating_selector'});}}
 if(!rating){
   const labels=[...root.querySelectorAll('label,button,[role="radio"],[role="button"],span,div')].filter(element=>displayed(element)&&element.childElementCount<4&&/^好评$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label'))));
   rating=labels[0]||root.querySelector('[data-score="5"],[data-rating="5"],[aria-label*="5星"],[title*="5星"]');
   if(!rating){const stars=[...root.querySelectorAll('[role="radio"],[class*="star" i]')].filter(displayed);if(stars.length>=5)rating=stars[4];}
 }
 if(!rating)return JSON.stringify({submitted:false,reason:'positive_rating_not_found'});
 const ratingTarget=rating.closest('label,button,[role="radio"],[role="button"]')||rating;ratingTarget.click();
 input.focus();
 if(input.isContentEditable){
   input.textContent=review;
   input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:review}));
 }else{
   const proto=input.tagName==='TEXTAREA'?HTMLTextAreaElement.prototype:HTMLInputElement.prototype;
   const descriptor=Object.getOwnPropertyDescriptor(proto,'value');
   if(descriptor&&descriptor.set)descriptor.set.call(input,review);else input.value=review;
   input.dispatchEvent(new InputEvent('input',{bubbles:true,inputType:'insertText',data:review}));
   input.dispatchEvent(new Event('change',{bubbles:true}));
 }
 let buttons=[];
 if(customSubmit){try{buttons=[...root.querySelectorAll(customSubmit)];if(!buttons.length)buttons=[...document.querySelectorAll(customSubmit)];}catch(_){return JSON.stringify({submitted:false,reason:'invalid_review_submit_selector'});}}
 if(!buttons.length)buttons=[...root.querySelectorAll('button,a,[role="button"]')].filter(element=>/^(确认评价|提交评价|发布评价|发表评价|确认提交|提交)$/.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label'))));
 const button=buttons.find(element=>displayed(element)&&!disabled(element));
 if(!button)return JSON.stringify({submitted:false,reason:'review_submit_button_not_found'});
 button.scrollIntoView({block:'center',inline:'nearest'});
 button.click();
 return JSON.stringify({submitted:true,reason:''});
})()`, dialogJSON, inputJSON, ratingJSON, submitJSON, textJSON)
}

func buildAgentXianyuReviewConfirmationExpression(inputSelector, orderKey string) string {
	inputJSON := quoteAgentXianyuJS(inputSelector)
	orderJSON := quoteAgentXianyuJS(orderKey)
	return fmt.Sprintf(`(() => {
 const customInput=%s,orderKey=%s;
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden'&&style.opacity!=='0';};
 const notices=[...document.querySelectorAll('[role="alert"],[class*="message" i],[class*="toast" i],[class*="notification" i]')].filter(element=>displayed(element)&&clean(element.innerText||element.textContent).length<240).map(element=>clean(element.innerText||element.textContent));
 const failed=notices.find(text=>/(评价失败|提交失败|发布失败|请重试)/.test(text));
 if(failed)return JSON.stringify({confirmed:false,failed:true,reason:failed});
 const success=notices.find(text=>/(评价成功|提交成功|发布成功|发表成功)/.test(text));
 if(success)return JSON.stringify({confirmed:true,failed:false,reason:success});
 let editors=[];
 if(customInput){try{editors=[...document.querySelectorAll(customInput)];}catch(_){}}
 if(!editors.length)editors=[...document.querySelectorAll('[role="dialog"] textarea,[aria-modal="true"] textarea,[role="dialog"] [contenteditable="true"],[aria-modal="true"] [contenteditable="true"]')];
 const editorOpen=editors.some(displayed);
 if(!editorOpen)return JSON.stringify({confirmed:true,failed:false,reason:'review_editor_closed'});
 return JSON.stringify({confirmed:false,failed:false,reason:'review_editor_still_open'});
})()`, inputJSON, orderJSON)
}

func buildAgentXianyuReviewNextPageExpression() string {
	return `(() => {
 const clean=value=>String(value||'').replace(/\s+/g,' ').trim();
 const displayed=element=>{if(!element)return false;const rect=element.getBoundingClientRect(),style=getComputedStyle(element);return rect.width>0&&rect.height>0&&style.display!=='none'&&style.visibility!=='hidden';};
 const disabled=element=>!element||element.disabled||element.getAttribute('aria-disabled')==='true'||/(^|\s)(disabled|is-disabled)(\s|$)/i.test(String(element.className||''));
 const selectors=['.ant-pagination-next button','.ant-pagination-next','.rc-pagination-next button','.rc-pagination-next','.semi-page-next','.semi-pagination-next','[class*="pagination-next"] button','[class*="pagination-next"]','[class*="page-next"]','[aria-label="下一页"]','[title="下一页"]'];
 let next=null;
 for(const selector of selectors){try{const candidate=[...document.querySelectorAll(selector)].find(displayed);if(candidate){next=candidate;break;}}catch(_){}}
 if(!next)next=[...document.querySelectorAll('button,a,[role="button"],li')].find(element=>displayed(element)&&/^(下一页|下页|next|›|>)$/i.test(clean(element.innerText||element.textContent||element.getAttribute('aria-label')||element.getAttribute('title'))));
 if(!next)return JSON.stringify({clicked:false,at_end:true,reason:'next_page_not_found'});
 const target=next.matches('button,a,[role="button"]')?next:(next.querySelector('button,a,[role="button"]')||next);
 if(disabled(next)||disabled(target))return JSON.stringify({clicked:false,at_end:true,reason:'last_page'});
 target.scrollIntoView({block:'center',inline:'nearest'});
 target.click();
 return JSON.stringify({clicked:true,at_end:false,reason:''});
})()`
}

func agentXianyuReviewMapKeys(values map[string]int64) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func agentXianyuReviewSetKeys(values map[string]struct{}) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		if key = strings.TrimSpace(key); key != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

func failAgentXianyuReviewWithStats(result agentBlueprintNodeResult, message, errText string, stats agentXianyuReviewStats) agentBlueprintNodeResult {
	result.Status = "failed"
	result.Message = message
	result.Error = errText
	result.Output = map[string]interface{}{
		"source":                 "闲鱼评价管理",
		"scanned_order_count":    stats.Scanned,
		"eligible_order_count":   stats.Eligible,
		"submitted_review_count": stats.Submitted,
		"skipped_count":          stats.Skipped,
		"failed_count":           stats.Failed,
		"error":                  errText,
	}
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
