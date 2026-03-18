package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ali/flowgate/internal/group"
	_ "modernc.org/sqlite"
)

// ErrHasActiveTransfers is returned when deleting a resource that still has
// in_progress or pending transfers referencing it.
var ErrHasActiveTransfers = errors.New("resource has active transfers")

// TransferStats holds aggregate counts returned by GetStats.
type TransferStats struct {
	TotalTransfers  int64      `json:"total_transfers"`
	SuccessCount    int64      `json:"success_count"`
	FailedCount     int64      `json:"failed_count"`
	InProgressCount int64      `json:"in_progress_count"`
	PendingCount    int64      `json:"pending_count"`
	TotalBytes      int64      `json:"total_bytes"`
	AvgDurationMs   float64    `json:"avg_duration_ms"`
	LastTransferAt  *time.Time `json:"last_transfer_at"`
}

// ListTransfersOpts filters for ListTransfers.
type ListTransfersOpts struct {
	AppID     string
	GroupID string
	Status    string
	Limit     int
	Offset    int
}

// Transfer mirrors the transfers table row.
type Transfer struct {
	ID               string
	AppID            string
	ObjectKey        string
	SrcBucket        string
	DstBucket        string
	ObjectSize       int64
	ETag             string
	Status           string
	ErrorMessage     string
	BytesTransferred int64
	StartedAt        *time.Time
	CompletedAt      *time.Time
	DurationMs       float64
	CreatedAt        time.Time
}

// Store is the persistence interface used by all packages.
type Store interface {
	// Group operations
	CreateGroup(ctx context.Context, g *group.Group) error
	GetGroup(ctx context.Context, id string) (*group.Group, error)
	ListGroups(ctx context.Context) ([]group.Group, error)
	UpdateGroup(ctx context.Context, g *group.Group) error
	DeleteGroup(ctx context.Context, id string) error

	// App operations
	CreateApp(ctx context.Context, a *group.App) error
	GetApp(ctx context.Context, id string) (*group.App, error)
	// GetAppByRoute resolves {groupSlug}/{appSlug} in a single indexed JOIN.
	GetAppByRoute(ctx context.Context, groupSlug, appSlug string) (*group.App, error)
	ListAppsByGroup(ctx context.Context, groupID string) ([]group.App, error)
	UpdateApp(ctx context.Context, a *group.App) error
	DeleteApp(ctx context.Context, id string) error

	// Transfer operations
	CreateTransfer(ctx context.Context, t *Transfer) error
	GetTransfer(ctx context.Context, id string) (*Transfer, error)
	UpdateTransfer(ctx context.Context, t *Transfer) error
	ListTransfers(ctx context.Context, opts ListTransfersOpts) ([]Transfer, int64, error)
	GetStats(ctx context.Context, appID, groupID string) (*TransferStats, error)

	// Liveness
	Ping(ctx context.Context) error

	Close() error
}

// SQLiteStore is the SQLite implementation of Store.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore opens (or creates) the SQLite database at path and runs Migrate.
func NewSQLiteStore(path string, maxOpen, maxIdle int) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %s: %w", path, err)
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	if err := Migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

func (s *SQLiteStore) Ping(ctx context.Context) error {
	return s.db.PingContext(ctx)
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// ── Group ──────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateGroup(ctx context.Context, g *group.Group) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO groups (id, name, description, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		g.ID, g.Name, g.Description,
		g.CreatedAt.Unix(), g.UpdatedAt.Unix(),
	)
	return err
}

func (s *SQLiteStore) GetGroup(ctx context.Context, id string) (*group.Group, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, description, created_at, updated_at FROM groups WHERE id = ?`, id)
	return scanGroup(row)
}

func (s *SQLiteStore) ListGroups(ctx context.Context) ([]group.Group, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, description, created_at, updated_at FROM groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []group.Group
	for rows.Next() {
		g, err := scanGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpdateGroup(ctx context.Context, g *group.Group) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE groups SET name=?, description=?, updated_at=? WHERE id=?`,
		g.Name, g.Description, g.UpdatedAt.Unix(), g.ID)
	return err
}

