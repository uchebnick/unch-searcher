package semsearch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Paths struct {
	LocalDir     string
	ManifestPath string
	ModelsDir    string
}

func PreparePaths(root string) (Paths, error) {
	localDir := filepath.Join(root, ".semsearch")
	globalDir, err := globalSemsearchDir()
	if err != nil {
		return Paths{}, err
	}
	modelsDir := filepath.Join(globalDir, "models")

	if err := os.MkdirAll(localDir, 0o755); err != nil {
		return Paths{}, fmt.Errorf("create local dir: %w", err)
	}
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		return Paths{}, fmt.Errorf("create global models dir: %w", err)
	}

	return Paths{
		LocalDir:     localDir,
		ManifestPath: ManifestFilePath(localDir),
		ModelsDir:    modelsDir,
	}, nil
}

func globalSemsearchDir() (string, error) {
	if custom := strings.TrimSpace(os.Getenv("SEMSEARCH_HOME")); custom != "" {
		return filepath.Abs(custom)
	}

	cacheDir, err := os.UserCacheDir()
	if err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "unch"), nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(homeDir) == "" {
		return "", fmt.Errorf("resolve global semsearch dir: %w", err)
	}

	return filepath.Join(homeDir, ".semsearch"), nil
}
