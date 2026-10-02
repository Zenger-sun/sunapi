package sunapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	agentRunLeaseTTL              = 2 * time.Minute
	agentRunLeaseHeartbeat        = 30 * time.Second
	agentRunLeaseHeartbeatTimeout = 5 * time.Second
	agentRunLeaseHeartbeatRetries = 3
	agentRunPauseLeaseWait        = 15 * time.Second
	agentRunPauseLeasePoll        = 100 * time.Millisecond
)

// agentRunLease is the database-backed ownership record for one blueprint run.
// The in-memory registry remains useful for cancellation, but it is not an
// execution lock because another process may be serving the same database.
type agentRunLease struct {
	BlueprintID string
	RunID       string
	Kind        string
}

func newAgentRunID() string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UnixNano())
	}
	return "run-" + hex.EncodeToString(buffer)
}

func (s *Store) acquireAgentRunLease(ctx context.Context, blueprintID, runID, kind string) (bool, error) {
	if s == nil || s.db == nil {
		return false, errors.New("agent store is not initialized")
	}
	blueprintID = strings.TrimSpace(blueprintID)
	runID = strings.TrimSpace(runID)
	if blueprintID == "" || runID == "" {
		return false, errors.New("blueprint id and run id are required")
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	rollback := func() {
		_ = tx.Rollback()
	}
	defer rollback()
	if _, err := tx.ExecContext(ctx, `
		DELETE FROM agent_run_leases
		WHERE blueprint_id=? AND heartbeat_time < ?`, blueprintID, now-agentRunLeaseTTL.Milliseconds()); err != nil {
		return false, err
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO agent_run_leases (blueprint_id, run_id, kind, acquired_time, heartbeat_time)
		VALUES (?, ?, ?, ?, ?)`, blueprintID, runID, strings.TrimSpace(kind), now, now); err != nil {
		// A uniqueness conflict means another worker owns this blueprint.
		if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "constraint") {
			return false, nil
		}
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) acquireAgentRunLeaseAfterPause(ctx context.Context, blueprintID, runID, kind string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	deadline := time.NewTimer(agentRunPauseLeaseWait)
	defer deadline.Stop()
	poll := time.NewTicker(agentRunPauseLeasePoll)
	defer poll.Stop()

	for {
		bp, err := s.GetBlueprintByID(ctx, blueprintID)
		if err != nil {
			return false, err
		}
		if bp == nil || bp.Status != "paused" {
			return false, nil
		}
		acquired, err := s.acquireAgentRunLease(ctx, blueprintID, runID, kind)
		if err != nil || acquired {
			return acquired, err
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, nil
		case <-poll.C:
		}
	}
}

func (s *Store) renewAgentRunLease(ctx context.Context, lease agentRunLease) error {
	if s == nil || s.db == nil || lease.BlueprintID == "" || lease.RunID == "" {
		return errors.New("invalid agent run lease")
	}
	result, err := s.db.ExecContext(ctx, `
		UPDATE agent_run_leases SET heartbeat_time=?
		WHERE blueprint_id=? AND run_id=?`, time.Now().UnixMilli(), lease.BlueprintID, lease.RunID)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows != 1 {
		return errors.New("agent run lease is no longer owned")
	}
	return nil
}

func (s *Store) ownsAgentRunLease(ctx context.Context, lease agentRunLease) (bool, error) {
	if s == nil || s.db == nil || lease.BlueprintID == "" || lease.RunID == "" {
		return false, errors.New("invalid agent run lease")
	}
	var owned int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM agent_run_leases WHERE blueprint_id=? AND run_id=?`, lease.BlueprintID, lease.RunID).Scan(&owned)
	if err != nil {
		return false, err
	}
	return owned == 1, nil
}

func (s *Store) releaseAgentRunLease(ctx context.Context, lease agentRunLease) error {
	if s == nil || s.db == nil || lease.BlueprintID == "" || lease.RunID == "" {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM agent_run_leases WHERE blueprint_id=? AND run_id=?`, lease.BlueprintID, lease.RunID)
	return err
}

// updateAgentBlueprintAfterRunForLease prevents a run that lost ownership from
// overwriting the status of the worker that replaced it. The lease predicate is
// part of the UPDATE, so checking ownership and writing the status is atomic.
func (s *Store) updateAgentBlueprintAfterRunForLease(ctx context.Context, lease agentRunLease, executionStatus, failureReason string) (bool, error) {
	if s == nil || s.db == nil || lease.BlueprintID == "" || lease.RunID == "" {
		return false, errors.New("invalid agent run lease")
	}
	for attempt := 0; attempt < 3; attempt++ {
		bp, err := s.GetBlueprintByID(ctx, lease.BlueprintID)
		if err != nil {
			return false, err
		}
		if bp == nil {
			return false, errors.New("blueprint not found")
		}
		status := executionStatus
		if bp.Status == "paused" {
			status = "paused"
		} else if isRecurringAgentScheduledBlueprint(bp) {
			status = "running"
		}
		result, err := s.db.ExecContext(ctx, `
			UPDATE agent_blueprints
			SET status=?, failure_reason=?, updated_time=?
			WHERE id=? AND trigger_config=? AND status=?
			  AND EXISTS (SELECT 1 FROM agent_run_leases WHERE blueprint_id=? AND run_id=?)`,
			status, failureReason, time.Now().UnixMilli(), lease.BlueprintID, bp.TriggerConfig, bp.Status, lease.BlueprintID, lease.RunID)
		if err != nil {
			return false, err
		}
		updated, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if updated == 1 {
			return true, nil
		}

		var owned int
		if err := s.db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM agent_run_leases WHERE blueprint_id=? AND run_id=?`, lease.BlueprintID, lease.RunID).Scan(&owned); err != nil {
			return false, err
		}
		if owned == 0 {
			return false, nil
		}
	}
	return false, errors.New("blueprint changed while updating execution status")
}

