package sunapi

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// ── 工具函数 ────────────────────────────────────────────────────────────────

func newAgentID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "ag-" + hex.EncodeToString(b)
}

func newBlueprintID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "bp-" + hex.EncodeToString(b)
}

func newSkillID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "sk-" + hex.EncodeToString(b)
}

func newComponentID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "cmp-" + hex.EncodeToString(b)
}

func newExecLogID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "ex-" + hex.EncodeToString(b)
}

// ── Agent CRUD ───────────────────────────────────────────────────────────────

func (s *Store) ListAgents(ctx context.Context, userID int64) ([]Agent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, avatar, status,
		       created_time, updated_time, last_executed_time, blueprint_count
		FROM agents
		WHERE user_id = ?
		ORDER BY updated_time DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Agent
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.UserID, &a.Name, &a.Description, &a.Avatar,
			&a.Status, &a.CreatedTime, &a.UpdatedTime, &a.LastExecutedTime, &a.BlueprintCount); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	if out == nil {
		out = []Agent{}
	}
	return out, rows.Err()
}

func (s *Store) GetAgent(ctx context.Context, id string, userID int64) (*Agent, error) {
	var a Agent
	err := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, name, description, avatar, status,
		       created_time, updated_time, last_executed_time, blueprint_count
		FROM agents WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&a.ID, &a.UserID, &a.Name, &a.Description, &a.Avatar,
			&a.Status, &a.CreatedTime, &a.UpdatedTime, &a.LastExecutedTime, &a.BlueprintCount)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (s *Store) CreateAgent(ctx context.Context, userID int64, f AgentFormData) (*Agent, error) {
	if strings.TrimSpace(f.Name) == "" {
		return nil, errors.New("agent name is required")
	}
	now := time.Now().UnixMilli()
	a := &Agent{
		ID:          newAgentID(),
		UserID:      userID,
		Name:        strings.TrimSpace(f.Name),
		Description: strings.TrimSpace(f.Description),
		Avatar:      strings.TrimSpace(f.Avatar),
		Status:      1,
		CreatedTime: now,
		UpdatedTime: now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agents (id, user_id, name, description, avatar, status,
		                    created_time, updated_time, last_executed_time, blueprint_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0, 0)`,
		a.ID, a.UserID, a.Name, a.Description, a.Avatar, a.Status, a.CreatedTime, a.UpdatedTime)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) UpdateAgent(ctx context.Context, id string, userID int64, f AgentFormData) (*Agent, error) {
	a, err := s.GetAgent(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if a == nil {
		return nil, errors.New("agent not found")
	}
	if strings.TrimSpace(f.Name) != "" {
		a.Name = strings.TrimSpace(f.Name)
	}
	a.Description = strings.TrimSpace(f.Description)
	if strings.TrimSpace(f.Avatar) != "" {
		a.Avatar = strings.TrimSpace(f.Avatar)
	}
	a.UpdatedTime = time.Now().UnixMilli()
	_, err = s.db.ExecContext(ctx, `
		UPDATE agents SET name=?, description=?, avatar=?, updated_time=?
		WHERE id=? AND user_id=?`,
		a.Name, a.Description, a.Avatar, a.UpdatedTime, id, userID)
	if err != nil {
		return nil, err
	}
	return a, nil
}

func (s *Store) DeleteAgent(ctx context.Context, id string, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM agents WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	// 同步删除所属蓝图
	_, _ = s.db.ExecContext(ctx, `DELETE FROM agent_blueprints WHERE agent_id=? AND user_id=?`, id, userID)
	return nil
}

// ── Blueprint CRUD ────────────────────────────────────────────────────────────

func (s *Store) ListBlueprints(ctx context.Context, agentID string, userID int64) ([]AgentBlueprint, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, user_id, name, description, status,
		       trigger_config, model_group, model_name, skills,
		       input_variables, archive_config,
		       created_time, updated_time, last_executed_time, next_execution_time,
		       execution_count, failure_reason, execution_duration_ms
		FROM agent_blueprints
		WHERE agent_id = ? AND user_id = ?
		ORDER BY updated_time DESC`, agentID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentBlueprint
	for rows.Next() {
		var b AgentBlueprint
		if err := rows.Scan(
			&b.ID, &b.AgentID, &b.UserID, &b.Name, &b.Description, &b.Status,
			&b.TriggerConfig, &b.ModelGroup, &b.ModelName, &b.Skills,
			&b.InputVariables, &b.ArchiveConfig,
			&b.CreatedTime, &b.UpdatedTime, &b.LastExecutedTime, &b.NextExecutionTime,
			&b.ExecutionCount, &b.FailureReason, &b.ExecutionDurationMs,
		); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	if out == nil {
		out = []AgentBlueprint{}
	}
	return out, rows.Err()
}

func (s *Store) GetBlueprint(ctx context.Context, id string, userID int64) (*AgentBlueprint, error) {
	var b AgentBlueprint
	err := s.db.QueryRowContext(ctx, `
		SELECT id, agent_id, user_id, name, description, status,
		       trigger_config, model_group, model_name, skills,
		       input_variables, archive_config,
		       created_time, updated_time, last_executed_time, next_execution_time,
		       execution_count, failure_reason, execution_duration_ms
		FROM agent_blueprints WHERE id=? AND user_id=?`, id, userID).
		Scan(&b.ID, &b.AgentID, &b.UserID, &b.Name, &b.Description, &b.Status,
			&b.TriggerConfig, &b.ModelGroup, &b.ModelName, &b.Skills,
			&b.InputVariables, &b.ArchiveConfig,
			&b.CreatedTime, &b.UpdatedTime, &b.LastExecutedTime, &b.NextExecutionTime,
			&b.ExecutionCount, &b.FailureReason, &b.ExecutionDurationMs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &b, nil
}

func (s *Store) CreateBlueprint(ctx context.Context, agentID string, userID int64, f BlueprintFormData) (*AgentBlueprint, error) {
	if strings.TrimSpace(f.Name) == "" {
		return nil, errors.New("blueprint name is required")
	}
	if strings.TrimSpace(f.ModelName) == "" {
		return nil, errors.New("model name is required")
	}
	now := time.Now().UnixMilli()

	rawTrigger := f.TriggerConfig
	if len(rawTrigger) == 0 || string(rawTrigger) == "null" {
		rawTrigger = json.RawMessage(`{}`)
	}
	rawSkills := f.Skills
	if len(rawSkills) == 0 || string(rawSkills) == "null" {
		rawSkills = json.RawMessage(`[]`)
	}
	rawVars := f.InputVariables
	if len(rawVars) == 0 || string(rawVars) == "null" {
		rawVars = json.RawMessage(`{}`)
	}
	rawArchive := f.ArchiveConfig
	if len(rawArchive) == 0 || string(rawArchive) == "null" {
		rawArchive = json.RawMessage(`{}`)
	}

	b := &AgentBlueprint{
		ID:             newBlueprintID(),
		AgentID:        agentID,
		UserID:         userID,
		Name:           strings.TrimSpace(f.Name),
		Description:    strings.TrimSpace(f.Description),
		Status:         "pending",
		TriggerConfig:  rawTrigger,
		ModelGroup:     strings.TrimSpace(f.ModelGroup),
		ModelName:      strings.TrimSpace(f.ModelName),
		Skills:         rawSkills,
		InputVariables: rawVars,
		ArchiveConfig:  rawArchive,
		CreatedTime:    now,
		UpdatedTime:    now,
	}

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_blueprints
		(id, agent_id, user_id, name, description, status,
		 trigger_config, model_group, model_name, skills,
		 input_variables, archive_config,
		 created_time, updated_time,
		 last_executed_time, next_execution_time,
		 execution_count, failure_reason, execution_duration_ms)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,0,0,0,'',0)`,
		b.ID, b.AgentID, b.UserID, b.Name, b.Description, b.Status,
		b.TriggerConfig, b.ModelGroup, b.ModelName, b.Skills,
		b.InputVariables, b.ArchiveConfig,
		b.CreatedTime, b.UpdatedTime)
	if err != nil {
		return nil, err
	}

	// 更新智能体蓝图计数
	_, _ = s.db.ExecContext(ctx,
		`UPDATE agents SET blueprint_count = blueprint_count + 1, updated_time=? WHERE id=? AND user_id=?`,
		now, agentID, userID)

	return b, nil
}

func (s *Store) UpdateBlueprint(ctx context.Context, id string, userID int64, f BlueprintFormData) (*AgentBlueprint, error) {
	b, err := s.GetBlueprint(ctx, id, userID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, errors.New("blueprint not found")
	}
	now := time.Now().UnixMilli()
	if strings.TrimSpace(f.Name) != "" {
		b.Name = strings.TrimSpace(f.Name)
	}
	b.Description = strings.TrimSpace(f.Description)
	if strings.TrimSpace(f.ModelName) != "" {
		b.ModelName = strings.TrimSpace(f.ModelName)
	}
	if strings.TrimSpace(f.ModelGroup) != "" {
		b.ModelGroup = strings.TrimSpace(f.ModelGroup)
	}
	if len(f.TriggerConfig) > 0 && string(f.TriggerConfig) != "null" {
		b.TriggerConfig = f.TriggerConfig
	}
	if len(f.Skills) > 0 && string(f.Skills) != "null" {
		b.Skills = f.Skills
	}
	if len(f.InputVariables) > 0 && string(f.InputVariables) != "null" {
		b.InputVariables = f.InputVariables
	}
	if len(f.ArchiveConfig) > 0 && string(f.ArchiveConfig) != "null" {
		b.ArchiveConfig = f.ArchiveConfig
	}
	b.UpdatedTime = now

	_, err = s.db.ExecContext(ctx, `
		UPDATE agent_blueprints
		SET name=?, description=?, trigger_config=?, model_group=?, model_name=?,
		    skills=?, input_variables=?, archive_config=?, updated_time=?
		WHERE id=? AND user_id=?`,
		b.Name, b.Description, b.TriggerConfig, b.ModelGroup, b.ModelName,
		b.Skills, b.InputVariables, b.ArchiveConfig, b.UpdatedTime, id, userID)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (s *Store) UpdateBlueprintStatus(ctx context.Context, id string, status string, failureReason string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_blueprints SET status=?, failure_reason=?, updated_time=? WHERE id=?`,
		status, failureReason, time.Now().UnixMilli(), id)
	return err
}

func (s *Store) DeleteBlueprint(ctx context.Context, id string, userID int64) error {
	b, err := s.GetBlueprint(ctx, id, userID)
	if err != nil {
		return err
	}
	if b == nil {
		return nil
	}
	_, err = s.db.ExecContext(ctx, `DELETE FROM agent_blueprints WHERE id=? AND user_id=?`, id, userID)
	if err != nil {
		return err
	}
	// 更新智能体蓝图计数
	_, _ = s.db.ExecContext(ctx, `
		UPDATE agents SET blueprint_count = MAX(0, blueprint_count - 1), updated_time=?
		WHERE id=? AND user_id=?`,
		time.Now().UnixMilli(), b.AgentID, userID)
	return nil
}

// ── Skill CRUD ────────────────────────────────────────────────────────────────

func (s *Store) ListSkills(ctx context.Context, userID int64) ([]AgentSkill, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, version, source, source_url,
		       enabled, config, permissions, created_time, updated_time
		FROM agent_skills
		WHERE user_id = ?
		ORDER BY source, created_time DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentSkill
	for rows.Next() {
		var sk AgentSkill
		var enabled int
		if err := rows.Scan(&sk.ID, &sk.UserID, &sk.Name, &sk.Description,
			&sk.Version, &sk.Source, &sk.SourceURL,
			&enabled, &sk.Config, &sk.Permissions,
			&sk.CreatedTime, &sk.UpdatedTime); err != nil {
			return nil, err
		}
		sk.Enabled = enabled == 1
		out = append(out, sk)
	}
	if out == nil {
		out = []AgentSkill{}
	}
	return out, rows.Err()
}

func (s *Store) CreateSkill(ctx context.Context, userID int64, f SkillFormData, config json.RawMessage, permissions json.RawMessage, version string) (*AgentSkill, error) {
	if strings.TrimSpace(f.Name) == "" {
		return nil, errors.New("skill name is required")
	}
	now := time.Now().UnixMilli()
	if len(config) == 0 || string(config) == "null" {
		config = f.Config
	}
	if len(config) == 0 || string(config) == "null" {
		config = json.RawMessage(`{}`)
	}
	if len(permissions) == 0 || string(permissions) == "null" {
		permissions = json.RawMessage(`[]`)
	}
	if strings.TrimSpace(version) == "" {
		version = "1.0.0"
	}
	installKeySource := strings.TrimSpace(f.SourceURL)
	if f.SourceType == "marketplace" {
		var cfg map[string]any
		if err := json.Unmarshal(config, &cfg); err == nil {
			if marketplaceID, ok := cfg["marketplace_id"].(string); ok && strings.TrimSpace(marketplaceID) != "" {
				installKeySource = marketplaceID
			}
		}
	}
	sk := &AgentSkill{
		ID:          stableSkillInstallKey(f.SourceType, installKeySource, strings.TrimSpace(f.Name)),
		UserID:      userID,
		Name:        strings.TrimSpace(f.Name),
		Description: strings.TrimSpace(f.Description),
		Version:     strings.TrimSpace(version),
		Source:      f.SourceType,
		SourceURL:   strings.TrimSpace(f.SourceURL),
		Enabled:     true,
		Config:      config,
		Permissions: permissions,
		CreatedTime: now,
		UpdatedTime: now,
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_skills
		(id, user_id, name, description, version, source, source_url, enabled, config, permissions, created_time, updated_time)
		VALUES (?,?,?,?,?,?,?,1,?,?,?,?)`,
		sk.ID, sk.UserID, sk.Name, sk.Description, sk.Version, sk.Source, sk.SourceURL,
		sk.Config, sk.Permissions, sk.CreatedTime, sk.UpdatedTime)
	if err == nil {
		return sk, nil
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE agent_skills
		SET name=?, description=?, version=?, source=?, source_url=?, enabled=1,
		    config=?, permissions=?, updated_time=?
		WHERE id=? AND user_id=?`,
		sk.Name, sk.Description, sk.Version, sk.Source, sk.SourceURL,
		sk.Config, sk.Permissions, sk.UpdatedTime, sk.ID, sk.UserID)
	if err != nil {
		return nil, err
	}
	var createdTime int64
	_ = s.db.QueryRowContext(ctx,
		`SELECT created_time FROM agent_skills WHERE id=? AND user_id=?`,
		sk.ID, sk.UserID).Scan(&createdTime)
	if createdTime > 0 {
		sk.CreatedTime = createdTime
	}
	return sk, nil
}

func (s *Store) ToggleSkill(ctx context.Context, id string, userID int64, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_skills SET enabled=?, updated_time=? WHERE id=? AND user_id=?`,
		v, time.Now().UnixMilli(), id, userID)
	return err
}

func (s *Store) DeleteSkill(ctx context.Context, id string, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM agent_skills WHERE id=? AND user_id=?`, id, userID)
	return err
}

