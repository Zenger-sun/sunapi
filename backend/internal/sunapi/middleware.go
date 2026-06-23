package sunapi

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

func BlockBundledManagementRoutes() gin.HandlerFunc {
	blockedExact := map[string]struct{}{
		"/management.html":    {},
		"/anthropic/callback": {},
		"/codex/callback":     {},
		"/google/callback":    {},
	}
	blockedPrefixes := []string{
		"/v0/management",
		"/v0/resource/plugins",
	}

	return func(c *gin.Context) {
		path := strings.TrimSpace(c.Request.URL.Path)
		if _, blocked := blockedExact[path]; blocked {
			c.AbortWithStatus(http.StatusNotFound)
			return
		}
		for _, prefix := range blockedPrefixes {
			if strings.HasPrefix(path, prefix) {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}
		}
		c.Next()
	}
}

func playgroundAccessMetadata() gin.HandlerFunc {
	return func(c *gin.Context) {
		metadata := map[string]string{
			"source": "sunapi-playground",
		}
		if group := playgroundRequestGroup(c); group != "" {
			metadata["api_key_group"] = normalizeGroupName(group)
			metadata["api_key_cross_group_retry"] = "false"
		}
		c.Set("userApiKey", "sunapi-playground")
		c.Set("accessProvider", "sunapi-playground")
		c.Set("accessMetadata", metadata)
		c.Next()
	}
}

func playgroundRequestGroup(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if group := strings.TrimSpace(c.Query("group")); group != "" {
		return group
	}
	if c.Request.Body == nil {
		return ""
	}
	contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
	if !strings.Contains(contentType, "application/json") {
		return ""
	}
	body, err := io.ReadAll(c.Request.Body)
	c.Request.Body = io.NopCloser(bytes.NewReader(body))
	if err != nil || len(bytes.TrimSpace(body)) == 0 {
		return ""
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	rawGroup, hasGroup := payload["group"]
	if !hasGroup {
		return ""
	}
	delete(payload, "group")
	if updated, err := json.Marshal(payload); err == nil {
		c.Request.Body = io.NopCloser(bytes.NewReader(updated))
		c.Request.ContentLength = int64(len(updated))
	}

	var group string
	if err := json.Unmarshal(rawGroup, &group); err != nil {
		return ""
	}
	return strings.TrimSpace(group)
}

func playgroundResolveAttachments(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil || c == nil || c.Request == nil || c.Request.Body == nil || c.Request.Method != http.MethodPost {
			c.Next()
			return
		}
		path := strings.TrimSpace(c.Request.URL.Path)
		if path != "/pg/chat/completions" && path != "/pg/images/edits" {
			c.Next()
			return
		}
		contentType := strings.ToLower(strings.TrimSpace(c.GetHeader("Content-Type")))
		if !strings.Contains(contentType, "application/json") {
			c.Next()
			return
		}

		body, err := io.ReadAll(c.Request.Body)
		c.Request.Body = io.NopCloser(bytes.NewReader(body))
		if err != nil || len(bytes.TrimSpace(body)) == 0 {
			c.Next()
			return
		}

		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			c.Next()
			return
		}
		changed := false
		if path == "/pg/chat/completions" {
			if messages, ok := payload["messages"].([]any); ok {
				for _, rawMessage := range messages {
					message, ok := rawMessage.(map[string]any)
					if !ok {
						continue
					}
					parts, ok := message["content"].([]any)
					if !ok {
						continue
					}
					for _, rawPart := range parts {
						part, ok := rawPart.(map[string]any)
						if !ok || part["type"] != "image_url" {
							continue
						}
						imageURL, ok := part["image_url"].(map[string]any)
						if !ok {
							continue
						}
						if playgroundResolveAttachmentImageURL(c, store, imageURL) {
							changed = true
						}
					}
				}
			}
		}
		if path == "/pg/images/edits" {
			if images, ok := payload["images"].([]any); ok {
				for _, rawImage := range images {
					image, ok := rawImage.(map[string]any)
					if !ok {
						continue
					}
					if playgroundResolveAttachmentImageString(c, store, image, "image_url") {
						changed = true
					}
					if playgroundResolveAttachmentImageString(c, store, image, "url") {
						changed = true
					}
				}
			}
			if image, ok := payload["image"].(map[string]any); ok {
				if playgroundResolveAttachmentImageString(c, store, image, "image_url") {
					changed = true
				}
				if playgroundResolveAttachmentImageString(c, store, image, "url") {
					changed = true
				}
			}
			if mask, ok := payload["mask"].(map[string]any); ok {
				if playgroundResolveAttachmentImageString(c, store, mask, "image_url") {
					changed = true
				}
				if playgroundResolveAttachmentImageString(c, store, mask, "url") {
					changed = true
				}
			}
		}

		if changed {
			if updated, err := json.Marshal(payload); err == nil {
				c.Request.Body = io.NopCloser(bytes.NewReader(updated))
				c.Request.ContentLength = int64(len(updated))
			}
		}
		c.Next()
	}
}

func playgroundResolveAttachmentImageURL(c *gin.Context, store *Store, imageURL map[string]any) bool {
	id := playgroundAttachmentIDFromImageURL(imageURL)
	if id == "" {
		return false
	}
	dataURL, ok := playgroundAttachmentDataURL(c, store, id)
	if !ok {
		return false
	}
	imageURL["url"] = dataURL
	delete(imageURL, "file_id")
	return true
}

func playgroundResolveAttachmentImageString(c *gin.Context, store *Store, image map[string]any, key string) bool {
	rawURL, _ := image[key].(string)
	id := playgroundAttachmentIDFromRawURL(rawURL)
	if id == "" {
		return false
	}
	dataURL, ok := playgroundAttachmentDataURL(c, store, id)
	if !ok {
		return false
	}
	image[key] = dataURL
	return true
}

func playgroundAttachmentDataURL(c *gin.Context, store *Store, id string) (string, bool) {
	if c == nil || store == nil || id == "" {
		return "", false
	}
	attachment, err := store.GetPlaygroundAttachment(c.Request.Context(), playgroundUserID(c), id)
	if err != nil || len(attachment.Data) == 0 || attachment.MediaType == "" {
		return "", false
	}
	return "data:" + attachment.MediaType + ";base64," + base64.StdEncoding.EncodeToString(attachment.Data), true
}

func playgroundAttachmentIDFromImageURL(imageURL map[string]any) string {
	if raw, ok := imageURL["file_id"].(string); ok {
		if id := strings.TrimSpace(raw); id != "" {
			return id
		}
	}
	rawURL, _ := imageURL["url"].(string)
	return playgroundAttachmentIDFromRawURL(rawURL)
}

func playgroundAttachmentIDFromRawURL(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if strings.HasPrefix(rawURL, "playground-attachment://") {
		return strings.TrimSpace(strings.TrimPrefix(rawURL, "playground-attachment://"))
	}
	if strings.HasPrefix(rawURL, "/api/playground/attachments/") {
		return strings.TrimSpace(strings.TrimPrefix(rawURL, "/api/playground/attachments/"))
	}
	return ""
}
