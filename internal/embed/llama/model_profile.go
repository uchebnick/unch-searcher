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

type embeddingModel interface {
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
