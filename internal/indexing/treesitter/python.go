//go:build cgo

package treesitter

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractPythonSymbols(source []byte, root *sitter.Node) []Symbol {
	children := nodeChildren(root)
	var symbols []Symbol

	for idx := range children {
		child := &children[idx]
		switch child.Kind() {
		case "class_definition":
			symbols = append(symbols, extractPythonClassSymbols(child, source, combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)))...)
		case "function_definition":
			docText := combineDocText(
				extractLeadingDoc(children, idx, source),
				extractTrailingComment(children, idx, source),
				pythonDocstring(child, source),
			)
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "function",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		case "expression_statement":
			if assignment := firstNamedChildOfKinds(child, "assignment"); assignment != nil {
				name := firstNamedIdentifierText(assignment, source)
				if isPythonConstantName(name) {
					symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
						Kind: "constant",
						Name: name,
						Doc:  combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
					}))
				}
			}
		}
	}

	return symbols
}

func extractPythonClassSymbols(node *sitter.Node, source []byte, docText string) []Symbol {
	name := nodeFieldText(node, "name", source)
	classSymbol := newSymbolFromNode(node, source, symbolMeta{
		Kind: "class",
		Name: name,
		Doc:  combineDocText(docText, pythonDocstring(node, source)),
	})

	symbols := []Symbol{classSymbol}
	body := firstNamedChildOfKinds(node, "block")
	if body == nil {
		return symbols
	}

	children := nodeChildren(body)
	for idx := range children {
		child := &children[idx]
		if child.Kind() != "function_definition" {
			continue
		}
		symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
			Kind:      "method",
			Name:      nodeFieldText(child, "name", source),
			Container: name,
			Doc: combineDocText(
				extractLeadingDoc(children, idx, source),
				extractTrailingComment(children, idx, source),
				pythonDocstring(child, source),
			),
		}))
	}

	return symbols
}

func pythonDocstring(node *sitter.Node, source []byte) string {
	body := firstNamedChildOfKinds(node, "block")
	if body == nil {
		return ""
	}

	children := nodeChildren(body)
	if len(children) == 0 {
		return ""
	}

	first := &children[0]
	if first.Kind() != "expression_statement" {
		return ""
	}

	str := firstNamedChildOfKinds(first, "string")
	if str == nil {
		return ""
	}

	return cleanPythonStringLiteral(str.Utf8Text(source))
}

func cleanPythonStringLiteral(raw string) string {
	raw = normalizeText(raw)
	for _, quote := range []string{`"""`, `'''`, `"`, `'`} {
		if strings.HasPrefix(raw, quote) && strings.HasSuffix(raw, quote) && len(raw) >= len(quote)*2 {
			raw = strings.TrimPrefix(raw, quote)
			raw = strings.TrimSuffix(raw, quote)
			break
		}
	}

	lines := strings.Split(raw, "\n")
	for i := range lines {
		lines[i] = strings.TrimSpace(lines[i])
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func isPythonConstantName(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' {
			return false
		}
	}
	return true
}
