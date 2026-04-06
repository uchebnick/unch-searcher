package runtime

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	stdruntime "runtime"
	"strings"
	"testing"
)

func TestFetchLatestLlamaVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/hybridgroup/llama-cpp-builder/releases/latest" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Accept"); got != "application/vnd.github+json" {
			t.Fatalf("Accept header = %q", got)
		}
		_, _ = io.WriteString(w, `{"tag_name":"b9999"}`)
	}))
	defer server.Close()

	rewriteDefaultTransportToServer(t, server)

	got, err := fetchLatestLlamaVersion()
	if err != nil {
		t.Fatalf("fetchLatestLlamaVersion() error = %v", err)
	}
	if got != "b9999" {
		t.Fatalf("fetchLatestLlamaVersion() = %q, want %q", got, "b9999")
	}
}

func TestDownloadToTempFileUsesProgressTracker(t *testing.T) {
	const body = "GGUFpayload"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "11")
		_, _ = io.WriteString(w, body)
	}))
	defer server.Close()

	tracker := &recordingProgressTracker{}
	path, err := downloadToTempFile(context.Background(), server.URL+"/model.gguf", t.TempDir(), "model-*.gguf", tracker)
	if err != nil {
		t.Fatalf("downloadToTempFile() error = %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	if got := string(data); got != body {
		t.Fatalf("downloaded body = %q, want %q", got, body)
	}
	if tracker.src != server.URL+"/model.gguf" {
		t.Fatalf("progress src = %q, want %q", tracker.src, server.URL+"/model.gguf")
	}
	if tracker.totalSize != int64(len(body)) {
		t.Fatalf("progress total = %d, want %d", tracker.totalSize, len(body))
	}
}

func TestDownloadAndExtractArchiveTarGz(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, ".tar.gz") {
			t.Fatalf("unexpected archive path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(tarGzBytes(t,
			tarEntry{name: "bundle/libllama.so", body: []byte("payload"), typ: tar.TypeReg, mode: 0o755},
		))
	}))
	defer server.Close()

	destDir := t.TempDir()
	if err := downloadAndExtractArchive(context.Background(), server.URL+"/runtime.tar.gz", destDir, nil); err != nil {
		t.Fatalf("downloadAndExtractArchive() error = %v", err)
	}

	data, err := os.ReadFile(filepath.Join(destDir, "libllama.so"))
	if err != nil {
		t.Fatalf("ReadFile(extracted) error = %v", err)
	}
	if got := string(data); got != "payload" {
		t.Fatalf("extracted body = %q, want %q", got, "payload")
	}
}

func TestDownloadYzmaArchiveReturnsNotFoundError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()

	rewriteDefaultTransportToServer(t, server)

	err := downloadYzmaArchive(context.Background(), "amd64", "linux", processorCPU, "b1234", t.TempDir(), nil)
	if !errors.Is(err, errYzmaArchiveNotFound) {
		t.Fatalf("downloadYzmaArchive() error = %v, want errYzmaArchiveNotFound", err)
	}
}

