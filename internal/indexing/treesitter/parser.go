//go:build cgo

package treesitter

import (
	"path/filepath"
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
	tsgo "github.com/tree-sitter/tree-sitter-go/bindings/go"
	tsjavascript "github.com/tree-sitter/tree-sitter-javascript/bindings/go"
	tspython "github.com/tree-sitter/tree-sitter-python/bindings/go"
	tsrust "github.com/tree-sitter/tree-sitter-rust/bindings/go"
	tstypescript "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

type spec struct {
	language func() *sitter.Language
	extract  func([]byte, *sitter.Node) []Symbol
}

var specs = map[string]spec{
	".go":  {language: func() *sitter.Language { return sitter.NewLanguage(tsgo.Language()) }, extract: extractGoSymbols},
	".js":  {language: func() *sitter.Language { return sitter.NewLanguage(tsjavascript.Language()) }, extract: extractJSSymbols},
	".jsx": {language: func() *sitter.Language { return sitter.NewLanguage(tsjavascript.Language()) }, extract: extractJSSymbols},
	".mjs": {language: func() *sitter.Language { return sitter.NewLanguage(tsjavascript.Language()) }, extract: extractJSSymbols},
	".cjs": {language: func() *sitter.Language { return sitter.NewLanguage(tsjavascript.Language()) }, extract: extractJSSymbols},
	".ts":  {language: func() *sitter.Language { return sitter.NewLanguage(tstypescript.LanguageTypescript()) }, extract: extractTSSymbols},
	".tsx": {language: func() *sitter.Language { return sitter.NewLanguage(tstypescript.LanguageTSX()) }, extract: extractTSSymbols},
	".py":  {language: func() *sitter.Language { return sitter.NewLanguage(tspython.Language()) }, extract: extractPythonSymbols},
	".rs":  {language: func() *sitter.Language { return sitter.NewLanguage(tsrust.Language()) }, extract: extractRustSymbols},
}

// Extract parses the file with the matching grammar and returns symbols only
// when the parse completed without syntax errors.
func Extract(path string, source []byte) ([]Symbol, bool) {
	spec, ok := specs[strings.ToLower(filepath.Ext(path))]
	if !ok {
		return nil, false
	}

	parser := sitter.NewParser()
	defer parser.Close()

	if err := parser.SetLanguage(spec.language()); err != nil {
		return nil, false
	}

	tree := parser.Parse(source, nil)
	if tree == nil {
		return nil, false
	}
	defer tree.Close()

	root := tree.RootNode()
	if root == nil || root.HasError() {
		return nil, false
	}

	return spec.extract(source, root), true
}
