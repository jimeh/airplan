package airplan

import (
	"fmt"
	stdhtml "html"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var kindMermaid = ast.NewNodeKind("Mermaid")

type mermaidNode struct {
	ast.BaseBlock
	source string
}

func (n *mermaidNode) Kind() ast.NodeKind { return kindMermaid }

func (n *mermaidNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{}, nil)
}

type mermaidExtension struct{}

func (mermaidExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(mermaidTransformer{}, 100),
	))
	markdown.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(mermaidRenderer{}, 100),
	))
}

type mermaidTransformer struct{}

func (mermaidTransformer) Transform(
	doc *ast.Document,
	reader text.Reader,
	_ parser.Context,
) {
	source := reader.Source()
	var blocks []*ast.FencedCodeBlock
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (
		ast.WalkStatus,
		error,
	) {
		if entering {
			if block, ok := node.(*ast.FencedCodeBlock); ok &&
				isMermaidFence(block, source) {
				blocks = append(blocks, block)
			}
		}
		return ast.WalkContinue, nil
	})
	for _, block := range blocks {
		node := &mermaidNode{source: string(block.Lines().Value(source))}
		block.Parent().ReplaceChild(block.Parent(), block, node)
	}
}

func isMermaidFence(block *ast.FencedCodeBlock, source []byte) bool {
	return block.Info != nil &&
		string(block.Info.Segment.Value(source)) == "mermaid"
}

type mermaidRenderer struct{}

func (mermaidRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindMermaid, renderMermaid)
}

func renderMermaid(
	w util.BufWriter,
	_ []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if entering {
		_, _ = fmt.Fprintf(w,
			"<pre class=\"mermaid\">%s</pre>\n",
			stdhtml.EscapeString(node.(*mermaidNode).source),
		)
	}
	return ast.WalkContinue, nil
}

func hasMermaid(doc ast.Node, _ []byte) bool {
	found := false
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (
		ast.WalkStatus,
		error,
	) {
		if entering && node.Kind() == kindMermaid {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}
