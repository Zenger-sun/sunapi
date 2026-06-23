package sunapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAdminAuthProtectsManagementRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	engine := gin.New()
	RegisterRoutes(engine, store, nil)

	response := performJSON(engine, http.MethodGet, "/api/dashboard", nil, "")
	if response.Code != http.StatusPreconditionRequired {
		t.Fatalf("dashboard before setup status = %d, want %d", response.Code, http.StatusPreconditionRequired)
	}

	setup := performJSON(engine, http.MethodPost, "/api/auth/setup", adminCredentialsPayload{
		Username: "admin",
		Password: "password123",
	}, "")
	if setup.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", setup.Code, setup.Body.String())
	}
	sessionCookie := setup.Result().Cookies()[0]

	response = performJSON(engine, http.MethodGet, "/api/dashboard", nil, sessionCookie.String())
	if response.Code != http.StatusOK {
		t.Fatalf("dashboard after setup status = %d, body = %s", response.Code, response.Body.String())
	}

	logout := performJSON(engine, http.MethodPost, "/api/auth/logout", nil, sessionCookie.String())
	if logout.Code != http.StatusOK {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}

	response = performJSON(engine, http.MethodGet, "/api/dashboard", nil, sessionCookie.String())
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard after logout status = %d, want %d", response.Code, http.StatusUnauthorized)
	}
}

func TestAdminPasswordCanBeChanged(t *testing.T) {
	gin.SetMode(gin.TestMode)

	store, err := OpenStore(filepath.Join(t.TempDir(), "sunapi.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	engine := gin.New()
	RegisterRoutes(engine, store, nil)

	setup := performJSON(engine, http.MethodPost, "/api/auth/setup", adminCredentialsPayload{
		Username: "admin",
		Password: "password123",
	}, "")
	if setup.Code != http.StatusOK {
		t.Fatalf("setup status = %d, body = %s", setup.Code, setup.Body.String())
	}
	sessionCookie := setup.Result().Cookies()[0]

	change := performJSON(engine, http.MethodPost, "/api/auth/password", adminPasswordUpdatePayload{
		CurrentPassword: "password123",
		NewPassword:     "password456",
	}, sessionCookie.String())
	if change.Code != http.StatusOK {
		t.Fatalf("change password status = %d, body = %s", change.Code, change.Body.String())
	}

	oldLogin := performJSON(engine, http.MethodPost, "/api/auth/login", adminCredentialsPayload{
		Username: "admin",
		Password: "password123",
	}, "")
	if oldLogin.Code != http.StatusUnauthorized {
		t.Fatalf("old password login status = %d, want %d", oldLogin.Code, http.StatusUnauthorized)
	}

	newLogin := performJSON(engine, http.MethodPost, "/api/auth/login", adminCredentialsPayload{
		Username: "admin",
		Password: "password456",
	}, "")
	if newLogin.Code != http.StatusOK {
		t.Fatalf("new password login status = %d, body = %s", newLogin.Code, newLogin.Body.String())
	}
}

func performJSON(engine http.Handler, method, target string, body any, cookie string) *httptest.ResponseRecorder {
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		data, _ := json.Marshal(body)
		reader = bytes.NewReader(data)
	}
	request := httptest.NewRequest(method, target, reader)
	request.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}
