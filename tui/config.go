package tui

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// ConfigDisplayMode toggles between structured and raw views.
type ConfigDisplayMode int

const (
	ConfigViewStructured ConfigDisplayMode = iota
	ConfigViewRaw
)

// ConfigView holds the parsed configuration and provides rendering.
type ConfigView struct {
	mode ConfigDisplayMode
	// raw is the original file content
	raw string
	// doc is the parsed YAML document tree (order-preserving)
	doc *yaml.Node
	// lines are the structured view, one string per rendered line
	lines []string
	// error holds any parsing error
	error error
}

// NewConfigView creates a ConfigView from raw YAML content.
func NewConfigView(content string) *ConfigView {
	cv := &ConfigView{
		raw:  content,
		mode: ConfigViewStructured,
	}
	if err := cv.parse(content); err != nil {
		cv.error = err
	}
	return cv
}

// parse unmarshals YAML into an order-preserving document tree and renders
// the sectioned view lines.
func (cv *ConfigView) parse(content string) error {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return err
	}
	cv.doc = &doc
	cv.lines = nil
	if doc.Kind == 0 {
		return nil // empty document
	}
	root := documentRoot(&doc)
	if root == nil {
		return nil
	}
	if root.Kind != yaml.MappingNode {
		cv.lines = []string{scalarString(root)}
		return nil
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, val := root.Content[i], root.Content[i+1]
		switch val.Kind {
		case yaml.MappingNode:
			cv.lines = append(cv.lines, key.Value)
			appendMapping(cv, val, 1)
		case yaml.SequenceNode:
			cv.lines = append(cv.lines, fmt.Sprintf("%s (%d)", key.Value, len(val.Content)))
			appendSequence(cv, key.Value, val, 1)
		default:
			cv.lines = append(cv.lines, key.Value+": "+scalarString(val))
		}
	}
	return nil
}

// documentRoot unwraps the document node to its content node (nil when the
// document carries no content).
func documentRoot(doc *yaml.Node) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}
	return doc.Content[0]
}

// indent returns n two-space indents.
func indent(n int) string {
	return strings.Repeat("  ", n)
}

// appendMapping renders mapping entries as "key: value" lines (nested maps
// recurse with a deeper indent).
func appendMapping(cv *ConfigView, m *yaml.Node, depth int) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i], m.Content[i+1]
		switch val.Kind {
		case yaml.MappingNode:
			cv.lines = append(cv.lines, indent(depth)+key.Value)
			appendMapping(cv, val, depth+1)
		case yaml.SequenceNode:
			cv.lines = append(cv.lines, indent(depth)+fmt.Sprintf("%s (%d)", key.Value, len(val.Content)))
			appendSequence(cv, key.Value, val, depth+1)
		default:
			cv.lines = append(cv.lines, indent(depth)+key.Value+": "+scalarString(val))
		}
	}
}

// appendSequence renders sequence entries as numbered items; each item's
// first line carries [i] plus its name/url summary (or the scalar itself),
// nested maps indent below.
func appendSequence(cv *ConfigView, key string, seq *yaml.Node, depth int) {
	for i, item := range seq.Content {
		label := fmt.Sprintf("[%d]", i+1)
		switch item.Kind {
		case yaml.MappingNode:
			if name := mapValue(item, "name"); name != "" {
				label += " " + name
			} else if url := mapValue(item, "url"); url != "" {
				label += " " + url
			}
			cv.lines = append(cv.lines, indent(depth)+label)
			appendMapping(cv, item, depth+1)
		default:
			cv.lines = append(cv.lines, indent(depth)+label+" "+scalarString(item))
		}
	}
}

// mapValue returns the string value of a mapping key, or "".
func mapValue(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return scalarString(m.Content[i+1])
		}
	}
	return ""
}

// scalarString renders a scalar node on one line, escaping embedded newlines
// so a value never splits the tree into misaligned rows.
func scalarString(n *yaml.Node) string {
	s := n.Value
	if n.Tag == "!!null" && n.Value == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.ReplaceAll(s, "\n", "\\n")
	return s
}

// Lines returns the structured tree view, one string per line.
func (cv *ConfigView) Lines() []string {
	return cv.lines
}

// Toggle switches between structured and raw view.
func (cv *ConfigView) Toggle() {
	if cv.mode == ConfigViewStructured {
		cv.mode = ConfigViewRaw
	} else {
		cv.mode = ConfigViewStructured
	}
}

// Mode returns the current display mode.
func (cv *ConfigView) Mode() ConfigDisplayMode {
	return cv.mode
}

// Error returns any parsing error.
func (cv *ConfigView) Error() error {
	return cv.error
}

// Raw returns the original YAML content.
func (cv *ConfigView) Raw() string {
	return cv.raw
}

// Render returns the string representation of the config view.
func (cv *ConfigView) Render() string {
	if cv.Error() != nil {
		return fmt.Sprintf("Error parsing config: %v", cv.Error())
	}
	switch cv.Mode() {
	case ConfigViewStructured:
		if len(cv.Lines()) == 0 {
			return "(empty)"
		}
		return strings.Join(cv.Lines(), "\n")
	case ConfigViewRaw:
		return cv.Raw()
	}
	return ""
}
