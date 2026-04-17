//go:build cgo

package treesitter

import sitter "github.com/tree-sitter/go-tree-sitter"

func extractJSSymbols(source []byte, root *sitter.Node) []Symbol {
	return extractJSLikeSymbols(source, root, false)
}

func extractTSSymbols(source []byte, root *sitter.Node) []Symbol {
	return extractJSLikeSymbols(source, root, true)
}

func extractJSLikeSymbols(source []byte, root *sitter.Node, isTypeScript bool) []Symbol {
	children := nodeChildren(root)
	var symbols []Symbol

	for idx := range children {
		child := &children[idx]
		docText := combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source))

		switch child.Kind() {
		case "export_statement":
			if decl := firstNamedNonCommentChild(child); decl != nil {
				symbols = append(symbols, extractJSTopLevelSymbols(decl, source, docText, "", isTypeScript)...)
			}
		default:
			symbols = append(symbols, extractJSTopLevelSymbols(child, source, docText, "", isTypeScript)...)
		}
	}

	return symbols
}

func extractJSTopLevelSymbols(node *sitter.Node, source []byte, docText string, fileContext string, isTypeScript bool) []Symbol {
	switch node.Kind() {
	case "class_declaration", "class", "abstract_class_declaration":
		return extractJSClassSymbols(node, source, docText, fileContext)
	case "function_declaration", "generator_function_declaration":
		return []Symbol{newSymbolFromNode(node, source, symbolMeta{
			Kind:        "function",
			Name:        nodeFieldText(node, "name", source),
			FileContext: fileContext,
			Doc:         docText,
		})}
	case "interface_declaration":
		return extractTSInterfaceSymbols(node, source, docText, fileContext)
	case "enum_declaration":
		return []Symbol{newSymbolFromNode(node, source, symbolMeta{
			Kind:        "enum",
			Name:        firstNamedIdentifierText(node, source),
			FileContext: fileContext,
			Doc:         docText,
		})}
	case "type_alias_declaration":
		return []Symbol{newSymbolFromNode(node, source, symbolMeta{
			Kind:        "type",
			Name:        nodeFieldText(node, "name", source),
			FileContext: fileContext,
			Doc:         docText,
		})}
	case "lexical_declaration", "variable_declaration":
		return extractJSVariableSymbols(node, source, docText, fileContext)
	default:
		if isTypeScript && node.Kind() == "function_signature" {
			return []Symbol{newSymbolFromNode(node, source, symbolMeta{
				Kind:        "function",
				Name:        nodeFieldText(node, "name", source),
				FileContext: fileContext,
				Doc:         docText,
			})}
		}
	}

	return nil
}

func extractJSClassSymbols(node *sitter.Node, source []byte, docText string, fileContext string) []Symbol {
	name := nodeFieldText(node, "name", source)
	classSymbol := newSymbolFromNode(node, source, symbolMeta{
		Kind:        "class",
		Name:        name,
		FileContext: fileContext,
		Doc:         docText,
	})

	symbols := []Symbol{classSymbol}
	body := firstNamedChildOfKinds(node, "class_body")
	if body == nil {
		return symbols
	}

	children := nodeChildren(body)
	for idx := range children {
		child := &children[idx]
		if child.Kind() != "method_definition" {
			continue
		}
		methodName := firstNamedIdentifierText(child, source)
		if methodName == "constructor" {
			continue
		}
		symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
			Kind:        "method",
			Name:        methodName,
			Container:   name,
			FileContext: fileContext,
			Doc:         combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
		}))
	}

	return symbols
}

func extractTSInterfaceSymbols(node *sitter.Node, source []byte, docText string, fileContext string) []Symbol {
	name := nodeFieldText(node, "name", source)
	symbols := []Symbol{newSymbolFromNode(node, source, symbolMeta{
		Kind:        "interface",
		Name:        name,
		FileContext: fileContext,
		Doc:         docText,
	})}

	body := firstNamedChildOfKinds(node, "interface_body")
	if body == nil {
		return symbols
	}

	children := nodeChildren(body)
	for idx := range children {
		child := &children[idx]
		switch child.Kind() {
		case "method_signature", "abstract_method_signature":
			symbols = append(symbols, newSymbolFromNode(child, source, symbolMeta{
				Kind:        "method",
				Name:        firstNamedIdentifierText(child, source),
				Container:   name,
				FileContext: fileContext,
				Doc:         combineDocText(extractLeadingDoc(children, idx, source), extractTrailingComment(children, idx, source)),
			}))
		}
	}

	return symbols
}

func extractJSVariableSymbols(node *sitter.Node, source []byte, docText string, fileContext string) []Symbol {
	declarationKind := jsVariableDeclarationKind(node)
	var symbols []Symbol

	for _, decl := range childNodesByKind(node, "variable_declarator") {
		declNode := decl
		name := nodeFieldText(&declNode, "name", source)
		if name == "" {
			continue
		}

		value := declNode.ChildByFieldName("value")
		kind := declarationKind
		if value != nil {
			switch value.Kind() {
			case "arrow_function", "function_expression", "generator_function", "generator_function_declaration":
				kind = "function"
			case "class", "class_declaration":
				kind = "class"
			}
		}

		symbol := newSymbolFromNode(&declNode, source, symbolMeta{
			Kind:        kind,
			Name:        name,
			FileContext: fileContext,
			Doc:         combineDocText(docText, extractTrailingComment(nodeChildren(&declNode), 0, source)),
		})
		if value != nil && (value.Kind() == "arrow_function" || value.Kind() == "function_expression" || value.Kind() == "generator_function") {
			symbol.Body = bodySnippet(value, source)
			symbol.Signature = firstNLines(declNode.Utf8Text(source), 2)
		} else if value != nil && kind == "class" {
			symbol.Body = bodySnippet(value, source)
			symbol.Signature = signatureText(&declNode, source)
		} else if value != nil {
			symbol.Body = firstNLines(value.Utf8Text(source), 6)
		}
		symbols = append(symbols, symbol)
	}

	return symbols
}

func jsVariableDeclarationKind(node *sitter.Node) string {
	if node == nil {
		return "variable"
	}
	for _, child := range nodeChildren(node) {
		switch child.Kind() {
		case "const":
			return "constant"
		case "let", "var":
			return "variable"
		}
	}
	return "variable"
}
