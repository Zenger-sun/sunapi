package sunapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db      *sql.DB
	dataDir string
}

const (
	localQuotaPerUnit    int64 = 1000000
	localQuotaBalanceUSD int64 = 200
)

func OpenStore(path string) (*Store, error) {
	if strings.TrimSpace(path) == "" {
		return nil, errors.New("database path is required")
	}
	if err := ensureDir(filepath.Dir(path)); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db, dataDir: filepath.Dir(path)}
	if err := store.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) DataDir() string {
	if s == nil {
		return ""
	}
	return s.dataDir
}

func ensureDir(path string) error {
	if strings.TrimSpace(path) == "" || path == "." {
		return nil
	}
	return os.MkdirAll(path, 0755)
}

func (s *Store) migrate(ctx context.Context) error {
	stmts := []string{
		`PRAGMA journal_mode=WAL`,
		`PRAGMA busy_timeout=5000`,
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			system_name TEXT NOT NULL,
			listen_host TEXT NOT NULL,
			listen_port INTEGER NOT NULL,
			default_group TEXT NOT NULL,
			default_input_price_per_1k REAL NOT NULL,
			default_output_price_per_1k REAL NOT NULL,
			currency_symbol TEXT NOT NULL,
			auto_open_browser INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			description TEXT NOT NULL DEFAULT '',
			price_multiplier REAL NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS channels (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			type INTEGER NOT NULL DEFAULT 1,
			name TEXT NOT NULL,
			group_name TEXT NOT NULL,
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL,
			models TEXT NOT NULL,
			enabled INTEGER NOT NULL DEFAULT 1,
			input_price_per_1k REAL NOT NULL DEFAULT 0,
			output_price_per_1k REAL NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 0,
			weight INTEGER NOT NULL DEFAULT 0,
			test_model TEXT NOT NULL DEFAULT '',
			test_time INTEGER NOT NULL DEFAULT 0,
			response_time INTEGER NOT NULL DEFAULT 0,
			balance REAL NOT NULL DEFAULT 0,
			balance_updated_time INTEGER NOT NULL DEFAULT 0,
			openai_organization TEXT NOT NULL DEFAULT '',
			model_mapping TEXT NOT NULL DEFAULT '',
			status_code_mapping TEXT NOT NULL DEFAULT '',
			auto_ban INTEGER NOT NULL DEFAULT 1,
			tag TEXT NOT NULL DEFAULT '',
			setting TEXT NOT NULL DEFAULT '',
			param_override TEXT NOT NULL DEFAULT '',
			header_override TEXT NOT NULL DEFAULT '',
			other TEXT NOT NULL DEFAULT '',
			other_info TEXT NOT NULL DEFAULT '',
			settings TEXT NOT NULL DEFAULT '{}',
			max_input_tokens INTEGER NOT NULL DEFAULT 0,
			remark TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS usage_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			created_at INTEGER NOT NULL,
			channel_id INTEGER NOT NULL DEFAULT 0,
			channel_name TEXT NOT NULL,
			username TEXT NOT NULL DEFAULT 'local',
			group_name TEXT NOT NULL,
			model TEXT NOT NULL,
			endpoint TEXT NOT NULL,
			prompt_tokens INTEGER NOT NULL DEFAULT 0,
			completion_tokens INTEGER NOT NULL DEFAULT 0,
			total_tokens INTEGER NOT NULL DEFAULT 0,
			cost REAL NOT NULL DEFAULT 0,
			duration_ms INTEGER NOT NULL DEFAULT 0,
			status_code INTEGER NOT NULL DEFAULT 200,
			error TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			token_key TEXT NOT NULL UNIQUE,
			status INTEGER NOT NULL DEFAULT 1,
			remain_quota INTEGER NOT NULL DEFAULT 0,
			used_quota INTEGER NOT NULL DEFAULT 0,
			unlimited_quota INTEGER NOT NULL DEFAULT 1,
			expired_time INTEGER NOT NULL DEFAULT -1,
			created_time INTEGER NOT NULL,
			accessed_time INTEGER NOT NULL DEFAULT 0,
			group_name TEXT NOT NULL DEFAULT 'default',
			cross_group_retry INTEGER NOT NULL DEFAULT 0,
			model_limits_enabled INTEGER NOT NULL DEFAULT 0,
			model_limits TEXT NOT NULL DEFAULT '',
			allow_ips TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS admin_users (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS admin_sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS playground_conversations (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL DEFAULT 1,
			title TEXT NOT NULL DEFAULT '',
			summary TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT '',
			pinned INTEGER NOT NULL DEFAULT 0,
			message_count INTEGER NOT NULL DEFAULT 0,
			config TEXT NOT NULL DEFAULT '{}',
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS playground_conversation_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			user_id INTEGER NOT NULL DEFAULT 1,
			messages TEXT NOT NULL DEFAULT '[]',
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL,
			UNIQUE(session_id, user_id)
		)`,
		`CREATE TABLE IF NOT EXISTS playground_attachments (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL DEFAULT 1,
			type TEXT NOT NULL DEFAULT 'image',
			media_type TEXT NOT NULL DEFAULT '',
			filename TEXT NOT NULL DEFAULT '',
			size INTEGER NOT NULL DEFAULT 0,
			data BLOB NOT NULL,
			created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS playground_image_generations (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL DEFAULT 1,
			prompt TEXT NOT NULL DEFAULT '',
			negative_prompt TEXT NOT NULL DEFAULT '',
			params TEXT NOT NULL DEFAULT '{}',
			model TEXT NOT NULL DEFAULT '',
			group_name TEXT NOT NULL DEFAULT '',
			reference_images TEXT NOT NULL DEFAULT '[]',
			image_urls TEXT NOT NULL DEFAULT '[]',
			status TEXT NOT NULL DEFAULT 'succeeded',
			error_message TEXT NOT NULL DEFAULT '',
			duration_ms INTEGER NOT NULL DEFAULT 0,
			created_time INTEGER NOT NULL,
			updated_time INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_logs_created_at ON usage_logs(created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_usage_logs_channel_id ON usage_logs(channel_id)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_name ON api_keys(name)`,
		`CREATE INDEX IF NOT EXISTS idx_admin_sessions_expires_at ON admin_sessions(expires_at)`,
		`CREATE INDEX IF NOT EXISTS idx_playground_conversations_user_updated ON playground_conversations(user_id, pinned, updated_time, id)`,
		`CREATE INDEX IF NOT EXISTS idx_playground_conversation_messages_user_updated ON playground_conversation_messages(user_id, updated_time)`,
		`CREATE INDEX IF NOT EXISTS idx_playground_attachments_user_created ON playground_attachments(user_id, created_at)`,
		`CREATE INDEX IF NOT EXISTS idx_playground_image_generations_user_created ON playground_image_generations(user_id, created_time DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	if err := s.ensureColumn(ctx, "usage_logs", "username", "TEXT NOT NULL DEFAULT 'local'"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "type", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "priority", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "weight", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "test_model", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "test_time", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "response_time", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "balance", "REAL NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "balance_updated_time", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "openai_organization", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "model_mapping", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "status_code_mapping", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "auto_ban", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "tag", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "setting", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "param_override", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "header_override", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "other", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "other_info", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "settings", "TEXT NOT NULL DEFAULT '{}'"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "channels", "max_input_tokens", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "api_keys", "token_key", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if hasKeyColumn, err := s.tableHasColumn(ctx, "api_keys", "key"); err != nil {
		return err
	} else if hasKeyColumn {
		if _, err := s.db.ExecContext(ctx, `UPDATE api_keys SET token_key = key WHERE token_key = '' AND COALESCE(key, '') <> ''`); err != nil {
			return err
		}
	}
	if _, err := s.db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_api_keys_token_key ON api_keys(token_key)`); err != nil {
		return err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO settings
		(id, system_name, listen_host, listen_port, default_group, default_input_price_per_1k, default_output_price_per_1k, currency_symbol, auto_open_browser)
		VALUES (1, 'SunAPI', '127.0.0.1', 8317, 'default', 0, 0, '$', 1)`); err != nil {
		return err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO groups
		(name, description, price_multiplier, created_at, updated_at)
		VALUES ('default', '默认分组', 1, ?, ?)`, now, now); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if strings.EqualFold(name, column) {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition)
	return err
}

func (s *Store) tableHasColumn(ctx context.Context, table, column string) (bool, error) {
	rows, err := s.db.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if strings.EqualFold(name, column) {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func (s *Store) Settings(ctx context.Context) (Settings, error) {
	var settings Settings
	var autoOpen int
	err := s.db.QueryRowContext(ctx, `SELECT system_name, listen_host, listen_port, default_group,
		default_input_price_per_1k, default_output_price_per_1k, currency_symbol, auto_open_browser
		FROM settings WHERE id = 1`).Scan(
		&settings.SystemName,
		&settings.ListenHost,
		&settings.ListenPort,
		&settings.DefaultGroup,
		&settings.DefaultInputPricePer1K,
		&settings.DefaultOutputPricePer1K,
		&settings.CurrencySymbol,
		&autoOpen,
	)
	settings.AutoOpenBrowser = autoOpen != 0
	return settings, err
}

func (s *Store) UpdateSettings(ctx context.Context, settings Settings) (Settings, error) {
	settings = normalizeSettings(settings)
	if err := s.EnsureGroup(ctx, settings.DefaultGroup); err != nil {
		return Settings{}, err
	}
	autoOpen := 0
	if settings.AutoOpenBrowser {
		autoOpen = 1
	}
	_, err := s.db.ExecContext(ctx, `UPDATE settings SET
		system_name = ?, listen_host = ?, listen_port = ?, default_group = ?,
		default_input_price_per_1k = ?, default_output_price_per_1k = ?,
		currency_symbol = ?, auto_open_browser = ?
		WHERE id = 1`,
		settings.SystemName,
		settings.ListenHost,
		settings.ListenPort,
		settings.DefaultGroup,
		settings.DefaultInputPricePer1K,
		settings.DefaultOutputPricePer1K,
		settings.CurrencySymbol,
		autoOpen,
	)
	return settings, err
}

func normalizeSettings(settings Settings) Settings {
	settings.SystemName = strings.TrimSpace(settings.SystemName)
	if settings.SystemName == "" {
		settings.SystemName = "SunAPI"
	}
	settings.ListenHost = strings.TrimSpace(settings.ListenHost)
	if settings.ListenHost == "" {
		settings.ListenHost = "127.0.0.1"
	}
	if settings.ListenPort <= 0 || settings.ListenPort > 65535 {
		settings.ListenPort = 8317
	}
	settings.DefaultGroup = normalizeGroupName(settings.DefaultGroup)
	settings.CurrencySymbol = strings.TrimSpace(settings.CurrencySymbol)
	if settings.CurrencySymbol == "" {
		settings.CurrencySymbol = "$"
	}
	if settings.DefaultInputPricePer1K < 0 {
		settings.DefaultInputPricePer1K = 0
	}
	if settings.DefaultOutputPricePer1K < 0 {
		settings.DefaultOutputPricePer1K = 0
	}
	return settings
}

func normalizeGroupName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "default"
	}
	return name
}

func (s *Store) ListChannels(ctx context.Context, includeKey bool) ([]Channel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.type, c.name, c.group_name, c.base_url, c.api_key, c.models,
		c.enabled, c.input_price_per_1k, c.output_price_per_1k, c.priority, c.weight, c.test_model,
		c.test_time, c.response_time, c.balance, c.balance_updated_time, c.openai_organization,
		c.model_mapping, c.status_code_mapping, c.auto_ban, c.tag, c.setting, c.param_override,
		c.header_override, c.other, c.other_info, c.settings, c.max_input_tokens, c.remark, c.created_at, c.updated_at,
		COALESCE(SUM(u.total_tokens), 0), COALESCE(COUNT(u.id), 0), COALESCE(SUM(u.cost), 0)
		FROM channels c
		LEFT JOIN usage_logs u ON u.channel_id = c.id
		GROUP BY c.id
		ORDER BY c.id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Channel, 0)
	for rows.Next() {
		var channel Channel
		var enabled int
		var usedCost float64
		if err := rows.Scan(
			&channel.ID,
			&channel.Type,
			&channel.Name,
			&channel.Group,
			&channel.BaseURL,
			&channel.APIKey,
			&channel.Models,
			&enabled,
			&channel.InputPricePer1K,
			&channel.OutputPricePer1K,
			&channel.Priority,
			&channel.Weight,
			&channel.TestModel,
			&channel.TestTime,
			&channel.ResponseTime,
			&channel.Balance,
			&channel.BalanceUpdatedTime,
			&channel.OpenAIOrganization,
			&channel.ModelMapping,
			&channel.StatusCodeMapping,
			&channel.AutoBan,
			&channel.Tag,
			&channel.Setting,
			&channel.ParamOverride,
			&channel.HeaderOverride,
			&channel.Other,
			&channel.OtherInfo,
			&channel.Settings,
			&channel.MaxInputTokens,
			&channel.Remark,
			&channel.CreatedAt,
			&channel.UpdatedAt,
			&channel.UsedTokens,
			&channel.RequestCount,
			&usedCost,
		); err != nil {
			return nil, err
		}
		channel = finalizeChannel(channel, enabled, usedCost, includeKey)
		out = append(out, channel)
	}
	return out, rows.Err()
}

func (s *Store) GetChannel(ctx context.Context, id int64, includeKey bool) (Channel, error) {
	var channel Channel
	var enabled int
	var usedCost float64
	err := s.db.QueryRowContext(ctx, `SELECT c.id, c.type, c.name, c.group_name, c.base_url, c.api_key, c.models,
		c.enabled, c.input_price_per_1k, c.output_price_per_1k, c.priority, c.weight, c.test_model,
		c.test_time, c.response_time, c.balance, c.balance_updated_time, c.openai_organization,
		c.model_mapping, c.status_code_mapping, c.auto_ban, c.tag, c.setting, c.param_override,
		c.header_override, c.other, c.other_info, c.settings, c.max_input_tokens, c.remark, c.created_at, c.updated_at,
		COALESCE(SUM(u.total_tokens), 0), COALESCE(COUNT(u.id), 0), COALESCE(SUM(u.cost), 0)
		FROM channels c
		LEFT JOIN usage_logs u ON u.channel_id = c.id
		WHERE c.id = ?
		GROUP BY c.id`, id).Scan(
		&channel.ID,
		&channel.Type,
		&channel.Name,
		&channel.Group,
		&channel.BaseURL,
		&channel.APIKey,
		&channel.Models,
		&enabled,
		&channel.InputPricePer1K,
		&channel.OutputPricePer1K,
		&channel.Priority,
		&channel.Weight,
		&channel.TestModel,
		&channel.TestTime,
		&channel.ResponseTime,
		&channel.Balance,
		&channel.BalanceUpdatedTime,
		&channel.OpenAIOrganization,
		&channel.ModelMapping,
		&channel.StatusCodeMapping,
		&channel.AutoBan,
		&channel.Tag,
		&channel.Setting,
		&channel.ParamOverride,
		&channel.HeaderOverride,
		&channel.Other,
		&channel.OtherInfo,
		&channel.Settings,
		&channel.MaxInputTokens,
		&channel.Remark,
		&channel.CreatedAt,
		&channel.UpdatedAt,
		&channel.UsedTokens,
		&channel.RequestCount,
		&usedCost,
	)
	if err != nil {
		return channel, err
	}
	return finalizeChannel(channel, enabled, usedCost, includeKey), nil
}

func finalizeChannel(channel Channel, enabled int, usedCost float64, includeKey bool) Channel {
	channel.Enabled = enabled != 0
	if channel.Enabled {
		channel.Status = 1
	} else {
		channel.Status = 2
	}
	channel.CreatedTime = channel.CreatedAt
	channel.Models = normalizeModels(channel.Models)
	channel.UsedQuota = quotaFromCost(usedCost)
	if strings.TrimSpace(channel.Settings) == "" {
		channel.Settings = "{}"
	}
	if channel.AutoBan == 0 {
		channel.AutoBan = 1
	}
	if channel.ChannelInfo.MultiKeyMode == "" {
		channel.ChannelInfo = ChannelInfo{
			IsMultiKey:           false,
			MultiKeySize:         0,
			MultiKeyPollingIndex: 0,
			MultiKeyMode:         "random",
		}
	}
	if includeKey {
		channel.Key = channel.APIKey
	} else {
		channel.APIKey = ""
		channel.Key = ""
	}
	return channel
}

func (s *Store) CreateChannel(ctx context.Context, payload ChannelPayload) (Channel, error) {
	payload = normalizeChannelPayload(payload)
	if payload.APIKey == "" {
		return Channel{}, errors.New("api_key is required")
	}
	now := time.Now().Unix()
	if err := s.EnsureGroup(ctx, payload.Group); err != nil {
		return Channel{}, err
	}
	enabled := boolInt(payload.Enabled)
	res, err := s.db.ExecContext(ctx, `INSERT INTO channels
		(type, name, group_name, base_url, api_key, models, enabled, input_price_per_1k, output_price_per_1k,
		priority, weight, test_model, test_time, response_time, balance, balance_updated_time,
		openai_organization, model_mapping, status_code_mapping, auto_ban, tag, setting,
		param_override, header_override, other, other_info, settings, max_input_tokens,
		remark, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		payload.Type,
		payload.Name,
		payload.Group,
		payload.BaseURL,
		payload.APIKey,
		payload.Models,
		enabled,
		payload.InputPricePer1K,
		payload.OutputPricePer1K,
		payload.Priority,
		payload.Weight,
		payload.TestModel,
		0,
		0,
		0,
		0,
		payload.OpenAIOrganization,
		payload.ModelMapping,
		payload.StatusCodeMapping,
		normalizeAutoBan(payload.AutoBan),
		payload.Tag,
		normalizeJSONText(payload.Setting, "{}"),
		payload.ParamOverride,
		payload.HeaderOverride,
		payload.Other,
		payload.OtherInfo,
		normalizeJSONText(payload.Settings, "{}"),
		0,
		payload.Remark,
		now,
		now,
	)
	if err != nil {
		return Channel{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetChannel(ctx, id, false)
}

func (s *Store) UpdateChannel(ctx context.Context, id int64, payload ChannelPayload) (Channel, error) {
	current, err := s.GetChannel(ctx, id, true)
	if err != nil {
		return Channel{}, err
	}
	payload = normalizeChannelPayload(payload)
	if err := s.EnsureGroup(ctx, payload.Group); err != nil {
		return Channel{}, err
	}
	if payload.APIKey == "" {
		payload.APIKey = current.APIKey
	}
	_, err = s.db.ExecContext(ctx, `UPDATE channels SET
		type = ?, name = ?, group_name = ?, base_url = ?, api_key = ?, models = ?, enabled = ?,
		input_price_per_1k = ?, output_price_per_1k = ?, priority = ?, weight = ?, test_model = ?,
		openai_organization = ?, model_mapping = ?, status_code_mapping = ?, auto_ban = ?, tag = ?,
		setting = ?, param_override = ?, header_override = ?, other = ?, other_info = ?, settings = ?,
		remark = ?, updated_at = ?
		WHERE id = ?`,
		payload.Type,
		payload.Name,
		payload.Group,
		payload.BaseURL,
		payload.APIKey,
		payload.Models,
		boolInt(payload.Enabled),
		payload.InputPricePer1K,
		payload.OutputPricePer1K,
		payload.Priority,
		payload.Weight,
		payload.TestModel,
		payload.OpenAIOrganization,
		payload.ModelMapping,
		payload.StatusCodeMapping,
		normalizeAutoBan(payload.AutoBan),
		payload.Tag,
		normalizeJSONText(payload.Setting, current.Setting),
		payload.ParamOverride,
		payload.HeaderOverride,
		payload.Other,
		payload.OtherInfo,
		normalizeJSONText(payload.Settings, current.Settings),
		payload.Remark,
		time.Now().Unix(),
		id,
	)
	if err != nil {
		return Channel{}, err
	}
	return s.GetChannel(ctx, id, false)
}

func (s *Store) PatchChannel(ctx context.Context, id int64, payload ChannelPayload, raw map[string]json.RawMessage) (Channel, error) {
	current, err := s.GetChannel(ctx, id, true)
	if err != nil {
		return Channel{}, err
	}
	if _, fullUpdate := raw["name"]; fullUpdate {
		return s.UpdateChannel(ctx, id, payload)
	}
	merged := channelToPayload(current)
	if _, ok := raw["type"]; ok {
		merged.Type = payload.Type
	}
	if _, ok := raw["group"]; ok {
		merged.Group = payload.Group
	}
	if _, ok := raw["base_url"]; ok {
		merged.BaseURL = payload.BaseURL
	}
	if _, ok := raw["api_key"]; ok {
		merged.APIKey = payload.APIKey
	}
	if _, ok := raw["key"]; ok {
		merged.Key = payload.Key
		merged.APIKey = payload.Key
	}
	if _, ok := raw["models"]; ok {
		merged.Models = payload.Models
	}
	if _, ok := raw["enabled"]; ok {
		merged.Enabled = payload.Enabled
		if payload.Enabled {
			merged.Status = 1
		} else {
			merged.Status = 2
		}
	}
	if _, ok := raw["status"]; ok {
		merged.Status = payload.Status
		merged.Enabled = payload.Status == 1
	}
	if _, ok := raw["input_price_per_1k"]; ok {
		merged.InputPricePer1K = payload.InputPricePer1K
	}
	if _, ok := raw["output_price_per_1k"]; ok {
		merged.OutputPricePer1K = payload.OutputPricePer1K
	}
	if _, ok := raw["priority"]; ok {
		merged.Priority = payload.Priority
	}
	if _, ok := raw["weight"]; ok {
		merged.Weight = payload.Weight
	}
	if _, ok := raw["test_model"]; ok {
		merged.TestModel = payload.TestModel
	}
	if _, ok := raw["openai_organization"]; ok {
		merged.OpenAIOrganization = payload.OpenAIOrganization
	}
	if _, ok := raw["model_mapping"]; ok {
		merged.ModelMapping = payload.ModelMapping
	}
	if _, ok := raw["status_code_mapping"]; ok {
		merged.StatusCodeMapping = payload.StatusCodeMapping
	}
	if _, ok := raw["auto_ban"]; ok {
		merged.AutoBan = payload.AutoBan
	}
	if _, ok := raw["tag"]; ok {
		merged.Tag = payload.Tag
	}
	if _, ok := raw["setting"]; ok {
		merged.Setting = payload.Setting
	}
	if _, ok := raw["param_override"]; ok {
		merged.ParamOverride = payload.ParamOverride
	}
	if _, ok := raw["header_override"]; ok {
		merged.HeaderOverride = payload.HeaderOverride
	}
	if _, ok := raw["other"]; ok {
		merged.Other = payload.Other
	}
	if _, ok := raw["other_info"]; ok {
		merged.OtherInfo = payload.OtherInfo
	}
	if _, ok := raw["settings"]; ok {
		merged.Settings = payload.Settings
	}
	if _, ok := raw["remark"]; ok {
		merged.Remark = payload.Remark
	}
	return s.UpdateChannel(ctx, id, merged)
}

func channelToPayload(channel Channel) ChannelPayload {
	return ChannelPayload{
		Type:               channel.Type,
		Name:               channel.Name,
		Group:              channel.Group,
		BaseURL:            channel.BaseURL,
		APIKey:             channel.APIKey,
		Key:                channel.APIKey,
		Models:             channel.Models,
		Enabled:            channel.Enabled,
		Status:             channel.Status,
		InputPricePer1K:    channel.InputPricePer1K,
		OutputPricePer1K:   channel.OutputPricePer1K,
		Priority:           channel.Priority,
		Weight:             channel.Weight,
		TestModel:          channel.TestModel,
		OpenAIOrganization: channel.OpenAIOrganization,
		ModelMapping:       channel.ModelMapping,
		StatusCodeMapping:  channel.StatusCodeMapping,
		AutoBan:            channel.AutoBan,
		Tag:                channel.Tag,
		Setting:            channel.Setting,
		ParamOverride:      channel.ParamOverride,
		HeaderOverride:     channel.HeaderOverride,
		Other:              channel.Other,
		OtherInfo:          channel.OtherInfo,
		Settings:           channel.Settings,
		Remark:             channel.Remark,
	}
}

func (s *Store) SetChannelEnabled(ctx context.Context, id int64, enabled bool) (Channel, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE channels SET enabled = ?, updated_at = ? WHERE id = ?`, boolInt(enabled), time.Now().Unix(), id)
	if err != nil {
		return Channel{}, err
	}
	return s.GetChannel(ctx, id, false)
}

func (s *Store) DeleteChannel(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM channels WHERE id = ?`, id)
	return err
}

func (s *Store) ListChannelsPage(ctx context.Context, opts ChannelListOptions, includeKey bool) ([]Channel, int64, map[string]int64, error) {
	if opts.Page <= 0 {
		opts.Page = 1
	}
	if opts.PageSize <= 0 || opts.PageSize > 100 {
		opts.PageSize = 20
	}
	where, args := channelListWhere(opts)
	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM channels c `+where, args...).Scan(&total); err != nil {
		return nil, 0, nil, err
	}
	typeCounts, err := s.channelTypeCounts(ctx, where, args)
	if err != nil {
		return nil, 0, nil, err
	}
	orderBy := channelListOrder(opts)
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, opts.PageSize, (opts.Page-1)*opts.PageSize)
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, c.type, c.name, c.group_name, c.base_url, c.api_key, c.models,
		c.enabled, c.input_price_per_1k, c.output_price_per_1k, c.priority, c.weight, c.test_model,
		c.test_time, c.response_time, c.balance, c.balance_updated_time, c.openai_organization,
		c.model_mapping, c.status_code_mapping, c.auto_ban, c.tag, c.setting, c.param_override,
		c.header_override, c.other, c.other_info, c.settings, c.max_input_tokens, c.remark, c.created_at, c.updated_at,
		COALESCE(u.tokens, 0), COALESCE(u.requests, 0), COALESCE(u.cost, 0)
		FROM channels c
		LEFT JOIN (SELECT channel_id, SUM(total_tokens) tokens, COUNT(*) requests, SUM(cost) cost FROM usage_logs GROUP BY channel_id) u
			ON u.channel_id = c.id
		`+where+`
		`+orderBy+`
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, nil, err
	}
	defer rows.Close()
	items, err := scanChannels(rows, includeKey)
	if err != nil {
		return nil, 0, nil, err
	}
	return items, total, typeCounts, nil
}

func channelListWhere(opts ChannelListOptions) (string, []any) {
	where := `WHERE 1 = 1`
	args := []any{}
	if opts.Group != "" && opts.Group != "all" {
		where += ` AND c.group_name = ?`
		args = append(args, opts.Group)
	}
	if opts.Status == "enabled" {
		where += ` AND c.enabled <> 0`
	} else if opts.Status == "disabled" {
		where += ` AND c.enabled = 0`
	}
	if opts.Type > 0 {
		where += ` AND c.type = ?`
		args = append(args, opts.Type)
	}
	if opts.Model != "" {
		where += ` AND c.models LIKE ?`
		args = append(args, "%"+opts.Model+"%")
	}
	if opts.Keyword != "" {
		if id, err := strconv.ParseInt(opts.Keyword, 10, 64); err == nil && id > 0 {
			where += ` AND (c.id = ? OR c.name LIKE ? OR c.api_key LIKE ? OR c.base_url LIKE ?)`
			like := "%" + opts.Keyword + "%"
			args = append(args, id, like, like, like)
		} else {
			where += ` AND (c.name LIKE ? OR c.api_key LIKE ? OR c.base_url LIKE ? OR c.remark LIKE ?)`
			like := "%" + opts.Keyword + "%"
			args = append(args, like, like, like, like)
		}
	}
	return where, args
}

func channelListOrder(opts ChannelListOptions) string {
	dir := "DESC"
	if strings.EqualFold(opts.SortOrder, "asc") {
		dir = "ASC"
	}
	switch opts.SortBy {
	case "id":
		return "ORDER BY c.id " + dir
	case "name":
		return "ORDER BY c.name " + dir + ", c.id DESC"
	case "priority":
		return "ORDER BY c.priority " + dir + ", c.id DESC"
	case "balance":
		return "ORDER BY c.balance " + dir + ", c.id DESC"
	case "response_time":
		return "ORDER BY c.response_time " + dir + ", c.id DESC"
	case "test_time":
		return "ORDER BY c.test_time " + dir + ", c.id DESC"
	default:
		if opts.IDSort {
			return "ORDER BY c.id DESC"
		}
		return "ORDER BY c.priority DESC, c.id DESC"
	}
}

func (s *Store) channelTypeCounts(ctx context.Context, where string, args []any) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT c.type, COUNT(*) FROM channels c `+where+` GROUP BY c.type`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var channelType int
		var count int64
		if err := rows.Scan(&channelType, &count); err != nil {
			return nil, err
		}
		out[strconv.Itoa(channelType)] = count
	}
	return out, rows.Err()
}

func scanChannels(rows *sql.Rows, includeKey bool) ([]Channel, error) {
	out := make([]Channel, 0)
	for rows.Next() {
		var channel Channel
		var enabled int
		var usedCost float64
		if err := rows.Scan(
			&channel.ID,
			&channel.Type,
			&channel.Name,
			&channel.Group,
			&channel.BaseURL,
			&channel.APIKey,
			&channel.Models,
			&enabled,
			&channel.InputPricePer1K,
			&channel.OutputPricePer1K,
			&channel.Priority,
			&channel.Weight,
			&channel.TestModel,
			&channel.TestTime,
			&channel.ResponseTime,
			&channel.Balance,
			&channel.BalanceUpdatedTime,
			&channel.OpenAIOrganization,
			&channel.ModelMapping,
			&channel.StatusCodeMapping,
			&channel.AutoBan,
			&channel.Tag,
			&channel.Setting,
			&channel.ParamOverride,
			&channel.HeaderOverride,
			&channel.Other,
			&channel.OtherInfo,
			&channel.Settings,
			&channel.MaxInputTokens,
			&channel.Remark,
			&channel.CreatedAt,
			&channel.UpdatedAt,
			&channel.UsedTokens,
			&channel.RequestCount,
			&usedCost,
		); err != nil {
			return nil, err
		}
		out = append(out, finalizeChannel(channel, enabled, usedCost, includeKey))
	}
	return out, rows.Err()
}

func (s *Store) UpdateChannelTestResult(ctx context.Context, id int64, responseTime int64) (Channel, error) {
	_, err := s.db.ExecContext(ctx, `UPDATE channels SET response_time = ?, test_time = ?, updated_at = ? WHERE id = ?`,
		responseTime, time.Now().Unix(), time.Now().Unix(), id)
	if err != nil {
		return Channel{}, err
	}
	return s.GetChannel(ctx, id, false)
}

func (s *Store) ResetChannelUsedQuota(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM usage_logs WHERE channel_id = ?`, id)
	return err
}

func (s *Store) ResetAllChannelUsedQuota(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM usage_logs`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) DeleteDisabledChannels(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM channels WHERE enabled = 0`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) CloneChannel(ctx context.Context, id int64, suffix string) (Channel, error) {
	current, err := s.GetChannel(ctx, id, true)
	if err != nil {
		return Channel{}, err
	}
	payload := ChannelPayload{
		Type:               current.Type,
		Name:               current.Name + strings.TrimSpace(suffix),
		Group:              current.Group,
		BaseURL:            current.BaseURL,
		APIKey:             current.APIKey,
		Models:             current.Models,
		Enabled:            current.Enabled,
		InputPricePer1K:    current.InputPricePer1K,
		OutputPricePer1K:   current.OutputPricePer1K,
		Priority:           current.Priority,
		Weight:             current.Weight,
		TestModel:          current.TestModel,
		OpenAIOrganization: current.OpenAIOrganization,
		ModelMapping:       current.ModelMapping,
		StatusCodeMapping:  current.StatusCodeMapping,
		AutoBan:            current.AutoBan,
		Tag:                current.Tag,
		Setting:            current.Setting,
		ParamOverride:      current.ParamOverride,
		HeaderOverride:     current.HeaderOverride,
		Other:              current.Other,
		OtherInfo:          current.OtherInfo,
		Settings:           current.Settings,
		Remark:             current.Remark,
	}
	if strings.TrimSpace(payload.Name) == strings.TrimSpace(current.Name) {
		payload.Name = current.Name + " Copy"
	}
	return s.CreateChannel(ctx, payload)
}

func (s *Store) AllModels(ctx context.Context, onlyEnabled bool) ([]string, error) {
	channels, err := s.ListChannels(ctx, false)
	if err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	models := make([]string, 0)
	for _, channel := range channels {
		if onlyEnabled && !channel.Enabled {
			continue
		}
		for _, model := range modelList(channel.Models) {
			if _, ok := seen[model]; ok {
				continue
			}
			seen[model] = struct{}{}
			models = append(models, model)
		}
	}
	sort.Strings(models)
	return models, nil
}

func normalizeChannelPayload(payload ChannelPayload) ChannelPayload {
	if payload.Type <= 0 {
		payload.Type = 1
	}
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Group = normalizeGroupName(payload.Group)
	payload.BaseURL = strings.TrimRight(strings.TrimSpace(payload.BaseURL), "/")
	if strings.TrimSpace(payload.APIKey) == "" {
		payload.APIKey = payload.Key
	}
	payload.APIKey = strings.TrimSpace(payload.APIKey)
	payload.Models = normalizeModels(payload.Models)
	if payload.Status != 0 {
		payload.Enabled = payload.Status == 1
	}
	payload.OpenAIOrganization = strings.TrimSpace(payload.OpenAIOrganization)
	payload.ModelMapping = strings.TrimSpace(payload.ModelMapping)
	payload.StatusCodeMapping = strings.TrimSpace(payload.StatusCodeMapping)
	payload.TestModel = strings.TrimSpace(payload.TestModel)
	payload.Tag = strings.TrimSpace(payload.Tag)
	payload.Setting = strings.TrimSpace(payload.Setting)
	payload.ParamOverride = strings.TrimSpace(payload.ParamOverride)
	payload.HeaderOverride = strings.TrimSpace(payload.HeaderOverride)
	payload.Other = strings.TrimSpace(payload.Other)
	payload.OtherInfo = strings.TrimSpace(payload.OtherInfo)
	payload.Settings = strings.TrimSpace(payload.Settings)
	payload.Remark = strings.TrimSpace(payload.Remark)
	if payload.InputPricePer1K < 0 {
		payload.InputPricePer1K = 0
	}
	if payload.OutputPricePer1K < 0 {
		payload.OutputPricePer1K = 0
	}
	return payload
}

func normalizeAutoBan(value int) int {
	if value == 0 {
		return 1
	}
	return value
}

func normalizeJSONText(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		value = strings.TrimSpace(fallback)
	}
	if value == "" {
		return "{}"
	}
	return value
}

func normalizeModels(models string) string {
	parts := modelList(models)
	return strings.Join(parts, ",")
}

func modelList(models string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	for _, raw := range strings.FieldsFunc(models, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r' || r == '\t'
	}) {
		model := strings.TrimSpace(raw)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func (s *Store) EnsureGroup(ctx context.Context, name string) error {
	name = normalizeGroupName(name)
	now := time.Now().Unix()
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO groups
		(name, description, price_multiplier, created_at, updated_at)
		VALUES (?, '', 1, ?, ?)`, name, now, now)
	return err
}

func (s *Store) ListGroups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT g.id, g.name, g.description, g.price_multiplier, g.created_at, g.updated_at,
		COALESCE(ch.channels, 0), COALESCE(u.tokens, 0), COALESCE(u.requests, 0), COALESCE(u.cost, 0)
		FROM groups g
		LEFT JOIN (SELECT group_name, COUNT(*) channels FROM channels GROUP BY group_name) ch ON ch.group_name = g.name
		LEFT JOIN (SELECT group_name, SUM(total_tokens) tokens, COUNT(*) requests, SUM(cost) cost FROM usage_logs GROUP BY group_name) u ON u.group_name = g.name
		ORDER BY g.name = 'default' DESC, g.name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Group, 0)
	for rows.Next() {
		var group Group
		if err := rows.Scan(&group.ID, &group.Name, &group.Description, &group.PriceMultiplier, &group.CreatedAt, &group.UpdatedAt, &group.Channels, &group.UsedTokens, &group.RequestCount, &group.Cost); err != nil {
			return nil, err
		}
		out = append(out, group)
	}
	return out, rows.Err()
}

func (s *Store) CreateGroup(ctx context.Context, payload GroupPayload) (Group, error) {
	payload = normalizeGroupPayload(payload)
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO groups
		(name, description, price_multiplier, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, payload.Name, payload.Description, payload.PriceMultiplier, now, now)
	if err != nil {
		return Group{}, err
	}
	id, _ := res.LastInsertId()
	return s.getGroup(ctx, id)
}

func (s *Store) UpsertGroupByName(ctx context.Context, payload GroupPayload) error {
	rawRatio := payload.PriceMultiplier
	payload = normalizeGroupPayload(payload)
	now := time.Now().Unix()
	if payload.Name == "" {
		payload.Name = "default"
	}
	current := Group{}
	rows, err := s.ListGroups(ctx)
	if err != nil {
		return err
	}
	for _, group := range rows {
		if group.Name == payload.Name {
			current = group
			break
		}
	}
	description := payload.Description
	ratio := payload.PriceMultiplier
	if current.ID > 0 {
		if description == "" {
			description = current.Description
		}
		if rawRatio <= 0 {
			ratio = current.PriceMultiplier
		}
		_, err := s.db.ExecContext(ctx, `UPDATE groups SET description = ?, price_multiplier = ?, updated_at = ? WHERE name = ?`,
			description, ratio, now, payload.Name)
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO groups
		(name, description, price_multiplier, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, payload.Name, description, ratio, now, now)
	return err
}

func (s *Store) UpdateGroup(ctx context.Context, id int64, payload GroupPayload) (Group, error) {
	payload = normalizeGroupPayload(payload)
	current, err := s.getGroup(ctx, id)
	if err != nil {
		return Group{}, err
	}
	name := payload.Name
	if current.Name == "default" {
		name = "default"
	}
	_, err = s.db.ExecContext(ctx, `UPDATE groups SET name = ?, description = ?, price_multiplier = ?, updated_at = ? WHERE id = ?`, name, payload.Description, payload.PriceMultiplier, time.Now().Unix(), id)
	if err != nil {
		return Group{}, err
	}
	if current.Name != name {
		_, err = s.db.ExecContext(ctx, `UPDATE channels SET group_name = ? WHERE group_name = ?`, name, current.Name)
		if err != nil {
			return Group{}, err
		}
	}
	return s.getGroup(ctx, id)
}

func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	group, err := s.getGroup(ctx, id)
	if err != nil {
		return err
	}
	if group.Name == "default" {
		return errors.New("default group cannot be deleted")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE channels SET group_name = 'default' WHERE group_name = ?`, group.Name); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM groups WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ListAPIKeys(ctx context.Context, keyword, token string, page, size int) ([]APIKey, int64, error) {
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 20
	}
	keyword = strings.TrimSpace(keyword)
	token = strings.TrimSpace(strings.TrimPrefix(token, "sk-"))

	where := `WHERE 1 = 1`
	args := []any{}
	if keyword != "" {
		where += ` AND name LIKE ?`
		args = append(args, "%"+keyword+"%")
	}
	if token != "" {
		where += ` AND token_key LIKE ?`
		args = append(args, "%"+token+"%")
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_keys `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, size, (page-1)*size)
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, token_key, status, remain_quota, used_quota,
		unlimited_quota, expired_time, created_time, accessed_time, group_name, cross_group_retry,
		model_limits_enabled, model_limits, allow_ips
		FROM api_keys `+where+`
		ORDER BY id DESC LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]APIKey, 0)
	for rows.Next() {
		item, err := scanAPIKey(rows, false)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) GetAPIKey(ctx context.Context, id int64, includeKey bool) (APIKey, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, token_key, status, remain_quota, used_quota,
		unlimited_quota, expired_time, created_time, accessed_time, group_name, cross_group_retry,
		model_limits_enabled, model_limits, allow_ips
		FROM api_keys WHERE id = ?`, id)
	return scanAPIKey(row, includeKey)
}

func (s *Store) GetAPIKeyBySecret(ctx context.Context, secret string) (APIKey, error) {
	secret = normalizeAPIKeySecret(secret)
	if secret == "" {
		return APIKey{}, sql.ErrNoRows
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, name, token_key, status, remain_quota, used_quota,
		unlimited_quota, expired_time, created_time, accessed_time, group_name, cross_group_retry,
		model_limits_enabled, model_limits, allow_ips
		FROM api_keys WHERE token_key = ?`, secret)
	return scanAPIKey(row, true)
}

func (s *Store) TouchAPIKey(ctx context.Context, id int64, accessedAt int64) error {
	if id <= 0 {
		return sql.ErrNoRows
	}
	if accessedAt <= 0 {
		accessedAt = time.Now().Unix()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE api_keys SET accessed_time = ? WHERE id = ?`, accessedAt, id)
	return err
}

func (s *Store) CreateAPIKey(ctx context.Context, payload APIKeyPayload) (APIKey, error) {
	payload = normalizeAPIKeyPayload(payload)
	if payload.Name == "" {
		return APIKey{}, errors.New("name is required")
	}
	if err := s.EnsureGroup(ctx, payload.Group); err != nil {
		return APIKey{}, err
	}
	secret, err := generateAPIKeySecret()
	if err != nil {
		return APIKey{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.ExecContext(ctx, `INSERT INTO api_keys
		(name, token_key, status, remain_quota, used_quota, unlimited_quota, expired_time, created_time,
		accessed_time, group_name, cross_group_retry, model_limits_enabled, model_limits, allow_ips)
		VALUES (?, ?, ?, ?, 0, ?, ?, ?, 0, ?, ?, ?, ?, ?)`,
		payload.Name,
		secret,
		normalizeAPIKeyStatus(payload.Status),
		payload.RemainQuota,
		boolInt(payload.UnlimitedQuota),
		payload.ExpiredTime,
		now,
		payload.Group,
		boolInt(payload.CrossGroupRetry),
		boolInt(payload.ModelLimitsEnabled),
		payload.ModelLimits,
		payload.AllowIPs,
	)
	if err != nil {
		return APIKey{}, err
	}
	id, _ := res.LastInsertId()
	return s.GetAPIKey(ctx, id, false)
}

func (s *Store) UpdateAPIKey(ctx context.Context, payload APIKeyPayload) (APIKey, error) {
	payload = normalizeAPIKeyPayload(payload)
	if payload.ID <= 0 {
		return APIKey{}, errors.New("id is required")
	}
	if payload.Name == "" {
		return APIKey{}, errors.New("name is required")
	}
	if err := s.EnsureGroup(ctx, payload.Group); err != nil {
		return APIKey{}, err
	}
	status := normalizeAPIKeyStatus(payload.Status)
	if payload.Status == 0 {
		current, err := s.GetAPIKey(ctx, payload.ID, false)
		if err != nil {
			return APIKey{}, err
		}
		status = current.Status
	}
	res, err := s.db.ExecContext(ctx, `UPDATE api_keys SET
		name = ?, status = ?, remain_quota = ?, unlimited_quota = ?, expired_time = ?,
		group_name = ?, cross_group_retry = ?, model_limits_enabled = ?, model_limits = ?, allow_ips = ?
		WHERE id = ?`,
		payload.Name,
		status,
		payload.RemainQuota,
		boolInt(payload.UnlimitedQuota),
		payload.ExpiredTime,
		payload.Group,
		boolInt(payload.CrossGroupRetry),
		boolInt(payload.ModelLimitsEnabled),
		payload.ModelLimits,
		payload.AllowIPs,
		payload.ID,
	)
	if err != nil {
		return APIKey{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return APIKey{}, err
	}
	if affected == 0 {
		return APIKey{}, sql.ErrNoRows
	}
	return s.GetAPIKey(ctx, payload.ID, false)
}

func (s *Store) SetAPIKeyStatus(ctx context.Context, id int64, status int) (APIKey, error) {
	status = normalizeAPIKeyStatus(status)
	res, err := s.db.ExecContext(ctx, `UPDATE api_keys SET status = ? WHERE id = ?`, status, id)
	if err != nil {
		return APIKey{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return APIKey{}, err
	}
	if affected == 0 {
		return APIKey{}, sql.ErrNoRows
	}
	return s.GetAPIKey(ctx, id, false)
}

func (s *Store) DeleteAPIKey(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) DeleteAPIKeys(ctx context.Context, ids []int64) (int64, error) {
	var deleted int64
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		res, err := s.db.ExecContext(ctx, `DELETE FROM api_keys WHERE id = ?`, id)
		if err != nil {
			return deleted, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return deleted, err
		}
		deleted += affected
	}
	return deleted, nil
}

func (s *Store) ListPlaygroundSessions(ctx context.Context, userID int64, page, size int) ([]PlaygroundSession, int64, error) {
	if userID <= 0 {
		userID = 1
	}
	if page <= 0 {
		page = 1
	}
	if size <= 0 || size > 100 {
		size = 50
	}

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM playground_conversations WHERE user_id = ?`, userID).Scan(&total); err != nil {
		return nil, 0, err
	}

	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, title, summary, model, group_name, pinned,
		message_count, config, created_time, updated_time
		FROM playground_conversations
		WHERE user_id = ?
		ORDER BY pinned DESC, updated_time DESC, id DESC
		LIMIT ? OFFSET ?`, userID, size, (page-1)*size)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	items := make([]PlaygroundSession, 0)
	for rows.Next() {
		item, err := scanPlaygroundSessionSummary(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, item)
	}
	return items, total, rows.Err()
}

func (s *Store) GetPlaygroundSession(ctx context.Context, userID, id int64) (PlaygroundSession, error) {
	if userID <= 0 {
		userID = 1
	}
	row := s.db.QueryRowContext(ctx, `SELECT s.id, s.user_id, s.title, s.summary, s.model, s.group_name, s.pinned,
		COALESCE(d.messages, '[]'), s.message_count, s.config, s.created_time, s.updated_time
		FROM playground_conversations s
		LEFT JOIN playground_conversation_messages d ON d.session_id = s.id AND d.user_id = s.user_id
		WHERE s.id = ? AND s.user_id = ?`, id, userID)
	return scanPlaygroundSession(row, true)
}

func (s *Store) SavePlaygroundSession(ctx context.Context, userID int64, payload PlaygroundSessionPayload) (PlaygroundSession, error) {
	if userID <= 0 {
		userID = 1
	}
	payload, messageCount, err := normalizePlaygroundSessionPayload(payload)
	if err != nil {
		return PlaygroundSession{}, err
	}

	now := time.Now().Unix()
	if payload.ID > 0 {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return PlaygroundSession{}, err
		}
		defer func() {
			_ = tx.Rollback()
		}()

		res, err := tx.ExecContext(ctx, `UPDATE playground_conversations SET
			message_count = ?, config = ?, updated_time = ?
			WHERE id = ? AND user_id = ?`,
			messageCount,
			string(payload.Config),
			now,
			payload.ID,
			userID,
		)
		if err != nil {
			return PlaygroundSession{}, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return PlaygroundSession{}, err
		}
		if affected == 0 {
			return PlaygroundSession{}, sql.ErrNoRows
		}
		if err := upsertPlaygroundSessionDetailsTx(ctx, tx, payload.ID, userID, payload.Messages, now); err != nil {
			return PlaygroundSession{}, err
		}
		if err := tx.Commit(); err != nil {
			return PlaygroundSession{}, err
		}
		return s.GetPlaygroundSession(ctx, userID, payload.ID)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlaygroundSession{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	res, err := tx.ExecContext(ctx, `INSERT INTO playground_conversations
		(user_id, title, summary, model, group_name, pinned, message_count, config, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, 0, ?, ?, ?, ?)`,
		userID,
		payload.Title,
		payload.Summary,
		payload.Model,
		payload.Group,
		messageCount,
		string(payload.Config),
		now,
		now,
	)
	if err != nil {
		return PlaygroundSession{}, err
	}
	id, _ := res.LastInsertId()
	if err := upsertPlaygroundSessionDetailsTx(ctx, tx, id, userID, payload.Messages, now); err != nil {
		return PlaygroundSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return PlaygroundSession{}, err
	}
	return s.GetPlaygroundSession(ctx, userID, id)
}

func (s *Store) PatchPlaygroundSession(ctx context.Context, userID, id int64, payload PlaygroundSessionMetaPayload) (PlaygroundSession, error) {
	if userID <= 0 {
		userID = 1
	}
	if id <= 0 {
		return PlaygroundSession{}, sql.ErrNoRows
	}
	if payload.Title == nil && payload.Pinned == nil {
		return PlaygroundSession{}, errors.New("title or pinned is required")
	}

	sets := make([]string, 0, 2)
	args := make([]any, 0, 4)
	if payload.Title != nil {
		title := truncateRunes(strings.TrimSpace(*payload.Title), 255)
		sets = append(sets, "title = ?")
		args = append(args, title)
	}
	if payload.Pinned != nil {
		sets = append(sets, "pinned = ?")
		args = append(args, boolInt(*payload.Pinned))
	}
	args = append(args, id, userID)

	res, err := s.db.ExecContext(ctx, `UPDATE playground_conversations SET `+strings.Join(sets, ", ")+` WHERE id = ? AND user_id = ?`, args...)
	if err != nil {
		return PlaygroundSession{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return PlaygroundSession{}, err
	}
	if affected == 0 {
		return PlaygroundSession{}, sql.ErrNoRows
	}
	return s.GetPlaygroundSession(ctx, userID, id)
}

func (s *Store) DeletePlaygroundSession(ctx context.Context, userID, id int64) error {
	if userID <= 0 {
		userID = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM playground_conversation_messages WHERE session_id = ? AND user_id = ?`, id, userID); err != nil {
		return err
	}
	res, err := tx.ExecContext(ctx, `DELETE FROM playground_conversations WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return sql.ErrNoRows
	}
	return tx.Commit()
}

func (s *Store) DeleteAllPlaygroundSessions(ctx context.Context, userID int64) error {
	if userID <= 0 {
		userID = 1
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	if _, err := tx.ExecContext(ctx, `DELETE FROM playground_conversation_messages WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM playground_conversations WHERE user_id = ?`, userID); err != nil {
		return err
	}
	return tx.Commit()
}

func upsertPlaygroundSessionDetailsTx(ctx context.Context, tx *sql.Tx, sessionID, userID int64, messages json.RawMessage, now int64) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO playground_conversation_messages
		(session_id, user_id, messages, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(session_id, user_id) DO UPDATE SET
			messages = excluded.messages,
			updated_time = excluded.updated_time`,
		sessionID,
		userID,
		string(messages),
		now,
		now,
	)
	return err
}

func (s *Store) SavePlaygroundAttachment(ctx context.Context, userID int64, mediaType, filename string, data []byte) (PlaygroundAttachment, error) {
	if userID <= 0 {
		userID = 1
	}
	mediaType = strings.TrimSpace(mediaType)
	filename = truncateRunes(strings.TrimSpace(filepath.Base(filename)), 255)
	if !strings.HasPrefix(strings.ToLower(mediaType), "image/") {
		return PlaygroundAttachment{}, errors.New("only image attachments are supported")
	}
	if len(data) == 0 {
		return PlaygroundAttachment{}, errors.New("attachment is empty")
	}

	id, err := generatePlaygroundAttachmentID()
	if err != nil {
		return PlaygroundAttachment{}, err
	}
	now := time.Now().Unix()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO playground_attachments
		(id, user_id, type, media_type, filename, size, data, created_at)
		VALUES (?, ?, 'image', ?, ?, ?, ?, ?)`,
		id,
		userID,
		mediaType,
		filename,
		int64(len(data)),
		data,
		now,
	); err != nil {
		return PlaygroundAttachment{}, err
	}

	return PlaygroundAttachment{
		ID:        id,
		FileID:    id,
		UserID:    userID,
		Type:      "image",
		URL:       "/api/playground/attachments/" + id,
		MediaType: mediaType,
		Filename:  filename,
		Size:      int64(len(data)),
		CreatedAt: now,
	}, nil
}

func (s *Store) GetPlaygroundAttachment(ctx context.Context, userID int64, id string) (PlaygroundAttachment, error) {
	if userID <= 0 {
		userID = 1
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return PlaygroundAttachment{}, sql.ErrNoRows
	}

	var item PlaygroundAttachment
	if err := s.db.QueryRowContext(ctx, `SELECT id, user_id, type, media_type, filename, size, data, created_at
		FROM playground_attachments WHERE id = ? AND user_id = ?`, id, userID).Scan(
		&item.ID,
		&item.UserID,
		&item.Type,
		&item.MediaType,
		&item.Filename,
		&item.Size,
		&item.Data,
		&item.CreatedAt,
	); err != nil {
		return PlaygroundAttachment{}, err
	}
	item.FileID = item.ID
	item.URL = "/api/playground/attachments/" + item.ID
	return item, nil
}

func (s *Store) ListPlaygroundImageGenerations(ctx context.Context, userID int64, limit int) ([]PlaygroundImageGeneration, error) {
	if userID <= 0 {
		userID = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, prompt, negative_prompt, params, model, group_name,
		reference_images, image_urls, status, error_message, duration_ms, created_time, updated_time
		FROM playground_image_generations
		WHERE user_id = ?
		ORDER BY created_time DESC, updated_time DESC
		LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PlaygroundImageGeneration, 0)
	for rows.Next() {
		item, err := scanPlaygroundImageGeneration(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) SavePlaygroundImageGeneration(ctx context.Context, userID int64, item PlaygroundImageGeneration) (PlaygroundImageGeneration, error) {
	if userID <= 0 {
		userID = 1
	}
	item.ID = strings.TrimSpace(item.ID)
	if item.ID == "" {
		return PlaygroundImageGeneration{}, errors.New("image generation id is required")
	}
	item.Prompt = truncateRunes(strings.TrimSpace(item.Prompt), 4000)
	item.NegativePrompt = truncateRunes(strings.TrimSpace(item.NegativePrompt), 2000)
	item.Model = truncateRunes(strings.TrimSpace(item.Model), 255)
	item.Group = truncateRunes(strings.TrimSpace(item.Group), 255)
	item.Status = normalizeImageGenerationStatus(item.Status)
	item.ErrorMessage = truncateRunes(strings.TrimSpace(item.ErrorMessage), 2000)
	item.Params = normalizeRawJSON(item.Params, "{}")
	item.ReferenceImages = normalizeRawJSON(item.ReferenceImages, "[]")
	if item.DurationMS < 0 {
		item.DurationMS = 0
	}

	urls, err := json.Marshal(cleanStringList(item.URLs, 16, 2048))
	if err != nil {
		return PlaygroundImageGeneration{}, err
	}
	now := time.Now().UnixMilli()
	if item.CreatedAt <= 0 {
		item.CreatedAt = now
	}
	item.UpdatedAt = now

	_, err = s.db.ExecContext(ctx, `INSERT INTO playground_image_generations
		(id, user_id, prompt, negative_prompt, params, model, group_name, reference_images, image_urls,
			status, error_message, duration_ms, created_time, updated_time)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			user_id = excluded.user_id,
			prompt = excluded.prompt,
			negative_prompt = excluded.negative_prompt,
			params = excluded.params,
			model = excluded.model,
			group_name = excluded.group_name,
			reference_images = excluded.reference_images,
			image_urls = excluded.image_urls,
			status = excluded.status,
			error_message = excluded.error_message,
			duration_ms = excluded.duration_ms,
			created_time = excluded.created_time,
			updated_time = excluded.updated_time`,
		item.ID,
		userID,
		item.Prompt,
		item.NegativePrompt,
		string(item.Params),
		item.Model,
		item.Group,
		string(item.ReferenceImages),
		string(urls),
		item.Status,
		item.ErrorMessage,
		item.DurationMS,
		item.CreatedAt,
		item.UpdatedAt,
	)
	if err != nil {
		return PlaygroundImageGeneration{}, err
	}
	item.UserID = userID
	item.URLs = cleanStringList(item.URLs, 16, 2048)
	return item, nil
}

func (s *Store) DeletePlaygroundImageGeneration(ctx context.Context, userID int64, id string) ([]string, error) {
	if userID <= 0 {
		userID = 1
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("image generation id is required")
	}

	var imageURLs string
	if err := s.db.QueryRowContext(ctx, `SELECT image_urls FROM playground_image_generations WHERE id = ? AND user_id = ?`, id, userID).Scan(&imageURLs); err != nil {
		return nil, err
	}

	var urls []string
	if err := json.Unmarshal([]byte(imageURLs), &urls); err != nil {
		urls = []string{}
	}

	res, err := s.db.ExecContext(ctx, `DELETE FROM playground_image_generations WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return nil, err
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return nil, sql.ErrNoRows
	}
	return cleanStringList(urls, 16, 2048), nil
}

