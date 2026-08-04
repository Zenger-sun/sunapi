package sunapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	_ "time/tzdata"

	log "github.com/sirupsen/logrus"
)

const agentSchedulerPollInterval = 5 * time.Second

type agentScheduleConfig struct {
	Type           string `json:"type"`
	Frequency      string `json:"frequency"`
	Time           string `json:"time"`
	Timezone       string `json:"timezone"`
	Weekdays       []int  `json:"weekdays"`
	DayOfMonth     int    `json:"dayOfMonth"`
	CronExpression string `json:"cronExpression"`
}

func (s *Store) startAgentScheduler() {
	ctx, cancel := context.WithCancel(context.Background())
	s.agentSchedulerCancel = cancel
	s.agentSchedulerWG.Add(1)
	go func() {
		defer s.agentSchedulerWG.Done()
		s.runAgentScheduler(ctx)
	}()
}

func (s *Store) runAgentScheduler(ctx context.Context) {
	if err := s.reconcileAgentSchedules(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
		log.WithError(err).Warn("agent scheduler: failed to initialize schedules")
	}

	ticker := time.NewTicker(agentSchedulerPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := s.dispatchDueAgentBlueprints(ctx, now); err != nil && !errors.Is(err, context.Canceled) {
				log.WithError(err).Warn("agent scheduler: dispatch failed")
			}
		}
	}
}

func (s *Store) reconcileAgentSchedules(ctx context.Context, now time.Time) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, user_id, name, description, status,
		       trigger_config, model_group, model_name, skills,
		       input_variables, archive_config, nodes, edges,
		       created_time, updated_time, last_executed_time, next_execution_time,
		       execution_count, failure_reason, execution_duration_ms
		FROM agent_blueprints
		WHERE status IN ('pending', 'running')`)
	if err != nil {
		return err
	}
	blueprints := make([]AgentBlueprint, 0)
	for rows.Next() {
		var bp AgentBlueprint
		if err := scanAgentBlueprint(rows, &bp); err != nil {
			_ = rows.Close()
			return err
		}
		blueprints = append(blueprints, bp)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range blueprints {
		bp := &blueprints[i]
		if _, ok := parseAgentScheduleConfig(bp); !ok {
			if bp.Status == "running" && !s.isAgentScheduledRunActive(bp.ID) {
				if err := s.UpdateBlueprintScheduleState(ctx, bp.ID, "pending", "", 0); err != nil {
					return err
				}
				continue
			}
			if bp.NextExecutionTime != 0 {
				if err := s.updateAgentNextExecutionTime(ctx, bp.ID, 0); err != nil {
					return err
				}
			}
			continue
		}
		if bp.Status == "pending" {
			bp.Status = "running"
		}
		if bp.NextExecutionTime <= now.UnixMilli() {
			if err := s.refreshAgentNextExecutionTime(ctx, bp, now); err != nil {
				log.WithError(err).WithField("blueprint_id", bp.ID).
					Warn("agent scheduler: invalid schedule")
				continue
			}
		}
		if err := s.UpdateBlueprintScheduleState(ctx, bp.ID, "running", bp.FailureReason, bp.NextExecutionTime); err != nil {
			return err
		}
	}
	return nil
}

type agentBlueprintScanner interface {
	Scan(dest ...any) error
}

func scanAgentBlueprint(scanner agentBlueprintScanner, bp *AgentBlueprint) error {
	return scanner.Scan(
		&bp.ID, &bp.AgentID, &bp.UserID, &bp.Name, &bp.Description, &bp.Status,
		&bp.TriggerConfig, &bp.ModelGroup, &bp.ModelName, &bp.Skills,
		&bp.InputVariables, &bp.ArchiveConfig, &bp.Nodes, &bp.Edges,
		&bp.CreatedTime, &bp.UpdatedTime, &bp.LastExecutedTime, &bp.NextExecutionTime,
		&bp.ExecutionCount, &bp.FailureReason, &bp.ExecutionDurationMs,
	)
}

func (s *Store) dispatchDueAgentBlueprints(ctx context.Context, now time.Time) error {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, agent_id, user_id, name, description, status,
		       trigger_config, model_group, model_name, skills,
		       input_variables, archive_config, nodes, edges,
		       created_time, updated_time, last_executed_time, next_execution_time,
		       execution_count, failure_reason, execution_duration_ms
		FROM agent_blueprints
		WHERE status='running' AND next_execution_time > 0 AND next_execution_time <= ?
		ORDER BY next_execution_time, id`, now.UnixMilli())
	if err != nil {
		return err
	}
	due := make([]AgentBlueprint, 0)
	for rows.Next() {
		var bp AgentBlueprint
		if err := scanAgentBlueprint(rows, &bp); err != nil {
			_ = rows.Close()
			return err
		}
		due = append(due, bp)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for i := range due {
		bp := due[i]
		if !s.beginAgentScheduledRun(bp.ID) {
			continue
		}
		claimed, err := s.claimAgentScheduledRun(ctx, &bp, now)
		if err != nil {
			s.endAgentScheduledRun(bp.ID)
			return err
		}
		if !claimed {
			s.endAgentScheduledRun(bp.ID)
			continue
		}

		runBP := buildAgentTriggeredBlueprint(&bp, "schedule")
		s.agentSchedulerWG.Add(1)
		go func() {
			defer s.agentSchedulerWG.Done()
			defer s.endAgentScheduledRun(bp.ID)
			runBlueprintTaskWithContext(ctx, s, runBP)
		}()
	}
	return nil
}

