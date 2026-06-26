package sunapi

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	adminSessionCookieName = "sunapi_admin_session"
	adminSessionTTL        = 7 * 24 * time.Hour
)

type adminCredentialsPayload struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type adminPasswordUpdatePayload struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func RegisterAuthRoutes(api *gin.RouterGroup, store *Store) {
	api.GET("/auth/status", func(c *gin.Context) {
		initialized, err := store.AdminInitialized(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		user, authenticated, err := adminFromSession(c, store)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, gin.H{
			"initialized":   initialized,
			"authenticated": initialized && authenticated,
			"user":          adminUserResponse(user, initialized && authenticated),
		})
	})

	api.GET("/auth/me", func(c *gin.Context) {
		initialized, err := store.AdminInitialized(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if !initialized {
			fail(c, http.StatusPreconditionRequired, errors.New("admin setup required"))
			return
		}
		user, authenticated, err := adminFromSession(c, store)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if !authenticated {
			fail(c, http.StatusUnauthorized, errors.New("admin login required"))
			return
		}
		ok(c, adminUserResponse(user, true))
	})

	api.POST("/auth/setup", func(c *gin.Context) {
		var payload adminCredentialsPayload
		if !bindJSON(c, &payload) {
			return
		}
		username, password, okPayload := validateAdminCredentials(payload, true)
		if !okPayload {
			fail(c, http.StatusBadRequest, errors.New("username and password are required; password must be at least 8 characters"))
			return
		}
		passwordHash, err := hashAdminPassword(password)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		user, err := store.CreateAdminUser(c.Request.Context(), username, passwordHash)
		if errors.Is(err, ErrAdminAlreadyInitialized) {
			fail(c, http.StatusConflict, err)
			return
		}
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		token, err := startAdminSession(c, store, user)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, adminLoginResponse(user, token))
	})

	api.POST("/auth/login", func(c *gin.Context) {
		initialized, err := store.AdminInitialized(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if !initialized {
			fail(c, http.StatusPreconditionRequired, errors.New("admin setup required"))
			return
		}

		var payload adminCredentialsPayload
		if !bindJSON(c, &payload) {
			return
		}
		username, password, okPayload := validateAdminCredentials(payload, false)
		if !okPayload {
			fail(c, http.StatusBadRequest, errors.New("username and password are required"))
			return
		}

		user, err := store.AdminUserByUsername(c.Request.Context(), username)
		if errors.Is(err, sql.ErrNoRows) {
			fail(c, http.StatusUnauthorized, errors.New("invalid username or password"))
			return
		}
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		if compareAdminPassword(user.PasswordHash, password) != nil {
			fail(c, http.StatusUnauthorized, errors.New("invalid username or password"))
			return
		}
		token, err := startAdminSession(c, store, user)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, adminLoginResponse(user, token))
	})

	api.POST("/auth/logout", func(c *gin.Context) {
		deleted := map[string]struct{}{}
		if token, ok := adminSessionCookie(c); ok {
			_ = store.DeleteAdminSession(c.Request.Context(), token)
			deleted[token] = struct{}{}
		}
		if token, ok := adminBearerToken(c); ok {
			if _, exists := deleted[token]; !exists {
				_ = store.DeleteAdminSession(c.Request.Context(), token)
			}
		}
		clearAdminSessionCookie(c)
		ok(c, gin.H{})
	})

	api.POST("/auth/password", RequireAdmin(store), func(c *gin.Context) {
		var payload adminPasswordUpdatePayload
		if !bindJSON(c, &payload) {
			return
		}
		currentPassword := payload.CurrentPassword
		newPassword := payload.NewPassword
		if strings.TrimSpace(currentPassword) == "" || len(newPassword) < 8 {
			fail(c, http.StatusBadRequest, errors.New("current password is required; new password must be at least 8 characters"))
			return
		}
		userValue, exists := c.Get("sunapi_admin_user")
		if !exists {
			fail(c, http.StatusUnauthorized, errors.New("admin login required"))
			return
		}
		user, okUser := userValue.(AdminUser)
		if !okUser {
			fail(c, http.StatusUnauthorized, errors.New("admin login required"))
			return
		}
		if compareAdminPassword(user.PasswordHash, currentPassword) != nil {
			fail(c, http.StatusUnauthorized, errors.New("invalid current password"))
			return
		}
		passwordHash, err := hashAdminPassword(newPassword)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		updated, err := store.UpdateAdminPassword(c.Request.Context(), user.ID, passwordHash)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, adminUserResponse(updated, true))
	})
}

