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
	ID              int64
	Email           string
	Name            string
	PasswordHash    string
	EmailVerifiedAt int64
	CreatedAt       int64
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
  email_verified_at INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS email_verification_tokens (
  id_hash TEXT PRIMARY KEY,
  user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  used_at INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS sessions (
  id_hash TEXT PRIMARY KEY,
  user_id INTEGER REFERENCES users(id) ON DELETE CASCADE,
  csrf_token TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  last_seen INTEGER NOT NULL,
  expires_at INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_user_id ON email_verification_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_expires_at ON email_verification_tokens(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
`
	if _, err := store.db.ExecContext(ctx, schema); err != nil {
		return err
	}

	addedEmailVerifiedAt, err := store.ensureColumn(ctx, "users", "email_verified_at", "INTEGER NOT NULL DEFAULT 0")
	if err != nil {
		return err
	}
	if addedEmailVerifiedAt {
		if _, err := store.db.ExecContext(ctx, `UPDATE users SET email_verified_at = created_at WHERE email_verified_at = 0`); err != nil {
			return err
		}
	}

	if _, err := store.db.ExecContext(ctx, "DELETE FROM sessions WHERE expires_at <= unixepoch()"); err != nil {
		return err
	}
	_, err = store.db.ExecContext(ctx, "DELETE FROM email_verification_tokens WHERE expires_at <= unixepoch() OR used_at > 0")
	return err
}

func (store *Store) CreateUser(ctx context.Context, name string, email string, passwordHash string, createdAt int64) (int64, error) {
	result, err := store.db.ExecContext(
		ctx,
		`INSERT INTO users (email, name, password_hash, email_verified_at, created_at) VALUES (?, ?, ?, 0, ?)`,
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
		`SELECT id, email, name, password_hash, email_verified_at, created_at FROM users WHERE email = ?`,
		email,
	)
	return scanUser(row)
}

func (store *Store) UserByID(ctx context.Context, userID int64) (*User, error) {
	row := store.db.QueryRowContext(
		ctx,
		`SELECT id, email, name, password_hash, email_verified_at, created_at FROM users WHERE id = ?`,
		userID,
	)
	return scanUser(row)
}

func (store *Store) CreateEmailVerificationToken(ctx context.Context, userID int64, idHash string, createdAt int64, expiresAt int64) error {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `DELETE FROM email_verification_tokens WHERE user_id = ? AND (used_at > 0 OR expires_at <= ?)`, userID, createdAt); err != nil {
		return err
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO email_verification_tokens (id_hash, user_id, created_at, expires_at, used_at) VALUES (?, ?, ?, ?, 0)`,
		idHash,
		userID,
		createdAt,
		expiresAt,
	); err != nil {
		return err
	}

	return tx.Commit()
}

func (store *Store) VerifyEmailByToken(ctx context.Context, idHash string, now int64) (*User, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var userID int64
	if err := tx.QueryRowContext(
		ctx,
		`SELECT user_id FROM email_verification_tokens WHERE id_hash = ? AND used_at = 0 AND expires_at > ?`,
		idHash,
		now,
	).Scan(&userID); err != nil {
		return nil, err
	}

	if _, err := tx.ExecContext(
		ctx,
		`UPDATE users SET email_verified_at = CASE WHEN email_verified_at = 0 THEN ? ELSE email_verified_at END WHERE id = ?`,
		now,
		userID,
	); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE email_verification_tokens SET used_at = ? WHERE id_hash = ?`, now, idHash); err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM email_verification_tokens WHERE user_id = ? AND id_hash <> ?`, userID, idHash); err != nil {
		return nil, err
	}

	row := tx.QueryRowContext(
		ctx,
		`SELECT id, email, name, password_hash, email_verified_at, created_at FROM users WHERE id = ?`,
		userID,
	)
	user, err := scanUser(row)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return user, nil
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

func (store *Store) ensureColumn(ctx context.Context, table string, column string, definition string) (bool, error) {
	rows, err := store.db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var (
			cid        int
			name       string
			columnType string
			notNull    int
			defaultVal sql.NullString
			primaryKey int
		)
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultVal, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return false, rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}

	if _, err := store.db.ExecContext(ctx, fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, definition)); err != nil {
		return false, err
	}
	return true, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanUser(row scanner) (*User, error) {
	var user User
	if err := row.Scan(&user.ID, &user.Email, &user.Name, &user.PasswordHash, &user.EmailVerifiedAt, &user.CreatedAt); err != nil {
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
