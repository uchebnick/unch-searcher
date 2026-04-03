package indexing

import (
	"context"
	"fmt"
)

type Reporter interface {
	Logf(format string, args ...any)
	CountProgress(label string, current, total int)
}

type Scanner interface {
	WalkFiles(root string, gitignorePath string, extraPatterns []string, visit func(path string, rel string, source []byte) error) error
	CollectJob(path string, rel string, source []byte, commentPrefix string, contextPrefix string) (FileJob, bool, error)
}

type Repository interface {
	BeginSnapshot(ctx context.Context, modelID string) (int64, error)
	CurrentSnapshotIfAny(ctx context.Context, modelID string) (int64, bool, error)
	ActivateSnapshot(ctx context.Context, modelID string, snapshotID int64) error
	EmbeddingExists(ctx context.Context, modelID string, embeddingHash string) (bool, error)
	AddEmbedding(ctx context.Context, modelID string, embeddingHash string, embedding []float32) error
	InsertSymbol(ctx context.Context, snapshotID int64, modelID string, path string, symbol IndexedSymbol, embeddingHash string) error
	CopyPathFromSnapshot(ctx context.Context, modelID string, srcSnapshotID, dstSnapshotID int64, path string) (int, error)
	CleanupInactiveSnapshots(ctx context.Context) error
	CleanupUnusedEmbeddings(ctx context.Context) error
}

type FileHashStore interface {
	InsertFileHash(ctx context.Context, stateVersion int64, path string, contentHash string) error
}

type Embedder interface {
	IndexedSymbolHash(path string, symbol IndexedSymbol) string
	EmbedIndexedSymbol(path string, symbol IndexedSymbol) ([]float32, error)
}

type Params struct {
	Root                 string
	GitignorePath        string
	Excludes             []string
	ContextPrefix        string
	CommentPrefix        string
	ModelID              string
	CurrentFileHashes    map[string]string
	NextFileHashes       map[string]string
	FileHashStateVersion int64
}

type Result struct {
	Version        int64
	IndexedFiles   int
	IndexedSymbols int
}

type Service struct {
	Scanner  Scanner
	Repo     Repository
	Embedder Embedder
	Hashes   FileHashStore
}

// Run scans the repository, embeds extracted symbols, and activates the new index version.
func (s Service) Run(ctx context.Context, params Params, reporter Reporter) (Result, error) {
	modelID := params.ModelID
	if modelID == "" {
		modelID = "embeddinggemma"
	}

	currentSnapshotID, hasCurrentSnapshot, err := s.Repo.CurrentSnapshotIfAny(ctx, modelID)
	if err != nil {
		return Result{}, fmt.Errorf("read current snapshot: %w", err)
	}

	snapshotID, err := s.Repo.BeginSnapshot(ctx, modelID)
	if err != nil {
		return Result{}, fmt.Errorf("begin snapshot: %w", err)
	}
	if reporter != nil {
		reporter.Logf("model=%s", modelID)
		reporter.Logf("snapshot id=%d", snapshotID)
	}

	totalFiles := len(params.NextFileHashes)
	if totalFiles == 0 && params.Root != "" {
		totalFiles = 1
	}

	reusedFiles := 0
	reusedSymbols := 0
	result := Result{Version: snapshotID}
	processedFiles := 0

	err = s.Scanner.WalkFiles(params.Root, params.GitignorePath, params.Excludes, func(path string, rel string, source []byte) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		contentHash, ok := params.NextFileHashes[rel]
		if !ok {
			contentHash = hashSourceBytes(source)
		}
		if s.Hashes != nil && params.FileHashStateVersion > 0 {
			if err := s.Hashes.InsertFileHash(ctx, params.FileHashStateVersion, rel, contentHash); err != nil {
				return fmt.Errorf("insert file hash for %s: %w", rel, err)
			}
		}

		if hasCurrentSnapshot && params.CurrentFileHashes[rel] == contentHash {
			copiedSymbols, err := s.Repo.CopyPathFromSnapshot(ctx, modelID, currentSnapshotID, snapshotID, rel)
			if err != nil {
				return fmt.Errorf("copy unchanged file %s: %w", rel, err)
			}
			if copiedSymbols > 0 {
				reusedFiles++
				reusedSymbols += copiedSymbols
				result.IndexedFiles++
				result.IndexedSymbols += copiedSymbols
			}
			processedFiles++
			if reporter != nil {
				reporter.CountProgress("Indexing", processedFiles, totalFiles)
			}
			return nil
		}

		job, ok, err := s.Scanner.CollectJob(path, rel, source, params.CommentPrefix, params.ContextPrefix)
		if err != nil {
			return err
		}
		if !ok {
			processedFiles++
			if reporter != nil {
				reporter.CountProgress("Indexing", processedFiles, totalFiles)
			}
			return nil
		}

		for _, symbol := range job.Symbols {
			hash := s.Embedder.IndexedSymbolHash(job.Path, symbol)

			exists, err := s.Repo.EmbeddingExists(ctx, modelID, hash)
			if err != nil {
				return fmt.Errorf("check embedding exists: %w", err)
			}
			if !exists {
				vec, err := s.Embedder.EmbedIndexedSymbol(job.Path, symbol)
				if err != nil {
					return fmt.Errorf("embed symbol at %s:%d: %w", job.Path, symbol.Line, err)
				}
				if err := s.Repo.AddEmbedding(ctx, modelID, hash, vec); err != nil {
					return fmt.Errorf("store embedding: %w", err)
				}
			}

			if err := s.Repo.InsertSymbol(ctx, snapshotID, modelID, job.Path, symbol, hash); err != nil {
				return fmt.Errorf("insert symbol: %w", err)
			}
		}

		result.IndexedFiles++
		result.IndexedSymbols += len(job.Symbols)
		processedFiles++
		if reporter != nil {
			reporter.CountProgress("Indexing", processedFiles, totalFiles)
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}

	if err := s.Repo.ActivateSnapshot(ctx, modelID, snapshotID); err != nil {
		return Result{}, fmt.Errorf("activate snapshot: %w", err)
	}
	if err := s.Repo.CleanupInactiveSnapshots(ctx); err != nil {
		return Result{}, fmt.Errorf("cleanup inactive snapshots: %w", err)
	}
	if err := s.Repo.CleanupUnusedEmbeddings(ctx); err != nil {
		return Result{}, fmt.Errorf("cleanup unused embeddings: %w", err)
	}
	if reporter != nil {
		reporter.Logf("reused files=%d", reusedFiles)
		reporter.Logf("reused symbols=%d", reusedSymbols)
		reporter.Logf("indexing completed")
	}

	return result, nil
}
