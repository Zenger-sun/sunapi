package sunapi

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var ErrAdminAlreadyInitialized = errors.New("admin user already initialized")

type AdminUser struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    int64
	UpdatedAt    int64
}

func (s *Store) AdminInitialized(ctx context.Context) (bool, error) {
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM admin_users WHERE id = 1`).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func (s *Store) CreateAdminUser(ctx context.Context, username string, passwordHash string) (AdminUser, error) {
	initialized, err := s.AdminInitialized(ctx)
	if err != nil {
		return AdminUser{}, err
	}
	if initialized {
		return AdminUser{}, ErrAdminAlreadyInitialized
	}

	now := time.Now().Unix()
	username = strings.TrimSpace(username)
	if username == "" {
		username = "admin"
	}
	if _, err := s.db.ExecContext(
		ctx,
		`INSERT INTO admin_users (id, username, password_hash, created_at, updated_at) VALUES (1, ?, ?, ?, ?)`,
		username,
		passwordHash,
		now,
		now,
	); err != nil {
		return AdminUser{}, err
	}
	return AdminUser{
		ID:           1,
		Username:     username,
		PasswordHash: passwordHash,
		CreatedAt:    now,
		UpdatedAt:    now,
	}, nil
}

func (s *Store) AdminUserByUsername(ctx context.Context, username string) (AdminUser, error) {
	var user AdminUser
	err := s.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, created_at, updated_at FROM admin_users WHERE username = ? LIMIT 1`,
		strings.TrimSpace(username),
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (s *Store) UpdateAdminPassword(ctx context.Context, userID int64, passwordHash string) (AdminUser, error) {
	now := time.Now().Unix()
	res, err := s.db.ExecContext(
		ctx,
		`UPDATE admin_users SET password_hash = ?, updated_at = ? WHERE id = ?`,
		passwordHash,
		now,
		userID,
	)
	if err != nil {
		return AdminUser{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return AdminUser{}, err
	}
	if affected == 0 {
		return AdminUser{}, sql.ErrNoRows
	}
	var user AdminUser
	err = s.db.QueryRowContext(
		ctx,
		`SELECT id, username, password_hash, created_at, updated_at FROM admin_users WHERE id = ? LIMIT 1`,
		userID,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	return user, err
}

func (s *Store) AdminUserBySession(ctx context.Context, token string) (AdminUser, bool, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return AdminUser{}, false, nil
	}

	var user AdminUser
	var expiresAt int64
	err := s.db.QueryRowContext(
		ctx,
		`SELECT u.id, u.username, u.password_hash, u.created_at, u.updated_at, s.expires_at
		 FROM admin_sessions s
		 JOIN admin_users u ON u.id = s.user_id
		 WHERE s.token = ?
		 LIMIT 1`,
		token,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AdminUser{}, false, nil
	}
	if err != nil {
		return AdminUser{}, false, err
	}
	if expiresAt <= time.Now().Unix() {
		_ = s.DeleteAdminSession(ctx, token)
		return AdminUser{}, false, nil
	}
	return user, true, nil
}

func (s *Store) CreateAdminSession(ctx context.Context, token string, userID int64, expiresAt int64) error {
	now := time.Now().Unix()
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO admin_sessions (token, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)`,
		token,
		userID,
		now,
		expiresAt,
	)
	return err
}

func (s *Store) DeleteAdminSession(ctx context.Context, token string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token = ?`, strings.TrimSpace(token))
	return err
}

func (s *Store) DeleteExpiredAdminSessions(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ?`, time.Now().Unix())
	return err
}
