package vault

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Repair returns data with its frontmatter name set to name, and its description
// set to description when the current one is missing or empty (spec §7.1 --fix).
// Other keys, their order and the body are kept. Frontmatter is created when data
// has none. It fails when the frontmatter is not closed or is not a YAML mapping.
func Repair(data []byte, name, description string) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	root := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	body := string(data)
	if isDelimiter(lines[0]) {
		end, err := closingLine(lines)
		if err != nil {
			return nil, err
		}
		if root, err = frontmatterMapping(lines[1:end]); err != nil {
			return nil, err
		}
		body = strings.Join(lines[end+1:], "\n")
	}

	setText(root, "name", name)
	if !hasText(root, "description") && description != "" {
		setText(root, "description", description)
	}

	var fm bytes.Buffer
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, fmt.Errorf("write frontmatter: %w", err)
	}
	if err := enc.Close(); err != nil {
		return nil, fmt.Errorf("write frontmatter: %w", err)
	}
	return []byte("---\n" + fm.String() + "---\n" + body), nil
}

// frontmatterMapping parses the frontmatter lines into a mapping node. Empty
// frontmatter gives an empty mapping.
func frontmatterMapping(lines []string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(strings.Join(lines, "\n")), &doc); err != nil {
		return nil, fmt.Errorf("invalid YAML: %w", err)
	}
	switch {
	case doc.Kind == 0:
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
	case doc.Kind == yaml.DocumentNode && len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode:
		return doc.Content[0], nil
	default:
		return nil, errors.New("frontmatter is not a YAML mapping")
	}
}

// setText sets the string value of key in the mapping m, appending the key when m has
// none. A key keeps its place when it is already there.
func setText(m *yaml.Node, key, value string) {
	val := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = val
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, val)
}

// hasText reports whether the mapping m holds a non-empty string under key.
func hasText(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			v := m.Content[i+1]
			return v.Kind == yaml.ScalarNode && v.Tag == "!!str" && v.Value != ""
		}
	}
	return false
}
