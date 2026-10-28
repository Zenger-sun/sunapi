package sunapi

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func playgroundTestArchive(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := zip.NewWriter(&data)
	for name, content := range entries {
		entry, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(entry, content); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

func TestPlaygroundAttachmentContent(t *testing.T) {
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aTfkAAAAASUVORK5CYII=")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, filename, kind, wantText string
		data                           []byte
		wantErr                        bool
	}{
		{name: "UTF-8 text", filename: "说明.txt", data: []byte("中文内容"), kind: "file", wantText: "中文内容"},
		{name: "CSV without MIME", filename: "report.CSV", data: []byte("name,value\nrevenue,100"), kind: "file", wantText: "revenue,100"},
		{name: "source code", filename: "main.go", data: []byte("package main"), kind: "file", wantText: "package main"},
		{name: "UTF-8 BOM", filename: "readme.md", data: []byte("\ufeff# Title"), kind: "file", wantText: "# Title"},
		{name: "PDF", filename: "report.pdf", data: []byte("%PDF-1.7\n%%EOF"), kind: "file"},
		{name: "image", filename: "image.png", data: png, kind: "image"},
		{name: "empty", filename: "empty.txt", wantErr: true},
		{name: "blank text", filename: "blank.txt", data: []byte(" \n"), wantErr: true},
		{name: "fake PDF", filename: "report.pdf", data: []byte("not a PDF"), wantErr: true},
		{name: "unsupported binary", filename: "program.exe", data: []byte("MZ binary"), wantErr: true},
		{name: "HTML is a document", filename: "page.html", data: []byte("<script>alert(1)</script>"), kind: "file", wantText: "<script>"},
		{name: "binary text", filename: "data.txt", data: []byte{0, 1, 2}, wantErr: true},
		{name: "non UTF-8", filename: "data.txt", data: []byte{0xff, 0xfe}, wantErr: true},
		{name: "file too large", filename: "data.txt", data: bytes.Repeat([]byte("x"), maxPlaygroundAttachmentBytes+1), wantErr: true},
		{name: "text too large", filename: "data.txt", data: bytes.Repeat([]byte("x"), maxPlaygroundDocumentTextBytes+1), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			kind, mediaType, text, err := playgroundAttachmentContent(tt.filename, tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr {
				return
			}
			if kind != tt.kind || mediaType == "" || !strings.Contains(text, tt.wantText) {
				t.Fatalf("unexpected attachment: kind=%s mediaType=%s text=%q", kind, mediaType, text)
			}
			if strings.HasPrefix(text, "\ufeff") {
				t.Fatal("UTF-8 BOM was not removed")
			}
		})
	}
}

func TestPlaygroundOfficeDocuments(t *testing.T) {
	t.Run("DOCX preserves full text and paragraphs", func(t *testing.T) {
		body := strings.Repeat("正文", 7000) + "END-OF-DOCUMENT"
		data := playgroundTestArchive(t, map[string]string{
			"word/document.xml": `<w:document xmlns:w="word"><w:body><w:p><w:r><w:t>First paragraph</w:t></w:r></w:p><w:p><w:r><w:t>` + body + `</w:t></w:r></w:p></w:body></w:document>`,
		})
		_, _, text, err := playgroundAttachmentContent("long.docx", data)
		if err != nil || !strings.Contains(text, "First paragraph\n"+body) {
			t.Fatalf("full DOCX text was not extracted: %v", err)
		}
	})
	t.Run("XLSX resolves shared strings and preserves sparse cell addresses", func(t *testing.T) {
		entries := map[string]string{
			"xl/sharedStrings.xml": `<sst><si><t>Revenue</t></si></sst>`,
		}
		for i := 1; i <= 8; i++ {
			entries[fmt.Sprintf("xl/worksheets/sheet%d.xml", i)] = `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c><c r="C1"><v>123.5</v></c><c r="D1" t="inlineStr"><is><t>备注</t></is></c></row></sheetData></worksheet>`
		}
		_, _, text, err := playgroundAttachmentContent("report.xlsx", playgroundTestArchive(t, entries))
		if err != nil || !strings.Contains(text, "sheet8.xml") || strings.Count(text, "A1\tRevenue\nC1\t123.5\nD1\t备注") != 8 {
			t.Fatalf("unexpected XLSX text: %q, error=%v", text, err)
		}
	})
	t.Run("reject invalid archives and XML", func(t *testing.T) {
		for _, data := range [][]byte{
			[]byte("not a zip"),
			playgroundTestArchive(t, map[string]string{"word/document.xml": `<document><t>broken`}),
			playgroundTestArchive(t, map[string]string{"word/document.xml": `<document/>`}),
			playgroundTestArchive(t, map[string]string{"word/document.xml": strings.Repeat("x", maxPlaygroundDocumentXMLBytes+1)}),
		} {
			if _, _, _, err := playgroundAttachmentContent("invalid.docx", data); err == nil {
				t.Fatal("expected invalid or oversized Office content to be rejected")
			}
		}
	})
	t.Run("reject empty XLSX worksheets", func(t *testing.T) {
		data := playgroundTestArchive(t, map[string]string{
			"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row><c r="A1"/></row></sheetData></worksheet>`,
		})
		if _, _, _, err := playgroundAttachmentContent("empty.xlsx", data); err == nil {
			t.Fatal("expected an empty workbook to be rejected")
		}
	})
}

func TestPlaygroundAttachmentUploadAndResolution(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Set("sunapi_admin_user", AdminUser{ID: 7})
		c.Next()
	})
	registerPlaygroundSessionRoutes(engine.Group("/api"), store)
	engine.POST("/pg/chat/completions", playgroundResolveAttachments(store), func(c *gin.Context) {
		body, err := io.ReadAll(c.Request.Body)
		if err != nil {
			t.Error(err)
		}
		if int64(len(body)) != c.Request.ContentLength {
			t.Error("request content length was not updated")
		}
		c.Data(http.StatusOK, "application/json", body)
	})

	for _, tt := range []struct {
		filename, content, partType string
	}{
		{"说明.txt", "附件中的真实内容", "text"},
		{"report.pdf", "%PDF-1.7\n%%EOF", "file"},
	} {
		t.Run(tt.filename, func(t *testing.T) {
			var body bytes.Buffer
			writer := multipart.NewWriter(&body)
			file, err := writer.CreateFormFile("file", tt.filename)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(file, tt.content)
			_ = writer.Close()
			req := httptest.NewRequest(http.MethodPost, "/api/playground/attachments", &body)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			var uploaded struct {
				Data PlaygroundAttachment `json:"data"`
			}
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &uploaded) != nil || uploaded.Data.Type != "file" {
				t.Fatalf("upload failed: %d %s", response.Code, response.Body.String())
			}
			attachment := uploaded.Data
			if attachment.FileID == "" || attachment.Filename != tt.filename {
				t.Fatalf("invalid upload metadata: %+v", attachment)
			}
			messages, _ := json.Marshal([]any{map[string]any{
				"from": "user", "versions": []any{map[string]any{"id": "v1", "content": ""}},
				"attachments": []PlaygroundAttachment{attachment},
			}})
			session, err := store.SavePlaygroundSession(context.Background(), 7, PlaygroundSessionPayload{
				Title: tt.filename, Model: "test-model", Messages: messages, Config: json.RawMessage(`{}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			loaded, err := store.GetPlaygroundSession(context.Background(), 7, session.ID)
			if err != nil || !bytes.Contains(loaded.Messages, []byte(attachment.ID)) {
				t.Fatalf("attachment metadata was not preserved in session history: %v", err)
			}
			if _, err := store.GetPlaygroundAttachment(context.Background(), 8, attachment.ID); err == nil {
				t.Fatal("attachments must be isolated by user")
			}
			response = httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, attachment.URL, nil))
			if response.Code != http.StatusOK || response.Body.String() != tt.content || !strings.HasPrefix(response.Header().Get("Content-Disposition"), "attachment;") || response.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("download failed: %d %s", response.Code, response.Body.String())
			}
			payload, _ := json.Marshal(map[string]any{"messages": []any{map[string]any{
				"role": "user", "content": []any{
					map[string]any{"type": "text", "text": "Summarize this"},
					map[string]any{"type": "file", "file": map[string]any{"file_id": attachment.ID, "filename": "spoofed.txt"}},
				},
			}}})
			response = httptest.NewRecorder()
			req = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", bytes.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			engine.ServeHTTP(response, req)
			var resolved struct {
				Messages []struct {
					Content []struct {
						Type string            `json:"type"`
						Text string            `json:"text"`
						File map[string]string `json:"file"`
					} `json:"content"`
				} `json:"messages"`
			}
			if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &resolved) != nil || len(resolved.Messages) != 1 || len(resolved.Messages[0].Content) != 2 {
				t.Fatalf("resolution failed: %d %s", response.Code, response.Body.String())
			}
			part := resolved.Messages[0].Content[1]
			if part.Type != tt.partType {
				t.Fatalf("unexpected part type: %s", part.Type)
			}
			if tt.partType == "text" && (!strings.Contains(part.Text, tt.content) || !strings.Contains(part.Text, tt.filename)) {
				t.Fatalf("missing attachment content: %s", part.Text)
			}
			if tt.partType == "file" && (part.File["file_id"] != "" || part.File["filename"] != tt.filename || part.File["file_data"] != "data:application/pdf;base64,"+base64.StdEncoding.EncodeToString([]byte(tt.content))) {
				t.Fatalf("invalid upstream PDF: %+v", part.File)
			}
		})
	}
	t.Run("images still resolve and documents cannot be used as images", func(t *testing.T) {
		png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+aTfkAAAAASUVORK5CYII=")
		image, err := store.SavePlaygroundAttachment(context.Background(), 7, "image/png", "image.png", png)
		if err != nil || image.Type != "image" {
			t.Fatalf("image upload failed: %v", err)
		}
		recorder := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(recorder)
		c.Request = httptest.NewRequest(http.MethodPost, "/pg/chat/completions", nil)
		c.Set("sunapi_admin_user", AdminUser{ID: 7})
		imageURL := map[string]any{"file_id": image.ID, "url": image.URL}
		if !playgroundResolveAttachmentImageURL(c, store, imageURL) || imageURL["url"] != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(png) || imageURL["file_id"] != nil {
			t.Fatalf("image resolution regressed: %+v", imageURL)
		}
		document, err := store.SavePlaygroundAttachment(context.Background(), 7, "image/png", "document.txt", []byte("This is text, not an image"))
		if err != nil || document.Type != "file" {
			t.Fatalf("content must determine attachment type: %v", err)
		}
		if _, ok := playgroundAttachmentDataURL(c, store, document.ID); ok {
			t.Fatal("document was incorrectly resolved as an image")
		}
	})
	t.Run("missing and foreign files fail before upstream", func(t *testing.T) {
		foreign, err := store.SavePlaygroundAttachment(context.Background(), 8, "text/plain", "private.txt", []byte("private content"))
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range []string{"missing", foreign.ID} {
			payload := fmt.Sprintf(`{"messages":[{"role":"user","content":[{"type":"file","file":{"file_id":%q}}]}]}`, id)
			req := httptest.NewRequest(http.MethodPost, "/pg/chat/completions", strings.NewReader(payload))
			req.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, req)
			if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "invalid_attachment") || strings.Contains(response.Body.String(), "private content") {
				t.Fatalf("expected attachment validation error: %d %s", response.Code, response.Body.String())
			}
		}
	})
}