func RequireAdmin(store *Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		initialized, err := store.AdminInitialized(c.Request.Context())
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			c.Abort()
			return
		}
		if !initialized {
			fail(c, http.StatusPreconditionRequired, errors.New("admin setup required"))
			c.Abort()
			return
		}

		user, authenticated, err := adminFromSession(c, store)
		if err != nil {
			fail(c, http.StatusInternalServerError, err)
			c.Abort()
			return
		}
		if !authenticated {
			fail(c, http.StatusUnauthorized, errors.New("admin login required"))
			c.Abort()
			return
		}
		c.Set("sunapi_admin_user", user)
		c.Next()
	}
}

func adminFromSession(c *gin.Context, store *Store) (AdminUser, bool, error) {
	if token, ok := adminSessionCookie(c); ok {
		user, authenticated, err := store.AdminUserBySession(c.Request.Context(), token)
		if err != nil || authenticated {
			return user, authenticated, err
		}
	}
	if token, ok := adminBearerToken(c); ok {
		return store.AdminUserBySession(c.Request.Context(), token)
	}
	return AdminUser{}, false, nil
}

func adminSessionCookie(c *gin.Context) (string, bool) {
	token, err := c.Cookie(adminSessionCookieName)
	if err != nil {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func adminBearerToken(c *gin.Context) (string, bool) {
	header := strings.TrimSpace(c.GetHeader("Authorization"))
	if header == "" {
		return "", false
	}
	prefix := "bearer "
	if !strings.HasPrefix(strings.ToLower(header), prefix) {
		return "", false
	}
	token := strings.TrimSpace(header[len(prefix):])
	return token, token != ""
}

func validateAdminCredentials(payload adminCredentialsPayload, setup bool) (string, string, bool) {
	password := payload.Password
	if password == "" {
		return "", "", false
	}
	if setup && len(password) < 8 {
		return "", "", false
	}
	return "admin", password, true
}

func hashAdminPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func compareAdminPassword(passwordHash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(password))
}

func startAdminSession(c *gin.Context, store *Store, user AdminUser) (string, error) {
	if err := store.DeleteExpiredAdminSessions(c.Request.Context()); err != nil {
		return "", err
	}
	token, err := randomSessionToken(32)
	if err != nil {
		return "", err
	}
	expires := time.Now().Add(adminSessionTTL)
	if err := store.CreateAdminSession(c.Request.Context(), token, user.ID, expires.Unix()); err != nil {
		return "", err
	}
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    token,
		Path:     "/",
		Expires:  expires,
		MaxAge:   int(adminSessionTTL.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(c),
	})
	return token, nil
}

func clearAdminSessionCookie(c *gin.Context) {
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     adminSessionCookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   requestIsHTTPS(c),
	})
}

func requestIsHTTPS(c *gin.Context) bool {
	if c.Request != nil && c.Request.TLS != nil {
		return true
	}
	return strings.EqualFold(c.GetHeader("X-Forwarded-Proto"), "https")
}

func randomSessionToken(size int) (string, error) {
	if size <= 0 {
		return "", fmt.Errorf("invalid token size")
	}
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func adminUserResponse(user AdminUser, authenticated bool) any {
	if !authenticated {
		return nil
	}
	return gin.H{
		"id":           user.ID,
		"username":     user.Username,
		"display_name": "SunAPI Admin",
		"role":         100,
		"status":       1,
		"group":        "default",
		"permissions":  gin.H{"sidebar_settings": true},
	}
}

func adminLoginResponse(user AdminUser, token string) any {
	return gin.H{
		"user":  adminUserResponse(user, true),
		"token": token,
	}
}