func generatePlaygroundAttachmentID() (string, error) {
	buf := make([]byte, 18)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "pgatt_" + base64.RawURLEncoding.EncodeToString(buf), nil
}

func scanPlaygroundImageGeneration(scanner interface {
	Scan(...any) error
}) (PlaygroundImageGeneration, error) {
	var item PlaygroundImageGeneration
	var params string
	var referenceImages string
	var imageURLs string
	if err := scanner.Scan(
		&item.ID,
		&item.UserID,
		&item.Prompt,
		&item.NegativePrompt,
		&params,
		&item.Model,
		&item.Group,
		&referenceImages,
		&imageURLs,
		&item.Status,
		&item.ErrorMessage,
		&item.DurationMS,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return PlaygroundImageGeneration{}, err
	}
	item.Params = normalizeRawJSON(json.RawMessage(params), "{}")
	item.ReferenceImages = normalizeRawJSON(json.RawMessage(referenceImages), "[]")
	if err := json.Unmarshal([]byte(imageURLs), &item.URLs); err != nil {
		item.URLs = []string{}
	}
	item.URLs = cleanStringList(item.URLs, 16, 2048)
	item.Status = normalizeImageGenerationStatus(item.Status)
	return item, nil
}

func normalizeRawJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) > 0 && json.Valid(raw) {
		return raw
	}
	return json.RawMessage(fallback)
}

func normalizeImageGenerationStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "running", "succeeded", "failed":
		return strings.ToLower(strings.TrimSpace(status))
	default:
		return "succeeded"
	}
}

func cleanStringList(values []string, maxItems, maxLen int) []string {
	if maxItems <= 0 {
		maxItems = len(values)
	}
	out := make([]string, 0, minInt(len(values), maxItems))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, truncateRunes(value, maxLen))
		if len(out) >= maxItems {
			break
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func scanAPIKey(scanner interface {
	Scan(...any) error
}, includeKey bool) (APIKey, error) {
	var item APIKey
	var unlimited int
	var crossGroupRetry int
	var modelLimitsEnabled int
	if err := scanner.Scan(
		&item.ID,
		&item.Name,
		&item.Key,
		&item.Status,
		&item.RemainQuota,
		&item.UsedQuota,
		&unlimited,
		&item.ExpiredTime,
		&item.CreatedTime,
		&item.AccessedTime,
		&item.Group,
		&crossGroupRetry,
		&modelLimitsEnabled,
		&item.ModelLimits,
		&item.AllowIPs,
	); err != nil {
		return APIKey{}, err
	}
	item.UnlimitedQuota = unlimited != 0
	item.CrossGroupRetry = crossGroupRetry != 0
	item.ModelLimitsEnabled = modelLimitsEnabled != 0
	if !includeKey {
		item.Key = maskAPIKeySecret(item.Key)
	}
	return item, nil
}

func scanPlaygroundSessionSummary(scanner interface {
	Scan(...any) error
}) (PlaygroundSession, error) {
	return scanPlaygroundSession(scanner, false)
}

func scanPlaygroundSession(scanner interface {
	Scan(...any) error
}, includeMessages bool) (PlaygroundSession, error) {
	var item PlaygroundSession
	var pinned int
	var messages string
	var config string
	dest := []any{
		&item.ID,
		&item.UserID,
		&item.Title,
		&item.Summary,
		&item.Model,
		&item.Group,
		&pinned,
	}
	if includeMessages {
		dest = append(dest, &messages)
	}
	dest = append(dest,
		&item.MessageCount,
		&config,
		&item.CreatedTime,
		&item.UpdatedTime,
	)
	if err := scanner.Scan(dest...); err != nil {
		return PlaygroundSession{}, err
	}
	item.Pinned = pinned != 0
	item.Config = json.RawMessage(defaultJSON(config, "{}"))
	if includeMessages {
		item.Messages = json.RawMessage(defaultJSON(messages, "[]"))
	}
	return item, nil
}

func normalizePlaygroundSessionPayload(payload PlaygroundSessionPayload) (PlaygroundSessionPayload, int, error) {
	payload.Title = truncateRunes(strings.TrimSpace(payload.Title), 255)
	payload.Summary = truncateRunes(strings.TrimSpace(payload.Summary), 512)
	payload.Model = strings.TrimSpace(payload.Model)
	payload.Group = normalizeGroupName(payload.Group)

	if len([]rune(payload.Model)) > 255 {
		return payload, 0, errors.New("model is too long")
	}
	if len([]rune(payload.Group)) > 64 {
		return payload, 0, errors.New("group is too long")
	}
	if len(payload.Messages) == 0 || string(payload.Messages) == "null" {
		payload.Messages = json.RawMessage("[]")
	}
	if len(payload.Config) == 0 || string(payload.Config) == "null" {
		payload.Config = json.RawMessage("{}")
	}
	if !json.Valid(payload.Messages) {
		return payload, 0, errors.New("messages must be valid JSON")
	}
	if !json.Valid(payload.Config) {
		return payload, 0, errors.New("config must be valid JSON")
	}

	var messages []json.RawMessage
	if err := json.Unmarshal(payload.Messages, &messages); err != nil {
		return payload, 0, errors.New("messages must be a JSON array")
	}
	if payload.Title == "" && len(messages) > 0 {
		payload.Title = "Current conversation"
	}
	return payload, len(messages), nil
}

func defaultJSON(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" || !json.Valid([]byte(value)) {
		return fallback
	}
	return value
}

