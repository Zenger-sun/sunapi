package sunapi

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	playgroundImageHistoryDir = "playground-images"
	maxGeneratedImageBytes    = 32 * 1024 * 1024
)

func persistPlaygroundImageURLs(ctx context.Context, dataDir, id string, urls []string, createdAt int64) []string {
	if strings.TrimSpace(dataDir) == "" || strings.TrimSpace(id) == "" {
		return cleanStringList(urls, 16, 2048)
	}
	out := make([]string, 0, len(urls))
	for index, rawURL := range urls {
		if index >= 16 {
			break
		}
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			continue
		}
		if isStoredPlaygroundImageURL(rawURL) {
			out = append(out, truncateRunes(rawURL, 2048))
			continue
		}
		data, mediaType, err := readImageURL(ctx, rawURL)
		if err != nil {
			out = append(out, truncateRunes(rawURL, 2048))
			continue
		}
		localURL, err := savePlaygroundImageFile(dataDir, id, index, createdAt, mediaType, data)
		if err != nil {
			out = append(out, truncateRunes(rawURL, 2048))
			continue
		}
		out = append(out, localURL)
	}
	if out == nil {
		return []string{}
	}
	return out
}

func isStoredPlaygroundImageURL(rawURL string) bool {
	return strings.HasPrefix(strings.TrimSpace(rawURL), "/api/playground/image-history/files/")
}

func deleteStoredPlaygroundImageURLs(dataDir string, urls []string) {
	for _, rawURL := range urls {
		rawURL = strings.TrimSpace(rawURL)
		if !isStoredPlaygroundImageURL(rawURL) {
			continue
		}
		relPath := strings.TrimPrefix(rawURL, "/api/playground/image-history/files/")
		path, err := playgroundImageHistoryFilePath(dataDir, relPath)
		if err != nil {
			continue
		}
		_ = os.Remove(path)
	}
}

func readImageURL(ctx context.Context, rawURL string) ([]byte, string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return nil, "", errors.New("image url is empty")
	}
	if strings.HasPrefix(rawURL, "data:") {
		return readDataImageURL(rawURL)
	}
	if !strings.HasPrefix(strings.ToLower(rawURL), "http://") && !strings.HasPrefix(strings.ToLower(rawURL), "https://") {
		return nil, "", errors.New("unsupported image url")
	}
	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("failed to download generated image: %s", resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxGeneratedImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxGeneratedImageBytes {
		return nil, "", errors.New("generated image is too large")
	}
	mediaType := strings.TrimSpace(resp.Header.Get("Content-Type"))
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		mediaType = parsed
	}
	if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		mediaType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return nil, "", errors.New("downloaded content is not an image")
	}
	return data, mediaType, nil
}

func readDataImageURL(rawURL string) ([]byte, string, error) {
	comma := strings.Index(rawURL, ",")
	if comma <= 5 {
		return nil, "", errors.New("invalid data url")
	}
	meta := rawURL[5:comma]
	if !strings.Contains(strings.ToLower(meta), ";base64") {
		return nil, "", errors.New("only base64 data urls are supported")
	}
	mediaType := strings.Split(meta, ";")[0]
	if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return nil, "", errors.New("data url is not an image")
	}
	data, err := base64.StdEncoding.DecodeString(rawURL[comma+1:])
	if err != nil {
		return nil, "", err
	}
	if len(data) > maxGeneratedImageBytes {
		return nil, "", errors.New("generated image is too large")
	}
	return data, mediaType, nil
}

func savePlaygroundImageFile(dataDir, id string, index int, createdAt int64, mediaType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", errors.New("image is empty")
	}
	dateDir := imageHistoryDateDir(createdAt)
	dir := filepath.Join(dataDir, playgroundImageHistoryDir, dateDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	ext := imageExtension(mediaType)
	name := fmt.Sprintf("%s-%02d%s", safeImageHistorySegment(id), index+1, ext)
	if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
		return "", err
	}
	rel := filepath.ToSlash(filepath.Join(playgroundImageHistoryDir, dateDir, name))
	return "/api/playground/image-history/files/" + rel, nil
}

func imageHistoryDateDir(createdAt int64) string {
	if createdAt <= 0 {
		return time.Now().Format("20060102")
	}
	ts := createdAt
	if ts > 9999999999 {
		ts = ts / 1000
	}
	return time.Unix(ts, 0).Format("20060102")
}

func safeImageHistorySegment(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	out := b.String()
	if out == "" {
		return "image"
	}
	return truncateRunes(out, 96)
}

func imageExtension(mediaType string) string {
	switch strings.ToLower(strings.TrimSpace(mediaType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	default:
		return ".png"
	}
}

func playgroundImageHistoryFilePath(dataDir, relPath string) (string, error) {
	relPath = strings.TrimPrefix(strings.TrimSpace(relPath), "/")
	relPath = filepath.Clean(filepath.FromSlash(relPath))
	if relPath == "." || filepath.IsAbs(relPath) || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) || relPath == ".." {
		return "", errors.New("invalid image path")
	}
	root := filepath.Clean(filepath.Join(dataDir, playgroundImageHistoryDir))
	full := filepath.Clean(filepath.Join(dataDir, relPath))
	if full != root && !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", errors.New("invalid image path")
	}
	info, err := os.Stat(full)
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", errors.New("invalid image path")
	}
	return full, nil
}
