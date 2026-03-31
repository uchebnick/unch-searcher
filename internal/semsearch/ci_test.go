package semsearch

import (
	"os"
	"strings"
	"testing"
)

func TestEnsureCIWorkflow(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	path, created, err := EnsureCIWorkflow(root)
	if err != nil {
		t.Fatalf("EnsureCIWorkflow() error: %v", err)
	}
	if !created {
		t.Fatalf("expected workflow to be created")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read workflow: %v", err)
	}

	content := string(data)
	if content != DefaultCIWorkflow {
		t.Fatalf("workflow content mismatch")
	}
	if !strings.Contains(content, "permissions:\n  contents: write") {
		t.Fatalf("workflow missing write permissions")
	}
	if !strings.Contains(content, "GITHUB_STEP_SUMMARY") {
		t.Fatalf("workflow missing GitHub summary step")
	}
	if !strings.Contains(content, "workflow_dispatch:\n    inputs:") {
		t.Fatalf("workflow missing workflow_dispatch inputs")
	}
	if !strings.Contains(content, "force_rebuild:") || !strings.Contains(content, "skip_remote_restore:") || !strings.Contains(content, "skip_publish:") {
		t.Fatalf("workflow missing manual dispatch controls")
	}
	if !strings.Contains(content, "export PATH=\"$bin_dir:$PATH\"") {
		t.Fatalf("workflow missing immediate PATH export")
	}
	if !strings.Contains(content, "GITHUB_TOKEN: ${{ github.token }}") {
		t.Fatalf("workflow missing GitHub token env for indexing")
	}
	if !strings.Contains(content, "SEMSEARCH_YZMA_PROCESSOR: cpu") {
		t.Fatalf("workflow missing pinned yzma processor for indexing")
	}
	if !strings.Contains(content, "SEMSEARCH_YZMA_VERSION: b8581") {
		t.Fatalf("workflow missing pinned yzma version for indexing")
	}
	if !strings.Contains(content, ".semsearch/logs/searcher-index.log") {
		t.Fatalf("workflow missing explicit searcher log output")
	}
	if !strings.Contains(content, "unch bind ci --root . \"$ci_url\"") {
		t.Fatalf("workflow missing bind ci step")
	}
	if !strings.Contains(content, "unch remote sync --root . --allow-missing") {
		t.Fatalf("workflow missing remote sync step")
	}
	if !strings.Contains(content, "uses: actions/download-artifact@v4") {
		t.Fatalf("workflow missing publish job artifact download step")
	}
	if !strings.Contains(content, "needs: index") {
		t.Fatalf("workflow missing publish job dependency on index")
	}
	if !strings.Contains(content, "runs-on: ubuntu-latest") {
		t.Fatalf("workflow missing dedicated publish job runner")
	}
	if !strings.Contains(content, "No compatible published remote index was restored; building from scratch") {
		t.Fatalf("workflow missing explicit rebuild notice when no compatible snapshot is restored")
	}
	if !strings.Contains(content, "git push origin HEAD:gh-pages") {
		t.Fatalf("workflow missing gh-pages publish step")
	}
	if !strings.Contains(content, "if-no-files-found: warn") {
		t.Fatalf("workflow missing artifact warning mode")
	}
	if strings.Contains(content, "### Artifact contents") {
		t.Fatalf("workflow summary should not include artifact contents section")
	}
	if !strings.Contains(content, ".github/workflows") && !strings.HasSuffix(path, "searcher.yml") {
		t.Fatalf("unexpected workflow path: %s", path)
	}
}

func TestEnsureCIWorkflowDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	root := t.TempDir()

	path, created, err := EnsureCIWorkflow(root)
	if err != nil {
		t.Fatalf("EnsureCIWorkflow() error: %v", err)
	}
	if !created {
		t.Fatalf("expected first call to create workflow")
	}

	const customWorkflow = "name: custom-searcher\n"
	if err := os.WriteFile(path, []byte(customWorkflow), 0o644); err != nil {
		t.Fatalf("write custom workflow: %v", err)
	}

	gotPath, created, err := EnsureCIWorkflow(root)
	if err != nil {
		t.Fatalf("EnsureCIWorkflow(second call) error: %v", err)
	}
	if created {
		t.Fatalf("expected second call not to overwrite workflow")
	}
	if gotPath != path {
		t.Fatalf("EnsureCIWorkflow() path = %q, want %q", gotPath, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read preserved workflow: %v", err)
	}
	if string(data) != customWorkflow {
		t.Fatalf("workflow was overwritten: %q", string(data))
	}
}
