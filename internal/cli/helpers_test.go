package cli

import "testing"

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
