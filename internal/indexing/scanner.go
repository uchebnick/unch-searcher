package indexing

// @filectx: Filesystem scanner that skips runtime state, extracts @search and @filectx directives, and reads result text from source files.

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	ignore "github.com/sabhiram/go-gitignore"
)

type FileScanner struct {
	Root string
}

// @search: CollectJobs walks the repository with .gitignore support, skips .git and .semsearch, and returns only files with indexable directives.
func (FileScanner) CollectJobs(root string, gitignorePath string, extraPatterns []string, commentPrefix string, contextPrefix string) ([]FileJob, int, error) {
	matcher, err := buildIgnoreMatcher(gitignorePath, extraPatterns)
	if err != nil {
		return nil, 0, fmt.Errorf("build ignore matcher: %w", err)
	}

	var jobs []FileJob
	totalComments := 0

	err = filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return fmt.Errorf("make relative path: %w", err)
		}
		rel = filepath.ToSlash(rel)

		if rel == "." {
			return nil
		}

		if shouldSkipIndexedPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if matcher != nil && matcher.MatchesPath(rel) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.IsDir() {
			return nil
		}

		comments, _, err := ExtractPrefixedBlocks(path, commentPrefix, contextPrefix)
		if err != nil {
			return fmt.Errorf("extract comments from %s: %w", path, err)
		}
		if len(comments) == 0 {
			return nil
		}

		jobs = append(jobs, FileJob{
			Path:          rel,
			SourcePath:    path,
			CommentsCount: len(comments),
		})
		totalComments += len(comments)
		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return jobs, totalComments, nil
}

func (s FileScanner) ReadSearchResultContent(path string, line int, commentPrefix string, contextPrefix string) (string, string, error) {
	localPath := s.resolvePath(path)

	comments, context, err := ExtractPrefixedBlocks(localPath, commentPrefix, contextPrefix)
	if err == nil {
		for _, comment := range comments {
			if comment.Line == line {
				return comment.Text, context, nil
			}
		}
	}

	data, readErr := os.ReadFile(localPath)
	if readErr != nil {
		if err != nil {
			return "", "", err
		}
		return "", "", readErr
	}

	lines := strings.Split(normalizeText(string(data)), "\n")
	if line <= 0 || line > len(lines) {
		if err != nil {
			return "", "", err
		}
		return "", strings.TrimSpace(context), nil
	}

	text := lines[line-1]
	if payload, ok := extractDirectivePayload(text, commentPrefix); ok {
		text = payload
	} else {
		text = strings.TrimSpace(text)
	}

	if err != nil {
		return text, "", err
	}
	return text, strings.TrimSpace(context), nil
}

func (s FileScanner) ExtractPrefixedBlocks(path string, searchPrefix string, ctxPrefix string) ([]IndexedComment, string, error) {
	return ExtractPrefixedBlocks(s.resolvePath(path), searchPrefix, ctxPrefix)
}

func (s FileScanner) resolvePath(path string) string {
	path = filepath.Clean(path)
	if filepath.IsAbs(path) || s.Root == "" {
		return path
	}
	return filepath.Join(s.Root, path)
}

func ResolveGitignorePath(root string, gitignorePath ...string) (string, error) {
	switch len(gitignorePath) {
	case 0:
		return filepath.Join(root, ".gitignore"), nil
	case 1:
		p := strings.TrimSpace(gitignorePath[0])
		if p == "" {
			return filepath.Join(root, ".gitignore"), nil
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(root, p)
		}
		return filepath.Clean(p), nil
	default:
		return "", fmt.Errorf("expected at most one gitignore path, got %d", len(gitignorePath))
	}
}

func buildIgnoreMatcher(gitignorePath string, extraPatterns []string) (*ignore.GitIgnore, error) {
	_, err := os.Stat(gitignorePath)
	switch {
	case err == nil:
		if len(extraPatterns) > 0 {
			return ignore.CompileIgnoreFileAndLines(gitignorePath, extraPatterns...)
		}
		return ignore.CompileIgnoreFile(gitignorePath)
	case os.IsNotExist(err):
		if len(extraPatterns) == 0 {
			return nil, nil
		}
		return ignore.CompileIgnoreLines(extraPatterns...), nil
	default:
		return nil, err
	}
}

func shouldSkipIndexedPath(rel string) bool {
	rel = strings.Trim(strings.TrimSpace(filepath.ToSlash(rel)), "/")
	if rel == "" || rel == "." {
		return false
	}

	base := strings.ToLower(strings.TrimSpace(filepath.Base(rel)))
	if strings.HasPrefix(base, "readme") {
		return true
	}

	top := rel
	if idx := strings.IndexByte(top, '/'); idx >= 0 {
		top = top[:idx]
	}

	switch top {
	case ".git", ".semsearch":
		return true
	default:
		return false
	}
}

