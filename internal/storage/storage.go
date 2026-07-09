package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type User struct {
	ID           int64
	Email        string
	Name         string
	PasswordHash string
	CreatedAt    int64
}

type Session struct {
	IDHash    string
	UserID    sql.NullInt64
	CSRFToken string
	CreatedAt int64
	LastSeen  int64
	ExpiresAt int64
}

func Open(ctx context.Context, dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	store := &Store{db: db}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (store *Store) Close() error {
	return store.db.Close()
}

func (store *Store) init(ctx context.Context) error {
	if _, err := store.db.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if _, err := store.db.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return err
	}

	schema := `
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  email TEXT NOT NULL UNIQUE COLLATE NOCASE,
  name TEXT NOT NULL,
  password_hash TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS sessions (
  id_hash TEXT PRIMARY KEY,
  user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
  csrf_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`
	if _, err := store.db.ExecContext(ctx, schema); err != nil {
		return err
	}

	_, err := store.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= unixepoch()")
	return err
}

func (store *Store) CreateUser(ctx context.Context, name string, email string, passwordHash string, createdAt int64) (int64, error) {
	result, err := store.db.ExecContext(
		ctx,
		`INSERT INTO users (email, name, password_hash, created_at) VALUES (?, ?, ?, ?)`,
		email,
		name,
		passwordHash,
		createdAt,
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func (store *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, email, name, password_hash, created_at FROM users WHERE email = ?`,
		email,
	)
	return scanUser(row)
}

func (store *Store) UserByID(ctx context.Context, userID int64) (*User, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, email, name, password_hash, created_at FROM users WHERE id = ?`,
		userID,
	)
	return scanUser(row)
}

func (store *Store) ReplaceSession(ctx context.Context, oldIDHash string, idHash string, userID *int64, csrfToken string, now int64, expiresAt int64) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if oldIDHash != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, oldIDHash); err != nil {
			return err
		}
	}

	var nullableUserID sql.NullInt64
	if userID != nil {
		nullableUserID = sql.NullInt64{Int64: *userID, Valid: true}
	}

	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO sessions (id_hash, user_id, csrf_token, created_at, last_seen, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		idHash,
		nullableUserID,
		csrfToken,
		now,
		now,
		expiresAt,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (store *Store) SessionByHash(ctx context.Context, idHash string, now int64) (*Session, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id_hash, user_id, csrf_token, created_at, last_seen, expires_at FROM sessions WHERE id_hash = ? AND expires_at > ?`,
		idHash,
		now,
	)

	session, err := scanSession(row)
	if err != nil {
		return nil, err
	}

	_, err = store.db.ExecContext(ctx, `UPDATE sessions SET last_seen = ? WHERE id_hash = ?`, now, idHash)
	if err != nil {
		return nil, err
	}

	return session, nil
}

func (store *Store) DeleteSession(ctx context.Context, idHash string) error {
	_, err := store.db.ExecContext(ctx, `DELETE FROM sessions WHERE id_hash = ?`, idHash)
	return err
}

func IsNotFound(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unique") || strings.Contains(message, "constraint failed")
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*User, error) {
	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.CreatedAt); err != nil {
		return nil, err
	}
	return &user, nil
}

func scanSession(row scanner) (*Session, error) {
	var session Session
	if err := row.Scan(&session.IDHash, &session.UserID, &session.CSRFToken, &session.CreatedAt, &session.LastSeen, &session.ExpiresAt); err != nil {
		return nil, err
	}
	return &session, nil
}
