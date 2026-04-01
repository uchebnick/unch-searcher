package bench

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	appsearch "github.com/uchebnick/unch/internal/search"
)

var commitSHAPattern = regexp.MustCompile(`^[a-f0-9]{40}$`)

func LoadSuite(path string) (Suite, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Suite{}, fmt.Errorf("read suite: %w", err)
	}

	var suite Suite
	if err := json.Unmarshal(data, &suite); err != nil {
		return Suite{}, fmt.Errorf("decode suite: %w", err)
	}
	if err := suite.Validate(); err != nil {
		return Suite{}, err
	}

	return suite, nil
}

func SuiteRevision(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read suite for revision: %w", err)
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (s Suite) Validate() error {
	if strings.TrimSpace(s.ID) == "" {
		return fmt.Errorf("suite id is required")
	}
	if s.Version <= 0 {
		return fmt.Errorf("suite version must be greater than zero")
	}
	if strings.TrimSpace(s.Name) == "" {
		return fmt.Errorf("suite name is required")
	}
	if len(s.Repositories) == 0 {
		return fmt.Errorf("suite must include at least one repository")
	}

	seenRepos := make(map[string]struct{}, len(s.Repositories))
	for i, repo := range s.Repositories {
		repoID := strings.TrimSpace(repo.ID)
		if repoID == "" {
			return fmt.Errorf("repository #%d has empty id", i+1)
		}
		if _, exists := seenRepos[repoID]; exists {
			return fmt.Errorf("repository %q is duplicated", repoID)
		}
		seenRepos[repoID] = struct{}{}

		if strings.TrimSpace(repo.URL) == "" {
			return fmt.Errorf("repository %q has empty url", repoID)
		}
		if !commitSHAPattern.MatchString(strings.TrimSpace(repo.Commit)) {
			return fmt.Errorf("repository %q must pin a 40-character commit sha", repoID)
		}
		if strings.TrimSpace(repo.Language) == "" {
			return fmt.Errorf("repository %q has empty language", repoID)
		}
		if len(repo.Queries) == 0 {
			return fmt.Errorf("repository %q must define at least one query", repoID)
		}

		seenQueries := make(map[string]struct{}, len(repo.Queries))
		for j, query := range repo.Queries {
			queryID := strings.TrimSpace(query.ID)
			if queryID == "" {
				return fmt.Errorf("repository %q query #%d has empty id", repoID, j+1)
			}
			if _, exists := seenQueries[queryID]; exists {
				return fmt.Errorf("repository %q query %q is duplicated", repoID, queryID)
			}
			seenQueries[queryID] = struct{}{}

			if strings.TrimSpace(query.Text) == "" {
				return fmt.Errorf("repository %q query %q has empty text", repoID, queryID)
			}

			mode, err := appsearch.NormalizeMode(query.Mode)
			if err != nil {
				return fmt.Errorf("repository %q query %q: %w", repoID, queryID, err)
			}
			if mode == "" {
				return fmt.Errorf("repository %q query %q has empty mode", repoID, queryID)
			}

			if len(query.ExpectedHits) == 0 {
				return fmt.Errorf("repository %q query %q must define at least one expected hit", repoID, queryID)
			}

			for _, hit := range query.ExpectedHits {
				if !strings.Contains(strings.TrimSpace(hit), ":") {
					return fmt.Errorf("repository %q query %q has invalid expected hit %q", repoID, queryID, hit)
				}
			}
		}
	}

	return nil
}

func exactRef(path string, line int) string {
	return fmt.Sprintf("%s:%d", strings.TrimSpace(path), line)
}
