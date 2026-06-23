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
		if err := startAdminSession(c, store, user); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, adminUserResponse(user, true))
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
		if err := startAdminSession(c, store, user); err != nil {
			fail(c, http.StatusInternalServerError, err)
			return
		}
		ok(c, adminUserResponse(user, true))
	})

	api.POST("/auth/logout", func(c *gin.Context) {
		if token, err := c.Cookie(adminSessionCookieName); err == nil {
			_ = store.DeleteAdminSession(c.Request.Context(), token)
		}
		clearAdminSessionCookie(c)
		ok(c, gin.H{})
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
	token, err := c.Cookie(adminSessionCookieName)
	if err != nil {
		return AdminUser{}, false, nil
	}
	return store.AdminUserBySession(c.Request.Context(), token)
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

func startAdminSession(c *gin.Context, store *Store, user AdminUser) error {
	if err := store.DeleteExpiredAdminSessions(c.Request.Context()); err != nil {
		return err
	}
	token, err := randomSessionToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(adminSessionTTL)
	if err := store.CreateAdminSession(c.Request.Context(), token, user.ID, expires.Unix()); err != nil {
		return err
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
	return nil
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
