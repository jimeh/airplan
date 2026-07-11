package airplan

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"
)

type frontMatter struct {
	body   []byte
	text   []byte
	format string
	title  string
}

func parseFrontMatter(src []byte) (frontMatter, error) {
	result := frontMatter{body: src}
	content := src
	prefix := 0
	if bytes.HasPrefix(content, []byte("\xef\xbb\xbf")) {
		content = content[3:]
		prefix = 3
	}

	lineEnd := bytes.IndexByte(content, '\n')
	if lineEnd < 0 {
		if string(content) == "---" || string(content) == "+++" {
			format := "yaml"
			if string(content) == "+++" {
				format = "toml"
			}
			return frontMatter{}, fmt.Errorf(
				"airplan: unclosed %s frontmatter", format,
			)
		}
		return result, nil
	}
	opener := strings.TrimSuffix(string(content[:lineEnd]), "\r")
	if opener != "---" && opener != "+++" {
		return result, nil
	}

	format := "yaml"
	if opener == "+++" {
		format = "toml"
	}
	bodyStart := lineEnd + 1
	search := bodyStart
	closingEnd := -1
	closingNext := -1
	for search <= len(content) {
		next := bytes.IndexByte(content[search:], '\n')
		end := len(content)
		advance := len(content)
		if next >= 0 {
			end = search + next
			advance = end + 1
		}
		line := strings.TrimSuffix(string(content[search:end]), "\r")
		if line == opener {
			closingEnd = end
			closingNext = advance
			break
		}
		if next < 0 {
			break
		}
		search = advance
	}
	if closingEnd < 0 {
		return frontMatter{}, fmt.Errorf(
			"airplan: unclosed %s frontmatter", format,
		)
	}

	payload := content[bodyStart:search]
	var title any
	switch format {
	case "yaml":
		var node yaml.Node
		if err := yaml.Unmarshal(payload, &node); err != nil {
			return frontMatter{}, fmt.Errorf(
				"airplan: parse YAML frontmatter: %w", err,
			)
		}
		if len(node.Content) == 0 ||
			node.Content[0].Kind != yaml.MappingNode {
			return frontMatter{}, errors.New(
				"airplan: YAML frontmatter must be a mapping",
			)
		}
		mapping := node.Content[0]
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			for j := i + 2; j+1 < len(mapping.Content); j += 2 {
				left, right := mapping.Content[i], mapping.Content[j]
				if left.Kind == right.Kind && left.Value == right.Value {
					return frontMatter{}, fmt.Errorf(
						"airplan: parse YAML frontmatter: duplicate key %q",
						right.Value,
					)
				}
			}
		}
		for i := 0; i+1 < len(mapping.Content); i += 2 {
			key := mapping.Content[i]
			if key.Value == "title" && title == nil {
				var value any
				if err := mapping.Content[i+1].Decode(&value); err == nil {
					title = value
				}
			}
		}
	case "toml":
		values := map[string]any{}
		if _, err := toml.Decode(string(payload), &values); err != nil {
			return frontMatter{}, fmt.Errorf(
				"airplan: parse TOML frontmatter: %w", err,
			)
		}
		title = values["title"]
	}

	titleString, _ := title.(string)
	titleString = strings.TrimSpace(titleString)
	return frontMatter{
		body:   content[closingNext:],
		text:   src[prefix : prefix+closingNext],
		format: format,
		title:  titleString,
	}, nil
}