func truncateRunes(value string, maxLength int) string {
	if maxLength <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxLength {
		return value
	}
	return string(runes[:maxLength])
}

func normalizeAPIKeyPayload(payload APIKeyPayload) APIKeyPayload {
	payload.Name = strings.TrimSpace(payload.Name)
	payload.Group = normalizeGroupName(payload.Group)
	payload.ModelLimits = strings.TrimSpace(payload.ModelLimits)
	payload.AllowIPs = strings.TrimSpace(payload.AllowIPs)
	if payload.RemainQuota < 0 {
		payload.RemainQuota = 0
	}
	if payload.ExpiredTime == 0 {
		payload.ExpiredTime = -1
	}
	return payload
}

func normalizeAPIKeyStatus(status int) int {
	switch status {
	case 1, 2, 3, 4:
		return status
	default:
		return 1
	}
}

func generateAPIKeySecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func normalizeAPIKeySecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(secret), "bearer ") {
		secret = strings.TrimSpace(secret[7:])
	}
	secret = strings.TrimSpace(secret)
	if strings.HasPrefix(secret, "sk-") {
		secret = strings.TrimPrefix(secret, "sk-")
	}
	return strings.TrimSpace(secret)
}

func maskAPIKeySecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return ""
	}
	if len(secret) <= 10 {
		return secret
	}
	return secret[:4] + strings.Repeat("*", 8) + secret[len(secret)-4:]
}

