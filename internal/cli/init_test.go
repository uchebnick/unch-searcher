package cli

import (
	"testing"
)

func TestResolveInitRoot(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		rootFlag   string
		positional []string
		want       string
		wantErr    bool
	}{
		{name: "default root", rootFlag: ".", want: "."},
		{name: "explicit root flag", rootFlag: "./repo", want: "./repo"},
		{name: "positional root", rootFlag: ".", positional: []string{"./repo"}, want: "./repo"},
		{name: "conflicting root", rootFlag: "./repo", positional: []string{"."}, wantErr: true},
		{name: "too many args", rootFlag: ".", positional: []string{".", "./other"}, wantErr: true},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveInitRoot(tt.rootFlag, tt.positional)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveInitRoot() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("resolveInitRoot() = %q, want %q", got, tt.want)
			}
		})
	}
}
