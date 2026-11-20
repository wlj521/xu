package store

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/sqlite"
)

type Profile struct {
	ID            int64
	Name          string
	Host          string
	Port          int
	Username      string
	AuthType      string
	GroupID       int64
	JumpProfileID int64
}

type Group struct {
	ID   int64
	Name string
}

type ForwardRule struct {
	ID        int64
	ProfileID int64
	Kind      int
	Listen    string
	Target    string
	AutoStart bool
}

// QuickCommand is a command the user can paste into a terminal. ProfileID 0
// makes it global; otherwise it belongs to one saved connection.
type QuickCommand struct {
	ID        int64
	ProfileID int64
	Name      string
	Command   string
}

type TransferRecord struct {
	ID        int64
	Kind      string
	Label     string
	Target    string
	Done      int64
	Total     int64
	Status    string
	Err       string
	CreatedAt int64
}

const transferHistoryRetention = 7 * 24 * time.Hour

type Store struct {
	db *sqlite.DB
}

func Open(ctx context.Context) (*Store, error) {
	dir, err := mygo.App.Path(mygo.PathUserData)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}

	db, err := sqlite.Open(ctx, filepath.Join(dir, "xu.db"), sqlite.OpenOptions{})
	if err != nil {
		return nil, err
	}
	if err := migrate(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

var migrations = [][]string{
	{
		`CREATE TABLE IF NOT EXISTS profiles (
			id         INTEGER PRIMARY KEY,
			name       TEXT NOT NULL,
			host       TEXT NOT NULL,
			port       INTEGER NOT NULL,
			username   TEXT NOT NULL,
			auth_type  TEXT NOT NULL DEFAULT 'password',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS known_hosts (
			host        TEXT NOT NULL,
			port        INTEGER NOT NULL,
			fingerprint TEXT NOT NULL,
			added_at    INTEGER NOT NULL,
			PRIMARY KEY (host, port)
		)`,
	},
	{
		`CREATE TABLE IF NOT EXISTS port_forwards (
			id          INTEGER PRIMARY KEY,
			profile_id  INTEGER NOT NULL REFERENCES profiles(id) ON DELETE CASCADE,
			kind        INTEGER NOT NULL,
			listen_addr TEXT NOT NULL,
			target_addr TEXT NOT NULL DEFAULT '',
			auto_start  INTEGER NOT NULL DEFAULT 0,
			created_at  INTEGER NOT NULL
		)`,
	},
	{
		`CREATE TABLE IF NOT EXISTS settings (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	},
	{
		`CREATE TABLE IF NOT EXISTS groups (
			id         INTEGER PRIMARY KEY,
			name       TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL
		)`,
		`ALTER TABLE profiles ADD COLUMN group_id INTEGER NOT NULL DEFAULT 0`,
	},
	{
		`CREATE TABLE IF NOT EXISTS quick_commands (
			id         INTEGER PRIMARY KEY,
			profile_id INTEGER NOT NULL DEFAULT 0,
			name       TEXT NOT NULL,
			command    TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			UNIQUE(profile_id, name)
		)`,
		`CREATE INDEX IF NOT EXISTS quick_commands_profile ON quick_commands(profile_id, id)`,
	},
	{
		`ALTER TABLE profiles ADD COLUMN jump_profile_id INTEGER NOT NULL DEFAULT 0`,
	},
	{
		`CREATE TABLE IF NOT EXISTS transfer_history (
			id         INTEGER PRIMARY KEY,
			kind       TEXT NOT NULL,
			label      TEXT NOT NULL,
			target     TEXT NOT NULL DEFAULT '',
			done       INTEGER NOT NULL DEFAULT 0,
			total      INTEGER NOT NULL DEFAULT 0,
			status     TEXT NOT NULL,
			err        TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS transfer_history_created ON transfer_history(created_at DESC)`,
	},
}

func migrate(ctx context.Context, db *sqlite.DB) error {
	rows, err := db.Query(ctx, `PRAGMA user_version`)
	if err != nil {
		return err
	}
	version := 0
	if len(rows.Values) > 0 && len(rows.Values[0]) > 0 && rows.Values[0][0] != nil {
		version = int(asInt64(rows.Values[0][0]))
	}

	for ; version < len(migrations); version++ {
		for _, stmt := range migrations[version] {
			if _, err := db.Execute(ctx, stmt); err != nil {
				return fmt.Errorf("migration %d: %w", version+1, err)
			}
		}
		if _, err := db.Execute(ctx, fmt.Sprintf(`PRAGMA user_version = %d`, version+1)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Profiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, name, host, port, username, auth_type, group_id, jump_profile_id
		FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make([]Profile, 0, len(rows.Values))
	for _, v := range rows.Values {
		out = append(out, Profile{
			ID:            asInt64(v[idx["id"]]),
			Name:          asString(v[idx["name"]]),
			Host:          asString(v[idx["host"]]),
			Port:          int(asInt64(v[idx["port"]])),
			Username:      asString(v[idx["username"]]),
			AuthType:      asString(v[idx["auth_type"]]),
			GroupID:       asInt64(v[idx["group_id"]]),
			JumpProfileID: asInt64(v[idx["jump_profile_id"]]),
		})
	}
	return out, nil
}

func (s *Store) SaveProfile(ctx context.Context, p *Profile) error {
	now := time.Now().Unix()
	if p.GroupID < 0 {
		p.GroupID = 0
	}
	if p.JumpProfileID < 0 || p.JumpProfileID == p.ID {
		p.JumpProfileID = 0
	}
	if p.ID == 0 {
		res, err := s.db.Execute(ctx, `
			INSERT INTO profiles (name, host, port, username, auth_type, group_id, jump_profile_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Name, p.Host, p.Port, p.Username, p.AuthType, p.GroupID, p.JumpProfileID, now, now)
		if err != nil {
			return err
		}
		p.ID = res.LastInsertID
		return nil
	}

	_, err := s.db.Execute(ctx, `
		UPDATE profiles SET name = ?, host = ?, port = ?, username = ?, auth_type = ?, group_id = ?, jump_profile_id = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Host, p.Port, p.Username, p.AuthType, p.GroupID, p.JumpProfileID, now, p.ID)
	return err
}

func (s *Store) Groups(ctx context.Context) ([]Group, error) {
	rows, err := s.db.Query(ctx, `SELECT id, name FROM groups ORDER BY id`)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make([]Group, 0, len(rows.Values))
	for _, v := range rows.Values {
		out = append(out, Group{
			ID:   asInt64(v[idx["id"]]),
			Name: asString(v[idx["name"]]),
		})
	}
	return out, nil
}

func (s *Store) SaveGroup(ctx context.Context, g *Group) error {
	if g.ID == 0 {
		res, err := s.db.Execute(ctx, `
			INSERT INTO groups (name, created_at) VALUES (?, ?)`,
			g.Name, time.Now().Unix())
		if err != nil {
			return err
		}
		g.ID = res.LastInsertID
		return nil
	}
	_, err := s.db.Execute(ctx, `UPDATE groups SET name = ? WHERE id = ?`, g.Name, g.ID)
	return err
}

func (s *Store) DeleteGroup(ctx context.Context, id int64) error {
	_, err := s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `UPDATE profiles SET group_id = 0 WHERE group_id = ?`, Args: []any{id}},
		{SQL: `DELETE FROM groups WHERE id = ?`, Args: []any{id}},
	})
	return err
}

func (s *Store) DeleteProfile(ctx context.Context, id int64) error {
	_, err := s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `UPDATE profiles SET jump_profile_id = 0 WHERE jump_profile_id = ?`, Args: []any{id}},
		{SQL: `DELETE FROM port_forwards WHERE profile_id = ?`, Args: []any{id}},
		{SQL: `DELETE FROM quick_commands WHERE profile_id = ?`, Args: []any{id}},
		{SQL: `DELETE FROM profiles WHERE id = ?`, Args: []any{id}},
	})
	return err
}

func (s *Store) QuickCommands(ctx context.Context) ([]QuickCommand, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, profile_id, name, command
		FROM quick_commands ORDER BY profile_id, id`)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make([]QuickCommand, 0, len(rows.Values))
	for _, v := range rows.Values {
		out = append(out, QuickCommand{
			ID:        asInt64(v[idx["id"]]),
			ProfileID: asInt64(v[idx["profile_id"]]),
			Name:      asString(v[idx["name"]]),
			Command:   asString(v[idx["command"]]),
		})
	}
	return out, nil
}

func (s *Store) SaveQuickCommand(ctx context.Context, q *QuickCommand) error {
	now := time.Now().Unix()
	if q.ID != 0 {
		_, err := s.db.Execute(ctx, `
			UPDATE quick_commands SET profile_id = ?, name = ?, command = ?, updated_at = ?
			WHERE id = ?`, q.ProfileID, q.Name, q.Command, now, q.ID)
		return err
	}

	res, err := s.db.Execute(ctx, `
		INSERT INTO quick_commands (profile_id, name, command, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)`, q.ProfileID, q.Name, q.Command, now, now)
	if err != nil {
		return err
	}
	q.ID = res.LastInsertID
	return nil
}

func (s *Store) DeleteQuickCommand(ctx context.Context, id int64) error {
	_, err := s.db.Execute(ctx, `DELETE FROM quick_commands WHERE id = ?`, id)
	return err
}

func (s *Store) Forwards(ctx context.Context, profileID int64) ([]ForwardRule, error) {
	rows, err := s.db.Query(ctx, `
		SELECT id, profile_id, kind, listen_addr, target_addr, auto_start
		FROM port_forwards WHERE profile_id = ? ORDER BY id`, profileID)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make([]ForwardRule, 0, len(rows.Values))
	for _, v := range rows.Values {
		out = append(out, ForwardRule{
			ID:        asInt64(v[idx["id"]]),
			ProfileID: asInt64(v[idx["profile_id"]]),
			Kind:      int(asInt64(v[idx["kind"]])),
			Listen:    asString(v[idx["listen_addr"]]),
			Target:    asString(v[idx["target_addr"]]),
			AutoStart: asInt64(v[idx["auto_start"]]) != 0,
		})
	}
	return out, nil
}

func (s *Store) SaveForward(ctx context.Context, r *ForwardRule) error {
	now := time.Now().Unix()
	auto := 0
	if r.AutoStart {
		auto = 1
	}

	if r.ID != 0 {
		_, err := s.db.Execute(ctx, `
			UPDATE port_forwards SET kind = ?, listen_addr = ?, target_addr = ?, auto_start = ?
			WHERE id = ?`,
			r.Kind, r.Listen, r.Target, auto, r.ID)
		return err
	}

	results, err := s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `DELETE FROM port_forwards WHERE profile_id = ? AND kind = ? AND listen_addr = ? AND target_addr = ?`,
			Args: []any{r.ProfileID, r.Kind, r.Listen, r.Target}},
		{SQL: `INSERT INTO port_forwards (profile_id, kind, listen_addr, target_addr, auto_start, created_at)
			VALUES (?, ?, ?, ?, ?, ?)`,
			Args: []any{r.ProfileID, r.Kind, r.Listen, r.Target, auto, now}},
	})
	if err != nil {
		return err
	}
	r.ID = results[len(results)-1].LastInsertID
	return nil
}