func (s *Store) claimAgentScheduledRun(ctx context.Context, bp *AgentBlueprint, now time.Time) (bool, error) {
	config, ok := parseAgentScheduleConfig(bp)
	if !ok {
		return false, nil
	}

	nextMillis := int64(0)
	if config.Frequency != "once" {
		next, err := nextAgentScheduleTime(config, now, time.UnixMilli(bp.CreatedTime))
		if err != nil {
			return false, err
		}
		nextMillis = next.UnixMilli()
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_blueprints
		SET next_execution_time=?, updated_time=?
		WHERE id=? AND status='running' AND next_execution_time=?`,
		nextMillis, now.UnixMilli(), bp.ID, bp.NextExecutionTime)
	if err != nil {
		return false, err
	}
	rowsAffected, err := result.RowsAffected()
	return rowsAffected == 1, err
}

func (s *Store) beginAgentScheduledRun(blueprintID string) bool {
	s.agentScheduledRunsMu.Lock()
	defer s.agentScheduledRunsMu.Unlock()
	if _, exists := s.agentScheduledRuns[blueprintID]; exists {
		return false
	}
	s.agentScheduledRuns[blueprintID] = struct{}{}
	return true
}

func (s *Store) endAgentScheduledRun(blueprintID string) {
	s.agentScheduledRunsMu.Lock()
	delete(s.agentScheduledRuns, blueprintID)
	s.agentScheduledRunsMu.Unlock()
}

func (s *Store) isAgentScheduledRunActive(blueprintID string) bool {
	s.agentScheduledRunsMu.Lock()
	defer s.agentScheduledRunsMu.Unlock()
	_, exists := s.agentScheduledRuns[blueprintID]
	return exists
}

func (s *Store) refreshAgentNextExecutionTime(ctx context.Context, bp *AgentBlueprint, after time.Time) error {
	config, ok := parseAgentScheduleConfig(bp)
	if !ok {
		bp.NextExecutionTime = 0
		return s.updateAgentNextExecutionTime(ctx, bp.ID, 0)
	}
	next, err := nextAgentScheduleTime(config, after, time.UnixMilli(bp.CreatedTime))
	if err != nil {
		return err
	}
	bp.NextExecutionTime = next.UnixMilli()
	return s.updateAgentNextExecutionTime(ctx, bp.ID, bp.NextExecutionTime)
}

func nextAgentScheduleTimeFromBlueprint(bp *AgentBlueprint, after time.Time) (int64, error) {
	config, ok := parseAgentScheduleConfig(bp)
	if !ok {
		return 0, nil
	}
	next, err := nextAgentScheduleTime(config, after, time.UnixMilli(bp.CreatedTime))
	if err != nil {
		return 0, err
	}
	return next.UnixMilli(), nil
}

func (s *Store) updateAgentNextExecutionTime(ctx context.Context, blueprintID string, next int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE agent_blueprints SET next_execution_time=?, updated_time=? WHERE id=?`,
		next, time.Now().UnixMilli(), blueprintID)
	return err
}

func parseAgentScheduleConfig(bp *AgentBlueprint) (agentScheduleConfig, bool) {
	if bp == nil || len(bp.TriggerConfig) == 0 {
		return agentScheduleConfig{}, false
	}
	var config agentScheduleConfig
	if err := json.Unmarshal(bp.TriggerConfig, &config); err != nil {
		return agentScheduleConfig{}, false
	}
	if strings.ToLower(strings.TrimSpace(config.Type)) != "scheduled" {
		return agentScheduleConfig{}, false
	}
	config.Frequency = strings.ToLower(strings.TrimSpace(config.Frequency))
	if config.Frequency == "" {
		config.Frequency = "daily"
	}
	if strings.TrimSpace(config.Timezone) == "" {
		config.Timezone = "Asia/Shanghai"
	}
	return config, true
}

func isAgentScheduledBlueprint(bp *AgentBlueprint) bool {
	_, ok := parseAgentScheduleConfig(bp)
	return ok
}

func isRecurringAgentScheduledBlueprint(bp *AgentBlueprint) bool {
	config, ok := parseAgentScheduleConfig(bp)
	return ok && config.Frequency != "once"
}

