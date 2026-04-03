package indexing

import (
	"fmt"
	"sort"
	"strings"

	"github.com/cespare/xxhash/v2"
)

const fileHashFingerprintVersion = "index-input-v1"

func CollectFileHashes(root string, gitignorePath string, extraPatterns []string) (map[string]string, error) {
	hashes := make(map[string]string)
	err := walkSourceFiles(root, gitignorePath, extraPatterns, func(_ string, rel string, source []byte) error {
		hashes[rel] = hashSourceBytes(source)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return hashes, nil
}

func BuildScannerFingerprint(commentPrefix string, contextPrefix string, excludes []string) string {
	normalizedExcludes := append([]string(nil), excludes...)
	sort.Strings(normalizedExcludes)

	payload := strings.Join([]string{
		fileHashFingerprintVersion,
		strings.TrimSpace(commentPrefix),
		strings.TrimSpace(contextPrefix),
		strings.Join(normalizedExcludes, "\x00"),
	}, "\x1f")

	return fmt.Sprintf("%s:%016x", fileHashFingerprintVersion, xxhash.Sum64String(payload))
}

func hashSourceBytes(source []byte) string {
	return fmt.Sprintf("%016x", xxhash.Sum64(source))
}
