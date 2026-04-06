package runtime

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestStdoutProgressTrackerReportsReadAndClose(t *testing.T) {
	restoreStdout, readOutput := captureStdout(t)

	reader := (&stdoutProgressTracker{}).TrackProgress(
		"https://example.com/models/file.gguf",
		0,
		int64(len("payload")),
		io.NopCloser(strings.NewReader("payload")),
	)

	data, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got := string(data); got != "payload" {
		t.Fatalf("ReadAll() = %q, want %q", got, "payload")
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	restoreStdout()
	output := readOutput()
	if !strings.Contains(output, "downloading file.gguf... 100% (7 B/7 B)") {
		t.Fatalf("progress output = %q", output)
	}
}

func TestProgressFormattingHelpers(t *testing.T) {
	if got := formatDownloadProgress("", 2048, 0); got != "downloading file... 2 KiB" {
		t.Fatalf("formatDownloadProgress(unknown total) = %q", got)
	}

	if got := formatDownloadProgress("https://example.com/archive.tar.gz", 2048, 1024); got != "downloading archive.tar.gz... 100% (2 KiB/2 KiB)" {
		t.Fatalf("formatDownloadProgress(clamped total) = %q", got)
	}

	if got := humanDownloadBytes(3 << 20); got != "3.0 MiB" {
		t.Fatalf("humanDownloadBytes(3 MiB) = %q", got)
	}
}

func captureStdout(t *testing.T) (func(), func() string) {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe() error = %v", err)
	}
	os.Stdout = writer

	restore := func() {
		_ = writer.Close()
		os.Stdout = original
	}
	readOutput := func() string {
		data, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("ReadAll(stdout pipe) error = %v", err)
		}
		return string(data)
	}

	return restore, readOutput
}
