//go:build cgo

package treesitter

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

type symbolMeta struct {
	Kind        string
	Name        string
	Container   string
	FileContext string
	Doc         string
}

func newSymbolFromNode(node *sitter.Node, source []byte, meta symbolMeta) Symbol {
	signature := signatureText(node, source)
	body := bodySnippet(node, source)
	qualifiedName := strings.TrimSpace(meta.Name)
	if meta.Container != "" && meta.Name != "" {
		qualifiedName = meta.Container + "." + meta.Name
	}
	if qualifiedName == "" {
		qualifiedName = strings.TrimSpace(firstNamedIdentifierText(node, source))
	}

	return Symbol{
		Line:          int(node.StartPosition().Row) + 1,
		Kind:          strings.TrimSpace(meta.Kind),
		Name:          strings.TrimSpace(meta.Name),
		Container:     strings.TrimSpace(meta.Container),
		QualifiedName: strings.TrimSpace(qualifiedName),
		Signature:     signature,
		Documentation: strings.TrimSpace(meta.Doc),
		Body:          body,
		FileContext:   strings.TrimSpace(meta.FileContext),
	}
}

func nodeChildren(node *sitter.Node) []sitter.Node {
	if node == nil {
		return nil
	}
	cursor := node.Walk()
	defer cursor.Close()
	return node.Children(cursor)
}

func childNodesByKind(node *sitter.Node, kind string) []sitter.Node {
	var out []sitter.Node
	for _, child := range nodeChildren(node) {
		if child.Kind() == kind {
			out = append(out, child)
		}
	}
	return out
}

func firstNamedNonCommentChild(node *sitter.Node) *sitter.Node {
	for _, child := range nodeChildren(node) {
		if !child.IsNamed() || isCommentNode(child) {
			continue
		}
		childCopy := child
		return &childCopy
	}
	return nil
}

func firstNamedChildMatching(node *sitter.Node, kinds map[string]struct{}) *sitter.Node {
	for _, child := range nodeChildren(node) {
		if _, ok := kinds[child.Kind()]; ok {
			childCopy := child
			return &childCopy
		}
	}
	return nil
}

func firstNamedChildOfKinds(node *sitter.Node, kinds ...string) *sitter.Node {
	allowed := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		allowed[kind] = struct{}{}
	}
	return firstNamedChildMatching(node, allowed)
}

func nodeFieldText(node *sitter.Node, field string, source []byte) string {
	if node == nil {
		return ""
	}
	child := node.ChildByFieldName(field)
	if child == nil {
		return ""
	}
	return normalizeText(child.Utf8Text(source))
}

func firstNamedIdentifierText(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}
	for _, child := range nodeChildren(node) {
		switch child.Kind() {
		case "identifier", "type_identifier", "field_identifier", "property_identifier", "private_property_identifier":
			return normalizeText(child.Utf8Text(source))
		}
	}
	return ""
}

func signatureText(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}

	body := declarationBodyNode(node)
	if body != nil && body.StartByte() > node.StartByte() {
		return firstNLines(string(source[node.StartByte():body.StartByte()]), 4)
	}
	return firstNLines(node.Utf8Text(source), 4)
}

func bodySnippet(node *sitter.Node, source []byte) string {
	if node == nil {
		return ""
	}

	body := declarationBodyNode(node)
	if body != nil {
		return firstNLines(body.Utf8Text(source), 10)
	}
	return ""
}

func declarationBodyNode(node *sitter.Node) *sitter.Node {
	if node == nil {
		return nil
	}

	if child := node.ChildByFieldName("body"); child != nil {
		return child
	}

	for _, kind := range []string{"block", "statement_block", "class_body", "interface_body", "enum_body", "struct_type", "interface_type"} {
		if child := firstNamedChildOfKinds(node, kind); child != nil {
			return child
		}
	}

	return nil
}

func extractLeadingDoc(children []sitter.Node, idx int, source []byte) string {
	if idx <= 0 || idx >= len(children) {
		return ""
	}

	current := children[idx]
	end := idx - 1
	if !isCommentNode(children[end]) {
		return ""
	}
	if hasBlankLineBetween(children[end], current, source) {
		return ""
	}

	start := end
	for start-1 >= 0 && isCommentNode(children[start-1]) {
		prev := children[start-1]
		next := children[start]
		if hasBlankLineBetween(prev, next, source) {
			break
		}
		start--
	}

	var parts []string
	for i := start; i <= end; i++ {
		text := cleanCommentText(children[i].Utf8Text(source))
		if text != "" {
			parts = append(parts, text)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func extractTrailingComment(children []sitter.Node, idx int, source []byte) string {
	if idx >= 0 && idx < len(children) {
		current := children[idx]
		for _, child := range nodeChildren(&current) {
			if isCommentNode(child) && int(child.StartPosition().Row) == int(current.StartPosition().Row) {
				return cleanCommentText(child.Utf8Text(source))
			}
		}

		if idx+1 < len(children) {
			next := children[idx+1]
			if isCommentNode(next) && int(next.StartPosition().Row) == int(current.StartPosition().Row) {
				return cleanCommentText(next.Utf8Text(source))
			}
		}
	}

	return ""
}

func cleanCommentText(raw string) string {
	raw = normalizeText(raw)
	if raw == "" {
		return ""
	}

	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "///")
		line = strings.TrimPrefix(line, "/*!")
		line = strings.TrimPrefix(line, "/**")
		line = strings.TrimPrefix(line, "//!")
		line = strings.TrimPrefix(line, "//")
		line = strings.TrimPrefix(line, "#")
		line = strings.TrimPrefix(line, "/*")
		line = strings.TrimPrefix(line, "*/")
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSuffix(line, "*/")
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func combineDocText(parts ...string) string {
	var filtered []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, "\n")
}

func isCommentNode(node sitter.Node) bool {
	return strings.HasSuffix(node.Kind(), "comment")
}

func hasBlankLineBetween(prev sitter.Node, next sitter.Node, source []byte) bool {
	if prev.EndByte() >= next.StartByte() {
		return false
	}
	return strings.Count(string(source[prev.EndByte():next.StartByte()]), "\n") > 1
}

func firstNLines(text string, maxLines int) string {
	text = normalizeText(text)
	if text == "" || maxLines <= 0 {
		return ""
	}

	lines := strings.Split(text, "\n")
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func normalizeText(text string) string {
	return strings.TrimSpace(strings.ReplaceAll(text, "\r\n", "\n"))
}
