package indexing

type IndexedComment struct {
	Line          int
	Text          string
	FollowingText string
}

type FileJob struct {
	Path          string
	CommentsCount int
}