func (s *SQLiteStore) DeleteGroup(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM groups WHERE id=?`, id)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanGroup(row rowScanner) (*group.Group, error) {
	var g group.Group
	var ca, ua int64
	err := row.Scan(&g.ID, &g.Name, &g.Description, &ca, &ua)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	g.CreatedAt = time.Unix(ca, 0).UTC()
	g.UpdatedAt = time.Unix(ua, 0).UTC()
	return &g, nil
}

// ── App ───────────────────────────────────────────────────────────────────────

const appColumns = `
	a.id, a.group_id, a.name, a.description,
	a.src_endpoint, a.src_access_key, a.src_secret_key, a.src_bucket, a.src_region, a.src_use_ssl,
	a.dst_endpoint, a.dst_access_key, a.dst_secret_key, a.dst_bucket, a.dst_region, a.dst_use_ssl,
	a.webhook_secret, a.enabled, a.created_at, a.updated_at`

func (s *SQLiteStore) CreateApp(ctx context.Context, a *group.App) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO apps (
			id, group_id, name, description,
			src_endpoint, src_access_key, src_secret_key, src_bucket, src_region, src_use_ssl,
			dst_endpoint, dst_access_key, dst_secret_key, dst_bucket, dst_region, dst_use_ssl,
			webhook_secret, enabled, created_at, updated_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		a.ID, a.GroupID, a.Name, a.Description,
		a.Src.Endpoint, a.Src.AccessKey, a.Src.SecretKey, a.Src.Bucket, a.Src.Region, boolToInt(a.Src.UseSSL),
		a.Dst.Endpoint, a.Dst.AccessKey, a.Dst.SecretKey, a.Dst.Bucket, a.Dst.Region, boolToInt(a.Dst.UseSSL),
		a.WebhookSecret, boolToInt(a.Enabled),
		a.CreatedAt.Unix(), a.UpdatedAt.Unix(),
	)
	return err
}

func (s *SQLiteStore) GetApp(ctx context.Context, id string) (*group.App, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+appColumns+` FROM apps a WHERE a.id = ?`, id)
	return scanApp(row)
}

// GetAppByRoute is the hot-path lookup used by the webhook handler.
// It resolves {groupSlug}/{appSlug} → full App in one indexed query.
func (s *SQLiteStore) GetAppByRoute(ctx context.Context, groupSlug, appSlug string) (*group.App, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT `+appColumns+`
		 FROM apps a
		 JOIN groups g ON g.id = a.group_id
		 WHERE g.name = ? AND a.name = ?`,
		groupSlug, appSlug)
	return scanApp(row)
}

func (s *SQLiteStore) ListAppsByGroup(ctx context.Context, groupID string) ([]group.App, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+appColumns+` FROM apps a WHERE a.group_id = ? ORDER BY a.name`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []group.App
	for rows.Next() {
		a, err := scanApp(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

func (s *SQLiteStore) UpdateApp(ctx context.Context, a *group.App) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE apps SET
			name=?, description=?,
			src_endpoint=?, src_access_key=?, src_secret_key=?, src_bucket=?, src_region=?, src_use_ssl=?,
			dst_endpoint=?, dst_access_key=?, dst_secret_key=?, dst_bucket=?, dst_region=?, dst_use_ssl=?,
			webhook_secret=?, enabled=?, updated_at=?
		 WHERE id=?`,
		a.Name, a.Description,
		a.Src.Endpoint, a.Src.AccessKey, a.Src.SecretKey, a.Src.Bucket, a.Src.Region, boolToInt(a.Src.UseSSL),
		a.Dst.Endpoint, a.Dst.AccessKey, a.Dst.SecretKey, a.Dst.Bucket, a.Dst.Region, boolToInt(a.Dst.UseSSL),
		a.WebhookSecret, boolToInt(a.Enabled), a.UpdatedAt.Unix(),
		a.ID,
	)
	return err
}

func (s *SQLiteStore) DeleteApp(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM apps WHERE id=?`, id)
	return err
}

func scanApp(row rowScanner) (*group.App, error) {
	var a group.App
	var ca, ua int64
	var srcSSL, dstSSL, enabled int
	err := row.Scan(
		&a.ID, &a.GroupID, &a.Name, &a.Description,
		&a.Src.Endpoint, &a.Src.AccessKey, &a.Src.SecretKey, &a.Src.Bucket, &a.Src.Region, &srcSSL,
		&a.Dst.Endpoint, &a.Dst.AccessKey, &a.Dst.SecretKey, &a.Dst.Bucket, &a.Dst.Region, &dstSSL,
		&a.WebhookSecret, &enabled, &ca, &ua,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Src.UseSSL = srcSSL == 1
	a.Dst.UseSSL = dstSSL == 1
	a.Enabled = enabled == 1
	a.CreatedAt = time.Unix(ca, 0).UTC()
	a.UpdatedAt = time.Unix(ua, 0).UTC()
	return &a, nil
}

// ── Transfer ──────────────────────────────────────────────────────────────────

func (s *SQLiteStore) CreateTransfer(ctx context.Context, t *Transfer) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO transfers (
			id, app_id, object_key, src_bucket, dst_bucket,
			object_size, etag, status, error_message, bytes_transferred,
			started_at, completed_at, duration_ms, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		t.ID, t.AppID, t.ObjectKey, t.SrcBucket, t.DstBucket,
		t.ObjectSize, t.ETag, t.Status, t.ErrorMessage, t.BytesTransferred,
		nullableUnix(t.StartedAt), nullableUnix(t.CompletedAt), t.DurationMs,
		t.CreatedAt.Unix(),
	)
	return err
}

func (s *SQLiteStore) GetTransfer(ctx context.Context, id string) (*Transfer, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, app_id, object_key, src_bucket, dst_bucket,
		        object_size, etag, status, error_message, bytes_transferred,
		        started_at, completed_at, duration_ms, created_at
		 FROM transfers WHERE id=?`, id)
	return scanTransfer(row)
}

func (s *SQLiteStore) UpdateTransfer(ctx context.Context, t *Transfer) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE transfers SET
			status=?, error_message=?, bytes_transferred=?,
			started_at=?, completed_at=?, duration_ms=?, etag=?
		 WHERE id=?`,
		t.Status, t.ErrorMessage, t.BytesTransferred,
		nullableUnix(t.StartedAt), nullableUnix(t.CompletedAt), t.DurationMs, t.ETag,
		t.ID,
	)
	return err
}

