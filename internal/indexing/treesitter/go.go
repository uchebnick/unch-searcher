//go:build cgo

package treesitter

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractGoSymbols(source []byte, root *sitter.Node) []Symbol {
	children := nodeChildren(root)
	fileContext := ""
	for idx, child := range children {
		if child.Kind() == "package_clause" {
			fileContext = extractLeadingDoc(children, idx, source)
			break
		}
	}

	var symbols []Symbol
	for idx := range children {
		child := &children[idx]
		switch child.Kind() {
		case "function_declaration":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind:        "function",
				Name:        nodeFieldText(child, "name", source),
				FileContext: fileContext,
				Doc:         combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
			}))
		case "method_declaration":
			container := goReceiverName(child, source)
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind:        "method",
				Name:        nodeFieldText(child, "name", source),
				Container:   container,
				FileContext: fileContext,
				Doc:         combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
			}))
		case "type_declaration":
			symbols = append(symbols, extractGoTypeDeclarationSymbols(child, source, fileContext, extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source))...)
		case "const_declaration":
			symbols = append(symbols, extractGoValueSymbols(child, source, fileContext, "constant", extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source))...)
		case "var_declaration":
			symbols = append(symbols, extractGoValueSymbols(child, source, fileContext, "variable", extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source))...)
		}
	}

	return symbols
}

func extractGoTypeDeclarationSymbols(node *sitter.Node, source []byte, fileContext string, leadingDoc string, trailing string) []Symbol {
	var symbols []Symbol
	typeSpecs := childNodesByKind(node, "type_spec")
	docText := combineDocText(leadingDoc, trailing)

	for _, spec := range typeSpecs {
		typeNode := firstNamedChildOfKinds(&spec, "struct_type", "interface_type")
		kind := "type"
		if typeNode != nil {
			switch typeNode.Kind() {
			case "struct_type":
				kind = "struct"
			case "interface_type":
				kind = "interface"
			}
		}

		specNode := spec
		name := nodeFieldText(&specNode, "name", source)
		symbols = append(symbols, newSymbolFromNode(&specNode, source, symbolMeta{
			Kind:        kind,
			Name:        name,
			FileContext: fileContext,
			Doc:         docText,
		}))

		if typeNode != nil && typeNode.Kind() == "interface_type" {
			container := name
			children := nodeChildren(typeNode)
			for idx := range children {
				child := &children[idx]
				if child.Kind() != "method_elem" {
					continue
				}
				symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
					Kind:        "method",
					Name:        nodeFieldText(child, "name", source),
					Container:   container,
					FileContext: fileContext,
					Doc:         combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
				}))
			}
		}
	}

	return symbols
}

func extractGoValueSymbols(node *sitter.Node, source []byte, fileContext string, kind string, leadingDoc string, trailing string) []Symbol {
	var symbols []Symbol
	docText := combineDocText(leadingDoc, trailing)

	for _, specKind := range []string{"const_spec", "var_spec"} {
		for _, spec := range childNodesByKind(node, specKind) {
			names := childNodesByKind(&spec, "identifier")
			for _, nameNode := range names {
				text := normalizeText(nameNode.Utf8Text(source))
				if text == "" {
					continue
				}
				specNode := spec
				symbols = append(symbols, newSymbolFromNode(&specNode, source, symbolMeta{
					Kind:        kind,
					Name:        text,
					FileContext: fileContext,
					Doc:         docText,
				}))
			}
		}
	}

	return symbols
}

func goReceiverName(node *sitter.Node, source []byte) string {
	receiver := node.NamedChild(0)
	if receiver == nil {
		return ""
	}
	text := normalizeText(receiver.Utf8Text(source))
	text = strings.TrimPrefix(text, "(")
	text = strings.TrimSuffix(text, ")")
	text = strings.ReplaceAll(text, "*", "")
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	return fields[len(fields)-1]
}
