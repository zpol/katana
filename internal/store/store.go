package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/goxray/goxray/internal/detect"
	"github.com/goxray/goxray/internal/policy"

	_ "modernc.org/sqlite"
)

// Store is the SQLite-backed persistence layer.
type Store struct {
	db   *sql.DB
	path string
}

// Open opens (or creates) the SQLite database and runs migrations.
func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, path: path}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, err
	}
	_ = os.Chmod(path, 0o600)
	return s, nil
}

// Close closes the database.
func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate(ctx context.Context) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS policies (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  action TEXT NOT NULL,
  match_json TEXT NOT NULL,
  exceptions_json TEXT NOT NULL DEFAULT '[]',
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS detections (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  severity TEXT NOT NULL,
  environment TEXT NOT NULL DEFAULT '',
  namespace TEXT NOT NULL DEFAULT '',
  registry TEXT NOT NULL DEFAULT '',
  image TEXT NOT NULL DEFAULT '',
  scanned INTEGER NOT NULL DEFAULT 0,
  source TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_detections_severity ON detections(severity);
CREATE INDEX IF NOT EXISTS idx_detections_env ON detections(environment);
`
	if _, err := s.db.ExecContext(ctx, ddl); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	// Best-effort add for older MVP DBs created before exceptions_json existed.
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE policies ADD COLUMN exceptions_json TEXT NOT NULL DEFAULT '[]'`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE detections ADD COLUMN policy_action TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE detections ADD COLUMN deployed INTEGER`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE detections ADD COLUMN dry_run INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE policies ADD COLUMN deny_message TEXT NOT NULL DEFAULT ''`)
	_, _ = s.db.ExecContext(ctx, `ALTER TABLE policies ADD COLUMN warn_message TEXT NOT NULL DEFAULT ''`)
	if err := s.migrateAuth(ctx); err != nil {
		return err
	}
	return nil
}

var defaultSystemExceptions = []string{"kube-system", "katana-system", "katana-poc-system", "cattle-*"}