func (s *Store) updateAgentRunMetricsForLease(ctx context.Context, lease agentRunLease, agentID string, now, durationMs int64) error {
	if s == nil || s.db == nil || lease.BlueprintID == "" || lease.RunID == "" {
		return errors.New("invalid agent run lease")
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE agent_blueprints
		SET execution_count = execution_count + 1,
		    last_executed_time = ?,
		    execution_duration_ms = ?,
		    updated_time = ?
		WHERE id=? AND EXISTS (SELECT 1 FROM agent_run_leases WHERE blueprint_id=? AND run_id=?)`,
		now, durationMs, now, lease.BlueprintID, lease.BlueprintID, lease.RunID)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
		UPDATE agents SET last_executed_time = ?, updated_time = ?
		WHERE id=? AND EXISTS (SELECT 1 FROM agent_run_leases WHERE blueprint_id=? AND run_id=?)`,
		now, now, agentID, lease.BlueprintID, lease.RunID)
	return err
}

func startAgentRunLeaseHeartbeat(ctx context.Context, store *Store, lease agentRunLease) (context.Context, func()) {
	if ctx == nil {
		ctx = context.Background()
	}
	if store == nil || lease.BlueprintID == "" || lease.RunID == "" {
		return ctx, func() {}
	}
	heartbeatCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(agentRunLeaseHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				err := renewAgentRunLeaseWithRetry(heartbeatCtx, store, lease)
				if err != nil {
					cancel()
					return
				}
			}
		}
	}()
	return heartbeatCtx, func() {
		cancel()
		<-done
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()
		if err := store.releaseAgentRunLease(cleanupCtx, lease); err != nil {
			// The execution result is already being flushed; lease cleanup is
			// best-effort and stale records are removed on the next acquisition.
			return
		}
	}
}

func renewAgentRunLeaseWithRetry(ctx context.Context, store *Store, lease agentRunLease) error {
	var lastErr error
	for attempt := 0; attempt < agentRunLeaseHeartbeatRetries; attempt++ {
		renewCtx, renewCancel := context.WithTimeout(ctx, agentRunLeaseHeartbeatTimeout)
		lastErr = store.renewAgentRunLease(renewCtx, lease)
		renewCancel()
		if lastErr == nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if attempt+1 < agentRunLeaseHeartbeatRetries {
			timer := time.NewTimer(250 * time.Millisecond)
			select {
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	return lastErr
}
