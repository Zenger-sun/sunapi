package sunapi

import (
	"archive/zip"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
)

const (
	agentDataTemplateDirReports  = "templates/reports"
	agentDataTemplateDirWeb      = "templates/web"
	agentDataOutputDirReports    = "output/reports"
	agentDataOutputDirWeb        = "output/web-reports"
	agentTemplateIndexFilename   = ".agent-template-index.json"
	maxAgentFileUploadBytes      = 32 * 1024 * 1024
	maxAgentTemplatePreviewBytes = 256 * 1024
	maxAgentTemplatePreviewRunes = 6000
)

var (
	agentTemplateIndexMu                  sync.Mutex
	agentReportTemplatePlaceholderPattern = regexp.MustCompile(`\{\{(?:title|标题|output_format|输出格式|content|内容|generated_at|生成时间)\}\}`)
)

var allowedAgentFileExtensions = map[string]bool{
	".csv":  true,
	".doc":  true,
	".docx": true,
	".htm":  true,
	".html": true,
	".json": true,
	".md":   true,
	".pdf":  true,
	".txt":  true,
	".xls":  true,
	".xlsx": true,
}

var allowedAgentReportOutputExtensions = map[string]bool{
	".csv":  true,
	".docx": true,
	".html": true,
	".json": true,
	".md":   true,
	".pdf":  true,
	".txt":  true,
	".xlsx": true,
}

type AgentFileUpload struct {
	Filename        string `json:"filename"`
	OriginalName    string `json:"original_name"`
	Directory       string `json:"directory"`
	Path            string `json:"path"`
	MediaType       string `json:"media_type"`
	Size            int64  `json:"size"`
	Hash            string `json:"hash,omitempty"`
	ContentPreview  string `json:"content_preview,omitempty"`
	OutputDirectory string `json:"output_directory,omitempty"`
	Reused          bool   `json:"reused,omitempty"`
	Replaced        bool   `json:"replaced,omitempty"`
	CreatedAt       int64  `json:"created_time"`
}

type AgentReportWriteRequest struct {
	Title           string                  `json:"title"`
	OutputDirectory string                  `json:"output_directory"`
	Files           []AgentReportOutputFile `json:"files"`
	Markdown        string                  `json:"markdown,omitempty"`
	CSV             string                  `json:"csv,omitempty"`
}

type AgentReportOutputFile struct {
	Label                string `json:"label"`
	Extension            string `json:"extension"`
	Content              string `json:"content"`
	TemplatePath         string `json:"template_path,omitempty"`
	TemplateOriginalName string `json:"template_original_name,omitempty"`
}

type AgentReportWrittenFile struct {
	Label     string `json:"label"`
	Path      string `json:"path"`
	Extension string `json:"extension"`
	Size      int64  `json:"size"`
}

type AgentReportWriteResult struct {
	OutputDirectory string                   `json:"output_directory"`
	Files           []AgentReportWrittenFile `json:"files"`
	MarkdownPath    string                   `json:"markdown_path,omitempty"`
	CSVPath         string                   `json:"csv_path,omitempty"`
	CreatedAt       int64                    `json:"created_time"`
}

type agentTemplateIndex struct {
	Templates []agentTemplateRecord `json:"templates"`
}

type agentTemplateRecord struct {
	Filename        string `json:"filename"`
	OriginalName    string `json:"original_name"`
	Directory       string `json:"directory"`
	Path            string `json:"path"`
	MediaType       string `json:"media_type"`
	Size            int64  `json:"size"`
	Hash            string `json:"hash"`
	ContentPreview  string `json:"content_preview,omitempty"`
	OutputDirectory string `json:"output_directory,omitempty"`
	CreatedAt       int64  `json:"created_time"`
	UpdatedAt       int64  `json:"updated_time"`
}

