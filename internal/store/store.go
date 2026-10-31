package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct{ DB *sql.DB }

type Profile struct {
	ID                  string  `json:"id"`
	Name                string  `json:"name"`
	UniqueID            *string `json:"uniqueId"`
	ConnectType         string  `json:"connectType"`
	Command             *string `json:"command"`
	Host                string  `json:"host"`
	Port                int     `json:"port"`
	Username            string  `json:"username"`
	AuthType            string  `json:"authType"`
	EncryptedPassword   *string `json:"-"`
	EncryptedPrivateKey *string `json:"-"`
	EncryptedPassphrase *string `json:"-"`
	Term                string  `json:"term"`
	Cols                int     `json:"cols"`
	Rows                int     `json:"rows"`
	PromptRegex         *string `json:"promptRegex"`
	CreatedAt           int64   `json:"createdAt"`
	UpdatedAt           int64   `json:"updatedAt"`
}

type SessionRecord struct {
	Name        string  `json:"name"`
	Source      string  `json:"source"`
	ID          string  `json:"id"`
	ProfileID   string  `json:"profileId"`
	ProfileName string  `json:"profileName"`
	Target      string  `json:"target"`
	State       string  `json:"state"`
	Mode        string  `json:"mode"`
	Cols        int     `json:"cols"`
	Rows        int     `json:"rows"`
	CreatedAt   int64   `json:"createdAt"`
	ClosedAt    *int64  `json:"closedAt"`
	CloseReason *string `json:"closeReason"`
	ExitCode    *int    `json:"exitCode"`
}

type HistoryChunk struct {
	ID        int64  `json:"id"`
	SessionID string `json:"sessionId"`
	Seq       int64  `json:"seq"`
	Direction string `json:"direction"`
	DataRaw   string `json:"dataRaw"`
	DataPlain string `json:"dataPlain"`
	Internal  bool   `json:"internal"`
	CreatedAt int64  `json:"createdAt"`
}

type Webhook struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Events    []string `json:"events"`
	CreatedAt int64    `json:"createdAt"`
}

func Open(path string) (*Store, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{DB: db}
	if _, err := db.Exec(`PRAGMA busy_timeout = 5000; PRAGMA journal_mode = WAL; PRAGMA foreign_keys = ON;`); err != nil {
		db.Close()
		return nil, err
	}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.DB.Close() }