// SeedDefaults inserts default policies and sample detections when empty.
func (s *Store) SeedDefaults(ctx context.Context) error {
	skipPolicies := strings.EqualFold(os.Getenv("KATANA_POLICY_SOURCE"), "crd") ||
		strings.EqualFold(os.Getenv("KATANA_POLICY_SOURCE"), "kubernetes") ||
		strings.EqualFold(os.Getenv("KATANA_POLICY_SOURCE"), "k8s")

	if !skipPolicies {
		var n int
		if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM policies`).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			if err := s.seedPolicies(ctx); err != nil {
				return err
			}
		}
	}

	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections`).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		if err := s.seedDetections(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedPolicies(ctx context.Context) error {
	falseVal := false
	unsafe := true
	defaultMsgs := policy.DefaultDenyMessages()
	defaults := []policy.Policy{
		{
			Name:        "Block Critical",
			Description: "Deny workloads with any Critical CVE (system NS excepted)",
			Enabled:     true,
			Action:      policy.ActionDeny,
			Match:       policy.MatchCriteria{Severity: "critical"},
			Exceptions:  append([]string{}, defaultSystemExceptions...),
			DenyMessage: defaultMsgs["Block Critical"],
		},
		{
			Name:        "Block High in Prod",
			Description: "Deny High severity findings in production environments",
			Enabled:     true,
			Action:      policy.ActionDeny,
			Match:       policy.MatchCriteria{Severity: "high", Environment: "prod"},
			Exceptions:  append([]string{}, defaultSystemExceptions...),
			DenyMessage: defaultMsgs["Block High in Prod"],
		},
		{
			Name:        "Require Scanned Image",
			Description: "Deny images with no Xray scan result",
			Enabled:     true,
			Action:      policy.ActionDeny,
			Match:       policy.MatchCriteria{Scanned: &falseVal},
			Exceptions:  append([]string{}, defaultSystemExceptions...),
			DenyMessage: defaultMsgs["Require Scanned Image"],
		},
		{
			Name:        "Allowlist System NS",
			Description: "Audit-only log of Pods in platform namespaces (kube-system, katana-system, katana-poc-system, cattle-*). Does not allow or block; deny exceptions do that. Safe to disable if you only need enforcement.",
			Enabled:     true,
			Action:      policy.ActionAudit,
			Match: policy.MatchCriteria{NamespaceAllowlist: []string{
				"kube-system", "katana-system", "katana-poc-system", "cattle-*",
			}},
			WarnMessage: defaultMsgs["Allowlist System NS"],
		},
		{
			Name:        "Registry Allowlist",
			Description: "Deny images not from approved registries",
			Enabled:     true,
			Action:      policy.ActionDeny,
			Match: policy.MatchCriteria{RegistryAllowlist: []string{
				"artifactory.example.com",
				"123456789012.dkr.ecr.eu-west-3.amazonaws.com",
			}},
			Exceptions:  append([]string{}, defaultSystemExceptions...),
			DenyMessage: defaultMsgs["Registry Allowlist"],
		},
		{
			Name:        "Deny Unsafe Pod Security",
			Description: "Deny privileged containers, root (UID 0), or allowPrivilegeEscalation",
			Enabled:     true,
			Action:      policy.ActionDeny,
			Match:       policy.MatchCriteria{UnsafePodSecurity: &unsafe},
			Exceptions:  append([]string{}, defaultSystemExceptions...),
			DenyMessage: defaultMsgs["Deny Unsafe Pod Security"],
		},
	}
	for _, p := range defaults {
		if _, err := s.CreatePolicy(ctx, p); err != nil {
			return fmt.Errorf("seed policy %s: %w", p.Name, err)
		}
	}
	return nil
}

func (s *Store) seedDetections(ctx context.Context) error {
	samples := []detect.Detection{
		{
			Title: "Critical CVE in base image", Description: "CVE-2024-0001 in openssl — policy: Block Critical",
			Severity: detect.SeverityCritical, Environment: "prod", Namespace: "payments",
			Registry: "evil.example", Image: "evil.example/pay:1.0", Scanned: true, Source: "seed",
		},
		{
			Title: "High severity package", Description: "CVE-2024-1002 in libxyz — policy: Block High in Prod",
			Severity: detect.SeverityHigh, Environment: "prod", Namespace: "api",
			Registry: "artifactory.example.com", Image: "artifactory.example.com/api:2.1", Scanned: true, Source: "seed",
		},
		{
			Title: "Unscanned image deploy", Description: "Image missing Xray scan — policy: Require Scanned Image",
			Severity: detect.SeverityMedium, Environment: "dev", Namespace: "demo",
			Registry: "artifactory.example.com", Image: "artifactory.example.com/demo:dev", Scanned: false, Source: "seed",
		},
		{
			Title: "Kube-system informational", Description: "Audit trail for system NS traffic",
			Severity: detect.SeverityLow, Environment: "prod", Namespace: "kube-system",
			Registry: "123456789012.dkr.ecr.eu-west-3.amazonaws.com",
			Image:    "123456789012.dkr.ecr.eu-west-3.amazonaws.com/pause:3.9",
			Scanned:  true, Source: "seed",
		},
	}
	for _, d := range samples {
		if _, err := s.CreateDetection(ctx, d); err != nil {
			return fmt.Errorf("seed detection: %w", err)
		}
	}
	return nil
}

func (s *Store) CreatePolicy(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	now := time.Now().UTC()
	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	if p.Exceptions == nil {
		p.Exceptions = []string{}
	}
	p.CreatedAt = now
	p.UpdatedAt = now
	matchJSON, err := json.Marshal(p.Match)
	if err != nil {
		return policy.Policy{}, err
	}
	excJSON, err := json.Marshal(p.Exceptions)
	if err != nil {
		return policy.Policy{}, err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO policies (id, name, description, enabled, action, match_json, exceptions_json, deny_message, warn_message, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.Description, boolToInt(p.Enabled), string(p.Action), string(matchJSON), string(excJSON),
		p.DenyMessage, p.WarnMessage,
		p.CreatedAt.Format(time.RFC3339Nano), p.UpdatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return policy.Policy{}, err
	}
	return p, nil
}

func (s *Store) ListPolicies(ctx context.Context) ([]policy.Policy, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, name, description, enabled, action, match_json, exceptions_json, deny_message, warn_message, created_at, updated_at
FROM policies ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []policy.Policy
	for rows.Next() {
		p, err := scanPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) GetPolicy(ctx context.Context, id string) (policy.Policy, error) {
	row := s.db.QueryRowContext(ctx, `
SELECT id, name, description, enabled, action, match_json, exceptions_json, deny_message, warn_message, created_at, updated_at
FROM policies WHERE id = ?`, id)
	return scanPolicy(row)
}

func (s *Store) UpdatePolicy(ctx context.Context, p policy.Policy) (policy.Policy, error) {
	existing, err := s.GetPolicy(ctx, p.ID)
	if err != nil {
		return policy.Policy{}, err
	}
	if p.Exceptions == nil {
		p.Exceptions = []string{}
	}
	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now().UTC()
	matchJSON, err := json.Marshal(p.Match)
	if err != nil {
		return policy.Policy{}, err
	}
	excJSON, err := json.Marshal(p.Exceptions)
	if err != nil {
		return policy.Policy{}, err
	}
	res, err := s.db.ExecContext(ctx, `
UPDATE policies SET name=?, description=?, enabled=?, action=?, match_json=?, exceptions_json=?, deny_message=?, warn_message=?, updated_at=?
WHERE id=?`,
		p.Name, p.Description, boolToInt(p.Enabled), string(p.Action), string(matchJSON), string(excJSON),
		p.DenyMessage, p.WarnMessage,
		p.UpdatedAt.Format(time.RFC3339Nano), p.ID,
	)
	if err != nil {
		return policy.Policy{}, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return policy.Policy{}, sql.ErrNoRows
	}
	return p, nil
}

func (s *Store) PatchPolicy(ctx context.Context, id string, patch map[string]json.RawMessage) (policy.Policy, error) {
	p, err := s.GetPolicy(ctx, id)
	if err != nil {
		return policy.Policy{}, err
	}
	if v, ok := patch["name"]; ok {
		_ = json.Unmarshal(v, &p.Name)
	}
	if v, ok := patch["description"]; ok {
		_ = json.Unmarshal(v, &p.Description)
	}
	if v, ok := patch["enabled"]; ok {
		_ = json.Unmarshal(v, &p.Enabled)
	}
	if v, ok := patch["action"]; ok {
		var a string
		_ = json.Unmarshal(v, &a)
		p.Action = policy.Action(a)
	}
	if v, ok := patch["match"]; ok {
		_ = json.Unmarshal(v, &p.Match)
	}
	if v, ok := patch["exceptions"]; ok {
		_ = json.Unmarshal(v, &p.Exceptions)
	}
	if v, ok := patch["deny_message"]; ok {
		_ = json.Unmarshal(v, &p.DenyMessage)
	}
	if v, ok := patch["warn_message"]; ok {
		_ = json.Unmarshal(v, &p.WarnMessage)
	}
	return s.UpdatePolicy(ctx, p)
}

func (s *Store) DeletePolicy(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM policies WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreateDetection(ctx context.Context, d detect.Detection) (detect.Detection, error) {
	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}
	var deployedVal interface{}
	if d.Deployed != nil {
		deployedVal = boolToInt(*d.Deployed)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO detections
(id, title, description, severity, environment, namespace, registry, image, scanned, source, policy_action, deployed, dry_run, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.Title, d.Description, string(d.Severity), d.Environment, d.Namespace, d.Registry, d.Image,
		boolToInt(d.Scanned), d.Source, d.PolicyAction, deployedVal, boolToInt(d.DryRun), d.CreatedAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		return detect.Detection{}, err
	}
	return d, nil
}

func (s *Store) ListDetections(ctx context.Context, f detect.Filter) ([]detect.Detection, error) {
	q := `
SELECT id, title, description, severity, environment, namespace, registry, image, scanned, source,
       policy_action, deployed, dry_run, created_at
FROM detections WHERE 1=1`
	args := []interface{}{}
	if f.Severity != "" {
		q += ` AND lower(severity) = lower(?)`
		args = append(args, f.Severity)
	}
	if f.Environment != "" {
		q += ` AND lower(environment) = lower(?)`
		args = append(args, f.Environment)
	}
	if f.Namespace != "" {
		q += ` AND lower(namespace) = lower(?)`
		args = append(args, f.Namespace)
	}
	if f.Registry != "" {
		q += ` AND lower(registry) = lower(?)`
		args = append(args, f.Registry)
	}
	switch f.Outcome {
	case "deployed":
		q += ` AND source = 'admission' AND deployed = 1`
	case "blocked":
		q += ` AND source = 'admission' AND deployed = 0`
	case "eval-blocked":
		q += ` AND source = 'evaluate' AND lower(policy_action) = 'deny'`
	case "dry-run":
		q += ` AND dry_run = 1 AND lower(policy_action) = 'deny'`
	}
	if f.Q != "" {
		q += ` AND (lower(title) LIKE ? OR lower(description) LIKE ? OR lower(image) LIKE ?)`
		like := "%" + strings.ToLower(f.Q) + "%"
		args = append(args, like, like, like)
	}
	q += ` ORDER BY created_at DESC`

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []detect.Detection
	for rows.Next() {
		d, err := scanDetection(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) GetStatsSummary(ctx context.Context) (detect.StatsSummary, error) {
	out := detect.StatsSummary{
		BySeverity:     map[string]int{},
		ByNamespace:    map[string]int{},
		ByPolicyAction: map[string]int{},
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections`).Scan(&out.TotalEvents); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections WHERE source = 'admission'`).Scan(&out.AdmissionEvents); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections WHERE source = 'admission' AND deployed = 1`).Scan(&out.Deployed); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections WHERE source = 'admission' AND deployed = 0`).Scan(&out.Blocked); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(1) FROM detections
WHERE dry_run = 1 AND lower(policy_action) = 'deny'`).Scan(&out.DryRunWouldDeny); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections WHERE scanned = 1`).Scan(&out.ScannedCount); err != nil {
		return out, err
	}
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM detections WHERE scanned = 0`).Scan(&out.UnscannedCount); err != nil {
		return out, err
	}
	admissionTotal := out.Deployed + out.Blocked
	if admissionTotal > 0 {
		out.DeployedPct = float64(out.Deployed) / float64(admissionTotal) * 100
		out.BlockedPct = float64(out.Blocked) / float64(admissionTotal) * 100
	}

	rows, err := s.db.QueryContext(ctx, `SELECT severity, COUNT(1) FROM detections GROUP BY severity`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var sev string
		var n int
		if err := rows.Scan(&sev, &n); err != nil {
			rows.Close()
			return out, err
		}
		out.BySeverity[sev] = n
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `
SELECT namespace, COUNT(1) FROM detections
WHERE namespace != '' GROUP BY namespace ORDER BY COUNT(1) DESC LIMIT 8`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var ns string
		var n int
		if err := rows.Scan(&ns, &n); err != nil {
			rows.Close()
			return out, err
		}
		out.ByNamespace[ns] = n
	}
	rows.Close()

	rows, err = s.db.QueryContext(ctx, `
SELECT policy_action, COUNT(1) FROM detections
WHERE policy_action != '' GROUP BY policy_action`)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var action string
		var n int
		if err := rows.Scan(&action, &n); err != nil {
			rows.Close()
			return out, err
		}
		out.ByPolicyAction[action] = n
	}
	return out, rows.Err()
}

type scannable interface {
	Scan(dest ...interface{}) error
}

func scanPolicy(row scannable) (policy.Policy, error) {
	var (
		p          policy.Policy
		enabled    int
		action     string
		matchJSON  string
		excJSON    string
		createdRaw string
		updatedRaw string
	)
	if err := row.Scan(&p.ID, &p.Name, &p.Description, &enabled, &action, &matchJSON, &excJSON, &p.DenyMessage, &p.WarnMessage, &createdRaw, &updatedRaw); err != nil {
		return policy.Policy{}, err
	}
	p.Enabled = enabled != 0
	p.Action = policy.Action(action)
	if err := json.Unmarshal([]byte(matchJSON), &p.Match); err != nil {
		return policy.Policy{}, err
	}
	if strings.TrimSpace(excJSON) == "" {
		p.Exceptions = []string{}
	} else if err := json.Unmarshal([]byte(excJSON), &p.Exceptions); err != nil {
		return policy.Policy{}, err
	}
	if p.Exceptions == nil {
		p.Exceptions = []string{}
	}
	var err error
	p.CreatedAt, err = time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return policy.Policy{}, err
	}
	p.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedRaw)
	if err != nil {
		return policy.Policy{}, err
	}
	return p, nil
}

func scanDetection(row scannable) (detect.Detection, error) {
	var (
		d           detect.Detection
		severity    string
		scanned     int
		policyAction string
		deployedRaw sql.NullInt64
		dryRun      int
		createdRaw  string
	)
	if err := row.Scan(
		&d.ID, &d.Title, &d.Description, &severity, &d.Environment, &d.Namespace,
		&d.Registry, &d.Image, &scanned, &d.Source, &policyAction, &deployedRaw, &dryRun, &createdRaw,
	); err != nil {
		return detect.Detection{}, err
	}
	d.Severity = detect.Severity(severity)
	d.Scanned = scanned != 0
	d.PolicyAction = policyAction
	d.DryRun = dryRun != 0
	if deployedRaw.Valid {
		v := deployedRaw.Int64 != 0
		d.Deployed = &v
	}
	var err error
	d.CreatedAt, err = time.Parse(time.RFC3339Nano, createdRaw)
	if err != nil {
		return detect.Detection{}, err
	}
	return d, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