func handleUploadAgentFile(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil || strings.TrimSpace(store.DataDir()) == "" {
			fail(c, http.StatusInternalServerError, errors.New("data directory is unavailable"))
			return
		}

		file, err := c.FormFile("file")
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		if file.Size <= 0 {
			fail(c, http.StatusBadRequest, errors.New("file is empty"))
			return
		}
		if file.Size > maxAgentFileUploadBytes {
			fail(c, http.StatusBadRequest, errors.New("file is too large"))
			return
		}

		targetDir, outputDir := agentUploadDirectories(c.PostForm("purpose"))
		filename, originalName, err := safeAgentUploadFilename(file.Filename)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}

		src, err := file.Open()
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		defer src.Close()

		absDir := filepath.Join(store.DataDir(), filepath.FromSlash(targetDir))
		if err := os.MkdirAll(absDir, 0755); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if strings.TrimSpace(outputDir) != "" {
			if err := os.MkdirAll(filepath.Join(store.DataDir(), filepath.FromSlash(outputDir)), 0755); err != nil {
				fail(c, http.StatusInternalServerError, err)
				return
			}
		}

		absPath := filepath.Join(absDir, filename)
		dst, err := os.OpenFile(absPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}

		hasher := sha256.New()
		written, copyErr := io.Copy(io.MultiWriter(dst, hasher), io.LimitReader(src, maxAgentFileUploadBytes+1))
		closeErr := dst.Close()
		if copyErr != nil {
			_ = os.Remove(absPath)
			fail(c, http.StatusBadRequest, copyErr)
			return
		}
		if closeErr != nil {
			_ = os.Remove(absPath)
			fail(c, http.StatusInternalServerError, closeErr)
			return
		}
		if written > maxAgentFileUploadBytes {
			_ = os.Remove(absPath)
			fail(c, http.StatusBadRequest, errors.New("file is too large"))
			return
		}
		fileHash := hex.EncodeToString(hasher.Sum(nil))

		mediaType := strings.TrimSpace(file.Header.Get("Content-Type"))
		if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
			mediaType = parsed
		}
		if mediaType == "" {
			mediaType = mime.TypeByExtension(strings.ToLower(filepath.Ext(filename)))
		}
		if mediaType == "" {
			mediaType = "application/octet-stream"
		}
		contentPreview, previewErr := agentTemplateFilePreview(absPath, filename)
		if previewErr != nil {
			log.WithError(previewErr).WithField("path", absPath).
				Debug("agent: template preview unavailable")
		}

		relativeDir := agentDataDisplayPath(targetDir)
		now := time.Now().Unix()
		record := agentTemplateRecord{
			Filename:        filename,
			OriginalName:    originalName,
			Directory:       relativeDir,
			Path:            relativeDir + "/" + filename,
			MediaType:       mediaType,
			Size:            written,
			Hash:            fileHash,
			ContentPreview:  contentPreview,
			OutputDirectory: agentDataDisplayPath(outputDir),
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		upload, err := upsertAgentTemplateRecord(store.DataDir(), targetDir, record, absPath)
		if err != nil {
			_ = os.Remove(absPath)
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, upload)
	}
}

func handleListAgentFiles(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil || strings.TrimSpace(store.DataDir()) == "" {
			fail(c, http.StatusInternalServerError, errors.New("data directory is unavailable"))
			return
		}

		targetDir, _ := agentUploadDirectories(c.Query("purpose"))
		records, err := listAgentTemplateRecords(store.DataDir(), targetDir)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}

		files := make([]AgentFileUpload, 0, len(records))
		for _, record := range records {
			files = append(files, agentTemplateRecordToUpload(record))
		}
		ok(c, files)
	}
}

func handleWriteAgentReport(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		if store == nil || strings.TrimSpace(store.DataDir()) == "" {
			fail(c, http.StatusInternalServerError, errors.New("data directory is unavailable"))
			return
		}

		var req AgentReportWriteRequest
		if !bindJSON(c, &req) {
			return
		}

		result, err := writeAgentReportFiles(store.DataDir(), req)
		if err != nil {
			fail(c, http.StatusBadRequest, err)
			return
		}
		ok(c, result)
	}
}

func writeAgentReportFiles(dataDir string, req AgentReportWriteRequest) (AgentReportWriteResult, error) {
	outputDir := normalizeAgentReportOutputDirectory(req.OutputDirectory)
	absDir, err := agentOutputAbsDir(dataDir, outputDir)
	if err != nil {
		return AgentReportWriteResult{}, err
	}
	if err := os.MkdirAll(absDir, 0755); err != nil {
		return AgentReportWriteResult{}, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "agent-report"
	}
	files := normalizeAgentReportOutputFiles(req)
	if len(files) == 0 {
		return AgentReportWriteResult{}, errors.New("report output files are required")
	}
	ts := time.Now().Format("20060102-150405")

	writtenFiles := make([]AgentReportWrittenFile, 0, len(files))
	usedNames := make(map[string]int)
	for i, file := range files {
		ext := normalizeAgentReportOutputExtension(file.Extension, file.Label)
		label := strings.TrimSpace(file.Label)
		if label == "" {
			label = fmt.Sprintf("输出 %d", i+1)
		}
		base := agentReportOutputFileBaseName(title, file)
		labelSegment := safeAgentReportFilenameSegment(label)
		nameStem := fmt.Sprintf("%s-%s", base, ts)
		if len(files) > 1 {
			nameStem = fmt.Sprintf("%s-%s-%s", base, ts, labelSegment)
		}
		filename := nameStem + ext
		if count := usedNames[filename]; count > 0 {
			filename = fmt.Sprintf("%s-%d%s", nameStem, count+1, ext)
		}
		usedNames[filename]++

		content := strings.TrimRight(file.Content, "\r\n")
		if content == "" {
			content = fmt.Sprintf("%s\n\n暂无输出内容。", title)
		}
		if ext == ".json" && !json.Valid([]byte(content)) {
			content = string(mustMarshalAgentReportFallbackJSON(title, label, content))
		}
		fileBytes, err := encodeAgentReportOutputFile(dataDir, ext, title, label, content, file)
		if err != nil {
			return AgentReportWriteResult{}, err
		}
		if err := os.WriteFile(filepath.Join(absDir, filename), fileBytes, 0600); err != nil {
			return AgentReportWriteResult{}, err
		}
		writtenFiles = append(writtenFiles, AgentReportWrittenFile{
			Label:     label,
			Path:      outputDir + "/" + filename,
			Extension: strings.TrimPrefix(ext, "."),
			Size:      int64(len(fileBytes)),
		})
	}

	markdownPath := ""
	csvPath := ""
	for _, file := range writtenFiles {
		if file.Extension == "md" && markdownPath == "" {
			markdownPath = file.Path
		}
		if file.Extension == "csv" && csvPath == "" {
			csvPath = file.Path
		}
	}

	return AgentReportWriteResult{
		OutputDirectory: outputDir,
		Files:           writtenFiles,
		MarkdownPath:    markdownPath,
		CSVPath:         csvPath,
		CreatedAt:       time.Now().Unix(),
	}, nil
}

