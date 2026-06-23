package sunapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v7/sdk/api/handlers/openai"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/sjson"
)

const playgroundImageTaskTimeout = 30 * time.Minute

type playgroundImageTaskPayload struct {
	PlaygroundImageGeneration
	Request json.RawMessage `json:"request"`
}

func registerPlaygroundImageTaskRoutes(engine *gin.Engine, store *Store, imageHandlers *openai.OpenAIAPIHandler) {
	if engine == nil || store == nil || imageHandlers == nil {
		return
	}

	api := engine.Group("/api")
	api.Use(RequireAdmin(store))
	api.POST("/playground/image-tasks", func(c *gin.Context) {
		var payload playgroundImageTaskPayload
		if !bindJSON(c, &payload) {
			return
		}
		if len(bytes.TrimSpace(payload.Request)) == 0 {
			fail(c, http.StatusBadRequest, errors.New("image task request is required"))
			return
		}

		userID := playgroundUserID(c)
		item := payload.PlaygroundImageGeneration
		item.Status = "running"
		item.ErrorMessage = ""
		item.URLs = []string{}
		if item.CreatedAt <= 0 {
			item.CreatedAt = time.Now().UnixMilli()
		}
		saved, err := store.SavePlaygroundImageGeneration(c.Request.Context(), userID, item)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}

		requestBody := normalizePlaygroundImageTaskRequest(payload.Request, saved)
		go runPlaygroundImageTask(store, imageHandlers, userID, saved, requestBody)
		ok(c, saved)
	})
}

func normalizePlaygroundImageTaskRequest(raw json.RawMessage, item PlaygroundImageGeneration) []byte {
	requestBody := append([]byte(nil), raw...)
	if !json.Valid(requestBody) {
		return requestBody
	}
	if model := strings.TrimSpace(item.Model); model != "" {
		updated, err := sjson.SetBytes(requestBody, "model", model)
		if err == nil {
			requestBody = updated
		}
	}
	group := normalizeGroupName(item.Group)
	if group != "" {
		updated, err := sjson.SetBytes(requestBody, "group", group)
		if err == nil {
			requestBody = updated
		}
	}
	return requestBody
}

func runPlaygroundImageTask(store *Store, imageHandlers *openai.OpenAIAPIHandler, userID int64, item PlaygroundImageGeneration, requestBody []byte) {
	startedAt := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), playgroundImageTaskTimeout)
	defer cancel()

	path, usesReferences := playgroundImageTaskRoute(requestBody)
	logFields := playgroundImageTaskLogFields(item, requestBody, path, usesReferences)
	log.Infof("sunapi image task debug: started %s", playgroundImageTaskLogLine(logFields))

	body, statusCode, err := executePlaygroundImageTaskRequest(ctx, store, imageHandlers, requestBody)
	item.DurationMS = time.Since(startedAt).Milliseconds()
	if err != nil {
		fields := playgroundImageTaskCopyLogFields(logFields)
		fields["duration_ms"] = item.DurationMS
		fields["error"] = err.Error()
		log.Warnf("sunapi image task debug: internal request failed %s", playgroundImageTaskLogLine(fields))
		savePlaygroundImageTaskFailure(ctx, store, userID, item, err.Error())
		return
	}
	if statusCode < http.StatusOK || statusCode >= http.StatusBadRequest {
		message := playgroundImageTaskErrorMessage(statusCode, body)
		fields := playgroundImageTaskCopyLogFields(logFields)
		fields["duration_ms"] = item.DurationMS
		fields["status_code"] = statusCode
		fields["error_message"] = message
		fields["response_excerpt"] = playgroundImageTaskBodyExcerpt(body, 1000)
		log.Warnf("sunapi image task debug: image handler returned error %s", playgroundImageTaskLogLine(fields))
		savePlaygroundImageTaskFailure(ctx, store, userID, item, message)
		return
	}

	urls, err := playgroundImageTaskURLs(body)
	if err != nil {
		fields := playgroundImageTaskCopyLogFields(logFields)
		fields["duration_ms"] = item.DurationMS
		fields["status_code"] = statusCode
		fields["error"] = err.Error()
		fields["response_excerpt"] = playgroundImageTaskBodyExcerpt(body, 1000)
		log.Warnf("sunapi image task debug: image response parse failed %s", playgroundImageTaskLogLine(fields))
		savePlaygroundImageTaskFailure(ctx, store, userID, item, err.Error())
		return
	}
	urls = persistPlaygroundImageURLs(ctx, store.DataDir(), item.ID, urls, item.CreatedAt)
	if len(urls) == 0 {
		fields := playgroundImageTaskCopyLogFields(logFields)
		fields["duration_ms"] = item.DurationMS
		fields["status_code"] = statusCode
		fields["response_excerpt"] = playgroundImageTaskBodyExcerpt(body, 1000)
		log.Warnf("sunapi image task debug: image response had no usable urls %s", playgroundImageTaskLogLine(fields))
		savePlaygroundImageTaskFailure(ctx, store, userID, item, "image response is empty")
		return
	}

	item.Status = "succeeded"
	item.ErrorMessage = ""
	item.URLs = urls
	_, _ = store.SavePlaygroundImageGeneration(ctx, userID, item)
	fields := playgroundImageTaskCopyLogFields(logFields)
	fields["duration_ms"] = item.DurationMS
	fields["status_code"] = statusCode
	fields["url_count"] = len(urls)
	log.Infof("sunapi image task debug: succeeded %s", playgroundImageTaskLogLine(fields))
}