func (s *Store) APIKeySecret(ctx context.Context, id int64) (string, error) {
	var secret string
	err := s.db.QueryRowContext(ctx, `SELECT token_key FROM api_keys WHERE id = ?`, id).Scan(&secret)
	return secret, err
}

func (s *Store) APIKeySecrets(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		secret, err := s.APIKeySecret(ctx, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		out[id] = secret
	}
	return out, nil
}

func normalizeGroupPayload(payload GroupPayload) GroupPayload {
	payload.Name = normalizeGroupName(payload.Name)
	payload.Description = strings.TrimSpace(payload.Description)
	if payload.PriceMultiplier < 0 {
		payload.PriceMultiplier = 0
	}
	if payload.PriceMultiplier == 0 {
		payload.PriceMultiplier = 1
	}
	return payload
}

func (s *Store) getGroup(ctx context.Context, id int64) (Group, error) {
	groups, err := s.ListGroups(ctx)
	if err != nil {
		return Group{}, err
	}
	for _, group := range groups {
		if group.ID == id {
			return group, nil
		}
	}
	return Group{}, sql.ErrNoRows
}

func (s *Store) InsertUsageLog(ctx context.Context, log UsageLog) error {
	if log.CreatedAt == 0 {
		log.CreatedAt = time.Now().Unix()
	}
	if log.StatusCode == 0 {
		log.StatusCode = 200
	}
	if strings.TrimSpace(log.Username) == "" {
		log.Username = "local"
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO usage_logs
		(created_at, channel_id, channel_name, username, group_name, model, endpoint, prompt_tokens, completion_tokens, total_tokens, cost, duration_ms, status_code, error)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.CreatedAt,
		log.ChannelID,
		log.ChannelName,
		log.Username,
		log.Group,
		log.Model,
		log.Endpoint,
		log.PromptTokens,
		log.CompletionTokens,
		log.TotalTokens,
		log.Cost,
		log.DurationMS,
		log.StatusCode,
		log.Error,
	)
	return err
}