func normalizeAgentReportOutputFiles(req AgentReportWriteRequest) []AgentReportOutputFile {
	if len(req.Files) > 0 {
		out := make([]AgentReportOutputFile, 0, len(req.Files))
		for _, file := range req.Files {
			if strings.TrimSpace(file.Label) == "" && strings.TrimSpace(file.Content) == "" {
				continue
			}
			out = append(out, file)
		}
		return out
	}

	var out []AgentReportOutputFile
	if strings.TrimSpace(req.Markdown) != "" {
		out = append(out, AgentReportOutputFile{
			Label:     "Markdown 日报",
			Extension: "md",
			Content:   req.Markdown,
		})
	}
	if strings.TrimSpace(req.CSV) != "" {
		out = append(out, AgentReportOutputFile{
			Label:     "CSV 表格",
			Extension: "csv",
			Content:   req.CSV,
		})
	}
	return out
}

func normalizeAgentReportOutputExtension(value, label string) string {
	ext := strings.ToLower(strings.TrimSpace(value))
	ext = strings.TrimPrefix(ext, ".")
	if ext == "" {
		ext = agentReportExtensionForLabel(label)
	}
	ext = "." + ext
	switch ext {
	case ".htm":
		ext = ".html"
	case ".markdown":
		ext = ".md"
	case ".text":
		ext = ".txt"
	}
	if !allowedAgentReportOutputExtensions[ext] {
		return ".txt"
	}
	return ext
}

func encodeAgentReportOutputFile(dataDir, ext, title, label, content string, file AgentReportOutputFile) ([]byte, error) {
	if templateBytes, ok := readMatchingAgentReportTemplate(dataDir, ext, file); ok {
		switch ext {
		case ".xlsx":
			if fileBytes, err := buildAgentReportXLSXFromTemplate(templateBytes, title, label, content); err == nil {
				return fileBytes, nil
			}
			return buildAgentReportXLSX(title, label, content)
		case ".docx":
			return buildAgentReportDOCX(title, label, content)
		case ".pdf":
			return buildAgentReportPDF(title, label, content)
		default:
			return []byte(content + "\n"), nil
		}
	}

	switch ext {
	case ".xlsx":
		return buildAgentReportXLSX(title, label, content)
	case ".docx":
		return buildAgentReportDOCX(title, label, content)
	case ".pdf":
		return buildAgentReportPDF(title, label, content)
	default:
		if ext == ".html" {
			return []byte(content + "\n"), nil
		}
		return []byte(content + "\n"), nil
	}
}

func readMatchingAgentReportTemplate(dataDir, ext string, file AgentReportOutputFile) ([]byte, bool) {
	templatePath := strings.TrimSpace(file.TemplatePath)
	if templatePath == "" {
		return nil, false
	}
	templateExt := strings.ToLower(filepath.Ext(filepath.FromSlash(templatePath)))
	if templateExt == ".htm" {
		templateExt = ".html"
	}
	if templateExt == ".xls" && ext == ".xlsx" {
		templateExt = ".xlsx"
	}
	if templateExt == ".doc" && ext == ".docx" {
		templateExt = ".docx"
	}
	if templateExt != ext {
		return nil, false
	}
	absPath, err := agentUploadedTemplateAbsPath(dataDir, templatePath)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(absPath)
	if err != nil || len(data) == 0 {
		return nil, false
	}
	return data, true
}

