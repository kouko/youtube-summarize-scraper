package tui

import (
	"fmt"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
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
	// keyCols[i] is the indented key column of row i (headings and item
	// labels span the full row, value rows leave the value column empty
	// here — see values).
	keyCols []string
	// values[i] is the scalar value of row i ("" for headings/labels).
	values []string
	// linePaths[i] is the dotted key path of row i ("" for headings); a row
	// is editable when it has a path and a value.
	linePaths []string
	// lineIsValue[i] marks scalar value rows (editable).
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
	cv.keyCols = nil
	cv.values = nil
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
		cv.addHeading(scalarString(root))
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

// addHeading appends a full-row line (section heading or list item label).
func (cv *ConfigView) addHeading(text string) {
	cv.keyCols = append(cv.keyCols, text)
	cv.values = append(cv.values, "")
	cv.linePaths = append(cv.linePaths, "")
	cv.lineIsValue = append(cv.lineIsValue, false)
}

// addKeyValue appends an indented "key | value" row with edit metadata.
func (cv *ConfigView) addKeyValue(keyCol, value, path string) {
	cv.keyCols = append(cv.keyCols, keyCol)
	cv.values = append(cv.values, value)
	cv.linePaths = append(cv.linePaths, path)
	cv.lineIsValue = append(cv.lineIsValue, true)
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
			cv.addHeading(indent(depth) + key.Value)
			cv.walkMapping(val, path, depth+1)
		case yaml.SequenceNode:
			cv.addHeading(indent(depth) + fmt.Sprintf("%s (%d)", key.Value, len(val.Content)))
			cv.walkSequence(val, path, depth+1)
		default:
			cv.addKeyValue(indent(depth)+key.Value, scalarString(val), path)
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
			cv.addHeading(indent(depth) + label)
			cv.walkMapping(item, path, depth+1)
		default:
			cv.addKeyValue(indent(depth)+label, scalarString(item), path)
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

// Lines returns the structured tree view, one string per line. Value rows
// are aligned into two columns: the widest key column sets the value column
// start (a table look, user feedback 2026-10-09).
func (cv *ConfigView) Lines() []string {
	maxKey := 0
	for i, isV := range cv.lineIsValue {
		if isV {
			if w := lipgloss.Width(cv.keyCols[i]); w > maxKey {
				maxKey = w
			}
		}
	}
	out := make([]string, len(cv.keyCols))
	for i := range cv.keyCols {
		if cv.lineIsValue[i] {
			pad := maxKey + 2 - lipgloss.Width(cv.keyCols[i])
			if pad < 2 {
				pad = 2
			}
			out[i] = cv.keyCols[i] + strings.Repeat(" ", pad) + cv.values[i]
		} else {
			out[i] = cv.keyCols[i]
		}
	}
	return out
}

// EditTargetForLine returns the dotted key path and scalar value of a value
// line (spec amend2 REQ-8). ok is false for headings and out-of-range lines.
func (cv *ConfigView) EditTargetForLine(line int) (path, value string, ok bool) {
	if line < 0 || line >= len(cv.keyCols) || !cv.lineIsValue[line] {
		return "", "", false
	}
	return cv.linePaths[line], cv.values[line], true
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
	cv.keyCols = nil
	cv.values = nil
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
		cv.addHeading(scalarString(root))
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
