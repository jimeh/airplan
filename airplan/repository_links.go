package airplan

import (
	"bytes"
	"net/url"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var repositoryReference = regexp.MustCompile(
	`[A-Za-z0-9](?:[A-Za-z0-9-]{0,37}[A-Za-z0-9])?/[A-Za-z0-9._-]+#[1-9][0-9]*|#[1-9][0-9]*|[0-9a-fA-F]{40}`,
)

type repositoryLinkExtension struct {
	base string
	host string
}

func (e repositoryLinkExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithASTTransformers(
		util.Prioritized(e, 200),
	))
}

func (e repositoryLinkExtension) Transform(
	document *ast.Document,
	reader text.Reader,
	_ parser.Context,
) {
	source := reader.Source()
	var nodes []*ast.Text
	_ = ast.Walk(document, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		textNode, ok := n.(*ast.Text)
		if ok && !protectedRepositoryReference(textNode) {
			nodes = append(nodes, textNode)
		}
		return ast.WalkContinue, nil
	})

	for _, node := range nodes {
		parent := node.Parent()
		if parent == nil {
			continue
		}
		value := node.Segment.Value(source)
		segmentStart := node.Segment.Start
		matches := repositoryReference.FindAllIndex(value, -1)
		cursor := 0
		changed := false
		for _, match := range matches {
			if !repositoryReferenceBoundary(value, match[0], match[1]) {
				continue
			}
			if match[0] > cursor {
				prefix := ast.NewTextSegment(text.NewSegment(
					segmentStart+cursor, segmentStart+match[0],
				))
				prefix.SetRaw(node.IsRaw())
				parent.InsertBefore(parent, node, prefix)
			}
			label := value[match[0]:match[1]]
			link := ast.NewLink()
			link.Destination = []byte(e.destination(string(label)))
			link.AppendChild(link, ast.NewString(label))
			parent.InsertBefore(parent, node, link)
			cursor = match[1]
			changed = true
		}
		if changed {
			node.Segment.Start = segmentStart + cursor
			node.Segment.Padding = 0
		}
	}
}

func protectedRepositoryReference(n ast.Node) bool {
	for parent := n.Parent(); parent != nil; parent = parent.Parent() {
		switch parent.Kind() {
		case ast.KindLink, ast.KindImage, ast.KindCodeSpan,
			ast.KindCodeBlock, ast.KindFencedCodeBlock, ast.KindRawHTML:
			return true
		}
		if parent.Type() == ast.TypeBlock && containsRawHTML(parent) {
			return true
		}
	}
	return false
}

func containsRawHTML(node ast.Node) bool {
	found := false
	_ = ast.Walk(node, func(child ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && child.Kind() == ast.KindRawHTML {
			found = true
			return ast.WalkStop, nil
		}
		return ast.WalkContinue, nil
	})
	return found
}

func repositoryReferenceBoundary(value []byte, start, end int) bool {
	reference := value[start:end]
	if start > 0 && value[start-1] == '/' &&
		bytes.IndexByte(reference, '/') >= 0 &&
		bytes.IndexByte(reference, '#') >= 0 {
		return false
	}
	if value[start] == '#' {
		backslashes := 0
		for i := start - 1; i >= 0 && value[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 1 {
			return false
		}
	}
	if start > 0 && repositoryWord(value[start-1]) {
		return false
	}
	return end == len(value) || !repositoryWord(value[end])
}

func repositoryWord(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' ||
		b >= '0' && b <= '9' || b == '_' || b == '-'
}

func (e repositoryLinkExtension) destination(reference string) string {
	if strings.HasPrefix(reference, "#") {
		return e.base + "/issues/" + reference[1:]
	}
	if strings.Contains(reference, "#") {
		parts := strings.SplitN(reference, "#", 2)
		return "https://" + e.host + "/" + parts[0] + "/issues/" + parts[1]
	}
	return e.base + "/commit/" + reference
}

func newRepositoryLinkExtension(base string) goldmark.Extender {
	host := ""
	if u, err := url.Parse(base); err == nil && u != nil {
		host = u.Host
	}
	return repositoryLinkExtension{base: base, host: host}
}