func TestExtractTarGzRejectsEscapingSymlink(t *testing.T) {
	if stdruntime.GOOS == "windows" {
		t.Skip("symlink extraction tests are unreliable on Windows CI")
	}

	archivePath := filepath.Join(t.TempDir(), "payload.tar.gz")
	writeTarGz(t, archivePath, tarEntry{
		name:     "bundle/libllama.so",
		linkname: "../../escape",
		typ:      tar.TypeSymlink,
		mode:     0o777,
	})

	err := extractTarGz(archivePath, t.TempDir())
	if err == nil {
		t.Fatalf("expected escaping symlink archive to fail")
	}
	if !strings.Contains(err.Error(), "escapes destination") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractTarGzRejectsOverwriteThroughSymlink(t *testing.T) {
	if stdruntime.GOOS == "windows" {
		t.Skip("symlink extraction tests are unreliable on Windows CI")
	}

	archivePath := filepath.Join(t.TempDir(), "payload.tar.gz")
	writeTarGz(t, archivePath,
		tarEntry{name: "bundle/lib", typ: tar.TypeDir, mode: 0o755},
		tarEntry{name: "bundle/lib/llama.so", linkname: "../llama-real.so", typ: tar.TypeSymlink, mode: 0o777},
		tarEntry{name: "bundle/lib/llama.so", body: []byte("oops"), typ: tar.TypeReg, mode: 0o644},
	)

	err := extractTarGz(archivePath, t.TempDir())
	if err == nil {
		t.Fatalf("expected archive to fail when a regular file reuses a symlink path")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExtractTarGzMaterializesSymlinkChain(t *testing.T) {
	if stdruntime.GOOS == "windows" {
		t.Skip("symlink extraction tests are unreliable on Windows CI")
	}

	destDir := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "payload.tar.gz")
	writeTarGz(t, archivePath,
		tarEntry{name: "bundle/libggml.dylib", linkname: "libggml.0.dylib", typ: tar.TypeSymlink, mode: 0o777},
		tarEntry{name: "bundle/libggml.0.dylib", linkname: "libggml.0.9.8.dylib", typ: tar.TypeSymlink, mode: 0o777},
		tarEntry{name: "bundle/libggml.0.9.8.dylib", body: []byte("payload"), typ: tar.TypeReg, mode: 0o755},
	)

	if err := extractTarGz(archivePath, destDir); err != nil {
		t.Fatalf("extractTarGz() error = %v", err)
	}

	for _, name := range []string{"libggml.dylib", "libggml.0.dylib", "libggml.0.9.8.dylib"} {
		path := filepath.Join(destDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if got := string(data); got != "payload" {
			t.Fatalf("ReadFile(%s) = %q, want %q", path, got, "payload")
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Lstat(%s) error = %v", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s should be materialized as a regular file", path)
		}
	}
}

func TestExtractZIPMaterializesSymlinkChain(t *testing.T) {
	destDir := t.TempDir()
	archivePath := filepath.Join(t.TempDir(), "payload.zip")
	writeZIP(t, archivePath,
		zipEntry{name: "bundle/libggml-link", linkTarget: "libggml-real", mode: os.ModeSymlink | 0o777},
		zipEntry{name: "bundle/libggml-real", body: []byte("payload"), mode: 0o755},
	)

	if err := extractZIP(archivePath, destDir); err != nil {
		t.Fatalf("extractZIP() error = %v", err)
	}

	for _, name := range []string{"libggml-link", "libggml-real"} {
		path := filepath.Join(destDir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", path, err)
		}
		if got := string(data); got != "payload" {
			t.Fatalf("ReadFile(%s) = %q, want %q", path, got, "payload")
		}
		info, err := os.Lstat(path)
		if err != nil {
			t.Fatalf("Lstat(%s) error = %v", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s should be materialized as a regular file", path)
		}
	}
}

type tarEntry struct {
	name     string
	body     []byte
	linkname string
	typ      byte
	mode     int64
}

func writeTarGz(t *testing.T, archivePath string, entries ...tarEntry) {
	t.Helper()

	if err := os.WriteFile(archivePath, tarGzBytes(t, entries...), 0o644); err != nil {
		t.Fatalf("write archive file: %v", err)
	}
}

func tarGzBytes(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for _, entry := range entries {
		header := &tar.Header{
			Name:     entry.name,
			Typeflag: entry.typ,
			Mode:     entry.mode,
			Linkname: entry.linkname,
			Size:     int64(len(entry.body)),
		}
		if entry.typ == tar.TypeDir {
			header.Size = 0
		}
		if entry.typ == 0 {
			header.Typeflag = tar.TypeReg
		}
		if err := tw.WriteHeader(header); err != nil {
			t.Fatalf("write tar header %s: %v", entry.name, err)
		}
		if len(entry.body) > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatalf("write tar body %s: %v", entry.name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}
	return buf.Bytes()
}

type zipEntry struct {
	name       string
	body       []byte
	linkTarget string
	mode       os.FileMode
}

func writeZIP(t *testing.T, archivePath string, entries ...zipEntry) {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	for _, entry := range entries {
		header := &zip.FileHeader{
			Name:   entry.name,
			Method: zip.Deflate,
		}
		header.SetMode(entry.mode)

		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatalf("create zip header %s: %v", entry.name, err)
		}

		payload := entry.body
		if entry.mode&os.ModeSymlink != 0 {
			payload = []byte(entry.linkTarget)
		}
		if len(payload) > 0 {
			if _, err := writer.Write(payload); err != nil {
				t.Fatalf("write zip payload %s: %v", entry.name, err)
			}
		}
	}

	if err := zw.Close(); err != nil {
		t.Fatalf("close zip writer: %v", err)
	}
	if err := os.WriteFile(archivePath, buf.Bytes(), 0o644); err != nil {
		t.Fatalf("write zip archive: %v", err)
	}
}

type recordingProgressTracker struct {
	src         string
	currentSize int64
	totalSize   int64
}

func (r *recordingProgressTracker) TrackProgress(src string, currentSize, totalSize int64, stream io.ReadCloser) io.ReadCloser {
	r.src = src
	r.currentSize = currentSize
	r.totalSize = totalSize
	return stream
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func rewriteDefaultTransportToServer(t *testing.T, server *httptest.Server) {
	t.Helper()

	target, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}

	previous := http.DefaultTransport
	baseTransport, ok := previous.(*http.Transport)
	if !ok {
		t.Fatalf("http.DefaultTransport has unexpected type %T", previous)
	}
	cloned := baseTransport.Clone()
	http.DefaultTransport = roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		rewritten := req.Clone(req.Context())
		rewritten.URL.Scheme = target.Scheme
		rewritten.URL.Host = target.Host
		return cloned.RoundTrip(rewritten)
	})
	t.Cleanup(func() {
		http.DefaultTransport = previous
	})
}
