package sunapi

import (
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

const maxKnowledgeDocumentBytes int64 = 16 << 20

var knowledgeDocumentExtensions = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".html": true, ".htm": true, ".docx": true, ".xlsx": true,
}

func handleListKnowledgeBases(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := store.ListKnowledgeBases(c.Request.Context(), playgroundUserID(c))
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, items)
	}
}

func handleCreateKnowledgeBase(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var form KnowledgeBaseFormData
		if !bindJSON(c, &form) {
			return
		}
		item, err := store.CreateKnowledgeBase(c.Request.Context(), playgroundUserID(c), form)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, item)
	}
}

func handleUpdateKnowledgeBase(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var form KnowledgeBaseFormData
		if !bindJSON(c, &form) {
			return
		}
		item, err := store.UpdateKnowledgeBase(c.Request.Context(), strings.TrimSpace(c.Param("id")), playgroundUserID(c), form)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, item)
	}
}

func handleDeleteKnowledgeBase(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := store.DeleteKnowledgeBase(c.Request.Context(), strings.TrimSpace(c.Param("id")), playgroundUserID(c)); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}

func handleListKnowledgeBindings(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := store.ListKnowledgeBindings(c.Request.Context(), strings.TrimSpace(c.Param("id")), playgroundUserID(c))
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, items)
	}
}

func handleSetKnowledgeBindings(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var payload KnowledgeBindingPayload
		if !bindJSON(c, &payload) {
			return
		}
		items, err := store.SetKnowledgeBindings(c.Request.Context(), strings.TrimSpace(c.Param("id")), playgroundUserID(c), payload.Bindings)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, items)
	}
}

func handleListKnowledgeDocuments(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		items, err := store.ListKnowledgeDocuments(c.Request.Context(), strings.TrimSpace(c.Param("id")), playgroundUserID(c))
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, items)
	}
}

func handleGetKnowledgeDocument(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		item, err := store.GetKnowledgeDocument(
			c.Request.Context(), strings.TrimSpace(c.Param("id")), strings.TrimSpace(c.Param("docID")), playgroundUserID(c),
		)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if item == nil {
			fail(c, http.StatusNotFound, errors.New("knowledge document not found"))
			return
		}
		ok(c, item)
	}
}

func handleUploadKnowledgeDocument(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		header, err := c.FormFile("file")
		if err != nil {
			fail(c, http.StatusBadRequest, errors.New("document file is required"))
			return
		}
		if header.Size <= 0 || header.Size > maxKnowledgeDocumentBytes {
			fail(c, http.StatusBadRequest, errors.New("document must be between 1 byte and 16 MB"))
			return
		}
		filename := filepath.Base(strings.TrimSpace(header.Filename))
		ext := strings.ToLower(filepath.Ext(filename))
		if !knowledgeDocumentExtensions[ext] {
			fail(c, http.StatusBadRequest, errors.New("supported document types: txt, md, csv, json, html, docx, xlsx"))
			return
		}
		content, err := readKnowledgeUpload(header, ext)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		mediaType := strings.TrimSpace(header.Header.Get("Content-Type"))
		item, err := store.AddKnowledgeDocument(
			c.Request.Context(), strings.TrimSpace(c.Param("id")), playgroundUserID(c),
			filename, mediaType, header.Size, content,
		)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, item)
	}
}

func handleDeleteKnowledgeDocument(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if err := store.DeleteKnowledgeDocument(
			c.Request.Context(), strings.TrimSpace(c.Param("id")), strings.TrimSpace(c.Param("docID")), playgroundUserID(c),
		); err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, gin.H{"deleted": true})
	}
}

func handleSearchKnowledge(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body struct {
			Query            string   `json:"query"`
			Limit            int      `json:"limit"`
			KnowledgeBaseIDs []string `json:"knowledge_base_ids"`
			Disabled         bool     `json:"disabled"`
		}
		if !bindJSON(c, &body) {
			return
		}
		if strings.TrimSpace(body.Query) == "" {
			fail(c, http.StatusBadRequest, errors.New("search query is required"))
			return
		}
		items, err := store.SearchKnowledgeForScope(c.Request.Context(), playgroundUserID(c), body.Query, body.Limit, KnowledgeSearchScope{
			KnowledgeBaseIDs: body.KnowledgeBaseIDs,
			Disabled:         body.Disabled,
		})
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, items)
	}
}

func readKnowledgeUpload(header *multipart.FileHeader, ext string) (string, error) {
	file, err := header.Open()
	if err != nil {
		return "", err
	}
	defer file.Close()
	if ext != ".docx" && ext != ".xlsx" {
		data, err := io.ReadAll(io.LimitReader(file, maxKnowledgeDocumentBytes+1))
		if err != nil {
			return "", err
		}
		if int64(len(data)) > maxKnowledgeDocumentBytes {
			return "", errors.New("document exceeds 16 MB")
		}
		content := string(data)
		if ext == ".html" || ext == ".htm" {
			content = stripAgentHTMLTags(content)
		}
		return strings.TrimSpace(content), nil
	}
	temp, err := os.CreateTemp("", "sunapi-knowledge-upload-*")
	if err != nil {
		return "", err
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if _, err = io.Copy(temp, io.LimitReader(file, maxKnowledgeDocumentBytes+1)); err != nil {
		temp.Close()
		return "", err
	}
	if err = temp.Close(); err != nil {
		return "", err
	}
	return agentTemplateFilePreview(tempPath, "document"+ext)
}
