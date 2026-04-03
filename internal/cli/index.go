package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/uchebnick/unch/internal/embed/llama"
	"github.com/uchebnick/unch/internal/filehashdb"
	"github.com/uchebnick/unch/internal/indexdb"
	"github.com/uchebnick/unch/internal/indexing"
	"github.com/uchebnick/unch/internal/runtime"
	"github.com/uchebnick/unch/internal/semsearch"
	"github.com/uchebnick/unch/internal/termui"
)

func runIndex(ctx context.Context, program string, args []string, paths semsearch.Paths, s *termui.Session, scanner indexing.FileScanner, runtimes runtime.YzmaResolver, models runtime.ModelCache) error {
	var excludes stringListFlag

	defaultDBPath := filepath.Join(paths.LocalDir, "index.db")
	defaultModelPath := runtime.DefaultModelPath(paths.ModelsDir)

	fs := flag.NewFlagSet(program+" index", flag.ContinueOnError)
	fs.SetOutput(nil)
	fs.Usage = func() {}

	root := fs.String("root", ".", "root directory to index")
	dbPath := fs.String("db", defaultDBPath, "path to sqlite db")
	modelPath := fs.String("model", defaultModelPath, "path to GGUF embedding model, or a known model id such as embeddinggemma or qwen3")
	libPath := fs.String("lib", "", "path to yzma library directory, or to one of its shared library files")
	contextPrefix := fs.String("context-prefix", "@filectx:", "legacy file context prefix used only by fallback indexers")
	commentPrefix := fs.String("comment-prefix", "@search:", "legacy comment prefix used only by fallback indexers")
	gitignorePath := fs.String("gitignore", "", "optional path to .gitignore; default is <root>/.gitignore")
	contextSize := fs.Int("ctx-size", 0, "llama context size; 0 uses the selected model default")
	batchSize := fs.Int("batch-size", 0, "llama batch size; 0 uses the selected model default")
	verbose := fs.Bool("verbose", false, "enable yzma verbose logging")
	fs.Var(&excludes, "exclude", "exclude pattern; can be used multiple times")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return printFlagSetHelp(
				os.Stdout,
				fs,
				cliName(program)+" index [flags]",
				"Build or refresh the local search index for a repository.",
				[]string{
					cliName(program) + " index --root .",
					cliName(program) + " index --exclude node_modules --exclude dist",
					cliName(program) + " index --model qwen3",
					cliName(program) + " index --model ~/.semsearch/models/Qwen3-Embedding-0.6B-Q8_0.gguf",
				},
				[]string{
					"Omit --model to auto-download the default embeddinggemma GGUF model.",
					"Known model ids today: embeddinggemma and qwen3.",
					"Changing --model changes the embedding space; rebuild the index before searching with the new model.",
					"--comment-prefix and --context-prefix are legacy fallback knobs for unsupported files or parser failures.",
				},
			)
		}
		return err
	}

	dbWasExplicit := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "db" {
			dbWasExplicit = true
		}
	})

	rootAbs, err := filepath.Abs(*root)
	if err != nil {
		return fmt.Errorf("resolve root: %w", err)
	}

	targetPaths, err := semsearch.PreparePaths(rootAbs)
	if err != nil {
		return err
	}
	if _, err := semsearch.EnsureFileWeights(targetPaths.LocalDir); err != nil {
		return err
	}
	scanner.Root = rootAbs

	currentManifest, err := semsearch.ReadManifest(targetPaths.LocalDir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read manifest: %w", err)
	}
	if err == nil && semsearch.HasRemoteBinding(currentManifest) {
		printSessionLine(s, "Warning: local indexing will detach this repository from remote CI updates and switch the manifest back to local")
		confirmed, confirmErr := confirmRemoteReindex(s, os.Stdin, os.Stderr, s != nil && s.Interactive() && isCharDevice(os.Stdin))
		if confirmErr != nil {
			return confirmErr
		}
		if !confirmed {
			return fmt.Errorf("local reindex canceled; remote CI binding preserved")
		}
	}

	resolvedDBPath := *dbPath
	if !dbWasExplicit {
		resolvedDBPath = filepath.Join(targetPaths.LocalDir, "index.db")
	}

	modelID, err := runtime.CanonicalModelID(*modelPath, defaultModelPath)
	if err != nil {
		return fmt.Errorf("resolve model id: %w", err)
	}
	resolvedGitignore, err := indexing.ResolveGitignorePath(rootAbs, *gitignorePath)
	if err != nil {
		return fmt.Errorf("resolve gitignore: %w", err)
	}

	hashStore, err := filehashdb.Open(ctx, targetPaths.FileHashDB)
	if err != nil {
		return fmt.Errorf("open file hash db: %w", err)
	}
	defer func() {
		_ = hashStore.Close()
	}()

	fileHashes, err := indexing.CollectFileHashes(rootAbs, resolvedGitignore, excludes)
	if err != nil {
		return fmt.Errorf("collect file hashes: %w", err)
	}
	scannerFingerprint := indexing.BuildScannerFingerprint(*commentPrefix, *contextPrefix, excludes)

	s.Logf("file_hashes=%d", len(fileHashes))
	s.Logf("scanner_fingerprint=%s", scannerFingerprint)

	var currentFileHashes map[string]string
	skipped, err := maybeSkipUnchangedIndex(
		ctx,
		targetPaths,
		resolvedDBPath,
		dbWasExplicit,
		currentManifest,
		hashStore,
		modelID,
		scannerFingerprint,
		fileHashes,
		s,
	)
	if err != nil {
		return err
	}
	if skipped {
		return nil
	}

	if currentState, ok, err := hashStore.Current(ctx, modelID); err != nil {
		return fmt.Errorf("read current file hash state: %w", err)
	} else if ok && currentState.ScannerFingerprint == scannerFingerprint {
		currentFileHashes = currentState.Files
	}

	fileHashStateVersion, err := hashStore.BeginState(ctx, modelID, scannerFingerprint)
	if err != nil {
		return fmt.Errorf("begin file hash state: %w", err)
	}
	s.Logf("staged_file_hash_state_version=%d", fileHashStateVersion)

	resolvedLibPath, libNote, err := runtimes.ResolveOrInstallYzmaLibPath(ctx, *libPath, targetPaths.LocalDir, s)
	if err != nil {
		return err
	}
	if libNote != "" {
		s.Logf("%s", libNote)
	}

	resolvedModelPath, modelNote, err := models.ResolveOrInstallModelPath(ctx, *modelPath, defaultModelPath, true, s)
	if err != nil {
		return err
	}
	if modelNote != "" {
		s.Logf("%s", modelNote)
	}

	resolvedContextSize := *contextSize
	if resolvedContextSize <= 0 {
		resolvedContextSize = defaultContextSize(resolvedModelPath)
	}
	resolvedBatchSize := *batchSize
	if resolvedBatchSize <= 0 {
		resolvedBatchSize = defaultBatchSize(resolvedModelPath)
	}

	s.Logf("db=%s", resolvedDBPath)
	s.Logf("lib=%s", resolvedLibPath)
	s.Logf("model=%s", resolvedModelPath)
	s.Logf("model_id=%s", modelID)
	s.Logf("ctx_size=%d", resolvedContextSize)
	s.Logf("batch_size=%d", resolvedBatchSize)
	s.Logf("root=%s", rootAbs)

	embedder, err := loadEmbedderWithSpinner(ctx, s, llamaembed.Config{
		ModelPath:   resolvedModelPath,
		LibPath:     resolvedLibPath,
		ContextSize: resolvedContextSize,
		BatchSize:   resolvedBatchSize,
		Verbose:     *verbose,
		Pooling:     defaultPooling(resolvedModelPath),
	})
	if err != nil {
		return err
	}
	defer embedder.Close()

	repo, err := indexdb.Open(ctx, resolvedDBPath, embedder.Dim())
	if err != nil {
		return err
	}
	defer func() {
		_ = repo.Close()
	}()

	service := indexing.Service{
		Scanner:  scanner,
		Repo:     repo,
		Embedder: embedder,
		Hashes:   hashStore,
	}

	result, err := service.Run(ctx, indexing.Params{
		Root:                 rootAbs,
		GitignorePath:        resolvedGitignore,
		Excludes:             excludes,
		ContextPrefix:        *contextPrefix,
		CommentPrefix:        *commentPrefix,
		ModelID:              modelID,
		CurrentFileHashes:    currentFileHashes,
		NextFileHashes:       fileHashes,
		FileHashStateVersion: fileHashStateVersion,
	}, s)
	if err != nil {
		return err
	}

	manifest, err := semsearch.UpdateIndexManifest(targetPaths.LocalDir, resolvedDBPath, result.Version)
	if err != nil {
		return fmt.Errorf("update manifest: %w", err)
	}
	s.Logf("manifest version=%d indexing_hash=%s", manifest.Version, manifest.IndexingHash)

	if err := hashStore.ActivateState(ctx, modelID, fileHashStateVersion); err != nil {
		return fmt.Errorf("activate file hash state: %w", err)
	}
	if err := hashStore.CleanupInactiveStates(ctx); err != nil {
		return fmt.Errorf("cleanup inactive file hash states: %w", err)
	}
	s.Logf("file_hash_state_version=%d", fileHashStateVersion)

	if result.IndexedSymbols == 0 {
		s.Finish("No symbols found")
		return nil
	}

	s.Finish(fmt.Sprintf("Indexed %d symbols in %d files", result.IndexedSymbols, result.IndexedFiles))
	return nil
}

func maybeSkipUnchangedIndex(
	ctx context.Context,
	paths semsearch.Paths,
	resolvedDBPath string,
	dbWasExplicit bool,
	currentManifest semsearch.Manifest,
	hashStore *filehashdb.Store,
	modelID string,
	scannerFingerprint string,
	fileHashes map[string]string,
	s *termui.Session,
) (bool, error) {
	defaultDBPath := filepath.Join(paths.LocalDir, "index.db")
	if dbWasExplicit || resolvedDBPath != defaultDBPath {
		return false, nil
	}
	if currentManifest.Version <= 0 || semsearch.HasRemoteBinding(currentManifest) {
		return false, nil
	}
	if _, err := os.Stat(resolvedDBPath); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat current index db: %w", err)
	}

	match, fileHashStateVersion, err := hashStore.Matches(ctx, modelID, scannerFingerprint, fileHashes)
	if err != nil {
		return false, fmt.Errorf("compare file hash state: %w", err)
	}
	if !match {
		return false, nil
	}

	if s != nil {
		s.Logf("file_hash_state_version=%d", fileHashStateVersion)
		s.Finish("Index already up to date")
	}
	return true, nil
}
