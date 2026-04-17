package indexing

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractSymbolsForPathGo(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.go")
	source := `// Package docs.
package demo

// Store docs.
type Store interface {
	// Get docs.
	Get(id string) string
}

// Run docs.
func Run() {}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	symbols, err := extractSymbolsForPath(path, "@search:", "@filectx:")
	if err != nil {
		t.Fatalf("extractSymbolsForPath() error: %v", err)
	}
	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %+v", len(symbols), symbols)
	}
	if symbols[0].QualifiedName != "Store" || symbols[0].Kind != "interface" {
		t.Fatalf("unexpected type symbol %+v", symbols[0])
	}
	if symbols[1].QualifiedName != "Store.Get" || symbols[1].Kind != "method" {
		t.Fatalf("unexpected method symbol %+v", symbols[1])
	}
	if symbols[2].QualifiedName != "Run" || symbols[2].FileContext != "Package docs." {
		t.Fatalf("unexpected function symbol %+v", symbols[2])
	}
}

func TestExtractSymbolsForPathTypeScript(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.ts")
	source := `/** Client docs. */
export class Client {
  // Run docs.
  run(): void {}
}

export interface Store {
  get(id: string): string
}

export const parse = (value: string) => value
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	symbols, err := extractSymbolsForPath(path, "@search:", "@filectx:")
	if err != nil {
		t.Fatalf("extractSymbolsForPath() error: %v", err)
	}
	if len(symbols) != 5 {
		t.Fatalf("expected 5 symbols, got %d: %+v", len(symbols), symbols)
	}
	if symbols[0].QualifiedName != "Client" || symbols[1].QualifiedName != "Client.run" {
		t.Fatalf("unexpected class symbols %+v", symbols[:2])
	}
	if symbols[2].QualifiedName != "Store" || symbols[3].QualifiedName != "Store.get" {
		t.Fatalf("unexpected interface symbols %+v", symbols[2:4])
	}
	if symbols[4].QualifiedName != "parse" || symbols[4].Kind != "function" {
		t.Fatalf("unexpected variable function symbol %+v", symbols[4])
	}
}

func TestExtractSymbolsForPathPython(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.py")
	source := `class Client:
    """Client docs."""
    # Run docs.
    def run(self):
        """Run body docs."""
        pass

def add(a, b):
    return a + b
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	symbols, err := extractSymbolsForPath(path, "@search:", "@filectx:")
	if err != nil {
		t.Fatalf("extractSymbolsForPath() error: %v", err)
	}
	if len(symbols) != 3 {
		t.Fatalf("expected 3 symbols, got %d: %+v", len(symbols), symbols)
	}
	if symbols[0].QualifiedName != "Client" || symbols[0].Documentation != "Client docs." {
		t.Fatalf("unexpected class symbol %+v", symbols[0])
	}
	if symbols[1].QualifiedName != "Client.run" || symbols[1].Documentation != "Run docs.\nRun body docs." {
		t.Fatalf("unexpected method symbol %+v", symbols[1])
	}
	if symbols[2].QualifiedName != "add" {
		t.Fatalf("unexpected function symbol %+v", symbols[2])
	}
}

func TestExtractSymbolsForPathRust(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.rs")
	source := `/// Store docs.
pub trait Store {
    /// Get docs.
    fn get(&self, id: &str) -> String;
}

/// PgStore docs.
pub struct PgStore {
    pool: usize,
}

impl PgStore {
    /// Delete docs.
    pub fn delete(&self, id: &str) {}
}

pub fn parse(value: &str) -> &str {
    value
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	symbols, err := extractSymbolsForPath(path, "@search:", "@filectx:")
	if err != nil {
		t.Fatalf("extractSymbolsForPath() error: %v", err)
	}
	if len(symbols) != 5 {
		t.Fatalf("expected 5 symbols, got %d: %+v", len(symbols), symbols)
	}
	if symbols[0].QualifiedName != "Store" || symbols[0].Kind != "interface" || symbols[0].Documentation != "Store docs." {
		t.Fatalf("unexpected trait symbol %+v", symbols[0])
	}
	if symbols[1].QualifiedName != "Store.get" || symbols[1].Kind != "method" || symbols[1].Documentation != "Get docs." {
		t.Fatalf("unexpected trait method symbol %+v", symbols[1])
	}
	if symbols[2].QualifiedName != "PgStore" || symbols[2].Kind != "struct" || symbols[2].Documentation != "PgStore docs." {
		t.Fatalf("unexpected struct symbol %+v", symbols[2])
	}
	if symbols[3].QualifiedName != "PgStore.delete" || symbols[3].Kind != "method" || symbols[3].Documentation != "Delete docs." {
		t.Fatalf("unexpected impl method symbol %+v", symbols[3])
	}
	if symbols[4].QualifiedName != "parse" || symbols[4].Kind != "function" {
		t.Fatalf("unexpected function symbol %+v", symbols[4])
	}
}

func TestExtractSymbolsForPathFallsBackToLegacyOnUnsupportedExtension(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "sample.sql")
	source := "-- @search: query entrypoint\nSELECT 1;\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatalf("write sample: %v", err)
	}

	symbols, err := extractSymbolsForPath(path, "@search:", "@filectx:")
	if err != nil {
		t.Fatalf("extractSymbolsForPath() error: %v", err)
	}
	if len(symbols) != 1 || symbols[0].Kind != "annotation" {
		t.Fatalf("expected legacy annotation symbol, got %+v", symbols)
	}
}
