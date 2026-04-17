//go:build cgo

package treesitter

import (
	"strings"

	sitter "github.com/tree-sitter/go-tree-sitter"
)

func extractRustSymbols(source []byte, root *sitter.Node) []Symbol {
	children := nodeChildren(root)
	var symbols []Symbol

	for idx := range children {
		child := &children[idx]
		docText := combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source))

		switch child.Kind() {
		case "function_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "function",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		case "struct_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "struct",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		case "enum_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "enum",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		case "trait_item":
			symbols = append(symbols, extractRustTraitSymbols(child, source, docText)...)
		case "impl_item":
			symbols = append(symbols, extractRustImplSymbols(child, source, docText)...)
		case "type_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "type",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		case "const_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "constant",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		case "static_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind: "variable",
				Name: nodeFieldText(child, "name", source),
				Doc:  docText,
			}))
		}
	}

	return symbols
}

func extractRustTraitSymbols(node *sitter.Node, source []byte, docText string) []Symbol {
	name := nodeFieldText(node, "name", source)
	symbols := []Symbol{newSymbolFromNode(node, source, symbolMeta{
		Kind: "interface",
		Name: name,
		Doc:  docText,
	})}

	body := node.ChildByFieldName("body")
	if body == nil {
		return symbols
	}

	children := nodeChildren(body)
	for idx := range children {
		child := &children[idx]
		switch child.Kind() {
		case "function_signature_item", "function_item":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind:      "method",
				Name:      nodeFieldText(child, "name", source),
				Container: name,
				Doc:       combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
			}))
		}
	}

	return symbols
}

func extractRustImplSymbols(node *sitter.Node, source []byte, docText string) []Symbol {
	body := node.ChildByFieldName("body")
	if body == nil {
		return nil
	}

	container := rustImplContainerName(node, source)
	children := nodeChildren(body)
	var symbols []Symbol
	for idx := range children {
		child := &children[idx]
		if child.Kind() != "function_item" {
			continue
		}
		symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
			Kind:      "method",
			Name:      nodeFieldText(child, "name", source),
			Container: container,
			Doc:       combineDocText(docText, extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
		}))
	}

	return symbols
}

func rustImplContainerName(node *sitter.Node, source []byte) string {
	typeText := nodeFieldText(node, "type", source)
	typeText = strings.TrimSpace(strings.TrimPrefix(typeText, "&"))
	typeText = strings.TrimSpace(strings.TrimPrefix(typeText, "mut "))
	if typeText == "" {
		return ""
	}
	return typeText
}
