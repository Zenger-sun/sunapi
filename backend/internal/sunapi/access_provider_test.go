package sunapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"testing"

	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

func TestSQLiteAPIKeyAccessProviderAuthenticatesFromSQLite(t *testing.T) {
	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	created, err := store.CreateAPIKey(context.Background(), APIKeyPayload{
		Name:           "local-client",
		UnlimitedQuota: true,
		AllowIPs:       "127.0.0.1,10.0.0.0/8",
	})
	if err != nil {
		t.Fatal(err)
	}
	secret, err := store.APIKeySecret(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}

	provider := NewSQLiteAPIKeyAccessProvider(store)
	if got := provider.Identifier(); got != sqliteAPIKeyAccessProviderType {
		t.Fatalf("Identifier() = %q, want %q", got, sqliteAPIKeyAccessProviderType)
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	missingReq.RemoteAddr = "127.0.0.1:1234"
	if _, authErr := provider.Authenticate(context.Background(), missingReq); !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeNoCredentials) {
		t.Fatalf("Authenticate(no credentials) error = %#v, want no credentials", authErr)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	invalidReq.RemoteAddr = "127.0.0.1:1234"
	invalidReq.Header.Set("Authorization", "Bearer sk-wrong")
	if _, authErr := provider.Authenticate(context.Background(), invalidReq); !sdkaccess.IsAuthErrorCode(authErr, sdkaccess.AuthErrorCodeInvalidCredential) {
		t.Fatalf("Authenticate(invalid) error = %#v, want invalid credential", authErr)
	}

	validReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	validReq.RemoteAddr = "127.0.0.1:1234"
	validReq.Header.Set("Authorization", "Bearer sk-"+secret)

	result, authErr := provider.Authenticate(context.Background(), validReq)
	if authErr != nil {
		t.Fatalf("Authenticate(valid) error = %v", authErr)
	}
	if result == nil {
		t.Fatal("Authenticate(valid) result = nil")
	}
	if got, want := result.Principal, "api-key:"+itoa(created.ID); got != want {
		t.Fatalf("Principal = %q, want %q", got, want)
	}
	if got := result.Metadata["api_key_id"]; got != itoa(created.ID) {
		t.Fatalf("metadata api_key_id = %q, want %q", got, itoa(created.ID))
	}
	if got := result.Metadata["client_ip"]; got != "127.0.0.1" {
		t.Fatalf("metadata client_ip = %q, want %q", got, "127.0.0.1")
	}

	updated, err := store.GetAPIKey(context.Background(), created.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if updated.AccessedTime <= 0 {
		t.Fatalf("AccessedTime = %d, want > 0", updated.AccessedTime)
	}
}

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
