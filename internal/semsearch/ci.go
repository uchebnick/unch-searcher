package semsearch

import (
	"fmt"
	"os"
	"path/filepath"
)

const DefaultCIWorkflow = `name: searcher

on:
  push:
    branches:
      - main
  workflow_dispatch:
    inputs:
      force_rebuild:
        description: Ignore the published snapshot and rebuild the index from scratch
        required: false
        default: false
        type: boolean
      skip_remote_restore:
        description: Skip downloading the published snapshot before indexing
        required: false
        default: false
        type: boolean
      skip_publish:
        description: Build the index but do not publish it back to gh-pages
        required: false
        default: false
        type: boolean

permissions:
  contents: write

jobs:
  index:
    name: build-search-index
    runs-on: macos-14
    env:
      FORCE_REBUILD: ${{ github.event.inputs.force_rebuild || 'false' }}
      SKIP_REMOTE_RESTORE: ${{ github.event.inputs.skip_remote_restore || 'false' }}

    steps:
      - name: Checkout
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25.3'

      - name: Build unch from source
        shell: bash
        run: |
          set -euo pipefail
          tool_dir="${RUNNER_TEMP}/unch-source"
          bin_dir="${RUNNER_TEMP}/unch-bin"
          probe_dir="${RUNNER_TEMP}/unch-probe"
          rm -rf "$tool_dir" "$bin_dir" "$probe_dir"
          mkdir -p "$bin_dir"
          git clone --depth 1 https://github.com/uchebnick/unch.git "$tool_dir"
          (
            cd "$tool_dir"
            go build -trimpath -o "$bin_dir/unch" .
          )
          export PATH="$bin_dir:$PATH"
          echo "$bin_dir" >> "$GITHUB_PATH"
          echo "::group::Tooling"
          command -v unch
          unch create ci --root "$probe_dir" >/dev/null 2>&1
          find "$probe_dir" -maxdepth 3 -type f | sort
          echo "::endgroup::"

      - name: Restore published remote index
        env:
          GITHUB_TOKEN: ${{ github.token }}
        shell: bash
        run: |
          set -euo pipefail
          ci_url="https://github.com/${GITHUB_REPOSITORY}/actions/workflows/searcher.yml"
          mkdir -p .semsearch/logs
          echo "::group::Bind CI manifest"
          unch bind ci --root . "$ci_url"
          cat .semsearch/manifest.json
          echo "::endgroup::"
          echo "::group::Restore published remote index"
          if [ "${FORCE_REBUILD}" = "true" ]; then
            echo "::notice::Force rebuild requested; skipping published remote index restore"
          elif [ "${SKIP_REMOTE_RESTORE}" = "true" ]; then
            echo "::notice::Published remote index restore skipped by workflow input"
          else
            unch remote sync --root . --allow-missing
            if [ -f .semsearch/index.db ]; then
              echo "::notice::Using the published remote index as the starting snapshot"
            else
              echo "::notice::No compatible published remote index was restored; building from scratch"
            fi
          fi
          if [ -f .semsearch/manifest.json ]; then
            cat .semsearch/manifest.json
          fi
          echo "::endgroup::"

      - name: Build local search index
        env:
          GITHUB_TOKEN: ${{ github.token }}
          SEMSEARCH_YZMA_PROCESSOR: cpu
          SEMSEARCH_YZMA_VERSION: b8581
        shell: bash
        run: |
          set -euo pipefail
          mkdir -p .semsearch/logs
          echo "::group::unch init"
          unch init --root .
          echo "::endgroup::"
          echo "::group::unch index"
          unch index --root . 2>&1 | tee .semsearch/logs/searcher-index.log
          echo "::endgroup::"
          echo "::group::Bind remote manifest"
          ci_url="https://github.com/${GITHUB_REPOSITORY}/actions/workflows/searcher.yml"
          unch bind ci --root . "$ci_url"
          cat .semsearch/manifest.json
          echo "::endgroup::"
          echo "::group::Generated search artifacts"
          find .semsearch -maxdepth 2 -type f | sort
          echo
          ls -lah .semsearch
          echo "::endgroup::"
          echo "::group::Manifest"
          cat .semsearch/manifest.json
          echo "::endgroup::"

      - name: Render GitHub summary
        if: ${{ always() }}
        shell: bash
        run: |
          set -euo pipefail
          {
            echo "## Search Index"
            echo
            echo "- Repository: <code>${GITHUB_REPOSITORY}</code>"
            echo "- Ref: <code>${GITHUB_REF_NAME}</code>"
            echo "- Commit: <code>${GITHUB_SHA::7}</code>"
            echo
            echo "### Manifest"
            echo
            echo '<pre>'
            if [ -f .semsearch/manifest.json ]; then
              cat .semsearch/manifest.json
            else
              echo "{}"
            fi
            echo '</pre>'
            if [ -f .semsearch/logs/searcher-index.log ]; then
              echo
              echo "### Index log tail"
              echo
              echo '<pre>'
              tail -n 80 .semsearch/logs/searcher-index.log
              echo '</pre>'
            fi
          } >> "$GITHUB_STEP_SUMMARY"

      - name: Upload search index
        if: ${{ always() }}
        uses: actions/upload-artifact@v4
        with:
          name: semsearch-index
          path: |
            .semsearch/index.db
            .semsearch/manifest.json
            .semsearch/logs/
          if-no-files-found: warn

  publish:
    name: publish-search-index
    runs-on: ubuntu-latest
    needs: index
    if: ${{ needs.index.result == 'success' && github.event.inputs.skip_publish != 'true' }}

    steps:
      - name: Download search index artifact
        uses: actions/download-artifact@v4
        with:
          name: semsearch-index
          path: semsearch-artifact

      - name: Publish remote search index
        env:
          GITHUB_TOKEN: ${{ github.token }}
        shell: bash
        run: |
          set -euo pipefail
          publish_dir="${RUNNER_TEMP}/gh-pages"
          repo_url="https://x-access-token:${GITHUB_TOKEN}@github.com/${GITHUB_REPOSITORY}.git"
          artifact_dir="${PWD}/semsearch-artifact"
          rm -rf "$publish_dir"
          git config user.name "github-actions[bot]"
          git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
          if git ls-remote --exit-code --heads "$repo_url" gh-pages >/dev/null 2>&1; then
            git clone --depth 1 --branch gh-pages "$repo_url" "$publish_dir"
          else
            git clone --depth 1 "$repo_url" "$publish_dir"
            (
              cd "$publish_dir"
              git checkout --orphan gh-pages
              git rm -rf . >/dev/null 2>&1 || true
            )
          fi
          mkdir -p "$publish_dir/semsearch"
          cp "$artifact_dir/index.db" "$publish_dir/semsearch/index.db"
          cp "$artifact_dir/manifest.json" "$publish_dir/semsearch/manifest.json"
          echo "::group::Publish payload"
          find "$publish_dir/semsearch" -maxdepth 1 -type f | sort
          echo "::endgroup::"
          (
            cd "$publish_dir"
            git add semsearch/index.db semsearch/manifest.json
            if git diff --cached --quiet; then
              echo "No gh-pages changes to publish."
            else
              git commit -m "Publish semsearch index for ${GITHUB_SHA}"
              git push origin HEAD:gh-pages
            fi
          )
`

// CIWorkflowPath returns the generated workflow location under .github/workflows.
func CIWorkflowPath(root string) string {
	return filepath.Join(root, ".github", "workflows", "searcher.yml")
}

// EnsureCIWorkflow writes the default search-index workflow without overwriting an existing file.
func EnsureCIWorkflow(root string) (string, bool, error) {
	path := CIWorkflowPath(root)
	if _, err := os.Stat(path); err == nil {
		return path, false, nil
	} else if !os.IsNotExist(err) {
		return "", false, fmt.Errorf("stat %s: %w", path, err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", false, fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(DefaultCIWorkflow), 0o644); err != nil {
		return "", false, fmt.Errorf("write %s: %w", path, err)
	}
	return path, true, nil
}
