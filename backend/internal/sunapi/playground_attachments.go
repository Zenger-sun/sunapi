package sunapi

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

const (
	maxPlaygroundAttachmentBytes   = 12 * 1024 * 1024
	maxPlaygroundDocumentXMLBytes  = 4 * 1024 * 1024
	maxPlaygroundDocumentTextBytes = 2 * 1024 * 1024
)

func playgroundAttachmentContent(filename string, data []byte) (kind, normalizedType, text string, err error) {
	if len(data) == 0 {
		return "", "", "", errors.New("attachment is empty")
	}
	if len(data) > maxPlaygroundAttachmentBytes {
		return "", "", "", errors.New("attachment is too large (maximum 12 MB)")
	}
	detected := http.DetectContentType(data)
	if strings.HasPrefix(detected, "image/") {
		return "image", detected, "", nil
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".pdf":
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return "", "", "", errors.New("invalid PDF attachment")
		}
		return "file", "application/pdf", "", nil
	case ".docx", ".xlsx":
		text, err = playgroundOfficeDocumentText(data, ext)
		normalizedType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		if ext == ".xlsx" {
			normalizedType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		}
	default:
		textExtensions := ".txt .md .csv .json .html .htm .xml .yaml .yml .log .js .jsx .ts .tsx .py .go .java .c .cpp .h .css .sql .sh .rs .toml .ini"
		if ext == "" || !strings.Contains(" "+textExtensions+" ", " "+ext+" ") {
			return "", "", "", errors.New("unsupported attachment type; upload an image, PDF, DOCX, XLSX, or UTF-8 text file")
		}
		if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
			return "", "", "", errors.New("text attachments must use UTF-8 encoding")
		}
		text = strings.TrimPrefix(string(data), "\ufeff")
		normalizedType = "text/plain; charset=utf-8"
	}
	if err != nil {
		return "", "", "", err
	}
	if strings.TrimSpace(text) == "" {
		return "", "", "", errors.New("attachment contains no readable text")
	}
	if len(text) > maxPlaygroundDocumentTextBytes {
		return "", "", "", errors.New("attachment text exceeds 2 MB; split the document before uploading")
	}
	return "file", normalizedType, text, nil
}

// Extract full document text instead of the truncated agent template preview.
func playgroundOfficeDocumentText(data []byte, ext string) (string, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", errors.New("invalid Office document")
	}
	entries := map[string][]byte{}
	remaining := int64(maxPlaygroundDocumentXMLBytes)
	for _, file := range archive.File {
		if !isOfficeTemplatePreviewEntry(file.Name, ext) {
			continue
		}
		if file.UncompressedSize64 > uint64(remaining) {
			return "", errors.New("Office document content exceeds 4 MB")
		}
		rc, err := file.Open()
		if err != nil {
			return "", errors.New("cannot read Office document")
		}
		content, readErr := io.ReadAll(io.LimitReader(rc, remaining+1))
		closeErr := rc.Close()
		if readErr != nil || closeErr != nil || int64(len(content)) > remaining {
			return "", errors.New("cannot read Office document within the 4 MB content limit")
		}
		remaining -= int64(len(content))
		entries[file.Name] = content
	}
	sharedStrings := []string{}
	names := make([]string, 0, len(entries))
	for name, content := range entries {
		if _, err := playgroundOfficeXMLText(content); err != nil {
			return "", errors.New("invalid Office document XML")
		}
		if strings.EqualFold(name, "xl/sharedStrings.xml") {
			sharedStrings = parseAgentXLSXSharedStrings(content)
		} else {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		if ext == ".docx" {
			text, _ := playgroundOfficeXMLText(entries[name])
			out.WriteString(text)
			out.WriteByte('\n')
			continue
		}
		hasContent := false
		for _, cell := range parseAgentReportWorksheetCells(entries[name], sharedStrings) {
			if cell.Value == "" {
				continue
			}
			if !hasContent {
				out.WriteString("Worksheet: " + name + "\n")
				hasContent = true
			}
			out.WriteString(cell.Reference + "\t" + cell.Value + "\n")
			if out.Len() > maxPlaygroundDocumentTextBytes {
				return "", errors.New("attachment text exceeds 2 MB; split the document before uploading")
			}
		}
	}
	return strings.TrimSpace(out.String()), nil
}

func playgroundOfficeXMLText(data []byte) (string, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var out strings.Builder
	inText := false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return out.String(), nil
		}
		if err != nil {
			return "", err
		}
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "t":
				inText = true
			case "tab":
				out.WriteByte('\t')
			case "br":
				out.WriteByte('\n')
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "t":
				inText = false
			case "p", "tr":
				out.WriteByte('\n')
			case "tc":
				out.WriteByte('\t')
			}
		case xml.CharData:
			if inText {
				out.Write(token)
			}
		}
	}
}

func playgroundResolveAttachmentFile(c *gin.Context, store *Store, part map[string]any) error {
	file, ok := part["file"].(map[string]any)
	if !ok {
		return errors.New("invalid file attachment")
	}
	id, _ := file["file_id"].(string)
	attachment, err := store.GetPlaygroundAttachment(c.Request.Context(), playgroundUserID(c), id)
	if err != nil {
		return errors.New("attachment is unavailable; upload the file again")
	}
	if attachment.Type != "file" {
		return errors.New("expected a document attachment")
	}
	_, mediaType, text, err := playgroundAttachmentContent(attachment.Filename, attachment.Data)
	if err != nil {
		return err
	}
	if mediaType == "application/pdf" {
		part["file"] = map[string]any{
			"filename":  attachment.Filename,
			"file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(attachment.Data),
		}
		return nil
	}
	delete(part, "file")
	part["type"] = "text"
	part["text"] = fmt.Sprintf("Attached file: %s\n<attachment_content>\n%s\n</attachment_content>", attachment.Filename, text)
	return nil
}