func agentReportExtensionForLabel(label string) string {
	lower := strings.ToLower(strings.TrimSpace(label))
	switch {
	case strings.Contains(lower, "markdown") || strings.Contains(lower, "md"):
		return "md"
	case strings.Contains(lower, "html") || strings.Contains(label, "网页"):
		return "html"
	case strings.Contains(lower, "json"):
		return "json"
	case strings.Contains(lower, "txt") || strings.Contains(label, "文本"):
		return "txt"
	case strings.Contains(lower, "xlsx") || strings.Contains(lower, "excel"):
		return "xlsx"
	case strings.Contains(lower, "pdf"):
		return "pdf"
	case strings.Contains(lower, "docx") || strings.Contains(lower, "word") ||
		(strings.Contains(label, "文档") && !strings.Contains(lower, "pdf")):
		return "docx"
	case strings.Contains(lower, "csv") || strings.Contains(lower, "excel") || strings.Contains(label, "表格"):
		return "csv"
	default:
		return "md"
	}
}

func agentReportOutputFileBaseName(title string, file AgentReportOutputFile) string {
	name := strings.TrimSpace(file.TemplateOriginalName)
	if name == "" {
		name = strings.TrimSpace(filepath.Base(filepath.FromSlash(file.TemplatePath)))
	}
	name = strings.TrimSuffix(name, filepath.Ext(name))
	if base := safeAgentReportFilenameSegment(name); base != "" && base != "output" {
		return base
	}
	if base := safeAgentReportFilenameSegment(title); base != "" && base != "output" {
		return base
	}
	return "agent-report"
}

func safeAgentReportFilenameSegment(value string) string {
	value = strings.TrimSpace(value)
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(strings.ToLower(b.String()), "-_")
	if out == "" {
		out = safeImageHistorySegment(value)
	}
	if out == "" || out == "image" {
		out = "output"
	}
	return truncateRunes(out, 48)
}

func buildAgentReportXLSX(title, label, content string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>
<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>
</Relationships>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
<sheets><sheet name="Report" sheetId="1" r:id="rId1"/></sheets>
</workbook>`,
		"xl/worksheets/sheet1.xml": buildAgentReportWorksheetXML(title, label, content),
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := io.WriteString(w, body); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildAgentReportWorksheetXML(title, label, content string) string {
	rows := agentReportWorksheetRows(title, label, content)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">`)
	sb.WriteString(buildAgentReportSheetDataXML(rows))
	sb.WriteString(`</worksheet>`)
	return sb.String()
}

func buildAgentReportSheetDataXML(rows [][]string) string {
	var sb strings.Builder
	sb.WriteString(`<sheetData>`)
	for i, row := range rows {
		rowIndex := i + 1
		sb.WriteString(fmt.Sprintf(`<row r="%d">`, rowIndex))
		for j, cell := range row {
			column := excelColumnName(j + 1)
			sb.WriteString(fmt.Sprintf(
				`<c r="%s%d" t="inlineStr"><is><t>%s</t></is></c>`,
				column,
				rowIndex,
				xmlEscape(cell),
			))
		}
		sb.WriteString(`</row>`)
	}
	sb.WriteString(`</sheetData>`)
	return sb.String()
}

func buildAgentReportXLSXFromTemplate(templateBytes []byte, title, label, content string) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(templateBytes), int64(len(templateBytes)))
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	rows := agentReportWorksheetRows(title, label, content)
	hasPlaceholders := agentReportTemplateHasPlaceholders(reader)
	worksheetUpdated := false
	placeholderReplaced := false

	for _, file := range reader.File {
		rc, err := file.Open()
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		data, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			_ = writer.Close()
			return nil, readErr
		}
		if closeErr != nil {
			_ = writer.Close()
			return nil, closeErr
		}

		if strings.HasSuffix(strings.ToLower(file.Name), ".xml") {
			var replaced bool
			data, replaced = replaceAgentReportTemplatePlaceholders(data, title, label, content)
			placeholderReplaced = placeholderReplaced || replaced
		}

		if !worksheetUpdated && isAgentReportWorksheetEntry(file.Name) && !hasPlaceholders {
			data = replaceAgentReportWorksheetData(data, []byte(buildAgentReportSheetDataXML(rows)))
			worksheetUpdated = true
		}

		header := file.FileHeader
		w, err := writer.CreateHeader(&header)
		if err != nil {
			_ = writer.Close()
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}
	if !worksheetUpdated && !placeholderReplaced {
		return nil, errors.New("template worksheet not found")
	}
	return buf.Bytes(), nil
}

func agentReportTemplateHasPlaceholders(reader *zip.Reader) bool {
	for _, file := range reader.File {
		if !strings.HasSuffix(strings.ToLower(file.Name), ".xml") {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, 2*1024*1024))
		_ = rc.Close()
		if readErr != nil {
			continue
		}
		if agentReportTemplatePlaceholderPattern.Match(data) {
			return true
		}
	}
	return false
}