func (s *Store) ListUsageLogs(ctx context.Context, limit int) ([]UsageLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, created_at, channel_id, channel_name, username, group_name, model, endpoint,
		prompt_tokens, completion_tokens, total_tokens, cost, duration_ms, status_code, error
		FROM usage_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]UsageLog, 0)
	for rows.Next() {
		var log UsageLog
		if err := rows.Scan(&log.ID, &log.CreatedAt, &log.ChannelID, &log.ChannelName, &log.Username, &log.Group, &log.Model, &log.Endpoint, &log.PromptTokens, &log.CompletionTokens, &log.TotalTokens, &log.Cost, &log.DurationMS, &log.StatusCode, &log.Error); err != nil {
			return nil, err
		}
		out = append(out, log)
	}
	return out, rows.Err()
}

func (s *Store) ClearUsageLogs(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM usage_logs`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) Dashboard(ctx context.Context) (Dashboard, error) {
	settings, err := s.Settings(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	var dashboard Dashboard
	dashboard.Settings = settings
	err = s.db.QueryRowContext(ctx, `SELECT
		COALESCE(SUM(cost), 0), COALESCE(SUM(total_tokens), 0), COALESCE(SUM(prompt_tokens), 0),
		COALESCE(SUM(completion_tokens), 0), COALESCE(COUNT(*), 0)
		FROM usage_logs`).Scan(&dashboard.TotalCost, &dashboard.TotalTokens, &dashboard.PromptTokens, &dashboard.CompletionTokens, &dashboard.TotalRequests)
	if err != nil {
		return Dashboard{}, err
	}
	cutoff := time.Now().Add(-24 * time.Hour).Unix()
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost), 0), COALESCE(SUM(total_tokens), 0), COALESCE(COUNT(*), 0)
		FROM usage_logs WHERE created_at >= ?`, cutoff).Scan(&dashboard.Last24hCost, &dashboard.Last24hTokens, &dashboard.Last24hRequests)
	if err != nil {
		return Dashboard{}, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(CASE WHEN enabled <> 0 THEN 1 ELSE 0 END), 0), COALESCE(COUNT(*), 0) FROM channels`).Scan(&dashboard.EnabledChannels, &dashboard.TotalChannels)
	if err != nil {
		return Dashboard{}, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM groups`).Scan(&dashboard.Groups)
	if err != nil {
		return Dashboard{}, err
	}
	dashboard.AverageRPM = float64(dashboard.Last24hRequests) / 1440
	dashboard.AverageTPM = float64(dashboard.Last24hTokens) / 1440
	dashboard.Chart, err = s.chart(ctx)
	if err != nil {
		return Dashboard{}, err
	}
	dashboard.TopChannels, err = s.topChannels(ctx)
	return dashboard, err
}

