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
	ID       int64
	Name     string
	Host     string
	Port     int
	Username string
	AuthType string
	GroupID  int64
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
		SELECT id, name, host, port, username, auth_type, group_id
		FROM profiles ORDER BY name`)
	if err != nil {
		return nil, err
	}

	idx := columns(rows.Columns)
	out := make([]Profile, 0, len(rows.Values))
	for _, v := range rows.Values {
		out = append(out, Profile{
			ID:       asInt64(v[idx["id"]]),
			Name:     asString(v[idx["name"]]),
			Host:     asString(v[idx["host"]]),
			Port:     int(asInt64(v[idx["port"]])),
			Username: asString(v[idx["username"]]),
			AuthType: asString(v[idx["auth_type"]]),
			GroupID:  asInt64(v[idx["group_id"]]),
		})
	}
	return out, nil
}

func (s *Store) SaveProfile(ctx context.Context, p *Profile) error {
	now := time.Now().Unix()
	if p.GroupID < 0 {
		p.GroupID = 0
	}
	if p.ID == 0 {
		res, err := s.db.Execute(ctx, `
			INSERT INTO profiles (name, host, port, username, auth_type, group_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			p.Name, p.Host, p.Port, p.Username, p.AuthType, p.GroupID, now, now)
		if err != nil {
			return err
		}
		p.ID = res.LastInsertID
		return nil
	}

	_, err := s.db.Execute(ctx, `
		UPDATE profiles SET name = ?, host = ?, port = ?, username = ?, auth_type = ?, group_id = ?, updated_at = ?
		WHERE id = ?`,
		p.Name, p.Host, p.Port, p.Username, p.AuthType, p.GroupID, now, p.ID)
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
		{SQL: `DELETE FROM port_forwards WHERE profile_id = ?`, Args: []any{id}},
		{SQL: `DELETE FROM profiles WHERE id = ?`, Args: []any{id}},
	})
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