func ExtractPrefixedBlocks(path string, searchPrefix string, ctxPrefix string) ([]IndexedComment, string, error) {
	const indexedTrailingLines = 10

	file, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer file.Close()

	isBinary, err := looksLikeBinaryFile(file)
	if err != nil {
		return nil, "", err
	}
	if isBinary {
		return nil, "", nil
	}

	var comments []IndexedComment
	var commentsContext strings.Builder
	var lines []string

	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	lineN := 0

	for scanner.Scan() {
		lineN++

		line := scanner.Text()
		lines = append(lines, line)

		if payload, ok := extractDirectivePayload(line, searchPrefix); ok {
			if payload != "" {
				comments = append(comments, IndexedComment{Line: lineN, Text: payload})
			}
			continue
		}

		if payload, ok := extractDirectivePayload(line, ctxPrefix); ok {
			if payload != "" {
				comments = append(comments, IndexedComment{Line: lineN, Text: payload})
				if commentsContext.Len() > 0 {
					commentsContext.WriteByte('\n')
				}
				commentsContext.WriteString(payload)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		if err == bufio.ErrTooLong {
			return nil, "", nil
		}
		return nil, "", err
	}

	for idx := range comments {
		comments[idx].FollowingText = collectFollowingLines(lines, comments[idx].Line, indexedTrailingLines)
	}

	return comments, commentsContext.String(), nil
}

func collectFollowingLines(lines []string, line int, limit int) string {
	if limit <= 0 || line <= 0 || line >= len(lines) {
		return ""
	}

	start := line
	end := start + limit
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end {
		return ""
	}

	return strings.Join(lines[start:end], "\n")
}

func extractDirectivePayload(line string, prefix string) (string, bool) {
	candidate := normalizeDirectiveLine(line)
	if candidate == "" {
		return "", false
	}

	if payload, ok := matchDirectivePrefix(candidate, prefix); ok {
		return payload, true
	}

	trimmedPrefix := strings.TrimSuffix(strings.TrimSpace(prefix), ":")
	if trimmedPrefix != "" && trimmedPrefix != prefix {
		return matchDirectivePrefix(candidate, trimmedPrefix)
	}

	return "", false
}

func matchDirectivePrefix(candidate string, prefix string) (string, bool) {
	if !strings.HasPrefix(candidate, prefix) {
		return "", false
	}

	payload := strings.TrimSpace(strings.TrimPrefix(candidate, prefix))
	if strings.HasPrefix(payload, ":") {
		payload = strings.TrimSpace(strings.TrimPrefix(payload, ":"))
	}
	payload = strings.TrimSpace(strings.TrimSuffix(payload, "*/"))
	return payload, true
}

func normalizeDirectiveLine(line string) string {
	candidate := strings.TrimSpace(line)
	for {
		updated := strings.TrimSpace(strings.TrimSuffix(candidate, "*/"))

		switch {
		case strings.HasPrefix(updated, "//"):
			candidate = strings.TrimSpace(strings.TrimPrefix(updated, "//"))
		case strings.HasPrefix(updated, "/*"):
			candidate = strings.TrimSpace(strings.TrimPrefix(updated, "/*"))
		case strings.HasPrefix(updated, "*"):
			candidate = strings.TrimSpace(strings.TrimPrefix(updated, "*"))
		case strings.HasPrefix(updated, "#"):
			candidate = strings.TrimSpace(strings.TrimPrefix(updated, "#"))
		case strings.HasPrefix(updated, "--"):
			candidate = strings.TrimSpace(strings.TrimPrefix(updated, "--"))
		case strings.HasPrefix(updated, ";"):
			candidate = strings.TrimSpace(strings.TrimPrefix(updated, ";"))
		default:
			return updated
		}
	}
}

func looksLikeBinaryFile(file *os.File) (bool, error) {
	buf := make([]byte, 8192)
	n, err := file.Read(buf)
	if err != nil && err != io.EOF {
		return false, fmt.Errorf("read probe: %w", err)
	}

	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return false, fmt.Errorf("reset probe offset: %w", err)
	}

	return looksLikeBinary(buf[:n]), nil
}

func looksLikeBinary(data []byte) bool {
	if len(data) == 0 {
		return false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return true
	}

	suspicious := 0
	for _, b := range data {
		if b < 0x20 && b != '\n' && b != '\r' && b != '\t' && b != '\f' {
			suspicious++
		}
	}

	return suspicious*100/len(data) > 10
}

func normalizeText(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}