func (s *Store) chart(ctx context.Context) ([]ChartPoint, error) {
	start := time.Now().AddDate(0, 0, -13)
	points := make([]ChartPoint, 0, 14)
	for i := 0; i < 14; i++ {
		day := start.AddDate(0, 0, i)
		date := day.Format("2006-01-02")
		from := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, time.Local).Unix()
		to := from + 86400
		point := ChartPoint{Date: date}
		err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost), 0), COALESCE(SUM(total_tokens), 0), COALESCE(COUNT(*), 0)
			FROM usage_logs WHERE created_at >= ? AND created_at < ?`, from, to).Scan(&point.Cost, &point.Tokens, &point.Requests)
		if err != nil {
			return nil, err
		}
		points = append(points, point)
	}
	return points, nil
}

func (s *Store) topChannels(ctx context.Context) ([]TopChannel, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT channel_id, channel_name, COALESCE(SUM(cost), 0), COALESCE(SUM(total_tokens), 0), COALESCE(COUNT(*), 0)
		FROM usage_logs GROUP BY channel_id, channel_name ORDER BY SUM(cost) DESC, COUNT(*) DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]TopChannel, 0)
	for rows.Next() {
		var item TopChannel
		if err := rows.Scan(&item.ID, &item.Name, &item.Cost, &item.Tokens, &item.Requests); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) ChannelForUsage(ctx context.Context, apiKey, baseURL, provider string) (Channel, float64, error) {
	apiKey = strings.TrimSpace(apiKey)
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	provider = strings.TrimSpace(provider)
	query := `SELECT c.id, c.type, c.name, c.group_name, c.base_url, c.api_key, c.models, c.enabled,
		c.input_price_per_1k, c.output_price_per_1k, c.remark, c.created_at, c.updated_at,
		COALESCE(g.price_multiplier, 1)
		FROM channels c
		LEFT JOIN groups g ON g.name = c.group_name
		WHERE 1 = 1`
	args := []any{}
	if channelID, ok := channelIDFromCompatProvider(provider); ok {
		query += ` AND c.id = ?`
		args = append(args, channelID)
	} else if apiKey != "" {
		query += ` AND c.api_key = ?`
		args = append(args, apiKey)
	} else if baseURL != "" {
		query += ` AND c.base_url = ?`
		args = append(args, baseURL)
	} else if provider != "" {
		query += ` AND lower(c.name) = lower(?)`
		args = append(args, provider)
	} else {
		return Channel{}, 1, sql.ErrNoRows
	}
	query += ` ORDER BY c.id LIMIT 1`
	var channel Channel
	var enabled int
	var multiplier float64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&channel.ID, &channel.Type, &channel.Name, &channel.Group, &channel.BaseURL, &channel.APIKey, &channel.Models, &enabled, &channel.InputPricePer1K, &channel.OutputPricePer1K, &channel.Remark, &channel.CreatedAt, &channel.UpdatedAt, &multiplier)
	channel.Enabled = enabled != 0
	return channel, multiplier, err
}

func channelIDFromCompatProvider(provider string) (int64, bool) {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if !strings.HasPrefix(provider, "sunapi-") {
		return 0, false
	}
	rest := strings.TrimPrefix(provider, "sunapi-")
	parts := strings.SplitN(rest, "-", 2)
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}

func UsageCost(channel Channel, multiplier float64, inputTokens, outputTokens int64) float64 {
	return UsageCostForModel(channel, Settings{}, multiplier, "", inputTokens, outputTokens)
}

func (s *Store) CountUsageLogs(ctx context.Context) (int64, error) {
	var count int64
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs`).Scan(&count)
	return count, err
}