func isAgentReportWorksheetEntry(name string) bool {
	name = strings.ToLower(filepath.ToSlash(name))
	return strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml")
}

func replaceAgentReportWorksheetData(worksheet, sheetData []byte) []byte {
	source := string(worksheet)
	start := strings.Index(source, "<sheetData")
	if start >= 0 {
		endRelative := strings.Index(source[start:], "</sheetData>")
		if endRelative >= 0 {
			end := start + endRelative + len("</sheetData>")
			return []byte(source[:start] + string(sheetData) + source[end:])
		}
	}
	insertAt := strings.Index(source, "</worksheet>")
	if insertAt < 0 {
		return worksheet
	}
	return []byte(source[:insertAt] + string(sheetData) + source[insertAt:])
}

func replaceAgentReportTemplatePlaceholders(data []byte, title, label, content string) ([]byte, bool) {
	source := string(data)
	replacements := map[string]string{
		"{{title}}":         xmlEscape(title),
		"{{标题}}":            xmlEscape(title),
		"{{output_format}}": xmlEscape(label),
		"{{输出格式}}":          xmlEscape(label),
		"{{content}}":       xmlEscape(content),
		"{{内容}}":            xmlEscape(content),
		"{{generated_at}}":  xmlEscape(time.Now().Format("2006-01-02 15:04:05")),
		"{{生成时间}}":          xmlEscape(time.Now().Format("2006-01-02 15:04:05")),
	}
	replaced := false
	for placeholder, value := range replacements {
		if strings.Contains(source, placeholder) {
			source = strings.ReplaceAll(source, placeholder, value)
			replaced = true
		}
	}
	if !replaced {
		return data, false
	}
	return []byte(source), true
}

func agentReportWorksheetRows(title, label, content string) [][]string {
	if rows := parseMarkdownTableRows(content); len(rows) > 0 {
		return rows
	}

	lines := strings.Split(content, "\n")
	rows := [][]string{
		{"标题", title},
		{"输出格式", label},
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rows = append(rows, []string{"内容", line})
	}
	if len(rows) == 2 {
		rows = append(rows, []string{"内容", "暂无输出内容。"})
	}
	return rows
}

func parseMarkdownTableRows(content string) [][]string {
	lines := strings.Split(content, "\n")
	var rows [][]string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
			continue
		}
		cells := markdownTableCells(line)
		if len(cells) == 0 || markdownTableSeparator(cells) {
			continue
		}
		rows = append(rows, cells)
	}
	return rows
}

func markdownTableCells(line string) []string {
	line = strings.Trim(line, "|")
	parts := strings.Split(line, "|")
	cells := make([]string, 0, len(parts))
	for _, part := range parts {
		cell := strings.TrimSpace(strings.ReplaceAll(part, `\|`, "|"))
		cells = append(cells, cell)
	}
	return cells
}

func markdownTableSeparator(cells []string) bool {
	for _, cell := range cells {
		normalized := strings.Trim(cell, " :-")
		if normalized != "" {
			return false
		}
	}
	return true
}

func excelColumnName(index int) string {
	if index <= 0 {
		return "A"
	}
	var chars []byte
	for index > 0 {
		index--
		chars = append([]byte{byte('A' + index%26)}, chars...)
		index /= 26
	}
	return string(chars)
}

