//go:build cgo

package treesitter

// Symbol is the normalized search unit produced by Tree-sitter extraction.
type Symbol struct {
	Line          int
	Kind          string
	Name          string
	Container     string
	QualifiedName string
	Signature     string
	Documentation string
	Body          string
	FileContext   string
}