func (s *Store) DeleteForward(ctx context.Context, id int64) error {
	_, err := s.db.Execute(ctx, `DELETE FROM port_forwards WHERE id = ?`, id)
	return err
}

func (s *Store) Settings(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `SELECT key, value FROM settings`)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make(map[string]string, len(rows.Values))
	for _, v := range rows.Values {
		out[asString(v[idx["key"]])] = asString(v[idx["value"]])
	}
	return out, nil
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `DELETE FROM settings WHERE key = ?`, Args: []any{key}},
		{SQL: `INSERT INTO settings (key, value) VALUES (?, ?)`, Args: []any{key, value}},
	})
	return err
}

func (s *Store) AddTransferRecord(ctx context.Context, r *TransferRecord) error {
	if r.CreatedAt == 0 {
		r.CreatedAt = time.Now().Unix()
	}
	cutoff := time.Now().Add(-transferHistoryRetention).Unix()
	results, err := s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `INSERT INTO transfer_history (kind, label, target, done, total, status, err, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{r.Kind, r.Label, r.Target, r.Done, r.Total, r.Status, r.Err, r.CreatedAt}},
		{SQL: `DELETE FROM transfer_history WHERE created_at < ?`, Args: []any{cutoff}},
		{SQL: `DELETE FROM transfer_history WHERE id NOT IN (
			SELECT id FROM transfer_history ORDER BY id DESC LIMIT 200
		)`},
	})
	if err != nil {
		return err
	}
	r.ID = results[0].LastInsertID
	return nil
}

func (s *Store) TransferHistory(ctx context.Context, limit int) ([]TransferRecord, error) {
	return s.TransferHistorySince(ctx, limit, time.Now().Add(-transferHistoryRetention))
}

func (s *Store) TransferHistorySince(ctx context.Context, limit int, since time.Time) ([]TransferRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `
		SELECT id, kind, label, target, done, total, status, err, created_at
		FROM transfer_history WHERE created_at >= ? ORDER BY id DESC LIMIT ?`, since.Unix(), limit)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make([]TransferRecord, 0, len(rows.Values))
	for _, v := range rows.Values {
		out = append(out, TransferRecord{
			ID:        asInt64(v[idx["id"]]),
			Kind:      asString(v[idx["kind"]]),
			Label:     asString(v[idx["label"]]),
			Target:    asString(v[idx["target"]]),
			Done:      asInt64(v[idx["done"]]),
			Total:     asInt64(v[idx["total"]]),
			Status:    asString(v[idx["status"]]),
			Err:       asString(v[idx["err"]]),
			CreatedAt: asInt64(v[idx["created_at"]]),
		})
	}
	return out, nil
}

func (s *Store) PruneTransferHistory(ctx context.Context) error {
	cutoff := time.Now().Add(-transferHistoryRetention).Unix()
	_, err := s.db.Execute(ctx, `DELETE FROM transfer_history WHERE created_at < ?`, cutoff)
	return err
}

func (s *Store) ClearTransferHistory(ctx context.Context) error {
	_, err := s.db.Execute(ctx, `DELETE FROM transfer_history`)
	return err
}

func (s *Store) KnownHosts(ctx context.Context) (map[string]string, error) {
	rows, err := s.db.Query(ctx, `SELECT host, port, fingerprint FROM known_hosts`)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make(map[string]string, len(rows.Values))
	for _, v := range rows.Values {
		host := asString(v[idx["host"]])
		port := int(asInt64(v[idx["port"]]))
		out[net.JoinHostPort(host, strconv.Itoa(port))] = asString(v[idx["fingerprint"]])
	}
	return out, nil
}

func (s *Store) SaveKnownHost(ctx context.Context, host string, port int, fingerprint string) error {
	_, err := s.db.Transaction(ctx, []sqlite.Statement{
		{SQL: `DELETE FROM known_hosts WHERE host = ? AND port = ?`, Args: []any{host, port}},
		{SQL: `INSERT INTO known_hosts (host, port, fingerprint, added_at) VALUES (?, ?, ?, ?)`,
			Args: []any{host, port, fingerprint, time.Now().Unix()}},
	})
	return err
}

func columns(cols []string) map[string]int {
	m := make(map[string]int, len(cols))
	for i, c := range cols {
		m[c] = i
	}
	return m
}

func asInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	}
	return 0
}

func asString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