func (s *Store) migrate() error {
	_, err := s.DB.Exec(`
CREATE TABLE IF NOT EXISTS profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  unique_id TEXT,
  connect_type TEXT NOT NULL DEFAULT 'ssh',
  command TEXT,
  host TEXT NOT NULL,
  port INTEGER NOT NULL DEFAULT 22,
  username TEXT NOT NULL,
  auth_type TEXT NOT NULL DEFAULT 'password',
  encrypted_password TEXT,
  encrypted_private_key TEXT,
  encrypted_passphrase TEXT,
  term TEXT NOT NULL DEFAULT 'xterm-256color',
  cols INTEGER NOT NULL DEFAULT 160,
  rows INTEGER NOT NULL DEFAULT 42,
  prompt_regex TEXT,
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS sessions (
  id TEXT PRIMARY KEY,
  profile_id TEXT NOT NULL,
  profile_name TEXT NOT NULL,
  target TEXT NOT NULL,
  state TEXT NOT NULL,
  mode TEXT NOT NULL DEFAULT 'AUTO',
  cols INTEGER NOT NULL,
  rows INTEGER NOT NULL,
  created_at INTEGER NOT NULL,
  closed_at INTEGER,
  close_reason TEXT,
  exit_code INTEGER
);
CREATE TABLE IF NOT EXISTS history_chunks (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  session_id TEXT NOT NULL,
  seq INTEGER NOT NULL,
  direction TEXT NOT NULL,
  data_raw TEXT NOT NULL,
  data_plain TEXT NOT NULL,
  internal INTEGER NOT NULL DEFAULT 0,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS webhooks (
  id TEXT PRIMARY KEY,
  url TEXT NOT NULL,
  events TEXT NOT NULL DEFAULT '[]',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_history_session_seq ON history_chunks(session_id, seq);
`)
	if err != nil {
		return err
	}
	if err := s.ensureColumn("history_chunks", "internal", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := s.ensureColumn("sessions", "name", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := s.ensureColumn("sessions", "source", "TEXT NOT NULL DEFAULT 'unknown'"); err != nil {
		return err
	}
	if err := s.ensureColumn("profiles", "unique_id", "TEXT"); err != nil {
		return err
	}
	_, err = s.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_profiles_unique_id ON profiles(unique_id) WHERE unique_id IS NOT NULL")
	return err
}

func (s *Store) ensureColumn(table, column, definition string) error {
	rows, err := s.DB.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notnull, pk int
		var name, typ string
		var dflt sql.NullString
		if err = rows.Scan(&cid, &name, &typ, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	_, err = s.DB.Exec("ALTER TABLE " + table + " ADD COLUMN " + column + " " + definition)
	return err
}

const profileColumns = `id, name, unique_id, connect_type, command, host, port, username, auth_type,
encrypted_password, encrypted_private_key, encrypted_passphrase, term, cols, rows, prompt_regex, created_at, updated_at`

func scanProfile(row interface{ Scan(...any) error }) (Profile, error) {
	var p Profile
	err := row.Scan(&p.ID, &p.Name, &p.UniqueID, &p.ConnectType, &p.Command, &p.Host, &p.Port, &p.Username,
		&p.AuthType, &p.EncryptedPassword, &p.EncryptedPrivateKey, &p.EncryptedPassphrase,
		&p.Term, &p.Cols, &p.Rows, &p.PromptRegex, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

func (s *Store) Profile(id string) (Profile, error) {
	p, err := scanProfile(s.DB.QueryRow("SELECT "+profileColumns+" FROM profiles WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	return p, err
}

func (s *Store) Profiles(query string) ([]Profile, error) {
	rows, err := s.DB.Query("SELECT " + profileColumns + " FROM profiles ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	query = strings.ToLower(strings.TrimSpace(query))
	profiles := make([]Profile, 0)
	for rows.Next() {
		p, scanErr := scanProfile(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		if query == "" || strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(strings.ToLower(profileTarget(p)), query) || (p.UniqueID != nil && strings.Contains(strings.ToLower(*p.UniqueID), query)) {
			profiles = append(profiles, p)
		}
	}
	return profiles, rows.Err()
}

func profileTarget(p Profile) string {
	if p.ConnectType == "command" {
		if p.Command == nil {
			return ""
		}
		return *p.Command
	}
	return fmt.Sprintf("%s@%s:%d", p.Username, p.Host, p.Port)
}

func (s *Store) InsertProfile(p Profile) error {
	_, err := s.DB.Exec(`INSERT INTO profiles (`+profileColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.UniqueID, p.ConnectType, p.Command, p.Host, p.Port, p.Username, p.AuthType,
		p.EncryptedPassword, p.EncryptedPrivateKey, p.EncryptedPassphrase, p.Term, p.Cols, p.Rows, p.PromptRegex, p.CreatedAt, p.UpdatedAt)
	return err
}

func (s *Store) UpdateProfile(p Profile) error {
	result, err := s.DB.Exec(`UPDATE profiles SET name=?, unique_id=?, connect_type=?, command=?, host=?, port=?, username=?, auth_type=?,
encrypted_password=?, encrypted_private_key=?, encrypted_passphrase=?, term=?, cols=?, rows=?, prompt_regex=?, updated_at=? WHERE id=?`,
		p.Name, p.UniqueID, p.ConnectType, p.Command, p.Host, p.Port, p.Username, p.AuthType,
		p.EncryptedPassword, p.EncryptedPrivateKey, p.EncryptedPassphrase, p.Term, p.Cols, p.Rows, p.PromptRegex, p.UpdatedAt, p.ID)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteProfile(id string) error {
	_, err := s.DB.Exec("DELETE FROM profiles WHERE id=?", id)
	return err
}

var ErrNotFound = errors.New("not found")

func (s *Store) InsertSession(r SessionRecord) error {
	_, err := s.DB.Exec(`INSERT INTO sessions (id, profile_id, profile_name, target, state, mode, cols, rows, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, r.ProfileID, r.ProfileName, r.Target, r.State, r.Mode, r.Cols, r.Rows, r.CreatedAt)
	return err
}

func (s *Store) Sessions() ([]SessionRecord, error) {
	rows, err := s.DB.Query(`SELECT id, profile_id, profile_name, target, state, mode, cols, rows, created_at, closed_at, close_reason, exit_code, name, source
FROM sessions ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SessionRecord
	for rows.Next() {
		var r SessionRecord
		if err = rows.Scan(&r.ID, &r.ProfileID, &r.ProfileName, &r.Target, &r.State, &r.Mode, &r.Cols, &r.Rows, &r.CreatedAt, &r.ClosedAt, &r.CloseReason, &r.ExitCode, &r.Name, &r.Source); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Store) Session(id string) (SessionRecord, error) {
	var r SessionRecord
	err := s.DB.QueryRow(`SELECT id, profile_id, profile_name, target, state, mode, cols, rows, created_at, closed_at, close_reason, exit_code, name, source
FROM sessions WHERE id=?`, id).Scan(&r.ID, &r.ProfileID, &r.ProfileName, &r.Target, &r.State, &r.Mode, &r.Cols, &r.Rows, &r.CreatedAt, &r.ClosedAt, &r.CloseReason, &r.ExitCode, &r.Name, &r.Source)
	if errors.Is(err, sql.ErrNoRows) {
		return SessionRecord{}, ErrNotFound
	}
	return r, err
}

func (s *Store) UpdateSessionState(id, state string) error {
	_, err := s.DB.Exec("UPDATE sessions SET state=? WHERE id=?", state, id)
	return err
}

func (s *Store) UpdateSessionMode(id, mode string) error {
	_, err := s.DB.Exec("UPDATE sessions SET mode=? WHERE id=?", mode, id)
	return err
}

func (s *Store) CloseSession(id, reason string, exitCode *int) error {
	_, err := s.DB.Exec("UPDATE sessions SET state='DISCONNECTED', closed_at=?, close_reason=?, exit_code=? WHERE id=?", time.Now().UnixMilli(), reason, exitCode, id)
	return err
}

func (s *Store) MarkStaleSessions() error {
	_, err := s.DB.Exec("UPDATE sessions SET state='DISCONNECTED', closed_at=?, close_reason='Service restarted' WHERE state <> 'DISCONNECTED'", time.Now().UnixMilli())
	return err
}

func (s *Store) AddHistory(chunks []HistoryChunk) error {
	if len(chunks) == 0 {
		return nil
	}
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	stmt, err := tx.Prepare(`INSERT INTO history_chunks (session_id, seq, direction, data_raw, data_plain, internal, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		tx.Rollback()
		return err
	}
	defer stmt.Close()
	for _, chunk := range chunks {
		internal := 0
		if chunk.Internal {
			internal = 1
		}
		if _, err = stmt.Exec(chunk.SessionID, chunk.Seq, chunk.Direction, chunk.DataRaw, chunk.DataPlain, internal, chunk.CreatedAt); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) History(id string, since int64, limit int) ([]HistoryChunk, error) {
	rows, err := s.DB.Query(`SELECT id, session_id, seq, direction, data_raw, data_plain, internal, created_at
FROM history_chunks WHERE session_id=? AND seq>? ORDER BY seq LIMIT ?`, id, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []HistoryChunk
	for rows.Next() {
		var c HistoryChunk
		var internal int
		if err = rows.Scan(&c.ID, &c.SessionID, &c.Seq, &c.Direction, &c.DataRaw, &c.DataPlain, &internal, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Internal = internal != 0
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

func (s *Store) AllHistory(id string, outputOnly bool) ([]HistoryChunk, error) {
	query := `SELECT id, session_id, seq, direction, data_raw, data_plain, internal, created_at FROM history_chunks WHERE session_id=?`
	if outputOnly {
		query += ` AND direction='output'`
	}
	query += ` ORDER BY seq`
	rows, err := s.DB.Query(query, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var chunks []HistoryChunk
	for rows.Next() {
		var c HistoryChunk
		var internal int
		if err = rows.Scan(&c.ID, &c.SessionID, &c.Seq, &c.Direction, &c.DataRaw, &c.DataPlain, &internal, &c.CreatedAt); err != nil {
			return nil, err
		}
		c.Internal = internal != 0
		chunks = append(chunks, c)
	}
	return chunks, rows.Err()
}

func (s *Store) DeleteSession(id string) error {
	tx, err := s.DB.Begin()
	if err != nil {
		return err
	}
	if _, err = tx.Exec("DELETE FROM history_chunks WHERE session_id=?", id); err == nil {
		_, err = tx.Exec("DELETE FROM sessions WHERE id=?", id)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) PurgeClosedSessions(olderThan *int64) (int64, error) {
	query := "DELETE FROM sessions WHERE state='DISCONNECTED'"
	var args []any
	if olderThan != nil {
		query += " AND COALESCE(closed_at, created_at) < ?"
		args = append(args, *olderThan)
	}
	result, err := s.DB.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) Webhooks() ([]Webhook, error) {
	rows, err := s.DB.Query("SELECT id, url, events, created_at FROM webhooks ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Webhook
	for rows.Next() {
		var w Webhook
		var events string
		if err = rows.Scan(&w.ID, &w.URL, &events, &w.CreatedAt); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(events), &w.Events); err != nil {
			return nil, err
		}
		if w.Events == nil {
			w.Events = []string{}
		}
		result = append(result, w)
	}
	return result, rows.Err()
}

func (s *Store) InsertWebhook(w Webhook) error {
	events, _ := json.Marshal(w.Events)
	_, err := s.DB.Exec("INSERT INTO webhooks (id, url, events, created_at) VALUES (?, ?, ?, ?)", w.ID, w.URL, string(events), w.CreatedAt)
	return err
}

func (s *Store) DeleteWebhook(id string) error {
	_, err := s.DB.Exec("DELETE FROM webhooks WHERE id=?", id)
	return err
}

func (s *Store) UpdateSessionMetadata(id, name, source string) error {
	_, err := s.DB.Exec("UPDATE sessions SET name=?, source=? WHERE id=?", name, source, id)
	return err
}
