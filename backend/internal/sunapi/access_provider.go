package sunapi

import (
	"context"
	"database/sql"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"

	sdkaccess "github.com/router-for-me/CLIProxyAPI/v7/sdk/access"
)

const sqliteAPIKeyAccessProviderType = "sunapi-sqlite-api-key"

// RegisterSQLiteAPIKeyAccessProvider exposes the local SQLite-backed API key
// provider to the shared access registry used by the proxy runtime.
func RegisterSQLiteAPIKeyAccessProvider(store *Store) {
	sdkaccess.RegisterProvider(sqliteAPIKeyAccessProviderType, NewSQLiteAPIKeyAccessProvider(store))
}

// NewSQLiteAPIKeyAccessProvider constructs an access provider that authenticates
// against the local api_keys table in SQLite.
func NewSQLiteAPIKeyAccessProvider(store *Store) sdkaccess.Provider {
	return &sqliteAPIKeyAccessProvider{store: store}
}

type sqliteAPIKeyAccessProvider struct {
	store *Store
}

func (p *sqliteAPIKeyAccessProvider) Identifier() string {
	return sqliteAPIKeyAccessProviderType
}

func (p *sqliteAPIKeyAccessProvider) Authenticate(ctx context.Context, r *http.Request) (*sdkaccess.Result, *sdkaccess.AuthError) {
	if p == nil || p.store == nil {
		return nil, sdkaccess.NewNotHandledError()
	}
	secret, source := extractSQLiteAPIKeyCredential(r)
	if secret == "" {
		return nil, sdkaccess.NewNoCredentialsError()
	}

	key, err := p.store.GetAPIKeyBySecret(ctx, secret)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sdkaccess.NewInvalidCredentialError()
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, sdkaccess.NewInternalAuthError("api key lookup canceled", err)
		}
		return nil, sdkaccess.NewInternalAuthError("failed to lookup api key", err)
	}

	if key.Status != 1 {
		return nil, sdkaccess.NewInvalidCredentialError()
	}
	if key.ExpiredTime > 0 && key.ExpiredTime < time.Now().Unix() {
		return nil, sdkaccess.NewInvalidCredentialError()
	}
	if !key.UnlimitedQuota && key.RemainQuota <= 0 {
		return nil, sdkaccess.NewInvalidCredentialError()
	}

	remoteIP := requestRemoteIP(r)
	if !apiKeyAllowsRemoteIP(key.AllowIPs, remoteIP) {
		return nil, sdkaccess.NewInvalidCredentialError()
	}

	if err := p.store.TouchAPIKey(ctx, key.ID, time.Now().Unix()); err != nil {
		return nil, sdkaccess.NewInternalAuthError("failed to update api key access time", err)
	}

	return &sdkaccess.Result{
		Provider:  p.Identifier(),
		Principal: "api-key:" + strconv.FormatInt(key.ID, 10),
		Metadata: map[string]string{
			"source":                    source,
			"api_key_id":                strconv.FormatInt(key.ID, 10),
			"api_key_name":              strings.TrimSpace(key.Name),
			"api_key_group":             strings.TrimSpace(key.Group),
			"api_key_cross_group_retry": boolString(key.CrossGroupRetry),
			"api_key_source":            "sqlite",
			"client_ip":                 remoteIP,
			"allow_ips":                 strings.TrimSpace(key.AllowIPs),
			"unlimited_quota":           boolString(key.UnlimitedQuota),
		},
	}, nil
}

func extractSQLiteAPIKeyCredential(r *http.Request) (string, string) {
	if r == nil {
		return "", ""
	}
	candidates := []struct {
		value  string
		source string
	}{
		{extractBearerToken(r.Header.Get("Authorization")), "authorization"},
		{strings.TrimSpace(r.Header.Get("X-Goog-Api-Key")), "x-goog-api-key"},
		{strings.TrimSpace(r.Header.Get("X-Api-Key")), "x-api-key"},
	}
	if r.URL != nil {
		query := r.URL.Query()
		candidates = append(candidates,
			struct {
				value  string
				source string
			}{strings.TrimSpace(query.Get("key")), "query-key"},
			struct {
				value  string
				source string
			}{strings.TrimSpace(query.Get("auth_token")), "query-auth-token"},
		)
	}

	for _, candidate := range candidates {
		secret := normalizeAPIKeySecret(candidate.value)
		if secret == "" {
			continue
		}
		return secret, candidate.source
	}
	return "", ""
}

func extractBearerToken(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 {
		return header
	}
	if strings.ToLower(parts[0]) != "bearer" {
		return header
	}
	return strings.TrimSpace(parts[1])
}

func requestRemoteIP(r *http.Request) string {
	if r == nil {
		return ""
	}
	host := strings.TrimSpace(r.RemoteAddr)
	if host == "" {
		return ""
	}
	if parsedHost, _, err := net.SplitHostPort(host); err == nil {
		host = parsedHost
	}
	host = strings.TrimSpace(host)
	return host
}

func apiKeyAllowsRemoteIP(allowIPs, remoteIP string) bool {
	allowIPs = strings.TrimSpace(allowIPs)
	if allowIPs == "" {
		return true
	}
	remoteIP = strings.TrimSpace(remoteIP)
	if remoteIP == "" {
		return false
	}

	remoteAddr, err := netip.ParseAddr(remoteIP)
	if err != nil {
		return false
	}
	remoteAddr = remoteAddr.Unmap()

	parts := strings.FieldsFunc(allowIPs, func(r rune) bool {
		switch r {
		case ',', ';', '\n', '\r', '\t', ' ':
			return true
		default:
			return false
		}
	})
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		if strings.Contains(trimmed, "/") {
			prefix, err := netip.ParsePrefix(trimmed)
			if err == nil && prefix.Contains(remoteAddr) {
				return true
			}
			continue
		}
		addr, err := netip.ParseAddr(trimmed)
		if err == nil && addr.Unmap() == remoteAddr {
			return true
		}
		if trimmed == remoteIP {
			return true
		}
	}
	return false
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}
