//go:build cgo
// +build cgo

package indexing

import ts "github.com/uchebnick/unch/internal/indexing/treesitter"

func extractTreeSitterSymbols(path string, source []byte) ([]IndexedSymbol, bool) {
	symbols, ok := ts.Extract(path, source)
	if !ok {
		return nil, false
	}

	out := make([]IndexedSymbol, 0, len(symbols))
	for _, symbol := range symbols {
		out = append(out, IndexedSymbol{
			Line:          symbol.Line,
			Kind:          symbol.Kind,
			Name:          symbol.Name,
			Container:     symbol.Container,
			QualifiedName: symbol.QualifiedName,
			Signature:     symbol.Signature,
			Documentation: symbol.Documentation,
			Body:          symbol.Body,
			FileContext:   symbol.FileContext,
		})
	}
	return out, true
}