func buildAgentReportDOCX(title, label, content string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
<Default Extension="xml" ContentType="application/xml"/>
<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`,
		"word/document.xml": buildAgentReportDocumentXML(title, label, content),
	}
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			_ = zw.Close()
			return nil, err
		}
		if _, err := io.WriteString(w, body); err != nil {
			_ = zw.Close()
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func buildAgentReportDocumentXML(title, label, content string) string {
	paragraphs := []string{title, "输出格式: " + label}
	paragraphs = append(paragraphs, strings.Split(content, "\n")...)
	var sb strings.Builder
	sb.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`)
	sb.WriteString(`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, paragraph := range paragraphs {
		sb.WriteString(`<w:p><w:r><w:t xml:space="preserve">`)
		sb.WriteString(xmlEscape(paragraph))
		sb.WriteString(`</w:t></w:r></w:p>`)
	}
	sb.WriteString(`</w:body></w:document>`)
	return sb.String()
}

func buildAgentReportPDF(title, label, content string) ([]byte, error) {
	lines := []string{title, "输出格式: " + label}
	lines = append(lines, strings.Split(content, "\n")...)
	if len(lines) > 36 {
		lines = lines[:36]
		lines = append(lines, "...")
	}
	stream := buildPDFTextStream(lines)

	var buf bytes.Buffer
	offsets := []int{0}
	writeObject := func(id int, body string) {
		offsets = append(offsets, buf.Len())
		fmt.Fprintf(&buf, "%d 0 obj\n%s\nendobj\n", id, body)
	}
	buf.WriteString("%PDF-1.4\n")
	writeObject(1, "<< /Type /Catalog /Pages 2 0 R >>")
	writeObject(2, "<< /Type /Pages /Kids [3 0 R] /Count 1 >>")
	writeObject(3, "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 595 842] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>")
	writeObject(4, "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>")
	writeObject(5, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(stream), stream))
	xrefStart := buf.Len()
	buf.WriteString("xref\n0 6\n")
	buf.WriteString("0000000000 65535 f \n")
	for i := 1; i <= 5; i++ {
		fmt.Fprintf(&buf, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&buf, "trailer\n<< /Size 6 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xrefStart)
	return buf.Bytes(), nil
}

func buildPDFTextStream(lines []string) string {
	var sb strings.Builder
	sb.WriteString("BT\n/F1 10 Tf\n50 790 Td\n14 TL\n")
	for _, line := range lines {
		ascii := toPDFSafeASCII(line)
		if strings.TrimSpace(ascii) == "" {
			sb.WriteString("T*\n")
			continue
		}
		sb.WriteString("(")
		sb.WriteString(escapePDFString(ascii))
		sb.WriteString(") Tj\nT*\n")
	}
	sb.WriteString("ET")
	return sb.String()
}

func toPDFSafeASCII(value string) string {
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 32 && r <= 126:
			b.WriteRune(r)
		case r == '\t':
			b.WriteRune(' ')
		case r == '\r' || r == '\n':
			b.WriteRune(' ')
		default:
			b.WriteRune('?')
		}
	}
	return b.String()
}

func escapePDFString(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `(`, `\(`)
	value = strings.ReplaceAll(value, `)`, `\)`)
	return value
}

func xmlEscape(value string) string {
	var b strings.Builder
	if err := xml.EscapeText(&b, []byte(value)); err != nil {
		return html.EscapeString(value)
	}
	return b.String()
}

func mustMarshalAgentReportFallbackJSON(title, label, content string) []byte {
	data, err := json.MarshalIndent(map[string]string{
		"title":   title,
		"label":   label,
		"content": content,
	}, "", "  ")
	if err != nil {
		return []byte("{}")
	}
	return data
}

func normalizeAgentReportOutputDirectory(value string) string {
	value = strings.Trim(strings.TrimSpace(filepath.ToSlash(value)), "/")
	if value == "" {
		return agentDataDisplayPath(agentDataOutputDirReports)
	}
	if strings.HasPrefix(value, "data/output/") {
		return value
	}
	if strings.HasPrefix(value, "output/") {
		return "data/" + value
	}
	return agentDataDisplayPath(agentDataOutputDirReports)
}

func agentOutputAbsDir(dataDir, displayPath string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	displayPath = strings.Trim(strings.TrimSpace(filepath.ToSlash(displayPath)), "/")
	if dataDir == "" || displayPath == "" {
		return "", errors.New("output directory is required")
	}
	if !strings.HasPrefix(displayPath, "data/output/") {
		return "", errors.New("output directory must be under data/output")
	}

	rel := strings.TrimPrefix(displayPath, "data/")
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || filepath.IsAbs(cleanRel) ||
		strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) ||
		cleanRel == ".." {
		return "", errors.New("invalid output directory")
	}

	root := filepath.Clean(filepath.Join(dataDir, "output"))
	full := filepath.Clean(filepath.Join(dataDir, cleanRel))
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", errors.New("invalid output directory")
	}
	return full, nil
}

func upsertAgentTemplateRecord(dataDir, targetDir string, record agentTemplateRecord, absPath string) (AgentFileUpload, error) {
	agentTemplateIndexMu.Lock()
	defer agentTemplateIndexMu.Unlock()

	indexPath := agentTemplateIndexPath(dataDir, targetDir)
	index, err := readAgentTemplateIndex(indexPath)
	if err != nil {
		return AgentFileUpload{}, err
	}

	records := filterExistingAgentTemplateRecords(dataDir, index.Templates)
	for _, existing := range records {
		if existing.Hash == record.Hash {
			_ = os.Remove(absPath)
			existing.UpdatedAt = time.Now().Unix()
			if err := writeAgentTemplateIndex(indexPath, agentTemplateIndex{Templates: records}); err != nil {
				return AgentFileUpload{}, err
			}
			upload := agentTemplateRecordToUpload(existing)
			upload.Reused = true
			return upload, nil
		}
	}

	replaced := false
	next := make([]agentTemplateRecord, 0, len(records)+1)
	recordNameKey := normalizeAgentTemplateOriginalName(record.OriginalName)
	for _, existing := range records {
		if normalizeAgentTemplateOriginalName(existing.OriginalName) == recordNameKey {
			replaced = true
			_ = removeAgentTemplateRecordFile(dataDir, existing)
			continue
		}
		next = append(next, existing)
	}

	next = append(next, record)
	sortAgentTemplateRecords(next)
	if err := writeAgentTemplateIndex(indexPath, agentTemplateIndex{Templates: next}); err != nil {
		return AgentFileUpload{}, err
	}

	upload := agentTemplateRecordToUpload(record)
	upload.Replaced = replaced
	return upload, nil
}

func listAgentTemplateRecords(dataDir, targetDir string) ([]agentTemplateRecord, error) {
	agentTemplateIndexMu.Lock()
	defer agentTemplateIndexMu.Unlock()

	indexPath := agentTemplateIndexPath(dataDir, targetDir)
	index, err := readAgentTemplateIndex(indexPath)
	if err != nil {
		return nil, err
	}
	records := filterExistingAgentTemplateRecords(dataDir, index.Templates)
	sortAgentTemplateRecords(records)
	if len(records) != len(index.Templates) {
		if err := writeAgentTemplateIndex(indexPath, agentTemplateIndex{Templates: records}); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func agentTemplateIndexPath(dataDir, targetDir string) string {
	return filepath.Join(dataDir, filepath.FromSlash(targetDir), agentTemplateIndexFilename)
}

func readAgentTemplateIndex(indexPath string) (agentTemplateIndex, error) {
	var index agentTemplateIndex
	data, err := os.ReadFile(indexPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return index, nil
		}
		return index, err
	}
	if len(data) == 0 {
		return index, nil
	}
	if err := json.Unmarshal(data, &index); err != nil {
		return agentTemplateIndex{}, err
	}
	return index, nil
}

func writeAgentTemplateIndex(indexPath string, index agentTemplateIndex) error {
	if err := os.MkdirAll(filepath.Dir(indexPath), 0755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(indexPath, data, 0600)
}

func filterExistingAgentTemplateRecords(dataDir string, records []agentTemplateRecord) []agentTemplateRecord {
	out := make([]agentTemplateRecord, 0, len(records))
	for _, record := range records {
		if record.Path == "" || record.Hash == "" {
			continue
		}
		if _, err := agentUploadedTemplateAbsPath(dataDir, record.Path); err != nil {
			continue
		}
		out = append(out, record)
	}
	return out
}

func removeAgentTemplateRecordFile(dataDir string, record agentTemplateRecord) error {
	absPath, err := agentUploadedTemplateAbsPath(dataDir, record.Path)
	if err != nil {
		return nil
	}
	return os.Remove(absPath)
}

func sortAgentTemplateRecords(records []agentTemplateRecord) {
	sort.SliceStable(records, func(i, j int) bool {
		left := records[i].UpdatedAt
		if left == 0 {
			left = records[i].CreatedAt
		}
		right := records[j].UpdatedAt
		if right == 0 {
			right = records[j].CreatedAt
		}
		if left == right {
			return records[i].OriginalName < records[j].OriginalName
		}
		return left > right
	})
}

func agentTemplateRecordToUpload(record agentTemplateRecord) AgentFileUpload {
	return AgentFileUpload{
		Filename:        record.Filename,
		OriginalName:    record.OriginalName,
		Directory:       record.Directory,
		Path:            record.Path,
		MediaType:       record.MediaType,
		Size:            record.Size,
		Hash:            record.Hash,
		ContentPreview:  record.ContentPreview,
		OutputDirectory: record.OutputDirectory,
		CreatedAt:       record.CreatedAt,
	}
}

func normalizeAgentTemplateOriginalName(name string) string {
	name = strings.TrimSpace(strings.ToLower(filepath.Base(filepath.FromSlash(name))))
	if name == "" {
		return "template"
	}
	return name
}

func agentUploadDirectories(purpose string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(purpose)) {
	case "web-template", "webpage-template", "web":
		return agentDataTemplateDirWeb, agentDataOutputDirWeb
	default:
		return agentDataTemplateDirReports, agentDataOutputDirReports
	}
}

func safeAgentUploadFilename(raw string) (string, string, error) {
	raw = strings.ReplaceAll(raw, "\\", "/")
	name := strings.TrimSpace(filepath.Base(filepath.FromSlash(raw)))
	name = strings.TrimSpace(strings.ReplaceAll(name, "\x00", ""))
	if name == "" || name == "." || name == ".." {
		return "", "", errors.New("filename is required")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !allowedAgentFileExtensions[ext] {
		return "", "", fmt.Errorf("unsupported file type %q", ext)
	}

	stem := strings.TrimSuffix(name, filepath.Ext(name))
	var b strings.Builder
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9',
			r == '-',
			r == '_',
			r == '.':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}

	cleaned := strings.Trim(b.String(), ".-")
	if cleaned == "" {
		cleaned = "template"
	}
	suffix := time.Now().Format("20060102-150405") + "-" + randomAgentUploadSuffix()
	maxStemRunes := 180 - len(ext) - len(suffix) - 1
	if maxStemRunes < 32 {
		maxStemRunes = 32
	}
	filename := truncateRunes(cleaned, maxStemRunes) + "-" + suffix + ext
	return filename, name, nil
}

func randomAgentUploadSuffix() string {
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	}
	return hex.EncodeToString(b[:])
}

func agentDataDisplayPath(rel string) string {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" {
		return ""
	}
	return "data/" + rel
}

func agentUploadedTemplateAbsPath(dataDir, displayPath string) (string, error) {
	dataDir = strings.TrimSpace(dataDir)
	displayPath = strings.Trim(strings.TrimSpace(filepath.ToSlash(displayPath)), "/")
	if dataDir == "" || displayPath == "" {
		return "", errors.New("template path is required")
	}
	if !strings.HasPrefix(displayPath, "data/templates/") {
		return "", errors.New("template path must be under data/templates")
	}

	rel := strings.TrimPrefix(displayPath, "data/")
	cleanRel := filepath.Clean(filepath.FromSlash(rel))
	if cleanRel == "." || filepath.IsAbs(cleanRel) ||
		strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) ||
		cleanRel == ".." {
		return "", errors.New("invalid template path")
	}

	root := filepath.Clean(filepath.Join(dataDir, "templates"))
	full := filepath.Clean(filepath.Join(dataDir, cleanRel))
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", errors.New("invalid template path")
	}

	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("template path is a directory")
	}
	return full, nil
}

func agentTemplateFilePreview(absPath, filename string) (string, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".csv", ".json", ".md", ".txt":
		raw, err := readAgentTemplateTextFile(absPath)
		if err != nil {
			return "", err
		}
		return normalizeAgentTemplatePreview(raw), nil
	case ".htm", ".html":
		raw, err := readAgentTemplateTextFile(absPath)
		if err != nil {
			return "", err
		}
		return normalizeAgentTemplatePreview(stripAgentHTMLTags(raw)), nil
	case ".docx", ".xlsx":
		return readOfficeTemplatePreview(absPath, ext)
	case ".doc", ".pdf", ".xls":
		return normalizeAgentTemplatePreview(fmt.Sprintf(
			"当前上传模板文件: %s\n文件类型: %s\n该二进制模板已绑定到当前节点，生成时以该文件和节点描述为准。",
			filename,
			strings.TrimPrefix(ext, "."),
		)), nil
	default:
		return "", fmt.Errorf("unsupported template preview type %q", ext)
	}
}

func readAgentTemplateTextFile(absPath string) (string, error) {
	file, err := os.Open(absPath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxAgentTemplatePreviewBytes+1))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func readOfficeTemplatePreview(absPath, ext string) (string, error) {
	reader, err := zip.OpenReader(absPath)
	if err != nil {
		return "", err
	}
	defer reader.Close()

	parts := make([]string, 0, 6)
	for _, file := range reader.File {
		if len(parts) >= 6 {
			break
		}
		if file.FileInfo().IsDir() || !isOfficeTemplatePreviewEntry(file.Name, ext) {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxAgentTemplatePreviewBytes+1))
		closeErr := rc.Close()
		if readErr != nil || closeErr != nil {
			continue
		}

		if text := extractAgentXMLText(data); text != "" {
			parts = append(parts, text)
		}
	}
	if len(parts) == 0 {
		return "", errors.New("no readable office template content")
	}
	return normalizeAgentTemplatePreview(strings.Join(parts, "\n")), nil
}

func isOfficeTemplatePreviewEntry(name, ext string) bool {
	name = strings.ToLower(filepath.ToSlash(name))
	if ext == ".docx" {
		return name == "word/document.xml" ||
			(strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml")) ||
			(strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml"))
	}
	if ext == ".xlsx" {
		return name == "xl/sharedstrings.xml" ||
			(strings.HasPrefix(name, "xl/worksheets/sheet") && strings.HasSuffix(name, ".xml"))
	}
	return false
}

func extractAgentXMLText(data []byte) string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var b strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			break
		}

		switch t := token.(type) {
		case xml.StartElement:
			switch strings.ToLower(t.Name.Local) {
			case "br", "p", "row", "tr":
				b.WriteByte('\n')
			}
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte(' ')
			}
			b.WriteString(text)
		}
	}
	return normalizeAgentTemplatePreview(b.String())
}

func stripAgentHTMLTags(value string) string {
	var b strings.Builder
	inTag := false
	for _, r := range value {
		switch r {
		case '<':
			inTag = true
			b.WriteByte(' ')
		case '>':
			inTag = false
			b.WriteByte(' ')
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return html.UnescapeString(b.String())
}

func normalizeAgentTemplatePreview(value string) string {
	value = strings.ReplaceAll(value, "\x00", "")
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")

	lines := strings.Split(value, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return truncateRunes(strings.Join(cleaned, "\n"), maxAgentTemplatePreviewRunes)
}
