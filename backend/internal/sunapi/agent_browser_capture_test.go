package sunapi

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAgentBrowserScreenshot(t *testing.T) {
	dataDir := t.TempDir()
	encoded := base64.StdEncoding.EncodeToString([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'})
	capturedAt := time.Date(2026, 7, 5, 10, 11, 12, 13, time.UTC)

	path, err := writeAgentBrowserScreenshot(dataDir, "https://seller.goofish.com/?site=COMM", capturedAt, encoded)
	if err != nil {
		t.Fatalf("writeAgentBrowserScreenshot error=%v", err)
	}
	if !strings.HasPrefix(path, "data/agent-browser-captures/seller-goofish-com-20260705-101112-") {
		t.Fatalf("path=%q does not include expected data-relative capture prefix", path)
	}
	if !strings.HasSuffix(path, ".png") {
		t.Fatalf("path=%q does not end with .png", path)
	}

	relative := strings.TrimPrefix(path, "data/")
	info, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("expected screenshot file to exist: %v", err)
	}
	if info.Size() == 0 {
		t.Fatal("expected screenshot file to contain bytes")
	}
}

func TestWriteAgentBrowserScreenshotRejectsInvalidData(t *testing.T) {
	_, err := writeAgentBrowserScreenshot(t.TempDir(), "https://example.com", time.Now(), "not base64")
	if err == nil {
		t.Fatal("expected invalid screenshot data to fail")
	}
}