func (s *Store) UserQuotaSummary(ctx context.Context) (int64, int64, error) {
	var totalCost float64
	var requestCount int64
	err := s.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(cost), 0), COALESCE(COUNT(*), 0) FROM usage_logs`).Scan(&totalCost, &requestCount)
	if err != nil {
		return 0, 0, err
	}
	return quotaFromCost(totalCost), requestCount, nil
}

func quotaFromCost(cost float64) int64 {
	if cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
		return 0
	}
	return int64(math.Round(cost * float64(localQuotaPerUnit)))
}

func normalizeTimeRange(start, end int64) (int64, int64) {
	now := time.Now().Unix()
	if end <= 0 {
		end = now
	}
	if start <= 0 || start > end {
		start = end - 30*24*3600
	}
	if start < 0 {
		start = 0
	}
	return start, end
}

func (s *Store) QueryQuotaData(ctx context.Context, start, end int64, username string, byUser bool) ([]QuotaData, error) {
	start, end = normalizeTimeRange(start, end)
	username = strings.TrimSpace(username)
	where := `WHERE created_at >= ? AND created_at <= ?`
	args := []any{start, end}
	if username != "" {
		where += ` AND COALESCE(NULLIF(TRIM(username), ''), 'local') = ?`
		args = append(args, username)
	}

	if byUser {
		rows, err := s.db.QueryContext(ctx, `
			SELECT ((created_at / 3600) * 3600) AS bucket,
				COALESCE(NULLIF(TRIM(username), ''), 'local') AS username,
				COALESCE(SUM(cost), 0) AS quota,
				COALESCE(SUM(total_tokens), 0) AS token_used,
				COALESCE(COUNT(*), 0) AS count
			FROM usage_logs
			`+where+`
			GROUP BY bucket, username
			ORDER BY bucket ASC, username ASC`, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		out := make([]QuotaData, 0)
		for rows.Next() {
			var item QuotaData
			var quota float64
			if err := rows.Scan(&item.CreatedAt, &item.Username, &quota, &item.TokenUsed, &item.Count); err != nil {
				return nil, err
			}
			item.UserID = 1
			item.ID = item.CreatedAt
			item.Quota = quotaFromCost(quota)
			out = append(out, item)
		}
		return out, rows.Err()
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT ((created_at / 3600) * 3600) AS bucket,
			COALESCE(NULLIF(TRIM(model), ''), 'unknown') AS model_name,
			COALESCE(NULLIF(TRIM(group_name), ''), 'unknown') AS group_name,
			COALESCE(SUM(cost), 0) AS quota,
			COALESCE(SUM(total_tokens), 0) AS token_used,
			COALESCE(COUNT(*), 0) AS count
		FROM usage_logs
		`+where+`
		GROUP BY bucket, model_name, group_name
		ORDER BY bucket ASC, model_name ASC, group_name ASC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]QuotaData, 0)
	for rows.Next() {
		var item QuotaData
		var quota float64
		if err := rows.Scan(&item.CreatedAt, &item.ModelName, &item.Group, &quota, &item.TokenUsed, &item.Count); err != nil {
			return nil, err
		}
		item.UserID = 1
		item.Username = "local"
		item.ID = item.CreatedAt
		item.Quota = quotaFromCost(quota)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) QueryQuotaDataByUsername(ctx context.Context, start, end int64, username string) ([]QuotaData, error) {
	start, end = normalizeTimeRange(start, end)
	username = strings.TrimSpace(username)
	if username == "" {
		username = "local"
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT ((created_at / 3600) * 3600) AS bucket,
			COALESCE(NULLIF(TRIM(username), ''), 'local') AS username,
			COALESCE(SUM(cost), 0) AS quota,
			COALESCE(SUM(total_tokens), 0) AS token_used,
			COALESCE(COUNT(*), 0) AS count
		FROM usage_logs
		WHERE created_at >= ? AND created_at <= ? AND COALESCE(NULLIF(TRIM(username), ''), 'local') = ?
		GROUP BY bucket, username
		ORDER BY bucket ASC`, start, end, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]QuotaData, 0)
	for rows.Next() {
		var item QuotaData
		var quota float64
		if err := rows.Scan(&item.CreatedAt, &item.Username, &quota, &item.TokenUsed, &item.Count); err != nil {
			return nil, err
		}
		item.UserID = 1
		item.ID = item.CreatedAt
		item.Quota = quotaFromCost(quota)
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) RepairUnknownQuotaDataGroups(ctx context.Context, start, end int64, username string) (RepairUnknownQuotaDataGroupsResult, error) {
	start, end = normalizeTimeRange(start, end)
	username = strings.TrimSpace(username)
	where := `WHERE created_at >= ? AND created_at <= ? AND (TRIM(COALESCE(group_name, '')) = '' OR lower(TRIM(group_name)) = 'unknown')`
	args := []any{start, end}
	if username != "" {
		where += ` AND COALESCE(NULLIF(TRIM(username), ''), 'local') = ?`
		args = append(args, username)
	}

	var result RepairUnknownQuotaDataGroupsResult
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM usage_logs `+where, args...).Scan(&result.TotalUnknown); err != nil {
		return result, err
	}
	if result.TotalUnknown == 0 {
		return result, nil
	}
	res, err := s.db.ExecContext(ctx, `UPDATE usage_logs SET group_name = 'default' `+where, args...)
	if err != nil {
		return result, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return result, err
	}
	result.Fixed = affected
	result.Skipped = result.TotalUnknown - affected
	if result.Skipped < 0 {
		result.Skipped = 0
	}
	return result, nil
}

func (s *Store) RecalculateUsageCosts(ctx context.Context, start, end int64, username string, onlyZero bool) (RecalculateUsageCostsResult, error) {
	start, end = normalizeTimeRange(start, end)
	username = strings.TrimSpace(username)
	settings, err := s.Settings(ctx)
	if err != nil {
		return RecalculateUsageCostsResult{}, err
	}

	where := `WHERE u.created_at >= ? AND u.created_at <= ? AND u.channel_id > 0
		AND (u.prompt_tokens <> 0 OR u.completion_tokens <> 0 OR u.total_tokens <> 0)`
	args := []any{start, end}
	if username != "" {
		where += ` AND COALESCE(NULLIF(TRIM(u.username), ''), 'local') = ?`
		args = append(args, username)
	}
	if onlyZero {
		where += ` AND COALESCE(u.cost, 0) <= 0`
	}

	rows, err := s.db.QueryContext(ctx, `SELECT
		u.id, u.model, u.prompt_tokens, u.completion_tokens,
		c.id, c.type, c.name, c.group_name, c.base_url, c.api_key, c.models, c.enabled,
		c.input_price_per_1k, c.output_price_per_1k, c.remark, c.created_at, c.updated_at,
		COALESCE(g.price_multiplier, 1)
		FROM usage_logs u
		JOIN channels c ON c.id = u.channel_id
		LEFT JOIN groups g ON g.name = CASE
			WHEN TRIM(COALESCE(u.group_name, '')) = '' OR lower(TRIM(COALESCE(u.group_name, ''))) = 'unknown' THEN c.group_name
			ELSE u.group_name
		END
		`+where+`
		ORDER BY u.id ASC`, args...)
	if err != nil {
		return RecalculateUsageCostsResult{}, err
	}
	defer rows.Close()

	type usageCostUpdate struct {
		ID   int64
		Cost float64
	}
	updates := make([]usageCostUpdate, 0)
	var result RecalculateUsageCostsResult
	for rows.Next() {
		var (
			id           int64
			model        string
			inputTokens  int64
			outputTokens int64
			channel      Channel
			enabled      int
			multiplier   float64
		)
		err := rows.Scan(
			&id,
			&model,
			&inputTokens,
			&outputTokens,
			&channel.ID,
			&channel.Type,
			&channel.Name,
			&channel.Group,
			&channel.BaseURL,
			&channel.APIKey,
			&channel.Models,
			&enabled,
			&channel.InputPricePer1K,
			&channel.OutputPricePer1K,
			&channel.Remark,
			&channel.CreatedAt,
			&channel.UpdatedAt,
			&multiplier,
		)
		if err != nil {
			return result, err
		}
		result.Total++
		channel.Enabled = enabled != 0
		cost := UsageCostForModel(channel, settings, multiplier, model, inputTokens, outputTokens)
		if cost <= 0 || math.IsNaN(cost) || math.IsInf(cost, 0) {
			result.Skipped++
			continue
		}
		updates = append(updates, usageCostUpdate{ID: id, Cost: cost})
	}
	if err := rows.Err(); err != nil {
		return result, err
	}
	if len(updates) == 0 {
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return result, err
	}
	defer func() {
		_ = tx.Rollback()
	}()
	stmt, err := tx.PrepareContext(ctx, `UPDATE usage_logs SET cost = ? WHERE id = ?`)
	if err != nil {
		return result, err
	}
	defer stmt.Close()
	for _, update := range updates {
		res, err := stmt.ExecContext(ctx, update.Cost, update.ID)
		if err != nil {
			return result, err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return result, err
		}
		if affected > 0 {
			result.Updated += affected
		} else {
			result.Skipped++
		}
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) PerfMetricsSummary(ctx context.Context, hours int) ([]PerfModelSummary, error) {
	if hours <= 0 {
		hours = 24
	}
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	rows, err := s.db.QueryContext(ctx, `
		SELECT COALESCE(NULLIF(TRIM(model), ''), 'unknown') AS model_name,
			COALESCE(AVG(CASE WHEN duration_ms > 0 THEN duration_ms END), 0) AS avg_latency_ms,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) AS success_rate,
			COALESCE(CASE WHEN SUM(duration_ms) > 0 THEN SUM(total_tokens) * 1000.0 / SUM(duration_ms) ELSE 0 END, 0) AS avg_tps,
			COUNT(*) AS request_count
		FROM usage_logs
		WHERE created_at >= ?
		GROUP BY model_name
		ORDER BY request_count DESC, model_name ASC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PerfModelSummary, 0)
	for rows.Next() {
		var item PerfModelSummary
		if err := rows.Scan(&item.ModelName, &item.AvgLatencyMS, &item.SuccessRate, &item.AvgTPS, &item.RequestCount); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) PerfMetrics(ctx context.Context, modelName string, hours int) (PerformanceMetrics, error) {
	if hours <= 0 {
		hours = 24
	}
	modelName = strings.TrimSpace(modelName)
	cutoff := time.Now().Add(-time.Duration(hours) * time.Hour).Unix()
	where := `WHERE created_at >= ?`
	args := []any{cutoff}
	if modelName != "" {
		where += ` AND COALESCE(NULLIF(TRIM(model), ''), 'unknown') = ?`
		args = append(args, modelName)
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT ((created_at / 3600) * 3600) AS bucket,
			COALESCE(NULLIF(TRIM(group_name), ''), 'unknown') AS group_name,
			COALESCE(AVG(CASE WHEN duration_ms > 0 THEN duration_ms END), 0) AS avg_latency_ms,
			COALESCE(SUM(CASE WHEN status_code >= 200 AND status_code < 400 THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 0) AS success_rate,
			COALESCE(CASE WHEN SUM(duration_ms) > 0 THEN SUM(total_tokens) * 1000.0 / SUM(duration_ms) ELSE 0 END, 0) AS avg_tps,
			COUNT(*) AS request_count
		FROM usage_logs
		`+where+`
		GROUP BY bucket, group_name
		ORDER BY bucket ASC, group_name ASC`, args...)
	if err != nil {
		return PerformanceMetrics{}, err
	}
	defer rows.Close()

	groupMap := map[string][]PerformanceSeriesPoint{}
	groupStats := map[string]PerformanceGroup{}
	for rows.Next() {
		var bucket int64
		var groupName string
		var avgLatencyMS, successRate, avgTPS float64
		var requestCount int64
		if err := rows.Scan(&bucket, &groupName, &avgLatencyMS, &successRate, &avgTPS, &requestCount); err != nil {
			return PerformanceMetrics{}, err
		}
		group := groupName
		point := PerformanceSeriesPoint{
			Timestamp:    bucket,
			AvgTTFTMS:    0,
			AvgLatencyMS: avgLatencyMS,
			SuccessRate:  successRate,
			AvgTPS:       avgTPS,
		}
		groupMap[group] = append(groupMap[group], point)
		stat := groupStats[group]
		stat.Group = group
		stat.AvgLatencyMS += avgLatencyMS
		stat.SuccessRate += successRate
		stat.AvgTPS += avgTPS
		stat.AvgTTFTMS += 0
		stat.Series = groupMap[group]
		groupStats[group] = stat
		_ = requestCount
	}
	if err := rows.Err(); err != nil {
		return PerformanceMetrics{}, err
	}

	groups := make([]PerformanceGroup, 0, len(groupStats))
	for _, group := range groupStats {
		points := group.Series
		var latencySum, tpsSum, successSum float64
		for _, point := range points {
			latencySum += point.AvgLatencyMS
			tpsSum += point.AvgTPS
			successSum += point.SuccessRate
		}
		count := float64(len(points))
		if count > 0 {
			group.AvgLatencyMS = latencySum / count
			group.AvgTPS = tpsSum / count
			group.SuccessRate = successSum / count
		}
		groups = append(groups, group)
	}

	if len(groups) == 0 {
		groups = []PerformanceGroup{}
	}
	return PerformanceMetrics{
		ModelName:    modelName,
		SeriesSchema: "hour",
		Groups:       groups,
	}, nil
}
