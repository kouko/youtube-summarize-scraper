package tui

import (
	"fmt"
	"strconv"
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
	// doc is the parsed YAML document tree (order-preserving); it is the
	// write-back target for edits (preserves key order and comments).
	doc *yaml.Node
	// lines are the structured view, one string per rendered line
	lines []string
	// linePaths[i] is the dotted key path of lines[i] ("" for headings); a
	// line is editable when it has a path and is a scalar value line.
	linePaths []string
	// lineIsValue[i] marks scalar "key: value" lines (editable).
	lineIsValue []bool
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
// the sectioned view lines (recording each line's key path for editing).
func (cv *ConfigView) parse(content string) error {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(content), &doc); err != nil {
		return err
	}
	cv.doc = &doc
	cv.lines = nil
	cv.linePaths = nil
	cv.lineIsValue = nil
	if doc.Kind == 0 {
		return nil // empty document
	}
	root := documentRoot(&doc)
	if root == nil {
		return nil
	}
	if root.Kind != yaml.MappingNode {
		cv.addLine(scalarString(root), "", false)
		return nil
	}
	cv.walkMapping(root, "", 0)
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

// addLine appends a rendered line with its edit metadata.
func (cv *ConfigView) addLine(text, path string, isValue bool) {
	cv.lines = append(cv.lines, text)
	cv.linePaths = append(cv.linePaths, path)
	cv.lineIsValue = append(cv.lineIsValue, isValue)
}

// walkMapping renders a mapping: headings at depth 0, indented key-value
// lines below; nested maps recurse with a deeper indent.
func (cv *ConfigView) walkMapping(m *yaml.Node, prefix string, depth int) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		key, val := m.Content[i], m.Content[i+1]
		path := key.Value
		if prefix != "" {
			path = prefix + "." + key.Value
		}
		switch val.Kind {
		case yaml.MappingNode:
			cv.addLine(indent(depth)+key.Value, "", false)
			cv.walkMapping(val, path, depth+1)
		case yaml.SequenceNode:
			cv.addLine(indent(depth)+fmt.Sprintf("%s (%d)", key.Value, len(val.Content)), "", false)
			cv.walkSequence(val, path, depth+1)
		default:
			cv.addLine(indent(depth)+key.Value+": "+scalarString(val), path, true)
		}
	}
}

// walkSequence renders sequence entries as numbered items; each item's first
// line carries [i] plus its name/url summary (or the scalar itself), nested
// maps indent below.
func (cv *ConfigView) walkSequence(seq *yaml.Node, prefix string, depth int) {
	for i, item := range seq.Content {
		path := fmt.Sprintf("%s.%d", prefix, i)
		label := fmt.Sprintf("[%d]", i+1)
		switch item.Kind {
		case yaml.MappingNode:
			if name := mapValue(item, "name"); name != "" {
				label += " " + name
			} else if url := mapValue(item, "url"); url != "" {
				label += " " + url
			}
			cv.addLine(indent(depth)+label, "", false)
			cv.walkMapping(item, path, depth+1)
		default:
			cv.addLine(indent(depth)+label+" "+scalarString(item), path, true)
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

// EditTargetForLine returns the dotted key path and scalar value of a value
// line (spec amend2 REQ-8). ok is false for headings and out-of-range lines.
func (cv *ConfigView) EditTargetForLine(line int) (path, value string, ok bool) {
	if line < 0 || line >= len(cv.lines) || !cv.lineIsValue[line] {
		return "", "", false
	}
	_, value, _ = strings.Cut(cv.lines[line], ": ")
	return cv.linePaths[line], value, true
}

// SetValue updates a scalar value by dotted key path, then re-renders the
// lines from the document tree. Non-scalar paths are rejected.
func (cv *ConfigView) SetValue(path, value string) error {
	if cv.doc == nil {
		return fmt.Errorf("no parsed document")
	}
	node := lookupNode(documentRoot(cv.doc), path)
	if node == nil {
		return fmt.Errorf("no such key: %s", path)
	}
	if node.Kind != yaml.ScalarNode {
		return fmt.Errorf("%s is not a scalar value", path)
	}
	node.Value = value
	cv.refreshLines()
	return nil
}

// Serialized returns the YAML for the current document tree (edits applied,
// key order and comments preserved via yaml.Node).
func (cv *ConfigView) Serialized() (string, error) {
	if cv.doc == nil {
		return "", fmt.Errorf("no parsed document")
	}
	out, err := yaml.Marshal(cv.doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// refreshLines re-renders the view lines after an edit.
func (cv *ConfigView) refreshLines() {
	cv.lines = nil
	cv.linePaths = nil
	cv.lineIsValue = nil
	if cv.doc == nil {
		return
	}
	root := documentRoot(cv.doc)
	if root == nil {
		return
	}
	if root.Kind != yaml.MappingNode {
		cv.addLine(scalarString(root), "", false)
		return
	}
	cv.walkMapping(root, "", 0)
}

// lookupNode finds the node at a dotted key path (segments are map keys or
// sequence indexes), or nil.
func lookupNode(n *yaml.Node, path string) *yaml.Node {
	cur := n
	for _, part := range strings.Split(path, ".") {
		if cur == nil {
			return nil
		}
		if idx, err := strconv.Atoi(part); err == nil {
			if cur.Kind != yaml.SequenceNode || idx < 0 || idx >= len(cur.Content) {
				return nil
			}
			cur = cur.Content[idx]
			continue
		}
		if cur.Kind != yaml.MappingNode {
			return nil
		}
		found := false
		for i := 0; i+1 < len(cur.Content); i += 2 {
			if cur.Content[i].Value == part {
				cur = cur.Content[i+1]
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}
	return cur
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
