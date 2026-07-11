package airplan

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var (
	columnsOpen = regexp.MustCompile(`^(:{4,})[ \t]+\{\.columns\}[ \t]*$`)
	columnOpen  = regexp.MustCompile(
		`^(:{3,})[ \t]+\{\.column(?:[ \t]+width=(?:"((?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+))%"|'((?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+))%'|((?:[0-9]+(?:\.[0-9]+)?|\.[0-9]+))%))?\}[ \t]*$`,
	)
	colonClose = regexp.MustCompile(`^(:{3,})[ \t]*$`)
)

var (
	kindColumns = ast.NewNodeKind("Columns")
	kindColumn  = ast.NewNodeKind("Column")
)

type columnsNode struct {
	ast.BaseBlock
	spec *columnsSpec
}

func (n *columnsNode) Kind() ast.NodeKind { return kindColumns }

func (n *columnsNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

type columnNode struct {
	ast.BaseBlock
	width string
	close int
}

func (n *columnNode) Kind() ast.NodeKind { return kindColumn }

func (n *columnNode) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, map[string]string{"Width": n.width}, nil)
}

type columnsSpec struct {
	open     int
	close    int
	children map[int]columnSpec
}

type columnSpec struct {
	open  int
	close int
	width string
}

type markdownLine struct {
	text  string
	start int
}

type columnsExtension struct {
	containers map[int]*columnsSpec
	children   map[int]columnSpec
}

func newColumnsExtension(source []byte) goldmark.Extender {
	containers := validateColumns(source)
	children := map[int]columnSpec{}
	for _, container := range containers {
		for start, child := range container.children {
			children[start] = child
		}
	}
	return columnsExtension{containers: containers, children: children}
}

func (e columnsExtension) Extend(markdown goldmark.Markdown) {
	markdown.Parser().AddOptions(parser.WithBlockParsers(
		util.Prioritized(columnsBlockParser{specs: e.containers}, 50),
		util.Prioritized(columnBlockParser{specs: e.children}, 51),
	))
	markdown.Renderer().AddOptions(renderer.WithNodeRenderers(
		util.Prioritized(columnsHTMLRenderer{}, 500),
	))
}

type columnsBlockParser struct {
	specs map[int]*columnsSpec
}

func (p columnsBlockParser) Trigger() []byte { return []byte{':'} }