func (s *SQLiteStore) ListTransfers(ctx context.Context, opts ListTransfersOpts) ([]Transfer, int64, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}
	if opts.Limit > 500 {
		opts.Limit = 500
	}

	where := []string{"1=1"}
	args := []any{}

	if opts.AppID != "" {
		where = append(where, "t.app_id = ?")
		args = append(args, opts.AppID)
	}
	if opts.GroupID != "" {
		where = append(where, "a.group_id = ?")
		args = append(args, opts.GroupID)
	}
	if opts.Status != "" {
		where = append(where, "t.status = ?")
		args = append(args, opts.Status)
	}

	whereClause := strings.Join(where, " AND ")
	join := ""
	if opts.GroupID != "" {
		join = "JOIN apps a ON a.id = t.app_id"
	}

	// Count
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	var total int64
	row := s.db.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM transfers t %s WHERE %s`, join, whereClause),
		countArgs...)
	_ = row.Scan(&total)

	// Data
	dataArgs := append(args, opts.Limit, opts.Offset)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`
			SELECT t.id, t.app_id, t.object_key, t.src_bucket, t.dst_bucket,
			       t.object_size, t.etag, t.status, t.error_message, t.bytes_transferred,
			       t.started_at, t.completed_at, t.duration_ms, t.created_at
			FROM transfers t %s
			WHERE %s
			ORDER BY t.created_at DESC
			LIMIT ? OFFSET ?`, join, whereClause),
		dataArgs...,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var out []Transfer
	for rows.Next() {
		t, err := scanTransfer(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, *t)
	}
	return out, total, rows.Err()
}

func (s *SQLiteStore) GetStats(ctx context.Context, appID, groupID string) (*TransferStats, error) {
	where := []string{"1=1"}
	args := []any{}
	join := ""

	if appID != "" {
		where = append(where, "t.app_id = ?")
		args = append(args, appID)
	}
	if groupID != "" {
		join = "JOIN apps a ON a.id = t.app_id"
		where = append(where, "a.group_id = ?")
		args = append(args, groupID)
	}

	whereClause := strings.Join(where, " AND ")
	q := fmt.Sprintf(`
		SELECT
			COUNT(*)                                          AS total,
			SUM(CASE WHEN status='success'     THEN 1 ELSE 0 END),
			SUM(CASE WHEN status='failed'      THEN 1 ELSE 0 END),
			SUM(CASE WHEN status='in_progress' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status='pending'     THEN 1 ELSE 0 END),
			COALESCE(SUM(bytes_transferred), 0),
			COALESCE(AVG(CASE WHEN status='success' THEN duration_ms END), 0),
			MAX(completed_at)
		FROM transfers t %s
		WHERE %s`, join, whereClause)

	var stats TransferStats
	var lastCA *int64
	err := s.db.QueryRowContext(ctx, q, args...).Scan(
		&stats.TotalTransfers,
		&stats.SuccessCount,
		&stats.FailedCount,
		&stats.InProgressCount,
		&stats.PendingCount,
		&stats.TotalBytes,
		&stats.AvgDurationMs,
		&lastCA,
	)
	if err != nil {
		return nil, err
	}
	if lastCA != nil {
		t := time.Unix(*lastCA, 0).UTC()
		stats.LastTransferAt = &t
	}
	return &stats, nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func scanTransfer(row rowScanner) (*Transfer, error) {
	var t Transfer
	var startedAt, completedAt *int64
	var createdAt int64
	err := row.Scan(
		&t.ID, &t.AppID, &t.ObjectKey, &t.SrcBucket, &t.DstBucket,
		&t.ObjectSize, &t.ETag, &t.Status, &t.ErrorMessage, &t.BytesTransferred,
		&startedAt, &completedAt, &t.DurationMs, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	t.CreatedAt = time.Unix(createdAt, 0).UTC()
	if startedAt != nil {
		ts := time.Unix(*startedAt, 0).UTC()
		t.StartedAt = &ts
	}
	if completedAt != nil {
		ts := time.Unix(*completedAt, 0).UTC()
		t.CompletedAt = &ts
	}
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullableUnix(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := t.Unix()
	return &v
}
