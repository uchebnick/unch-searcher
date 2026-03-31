package search

type SearchResult struct {
	Path          string
	Line          int
	SymbolID      string
	Kind          string
	Name          string
	Container     string
	QualifiedName string
	Signature     string
	Documentation string
	Body          string
	Distance      float64
}
