package indexing

// @filectx: Indexing use case that scans repository files, embeds annotations, and activates a new search version.

import (
	"context"
	"fmt"
)

type Reporter interface {
	Logf(format string, args ...any)
	CountProgress(label string, current, total int)
}

type Scanner interface {
	CollectJobs(root string, gitignorePath string, extraPatterns []string, commentPrefix string, contextPrefix string) ([]FileJob, int, error)
	ExtractPrefixedBlocks(path string, searchPrefix string, ctxPrefix string) ([]IndexedComment, string, error)
}

type Repository interface {
	WorkingVersion(ctx context.Context) (int64, error)
	ActivateVersion(ctx context.Context, version int64) error
	EmbeddingExists(ctx context.Context, commentHash string) (bool, error)
	AddEmbedding(ctx context.Context, commentHash string, embedding []float32) error
	UpsertComment(ctx context.Context, path string, line int, commentHash string, version int64) error
	CleanupOldVersions(ctx context.Context, activeVersion int64) error
	CleanupUnusedEmbeddings(ctx context.Context) error
}

type Embedder interface {
	EmbedIndexedComment(path string, comment string, commentContext string, followingText string) (string, []float32, error)
}

type Params struct {
	Root          string
	GitignorePath string
	Excludes      []string
	ContextPrefix string
	CommentPrefix string
}

type Result struct {
	Version         int64
	IndexedFiles    int
	IndexedComments int
}

type Service struct {
	Scanner  Scanner
	Repo     Repository
	Embedder Embedder
}

// @search: Run collects annotated files, reuses stored embeddings by hash, and writes the next active repository version.
func (s Service) Run(ctx context.Context, params Params, reporter Reporter) (Result, error) {
	jobs, totalComments, err := s.Scanner.CollectJobs(
		params.Root,
		params.GitignorePath,
		params.Excludes,
		params.CommentPrefix,
		params.ContextPrefix,
	)
	if err != nil {
		return Result{}, fmt.Errorf("collect jobs: %w", err)
	}
	if reporter != nil {
		reporter.Logf("files to index=%d", len(jobs))
		reporter.Logf("comments to index=%d", totalComments)
	}

	workingVersion, err := s.Repo.WorkingVersion(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("get working version: %w", err)
	}
	if reporter != nil {
		reporter.Logf("working version=%d", workingVersion)
	}

	processed := 0
	for _, job := range jobs {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		default:
		}

		sourcePath := job.SourcePath
		if sourcePath == "" {
			sourcePath = job.Path
		}

		comments, commentContext, err := s.Scanner.ExtractPrefixedBlocks(sourcePath, params.CommentPrefix, params.ContextPrefix)
		if err != nil {
			return Result{}, fmt.Errorf("extract blocks from %s: %w", job.Path, err)
		}

		for _, comment := range comments {
			hash, vec, err := s.Embedder.EmbedIndexedComment(job.Path, comment.Text, commentContext, comment.FollowingText)
			if err != nil {
				return Result{}, fmt.Errorf("embed comment at %s:%d: %w", job.Path, comment.Line, err)
			}

			exists, err := s.Repo.EmbeddingExists(ctx, hash)
			if err != nil {
				return Result{}, fmt.Errorf("check embedding exists: %w", err)
			}
			if !exists {
				if err := s.Repo.AddEmbedding(ctx, hash, vec); err != nil {
					return Result{}, fmt.Errorf("store embedding: %w", err)
				}
			}

			if err := s.Repo.UpsertComment(ctx, job.Path, comment.Line, hash, workingVersion); err != nil {
				return Result{}, fmt.Errorf("upsert comment: %w", err)
			}
		}

		processed += job.CommentsCount
		if reporter != nil {
			reporter.CountProgress("Indexing", processed, totalComments)
		}
	}

	if err := s.Repo.ActivateVersion(ctx, workingVersion); err != nil {
		return Result{}, fmt.Errorf("activate version: %w", err)
	}
	if err := s.Repo.CleanupOldVersions(ctx, workingVersion); err != nil {
		return Result{}, fmt.Errorf("cleanup old versions: %w", err)
	}
	if err := s.Repo.CleanupUnusedEmbeddings(ctx); err != nil {
		return Result{}, fmt.Errorf("cleanup unused embeddings: %w", err)
	}
	if reporter != nil {
		reporter.Logf("indexing completed")
	}

	return Result{
		Version:         workingVersion,
		IndexedFiles:    len(jobs),
		IndexedComments: totalComments,
	}, nil
}