func (p columnsBlockParser) Open(
	parent ast.Node,
	reader text.Reader,
	context parser.Context,
) (ast.Node, parser.State) {
	if _, nested := parent.(*columnNode); nested || context.BlockIndent() != 0 {
		return nil, parser.NoChildren
	}
	_, segment := reader.PeekLine()
	spec := p.specs[segment.Start]
	if spec == nil {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &columnsNode{spec: spec}, parser.HasChildren
}

func (p columnsBlockParser) Continue(
	node ast.Node,
	reader text.Reader,
	_ parser.Context,
) parser.State {
	_, segment := reader.PeekLine()
	container := node.(*columnsNode)
	if segment.Start == container.spec.close {
		reader.AdvanceToEOL()
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (columnsBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (columnsBlockParser) CanInterruptParagraph() bool                 { return true }
func (columnsBlockParser) CanAcceptIndentedLine() bool                 { return false }

type columnBlockParser struct {
	specs map[int]columnSpec
}

func (p columnBlockParser) Trigger() []byte { return []byte{':'} }

func (p columnBlockParser) Open(
	parent ast.Node,
	reader text.Reader,
	context parser.Context,
) (ast.Node, parser.State) {
	if _, ok := parent.(*columnsNode); !ok || context.BlockIndent() != 0 {
		return nil, parser.NoChildren
	}
	_, segment := reader.PeekLine()
	spec, ok := p.specs[segment.Start]
	if !ok {
		return nil, parser.NoChildren
	}
	reader.AdvanceToEOL()
	return &columnNode{width: spec.width, close: spec.close}, parser.HasChildren
}

func (columnBlockParser) Continue(
	node ast.Node,
	reader text.Reader,
	_ parser.Context,
) parser.State {
	_, segment := reader.PeekLine()
	column := node.(*columnNode)
	if segment.Start == column.close {
		reader.AdvanceToEOL()
		return parser.Close
	}
	return parser.Continue | parser.HasChildren
}

func (columnBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (columnBlockParser) CanInterruptParagraph() bool                 { return true }
func (columnBlockParser) CanAcceptIndentedLine() bool                 { return false }

type columnsHTMLRenderer struct{}

func (columnsHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindColumns, renderColumns)
	reg.Register(kindColumn, renderColumn)
}

func renderColumns(
	w util.BufWriter,
	_ []byte,
	_ ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString("<div class=\"columns\">\n")
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

func renderColumn(
	w util.BufWriter,
	_ []byte,
	node ast.Node,
	entering bool,
) (ast.WalkStatus, error) {
	column := node.(*columnNode)
	if entering {
		if column.width == "" {
			_, _ = w.WriteString("<div class=\"column\">\n")
		} else {
			_, _ = fmt.Fprintf(
				w,
				"<div class=\"column\" style=\"--column-width: %s%%\">\n",
				column.width,
			)
		}
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

func validateColumns(source []byte) map[int]*columnsSpec {
	lines := splitMarkdownLines(string(source))
	result := map[int]*columnsSpec{}
	for i := 0; i < len(lines); i++ {
		outer := columnsOpen.FindStringSubmatch(lines[i].text)
		if outer == nil || lineInsideFence(lines, i) {
			continue
		}
		end, spec, ok := parseColumns(lines, i, len(outer[1]))
		if !ok {
			if close := findColumnsClose(lines, i+1, len(outer[1])); close >= 0 {
				i = close
			}
			continue
		}
		result[spec.open] = spec
		i = end
	}
	return result
}

func splitMarkdownLines(source string) []markdownLine {
	if source == "" {
		return nil
	}
	var result []markdownLine
	offset := 0
	for len(source) > 0 {
		index := strings.IndexByte(source, '\n')
		if index < 0 {
			result = append(result, markdownLine{text: source, start: offset})
			break
		}
		result = append(result, markdownLine{
			text: strings.TrimSuffix(source[:index], "\r"), start: offset,
		})
		offset += index + 1
		source = source[index+1:]
	}
	return result
}

func parseColumns(
	lines []markdownLine,
	start int,
	outerLength int,
) (int, *columnsSpec, bool) {
	spec := &columnsSpec{
		open: lines[start].start, children: map[int]columnSpec{},
	}
	for i := start + 1; i < len(lines); {
		if strings.TrimSpace(lines[i].text) == "" {
			i++
			continue
		}
		if close := colonClose.FindStringSubmatch(lines[i].text); close != nil &&
			len(close[1]) == outerLength {
			if len(spec.children) < 2 {
				return 0, nil, false
			}
			spec.close = lines[i].start
			return i, spec, true
		}

		child := columnOpen.FindStringSubmatch(lines[i].text)
		if child == nil || len(child[1]) >= outerLength {
			return 0, nil, false
		}
		width, valid := columnWidth(child)
		if !valid {
			return 0, nil, false
		}
		childLength := len(child[1])
		childSpec := columnSpec{open: lines[i].start, width: width}

		closed := false
		inFence := false
		fenceChar := byte(0)
		fenceLength := 0
		for i = i + 1; i < len(lines); i++ {
			trimmed := strings.TrimLeft(lines[i].text, " \t")
			if char, length, ok := markdownFence(trimmed); ok {
				if !inFence {
					inFence, fenceChar, fenceLength = true, char, length
				} else if char == fenceChar && length >= fenceLength &&
					strings.Trim(strings.TrimLeft(trimmed, string(char)), " \t") == "" {
					inFence = false
				}
				continue
			}
			if inFence {
				continue
			}
			if columnsOpen.MatchString(lines[i].text) ||
				columnOpen.MatchString(lines[i].text) {
				return 0, nil, false
			}
			if close := colonClose.FindStringSubmatch(lines[i].text); close != nil &&
				len(close[1]) == childLength {
				childSpec.close = lines[i].start
				spec.children[childSpec.open] = childSpec
				i++
				closed = true
				break
			}
		}
		if !closed {
			return 0, nil, false
		}
	}
	return 0, nil, false
}

func findColumnsClose(lines []markdownLine, start, length int) int {
	for i := start; i < len(lines); i++ {
		close := colonClose.FindStringSubmatch(lines[i].text)
		if close != nil && len(close[1]) == length && !lineInsideFence(lines, i) {
			return i
		}
	}
	return -1
}

func columnWidth(match []string) (string, bool) {
	value := ""
	for _, candidate := range match[2:] {
		if candidate != "" {
			value = candidate
			break
		}
	}
	if value == "" {
		return "", true
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || number <= 0 || number > 100 {
		return "", false
	}
	return value, true
}

func lineInsideFence(lines []markdownLine, target int) bool {
	inFence := false
	char := byte(0)
	length := 0
	for i := 0; i < target; i++ {
		trimmed := strings.TrimLeft(lines[i].text, " \t")
		candidate, candidateLength, ok := markdownFence(trimmed)
		if !ok {
			continue
		}
		if !inFence {
			inFence, char, length = true, candidate, candidateLength
		} else if candidate == char && candidateLength >= length &&
			strings.Trim(strings.TrimLeft(trimmed, string(char)), " \t") == "" {
			inFence = false
		}
	}
	return inFence
}

func markdownFence(line string) (byte, int, bool) {
	if line == "" || line[0] != '`' && line[0] != '~' {
		return 0, 0, false
	}
	length := 0
	for length < len(line) && line[length] == line[0] {
		length++
	}
	return line[0], length, length >= 3
}