func executePlaygroundImageTaskRequest(ctx context.Context, store *Store, imageHandlers *openai.OpenAIAPIHandler, requestBody []byte) ([]byte, int, error) {
	if imageHandlers == nil {
		return nil, http.StatusInternalServerError, errors.New("image handler is unavailable")
	}

	path, _ := playgroundImageTaskRoute(requestBody)
	handler := imageHandlers.ImagesGenerations
	if path == "/pg/images/edits" {
		handler = imageHandlers.ImagesEdits
	}

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(requestBody)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	c.Request = req

	playgroundAccessMetadata()(c)
	playgroundResolveAttachments(store)(c)
	handler(c)

	res := recorder.Result()
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxGeneratedImageBytes*2))
	if err != nil {
		return nil, res.StatusCode, err
	}
	return body, res.StatusCode, nil
}

func playgroundImageTaskRoute(raw []byte) (string, bool) {
	if playgroundImageTaskUsesReferences(raw) {
		return "/pg/images/edits", true
	}
	return "/pg/images/generations", false
}

func playgroundImageTaskLogFields(item PlaygroundImageGeneration, raw []byte, path string, usesReferences bool) log.Fields {
	fields := log.Fields{
		"task_id":         item.ID,
		"item_model":      item.Model,
		"item_group":      item.Group,
		"path":            path,
		"uses_references": usesReferences,
		"request_bytes":   len(raw),
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		fields["request_json_error"] = err.Error()
		return fields
	}

	if model := playgroundImageTaskJSONString(payload["model"]); model != "" {
		fields["request_model"] = model
	}
	if group := playgroundImageTaskJSONString(payload["group"]); group != "" {
		fields["request_group"] = group
	}
	if prompt := playgroundImageTaskJSONString(payload["prompt"]); prompt != "" {
		fields["prompt_chars"] = utf8.RuneCountInString(prompt)
	}
	for _, key := range []string{"resolution", "aspect_ratio", "size", "quality", "style", "response_format"} {
		if value := playgroundImageTaskJSONString(payload[key]); value != "" {
			fields[key] = value
		}
	}
	if rawN := bytes.TrimSpace(payload["n"]); len(rawN) > 0 && string(rawN) != "null" {
		fields["n"] = string(rawN)
	}
	if rawSeed := bytes.TrimSpace(payload["seed"]); len(rawSeed) > 0 && string(rawSeed) != "null" {
		fields["seed_present"] = true
	}
	fields["images_count"] = playgroundImageTaskArrayLen(payload["images"])
	fields["has_image"] = playgroundImageTaskJSONPresent(payload["image"])
	fields["has_mask"] = playgroundImageTaskJSONPresent(payload["mask"])
	return fields
}

func playgroundImageTaskJSONString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return strings.TrimSpace(value)
}

func playgroundImageTaskArrayLen(raw json.RawMessage) int {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return 0
	}
	var values []json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return 0
	}
	return len(values)
}

func playgroundImageTaskJSONPresent(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && string(raw) != "null"
}

