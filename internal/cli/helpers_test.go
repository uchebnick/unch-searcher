package cli

import (
	"bytes"
	"strings"
	"testing"

	appsearch "github.com/uchebnick/unch/internal/search"
)

func TestFormatSearchResultPath(t *testing.T) {
	t.Parallel()

	if got := formatSearchResultPath("/tmp/project", "/tmp/project/internal/cli.go"); got != "internal/cli.go" {
		t.Fatalf("formatSearchResultPath returned %q", got)
	}
	if got := formatSearchResultPath("/tmp/project", "internal/cli.go"); got != "internal/cli.go" {
		t.Fatalf("formatSearchResultPath(relative) returned %q", got)
	}
	if got := formatSearchResultPath("/tmp/project", "/etc/hosts"); got != "/etc/hosts" {
		t.Fatalf("formatSearchResultPath for external path = %q, want absolute path", got)
	}
}

func TestCompactSearchField(t *testing.T) {
	t.Parallel()

	got := compactSearchField(" line one\n\n line   two ", 20)
	if got != "line one line two" {
		t.Fatalf("compactSearchField() = %q", got)
	}

	got = compactSearchField("abcdefghijklmnopqrstuvwxyz", 10)
	if got != "abcdefg..." {
		t.Fatalf("compactSearchField() truncated = %q", got)
	}
}

func TestRenderSearchResultCompact(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	result := appsearch.Result{
		SearchResult: appsearch.SearchResult{
			Path: "internal/cli/search.go",
			Line: 42,
		},
		DisplayMetric: "lexical",
	}

	if err := renderSearchResultCompact(&out, 1, "/tmp/project", result); err != nil {
		t.Fatalf("renderSearchResultCompact() error: %v", err)
	}

	if got := out.String(); got != " 1. internal/cli/search.go:42  lexical\n" {
		t.Fatalf("renderSearchResultCompact() = %q", got)
	}
}

func TestRenderSearchResultDetailed(t *testing.T) {
	t.Parallel()

	var out bytes.Buffer
	result := appsearch.Result{
		SearchResult: appsearch.SearchResult{
			Path:          "internal/search/service.go",
			Line:          15,
			Kind:          "method",
			Name:          "Run",
			QualifiedName: "Service.Run",
			Signature:     "func (s Service) Run(ctx context.Context, params Params, reporter Reporter) ([]Result, error)",
			Documentation: "Run executes lexical or semantic search and returns ranked symbol matches.",
			Body:          "line one\nline two\nline three",
		},
		DisplayMetric: "0.7747",
	}

	if err := renderSearchResultDetailed(&out, 1, "/tmp/project", result); err != nil {
		t.Fatalf("renderSearchResultDetailed() error: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		" 1. internal/search/service.go:15  0.7747\n",
		"   kind: method\n",
		"   name: Run\n",
		"   qualified: Service.Run\n",
		"   signature: func (s Service) Run(ctx context.Context, params Params, reporter Reporter) ([]Result, error)\n",
		"   docs: Run executes lexical or semantic search and returns ranked symbol matches.\n",
		"   body: line one line two line three\n",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderSearchResultDetailed() missing %q in %q", want, got)
		}
	}
}
