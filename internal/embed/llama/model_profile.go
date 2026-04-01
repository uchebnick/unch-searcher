package llamaembed

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hybridgroup/yzma/pkg/llama"
	"github.com/uchebnick/unch/internal/indexing"
)

const (
	embeddingGemmaRetrievalQueryPrefix = "task: code retrieval | query: "
	embeddingGemmaDocumentPrefix       = "title: %s | text: %s"

	qwen3CodeRetrievalInstruction = "Given a code search query, retrieve relevant code symbols and documentation that answer the query."
)

type ModelProfile struct {
	ID              string
	DisplayName     string
	Aliases         []string
	DefaultFilename string
	DownloadURL     string
}

type embeddingModel interface {
	Profile() ModelProfile
	ID() string
	DefaultPooling() llama.PoolingType
	Matches(modelPath string) bool
	FormatQuery(text string) string
	FormatIndexedSymbolDocument(path string, symbol indexing.IndexedSymbol) string
}

type embeddingGemmaModel struct{}

type qwen3EmbeddingModel struct{}

var embeddingModels = []embeddingModel{
	qwen3EmbeddingModel{},
	embeddingGemmaModel{},
}

// DefaultModelProfile returns the model profile used when --model is omitted.
func DefaultModelProfile() ModelProfile {
	return embeddingGemmaModel{}.Profile()
}

// KnownModelProfiles returns the built-in GGUF embedding model profiles supported by the CLI.
func KnownModelProfiles() []ModelProfile {
	profiles := make([]ModelProfile, 0, len(embeddingModels))
	for _, model := range embeddingModels {
		profiles = append(profiles, model.Profile())
	}
	return profiles
}

// ResolveKnownModelProfile resolves a short model alias such as "embeddinggemma" or "qwen3".
func ResolveKnownModelProfile(value string) (ModelProfile, bool) {
	token := normalizeModelToken(value)
	if token == "" {
		return ModelProfile{}, false
	}

	for _, model := range embeddingModels {
		profile := model.Profile()
		for _, alias := range profile.Aliases {
			if token == normalizeModelToken(alias) {
				return profile, true
			}
		}
	}

	return ModelProfile{}, false
}

// RecognizeModelProfileForPath returns a built-in model profile when the path matches a known GGUF filename family.
func RecognizeModelProfileForPath(modelPath string) (ModelProfile, bool) {
	for _, model := range embeddingModels {
		if model.Matches(modelPath) {
			return model.Profile(), true
		}
	}
	return ModelProfile{}, false
}

// DefaultPoolingForModelPath returns the pooling mode that matches the known GGUF embedding model.
func DefaultPoolingForModelPath(modelPath string) llama.PoolingType {
	return modelForPath(modelPath).DefaultPooling()
}

func modelForPath(modelPath string) embeddingModel {
	for _, model := range embeddingModels {
		if model.Matches(modelPath) {
			return model
		}
	}

	return embeddingGemmaModel{}
}

func (embeddingGemmaModel) Profile() ModelProfile {
	return ModelProfile{
		ID:              "embeddinggemma",
		DisplayName:     "embeddinggemma-300m",
		Aliases:         []string{"default", "embeddinggemma", "gemma", "embeddinggemma-300m"},
		DefaultFilename: "embeddinggemma-300m.gguf",
		DownloadURL:     "https://huggingface.co/ggml-org/embeddinggemma-300M-GGUF/resolve/main/embeddinggemma-300M-Q8_0.gguf?download=true",
	}
}

func (embeddingGemmaModel) ID() string {
	return "embeddinggemma-v4"
}

func (embeddingGemmaModel) DefaultPooling() llama.PoolingType {
	return llama.PoolingTypeMean
}

func (embeddingGemmaModel) Matches(modelPath string) bool {
	name, full := normalizedModelPath(modelPath)
	return strings.Contains(name, "embeddinggemma") || strings.Contains(full, "embeddinggemma")
}

func (embeddingGemmaModel) FormatQuery(text string) string {
	text = normalizeText(text)
	return embeddingGemmaRetrievalQueryPrefix + text
}

func (embeddingGemmaModel) FormatIndexedSymbolDocument(path string, symbol indexing.IndexedSymbol) string {
	title, body := indexedSymbolDocumentParts(path, symbol)
	return fmt.Sprintf(embeddingGemmaDocumentPrefix, normalizeDocumentTitle(title), normalizeText(body))
}

