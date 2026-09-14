package fleet

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	if path == "" {
		path = "./data/fleet.db"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS snapshots (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  cluster TEXT NOT NULL,
  katana_version TEXT,
  k8s_version TEXT,
  nodes_ready INTEGER,
  nodes_not_ready INTEGER,
  katana_up INTEGER,
  dry_run INTEGER
);
CREATE INDEX IF NOT EXISTS snapshots_cluster_ts ON snapshots(cluster, ts);
CREATE TABLE IF NOT EXISTS deploys (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  ts INTEGER NOT NULL,
  cluster TEXT NOT NULL,
  from_version TEXT,
  to_version TEXT
);
CREATE INDEX IF NOT EXISTS deploys_ts ON deploys(ts DESC);
`)
	return err
}

func (s *Store) RecordSnapshot(ctx context.Context, snap Snapshot) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var prev string
	_ = tx.QueryRowContext(ctx, `SELECT katana_version FROM snapshots WHERE cluster = ? ORDER BY ts DESC LIMIT 1`, snap.Cluster).Scan(&prev)

	up := 0
	if snap.KatanaUp {
		up = 1
	}
	dry := 0
	if snap.DryRun {
		dry = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO snapshots(ts,cluster,katana_version,k8s_version,nodes_ready,nodes_not_ready,katana_up,dry_run)
VALUES(?,?,?,?,?,?,?,?)`, snap.Ts.Unix(), snap.Cluster, snap.KatanaVersion, snap.K8sVersion, snap.NodesReady, snap.NodesNotReady, up, dry); err != nil {
		return err
	}
	if snap.KatanaVersion != "" && prev != "" && prev != snap.KatanaVersion {
		if _, err := tx.ExecContext(ctx, `INSERT INTO deploys(ts,cluster,from_version,to_version) VALUES(?,?,?,?)`,
			snap.Ts.Unix(), snap.Cluster, prev, snap.KatanaVersion); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListDeploys(ctx context.Context, limit int) ([]HistoryEvent, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, cluster, from_version, to_version FROM deploys ORDER BY ts DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEvent
	for rows.Next() {
		var e HistoryEvent
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Cluster, &e.FromVersion, &e.ToVersion); err != nil {
			return nil, err
		}
		e.Ts = time.Unix(ts, 0).UTC()
		out = append(out, e)
	}
	if out == nil {
		out = []HistoryEvent{}
	}
	return out, rows.Err()
}