func nextAgentScheduleTime(config agentScheduleConfig, after, anchor time.Time) (time.Time, error) {
	location, err := time.LoadLocation(config.Timezone)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid schedule timezone %q: %w", config.Timezone, err)
	}
	after = after.In(location)
	anchor = anchor.In(location)

	hour, minute, err := parseAgentScheduleClock(config.Time)
	if err != nil && config.Frequency != "hourly" {
		return time.Time{}, err
	}

	switch config.Frequency {
	case "hourly":
		return after.Truncate(time.Hour).Add(time.Hour), nil
	case "weekdays":
		for dayOffset := 0; dayOffset <= 7; dayOffset++ {
			day := after.AddDate(0, 0, dayOffset)
			if day.Weekday() == time.Saturday || day.Weekday() == time.Sunday {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, location)
			if candidate.After(after) {
				return candidate, nil
			}
		}
	case "daily", "once":
		candidate := time.Date(after.Year(), after.Month(), after.Day(), hour, minute, 0, 0, location)
		if !candidate.After(after) {
			candidate = candidate.AddDate(0, 0, 1)
		}
		return candidate, nil
	case "weekly":
		weekdays := normalizeAgentScheduleWeekdays(config.Weekdays, int(anchor.Weekday()))
		for dayOffset := 0; dayOffset <= 7; dayOffset++ {
			day := after.AddDate(0, 0, dayOffset)
			if _, ok := weekdays[int(day.Weekday())]; !ok {
				continue
			}
			candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, location)
			if candidate.After(after) {
				return candidate, nil
			}
		}
	case "monthly":
		dayOfMonth := config.DayOfMonth
		if dayOfMonth == 0 {
			dayOfMonth = anchor.Day()
		}
		for monthOffset := 0; monthOffset <= 12; monthOffset++ {
			monthStart := time.Date(after.Year(), after.Month(), 1, 0, 0, 0, 0, location).AddDate(0, monthOffset, 0)
			day := dayOfMonth
			lastDay := time.Date(monthStart.Year(), monthStart.Month()+1, 0, 0, 0, 0, 0, location).Day()
			if day >= 31 || day > lastDay {
				day = lastDay
			}
			candidate := time.Date(monthStart.Year(), monthStart.Month(), day, hour, minute, 0, 0, location)
			if candidate.After(after) {
				return candidate, nil
			}
		}
	case "custom":
		return time.Time{}, errors.New("custom cron schedules are not supported yet")
	default:
		return time.Time{}, fmt.Errorf("unsupported schedule frequency %q", config.Frequency)
	}
	return time.Time{}, errors.New("unable to calculate the next schedule time")
}

func parseAgentScheduleClock(value string) (int, int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		value = "08:30"
	}
	var hour, minute int
	if _, err := fmt.Sscanf(value, "%d:%d", &hour, &minute); err != nil || hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, 0, fmt.Errorf("invalid schedule time %q", value)
	}
	return hour, minute, nil
}

func normalizeAgentScheduleWeekdays(values []int, fallback int) map[int]struct{} {
	if len(values) == 0 {
		values = []int{fallback}
	}
	sort.Ints(values)
	out := make(map[int]struct{}, len(values))
	for _, value := range values {
		if value >= 0 && value <= 6 {
			out[value] = struct{}{}
		}
	}
	if len(out) == 0 {
		out[fallback] = struct{}{}
	}
	return out
}

func buildAgentTriggeredBlueprint(bp *AgentBlueprint, triggerKind string) *AgentBlueprint {
	if bp == nil {
		return nil
	}
	copyBP := *bp
	nodes, err := parseAgentBlueprintNodes(bp.Nodes)
	if err != nil || len(nodes) == 0 {
		return &copyBP
	}
	edges, err := parseAgentBlueprintEdges(bp.Edges)
	if err != nil {
		return &copyBP
	}

	nodeByID := make(map[string]agentBlueprintGraphNode, len(nodes))
	outgoing := make(map[string][]string)
	queue := make([]string, 0)
	for _, node := range nodes {
		nodeByID[node.ID] = node
		if node.Data.Kind == triggerKind {
			queue = append(queue, node.ID)
		}
	}
	if len(queue) == 0 {
		return &copyBP
	}
	for _, edge := range edges {
		outgoing[edge.Source] = append(outgoing[edge.Source], edge.Target)
	}

	reachable := make(map[string]struct{}, len(nodes))
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, exists := reachable[id]; exists {
			continue
		}
		node, exists := nodeByID[id]
		if !exists || (node.Data.Kind == "start" && triggerKind != "start") {
			continue
		}
		reachable[id] = struct{}{}
		queue = append(queue, outgoing[id]...)
	}

	filteredNodes := make([]agentBlueprintGraphNode, 0, len(reachable))
	for _, node := range nodes {
		if _, ok := reachable[node.ID]; ok {
			filteredNodes = append(filteredNodes, node)
		}
	}
	filteredEdges := make([]agentBlueprintGraphEdge, 0, len(edges))
	for _, edge := range edges {
		_, sourceOK := reachable[edge.Source]
		_, targetOK := reachable[edge.Target]
		if sourceOK && targetOK {
			filteredEdges = append(filteredEdges, edge)
		}
	}
	copyBP.Nodes, _ = json.Marshal(filteredNodes)
	copyBP.Edges, _ = json.Marshal(filteredEdges)
	return &copyBP
}
