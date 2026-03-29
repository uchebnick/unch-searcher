package search

type SearchResult struct {
	Path        string
	Line        int
	CommentHash string
	Distance    float64
}