// ── Component CRUD ───────────────────────────────────────────────────────────

func (s *Store) ListComponents(ctx context.Context, userID int64) ([]AgentComponent, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, name, description, kind, group_name, source,
		       enabled, config, created_time, updated_time
		FROM agent_components
		WHERE user_id = ?
		ORDER BY group_name, created_time DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]AgentComponent, 0)
	for rows.Next() {
		var component AgentComponent
		var enabled int
		if err := rows.Scan(&component.ID, &component.UserID, &component.Name,
			&component.Description, &component.Kind, &component.Group,
			&component.Source, &enabled, &component.Config,
			&component.CreatedTime, &component.UpdatedTime); err != nil {
			return nil, err
		}
		component.Enabled = enabled == 1
		out = append(out, component)
	}
	return out, rows.Err()
}

func (s *Store) CreateComponent(ctx context.Context, userID int64, f ComponentFormData, config json.RawMessage) (*AgentComponent, error) {
	if strings.TrimSpace(f.Name) == "" {
		return nil, errors.New("component name is required")
	}
	if len(config) == 0 || string(config) == "null" {
		config = json.RawMessage(`{}`)
	}
	now := time.Now().UnixMilli()
	component := &AgentComponent{
		ID:          newComponentID(),
		UserID:      userID,
		Name:        strings.TrimSpace(f.Name),
		Description: strings.TrimSpace(f.Description),
		Kind:        "custom",
		Group:       normalizeComponentGroup(f.Group),
		Source:      "custom",
		Enabled:     true,
		Config:      config,
		CreatedTime: now,
		UpdatedTime: now,
	}
	if component.Group == "" {
		component.Group = "process"
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_components
		(id, user_id, name, description, kind, group_name, source, enabled, config, created_time, updated_time)
		VALUES (?,?,?,?,?,?,?,1,?,?,?)`,
		component.ID, component.UserID, component.Name, component.Description,
		component.Kind, component.Group, component.Source, component.Config,
		component.CreatedTime, component.UpdatedTime)
	if err != nil {
		return nil, err
	}
	return component, nil
}

func (s *Store) DeleteComponent(ctx context.Context, id string, userID int64) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM agent_components WHERE id=? AND user_id=? AND source='custom'`, id, userID)
	return err
}

