package indexdb

// @filectx: SQLite repository adapter that stores indexed comment metadata and sqlite-vec embeddings for current-version search.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	sqlite_vec "github.com/asg017/sqlite-vec-go-bindings/cgo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/uchebnick/unch-searcher/internal/search"
)

type Store struct {
	db  *sql.DB
	dim int
}

// @search: Open initializes the SQLite schema and vec0 virtual table used for semantic comment retrieval.
func Open(ctx context.Context, dbPath string, dim int) (*Store, error) {
	sqlite_vec.Auto()

	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping db: %w", err)
	}

	store := &Store{db: db, dim: dim}
	if err := store.init(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init repository: %w", err)
	}

	return store, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) init(ctx context.Context) error {
	stmts := []string{
		`
		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value INTEGER NOT NULL
		);
		`,
		`
		INSERT INTO meta(key, value)
		VALUES ('current_version', 0)
		ON CONFLICT(key) DO NOTHING;
		`,
		`
		CREATE TABLE IF NOT EXISTS comments (
			path TEXT NOT NULL,
			line INTEGER NOT NULL,
			comment_hash TEXT NOT NULL,
			version INTEGER NOT NULL,
			PRIMARY KEY (path, line)
		);
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_comments_version
		ON comments(version);
		`,
		`
		CREATE INDEX IF NOT EXISTS idx_comments_comment_hash
		ON comments(comment_hash);
		`,
	}

	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec schema: %w", err)
		}
	}

	vecStmt := fmt.Sprintf(`
		CREATE VIRTUAL TABLE IF NOT EXISTS embeddings USING vec0(
			comment_hash TEXT PRIMARY KEY,
			embedding FLOAT[%d]
		);
	`, s.dim)
	if _, err := s.db.ExecContext(ctx, vecStmt); err != nil {
		return fmt.Errorf("create embeddings vec0: %w", err)
	}

	return nil
}

func (s *Store) CurrentVersion(ctx context.Context) (int64, error) {
	var version int64
	err := s.db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'current_version'`).Scan(&version)
	if err != nil {
		return 0, fmt.Errorf("select current_version: %w", err)
	}
	return version, nil
}

func (s *Store) WorkingVersion(ctx context.Context) (int64, error) {
	current, err := s.CurrentVersion(ctx)
	if err != nil {
		return 0, err
	}
	return current + 1, nil
}

func (s *Store) ActivateVersion(ctx context.Context, version int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE meta SET value = ? WHERE key = 'current_version'`, version)
	if err != nil {
		return fmt.Errorf("update current_version: %w", err)
	}
	return nil
}

func (s *Store) EmbeddingExists(ctx context.Context, commentHash string) (bool, error) {
	var exists int
	err := s.db.QueryRowContext(ctx, `SELECT 1 FROM embeddings WHERE comment_hash = ? LIMIT 1`, commentHash).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("select embedding: %w", err)
	}
	return true, nil
}

func (s *Store) AddEmbedding(ctx context.Context, commentHash string, embedding []float32) error {
	if len(embedding) != s.dim {
		return fmt.Errorf("invalid embedding dimension: got=%d want=%d", len(embedding), s.dim)
	}

	vec, err := sqlite_vec.SerializeFloat32(embedding)
	if err != nil {
		return fmt.Errorf("serialize embedding: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `INSERT INTO embeddings(comment_hash, embedding) VALUES (?, ?)`, commentHash, vec)
	if err != nil {
		return fmt.Errorf("insert embedding: %w", err)
	}
	return nil
}

func (s *Store) UpsertComment(ctx context.Context, path string, line int, commentHash string, version int64) error {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO comments(path, line, comment_hash, version)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(path, line) DO UPDATE SET
		   comment_hash = excluded.comment_hash,
		   version = excluded.version`,
		path,
		line,
		commentHash,
		version,
	)
	if err != nil {
		return fmt.Errorf("upsert comment: %w", err)
	}
	return nil
}

func (s *Store) CleanupOldVersions(ctx context.Context, activeVersion int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM comments WHERE version < ?`, activeVersion)
	if err != nil {
		return fmt.Errorf("delete old comments: %w", err)
	}
	return nil
}

func (s *Store) CleanupUnusedEmbeddings(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM embeddings
		WHERE comment_hash NOT IN (
			SELECT DISTINCT comment_hash FROM comments
		)`)
	if err != nil {
		return fmt.Errorf("delete unused embeddings: %w", err)
	}
	return nil
}

func (s *Store) SearchByVersion(ctx context.Context, queryEmbedding []float32, version int64, limit int) ([]search.SearchResult, error) {
	if len(queryEmbedding) != s.dim {
		return nil, fmt.Errorf("invalid query dimension: got=%d want=%d", len(queryEmbedding), s.dim)
	}
	if limit <= 0 {
		limit = 10
	}

	queryVec, err := sqlite_vec.SerializeFloat32(queryEmbedding)
	if err != nil {
		return nil, fmt.Errorf("serialize query vector: %w", err)
	}

	rows, err := s.db.QueryContext(
		ctx,
		`SELECT
			c.path,
			c.line,
			c.comment_hash,
			e.distance
		FROM embeddings e
		JOIN comments c ON c.comment_hash = e.comment_hash
		WHERE e.embedding MATCH ?
		  AND k = ?
		  AND c.version = ?
		ORDER BY e.distance ASC`,
		queryVec,
		limit,
		version,
	)
	if err != nil {
		return nil, fmt.Errorf("search query: %w", err)
	}
	defer rows.Close()

	results := make([]search.SearchResult, 0, limit)
	for rows.Next() {
		var item search.SearchResult
		if err := rows.Scan(&item.Path, &item.Line, &item.CommentHash, &item.Distance); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}

	return results, nil
}

func (s *Store) SearchCurrent(ctx context.Context, queryEmbedding []float32, limit int) ([]search.SearchResult, error) {
	version, err := s.CurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return s.SearchByVersion(ctx, queryEmbedding, version, limit)
}

func (s *Store) ListCommentsByVersion(ctx context.Context, version int64) ([]search.SearchResult, error) {
	rows, err := s.db.QueryContext(
		ctx,
		`SELECT path, line, comment_hash
		FROM comments
		WHERE version = ?
		ORDER BY path ASC, line ASC`,
		version,
	)
	if err != nil {
		return nil, fmt.Errorf("list comments: %w", err)
	}
	defer rows.Close()

	var results []search.SearchResult
	for rows.Next() {
		var item search.SearchResult
		if err := rows.Scan(&item.Path, &item.Line, &item.CommentHash); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows err: %w", err)
	}
	return results, nil
}

func (s *Store) ListCurrentComments(ctx context.Context) ([]search.SearchResult, error) {
	version, err := s.CurrentVersion(ctx)
	if err != nil {
		return nil, err
	}
	return s.ListCommentsByVersion(ctx, version)
}