func playgroundImageTaskCopyLogFields(src log.Fields) log.Fields {
	dst := make(log.Fields, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func playgroundImageTaskLogLine(fields log.Fields) string {
	if len(fields) == 0 {
		return ""
	}
	order := []string{
		"task_id",
		"path",
		"item_model",
		"item_group",
		"request_model",
		"request_group",
		"uses_references",
		"images_count",
		"has_image",
		"has_mask",
		"request_bytes",
		"prompt_chars",
		"resolution",
		"aspect_ratio",
		"size",
		"quality",
		"style",
		"response_format",
		"n",
		"seed_present",
		"duration_ms",
		"status_code",
		"url_count",
		"error",
		"error_message",
		"response_excerpt",
		"request_json_error",
	}
	parts := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, key := range order {
		value, ok := fields[key]
		if !ok {
			continue
		}
		parts = append(parts, playgroundImageTaskLogPair(key, value))
		seen[key] = struct{}{}
	}
	for key, value := range fields {
		if _, ok := seen[key]; ok {
			continue
		}
		parts = append(parts, playgroundImageTaskLogPair(key, value))
	}
	return strings.Join(parts, " ")
}

func playgroundImageTaskLogPair(key string, value any) string {
	text := strings.TrimSpace(fmt.Sprint(value))
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	if text == "" {
		text = "-"
	}
	if strings.ContainsAny(text, " \t\"'") {
		text = strconvQuote(text)
	}
	return key + "=" + text
}

func strconvQuote(text string) string {
	raw, err := json.Marshal(text)
	if err != nil {
		return `"` + strings.ReplaceAll(text, `"`, `\"`) + `"`
	}
	return string(raw)
}

func playgroundImageTaskBodyExcerpt(body []byte, maxRunes int) string {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return ""
	}
	text = strings.ReplaceAll(text, "\r", " ")
	text = strings.ReplaceAll(text, "\n", " ")
	text = strings.Join(strings.Fields(text), " ")
	text = playgroundImageTaskRedactDataURLs(text)
	if maxRunes > 0 {
		text = truncateRunes(text, maxRunes)
	}
	return text
}

func playgroundImageTaskRedactDataURLs(text string) string {
	const marker = "data:image/"
	for {
		lower := strings.ToLower(text)
		start := strings.Index(lower, marker)
		if start < 0 {
			return text
		}
		end := len(text)
		for offset, r := range text[start:] {
			if offset == 0 {
				continue
			}
			if r == '"' || r == '\'' || r == ' ' || r == '}' || r == ']' {
				end = start + offset
				break
			}
		}
		text = text[:start] + "[image-data-url-redacted]" + text[end:]
	}
}

func playgroundImageTaskUsesReferences(raw []byte) bool {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false
	}
	for _, key := range []string{"images", "image", "mask"} {
		if len(bytes.TrimSpace(payload[key])) > 0 && string(bytes.TrimSpace(payload[key])) != "null" {
			return true
		}
	}
	return false
}

func playgroundImageTaskURLs(body []byte) ([]string, error) {
	var payload struct {
		Data []struct {
			URL     string `json:"url"`
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	urls := make([]string, 0, len(payload.Data))
	for _, item := range payload.Data {
		if url := strings.TrimSpace(item.URL); url != "" {
			urls = append(urls, url)
			continue
		}
		if b64 := strings.TrimSpace(item.B64JSON); b64 != "" {
			urls = append(urls, "data:image/png;base64,"+b64)
		}
	}
	if len(urls) == 0 {
		return nil, errors.New("image response is empty")
	}
	return urls, nil
}

func playgroundImageTaskErrorMessage(statusCode int, body []byte) string {
	var payload struct {
		Message string `json:"message"`
		Error   any    `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err == nil {
		if msg := strings.TrimSpace(payload.Message); msg != "" {
			return msg
		}
		switch value := payload.Error.(type) {
		case string:
			if msg := strings.TrimSpace(value); msg != "" {
				return msg
			}
		case map[string]any:
			if msg, _ := value["message"].(string); strings.TrimSpace(msg) != "" {
				return strings.TrimSpace(msg)
			}
		}
	}
	if msg := strings.TrimSpace(string(body)); msg != "" && len(msg) <= 2000 {
		return msg
	}
	if text := http.StatusText(statusCode); text != "" {
		return text
	}
	return "image generation failed"
}

func savePlaygroundImageTaskFailure(ctx context.Context, store *Store, userID int64, item PlaygroundImageGeneration, message string) {
	if store == nil {
		return
	}
	item.Status = "failed"
	item.ErrorMessage = message
	item.URLs = []string{}
	_, _ = store.SavePlaygroundImageGeneration(ctx, userID, item)
}