// ── 执行日志 ──────────────────────────────────────────────────────────────────

func (s *Store) CreateExecutionLog(ctx context.Context, blueprintID, agentID string, userID int64) (*AgentExecutionLog, error) {
	now := time.Now().UnixMilli()
	log := &AgentExecutionLog{
		ID:          newExecLogID(),
		BlueprintID: blueprintID,
		AgentID:     agentID,
		UserID:      userID,
		Status:      "running",
		StartTime:   now,
		Output:      json.RawMessage(`{}`),
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO agent_execution_logs
		(id, blueprint_id, agent_id, user_id, status, start_time, end_time, duration_ms, output, error_msg, archive_path)
		VALUES (?,?,?,?,?,?,0,0,'{}','','')`,
		log.ID, log.BlueprintID, log.AgentID, log.UserID, log.Status, log.StartTime)
	if err != nil {
		return nil, err
	}
	return log, nil
}

func (s *Store) FinishExecutionLog(ctx context.Context, id string, status, errorMsg, archivePath string, output json.RawMessage) error {
	now := time.Now().UnixMilli()
	var startTime int64
	_ = s.db.QueryRowContext(ctx, `SELECT start_time FROM agent_execution_logs WHERE id=?`, id).Scan(&startTime)
	durationMs := int64(0)
	if startTime > 0 {
		durationMs = now - startTime
	}
	if len(output) == 0 || string(output) == "null" {
		output = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_execution_logs
		SET status=?, end_time=?, duration_ms=?, output=?, error_msg=?, archive_path=?
		WHERE id=?`,
		status, now, durationMs, output, errorMsg, archivePath, id)
	return err
}