func (qwen3EmbeddingModel) Profile() ModelProfile {
	return ModelProfile{
		ID:              "qwen3",
		DisplayName:     "Qwen3-Embedding-0.6B",
		Aliases:         []string{"qwen3", "qwen3-embedding", "qwen3embedding", "qwen"},
		DefaultFilename: "Qwen3-Embedding-0.6B-Q8_0.gguf",
		DownloadURL:     "https://huggingface.co/Qwen/Qwen3-Embedding-0.6B-GGUF/resolve/main/Qwen3-Embedding-0.6B-Q8_0.gguf?download=true",
	}
}

func (qwen3EmbeddingModel) ID() string {
	return "qwen3-embedding-v1"
}

func (qwen3EmbeddingModel) DefaultPooling() llama.PoolingType {
	return llama.PoolingTypeLast
}

func (qwen3EmbeddingModel) Matches(modelPath string) bool {
	name, full := normalizedModelPath(modelPath)
	return strings.Contains(name, "qwen3-embedding") ||
		strings.Contains(name, "qwen3embedding") ||
		(strings.Contains(full, "qwen3") && strings.Contains(full, "embed"))
}

func (qwen3EmbeddingModel) FormatQuery(text string) string {
	return formatQwen3Query(qwen3CodeRetrievalInstruction, text)
}

func (qwen3EmbeddingModel) FormatIndexedSymbolDocument(path string, symbol indexing.IndexedSymbol) string {
	title, body := indexedSymbolDocumentParts(path, symbol)
	return formatQwen3Document(title, body)
}

func indexedSymbolDocumentParts(path string, symbol indexing.IndexedSymbol) (string, string) {
	kind := normalizeText(symbol.Kind)
	name := normalizeText(symbol.Name)
	qualifiedName := normalizeText(symbol.QualifiedName)
	signature := normalizeText(symbol.Signature)
	documentation := normalizeText(symbol.Documentation)
	fileContext := normalizeText(symbol.FileContext)
	bodyText := normalizeText(symbol.Body)

	var body strings.Builder
	body.WriteString("Path: ")
	body.WriteString(path)
	if kind != "" {
		body.WriteString("\nKind: ")
		body.WriteString(kind)
	}
	if name != "" {
		body.WriteString("\nName: ")
		body.WriteString(name)
	}
	if qualifiedName != "" && qualifiedName != name {
		body.WriteString("\nQualified name: ")
		body.WriteString(qualifiedName)
	}
	if signature != "" {
		body.WriteString("\nSignature:\n")
		body.WriteString(signature)
	}
	if documentation != "" {
		body.WriteString("\nDocumentation:\n")
		body.WriteString(documentation)
	}
	if fileContext != "" {
		body.WriteString("\nFile context:\n")
		body.WriteString(fileContext)
	}
	if bodyText != "" {
		body.WriteString("\nBody snippet:\n")
		body.WriteString(bodyText)
	}

	title := normalizeDocumentTitle(strings.TrimSpace(filepath.Base(path)))
	if qualifiedName != "" {
		title = normalizeDocumentTitle(strings.TrimSpace(title + " " + qualifiedName))
	}
	if title == "" || title == "." || title == string(filepath.Separator) {
		title = "symbol"
	}

	return title, body.String()
}

func formatQwen3Query(instruction string, query string) string {
	instruction = normalizeText(instruction)
	query = normalizeText(query)
	return fmt.Sprintf("Instruct: %s\nQuery: %s", instruction, query)
}

func formatQwen3Document(title string, text string) string {
	title = normalizeDocumentTitle(title)
	text = normalizeText(text)

	switch {
	case title == "" && text == "":
		return ""
	case title == "":
		return text
	case text == "":
		return "Title: " + title
	default:
		return "Title: " + title + "\n" + text
	}
}

func normalizeDocumentTitle(title string) string {
	title = normalizeText(title)
	title = strings.ReplaceAll(title, "|", "/")
	if title == "" {
		return "none"
	}
	return title
}

func normalizedModelPath(modelPath string) (string, string) {
	full := strings.ToLower(strings.TrimSpace(modelPath))
	name := strings.ToLower(filepath.Base(full))
	return name, full
}

func normalizeModelToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("-", "", "_", "", " ", "")
	return replacer.Replace(value)
}