func (s *Store) ListExecutionLogs(ctx context.Context, blueprintID string, userID int64, limit int) ([]AgentExecutionLog, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, blueprint_id, agent_id, user_id, status, start_time, end_time, duration_ms, output, error_msg, archive_path
		FROM agent_execution_logs
		WHERE blueprint_id=? AND user_id=?
		ORDER BY start_time DESC LIMIT ?`, blueprintID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AgentExecutionLog
	for rows.Next() {
		var l AgentExecutionLog
		if err := rows.Scan(&l.ID, &l.BlueprintID, &l.AgentID, &l.UserID,
			&l.Status, &l.StartTime, &l.EndTime, &l.DurationMs,
			&l.Output, &l.ErrorMsg, &l.ArchivePath); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if out == nil {
		out = []AgentExecutionLog{}
	}
	return out, rows.Err()
}

func (s *Store) ListExecutionLogsPage(ctx context.Context, userID, start, end int64, filter string, page, pageSize int) ([]AgentExecutionLog, int64, error) {
	if userID <= 0 {
		userID = 1
	}
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 500 {
		pageSize = 20
	}
	clauses := []string{"user_id=?", "start_time>=?", "start_time<=?"}
	args := []any{userID, start * 1000, end * 1000}
	if filter = strings.TrimSpace(filter); filter != "" {
		clauses = append(clauses, "(id LIKE ? OR blueprint_id LIKE ? OR agent_id LIKE ?)")
		like := "%" + filter + "%"
		args = append(args, like, like, like)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")

	var total int64
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_execution_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, pageSize, (page-1)*pageSize)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, blueprint_id, agent_id, user_id, status, start_time, end_time, duration_ms, output, error_msg, archive_path
		FROM agent_execution_logs`+where+`
		ORDER BY start_time DESC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]AgentExecutionLog, 0)
	for rows.Next() {
		var l AgentExecutionLog
		if err := rows.Scan(&l.ID, &l.BlueprintID, &l.AgentID, &l.UserID,
			&l.Status, &l.StartTime, &l.EndTime, &l.DurationMs,
			&l.Output, &l.ErrorMsg, &l.ArchivePath); err != nil {
			return nil, 0, err
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}
