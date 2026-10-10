package tui

import (
	"fmt"
	"image"
	"strconv"
	"strings"

	// lipgloss v2: display-width measurement for the aligned two-column
	// config table (key column width, value truncation).
	"charm.land/lipgloss/v2"
	// yaml.v3: order-preserving parse of the config into a yaml.Node tree,
	// which both renders the table and is the write-back target for edits
	// (key order and comments survive).
	"gopkg.in/yaml.v3"
	// runewidth: display-width of runes (CJK = 2, ASCII = 1).
	"github.com/mattn/go-runewidth"
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
	// lineIsSummary[i] marks collapsed list-item rows: shown in the value
	// column but not editable (Enter toggles expand instead).
	lineIsSummary []bool
	// separators[i] marks content rows that get a table separator line before
	// them (section headings, numbered items). Built by walkMapping.
	separators []bool
	// itemExpanded records which list items (by path, e.g. "playlists.0")
	// are expanded into child rows for editing (user feedback option B).
	itemExpanded map[string]bool
	// table index mapping (built by RenderTable)
	tableRowToContent []int
	contentToTableRow []int
	sepFlags          []bool
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
	cv.lineIsSummary = nil
	cv.separators = nil
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
	cv.addHeadingSep(text, false)
}

// addHeadingSep appends a full-row line with a separator flag (a table
// separator line precedes top-level sections and numbered items).
func (cv *ConfigView) addHeadingSep(text string, sep bool) {
	cv.keyCols = append(cv.keyCols, text)
	cv.values = append(cv.values, "")
	cv.linePaths = append(cv.linePaths, "")
	cv.lineIsValue = append(cv.lineIsValue, false)
	cv.lineIsSummary = append(cv.lineIsSummary, false)
	cv.separators = append(cv.separators, sep)
}

// addHeadingPath appends a full-row line that carries an item path (used for
// expanded list-item headings so Enter can collapse them again).
func (cv *ConfigView) addHeadingPath(text, path string) {
	cv.keyCols = append(cv.keyCols, text)
	cv.values = append(cv.values, "")
	cv.linePaths = append(cv.linePaths, path)
	cv.lineIsValue = append(cv.lineIsValue, false)
	cv.lineIsSummary = append(cv.lineIsSummary, false)
	cv.separators = append(cv.separators, true)
}

// addSummary appends a collapsed list-item row (key | summary, not editable).
func (cv *ConfigView) addSummary(keyCol, summary, path string) {
	cv.keyCols = append(cv.keyCols, keyCol)
	cv.values = append(cv.values, summary)
	cv.linePaths = append(cv.linePaths, path)
	cv.lineIsValue = append(cv.lineIsValue, false)
	cv.lineIsSummary = append(cv.lineIsSummary, true)
	cv.separators = append(cv.separators, true)
}

// addKeyValue appends an indented "key | value" row with edit metadata.
func (cv *ConfigView) addKeyValue(keyCol, value, path string) {
	cv.keyCols = append(cv.keyCols, keyCol)
	cv.values = append(cv.values, value)
	cv.linePaths = append(cv.linePaths, path)
	cv.lineIsValue = append(cv.lineIsValue, true)
	cv.lineIsSummary = append(cv.lineIsSummary, false)
	cv.separators = append(cv.separators, false)
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
			cv.addHeadingSep(indent(depth)+key.Value, depth == 0)
			cv.walkMapping(val, path, depth+1)
		case yaml.SequenceNode:
			cv.addHeadingSep(indent(depth)+fmt.Sprintf("%s (%d)", key.Value, len(val.Content)), depth == 0)
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
			name := mapValue(item, "name")
			if name == "" {
				name = mapValue(item, "channel_name")
			}
			if name != "" {
				label += " " + name
			} else if url := mapValue(item, "url"); url != "" {
				label += " " + url
			}
			if cv.itemExpanded[path] {
				// Expanded: the item renders as a heading with its child
				// rows below, so Enter can edit a child value.
				cv.addHeadingPath(indent(depth)+label, path)
				cv.walkMapping(item, path, depth+1)
			} else {
				// Collapsed: one summary row "key=value key=value ...".
				cv.addSummary(indent(depth)+label, itemInline(item), path)
			}
		default:
			cv.addKeyValue(indent(depth)+label, scalarString(item), path)
		}
	}
}

// itemInline renders a list item's children as "key=value" pairs joined by
// spaces (nested maps flatten with dots), for the collapsed one-line view.
// Identifying fields (name, channel_name, url) come first so the row scans.
func itemInline(m *yaml.Node) string {
	var names, urls, rest []string
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		var parts []string
		switch v.Kind {
		case yaml.ScalarNode:
			parts = []string{k.Value + "=" + scalarString(v)}
		case yaml.MappingNode:
			parts = flattenMapInline(k.Value, v)
		}
		if k.Value == "url" {
			urls = append(urls, parts...)
		} else if k.Value == "name" || k.Value == "channel_name" {
			names = append(names, parts...)
		} else {
			rest = append(rest, parts...)
		}
	}
	return strings.Join(append(append(names, urls...), rest...), " ")
}

// flattenMapInline flattens a nested map into dot-prefixed key=value parts.
func flattenMapInline(prefix string, m *yaml.Node) []string {
	var out []string
	for i := 0; i+1 < len(m.Content); i += 2 {
		k, v := m.Content[i], m.Content[i+1]
		switch v.Kind {
		case yaml.MappingNode:
			out = append(out, flattenMapInline(prefix+"."+k.Value, v)...)
		case yaml.ScalarNode:
			out = append(out, prefix+"."+k.Value+"="+scalarString(v))
		}
	}
	return out
}

// ExpandItem expands a collapsed list item (row = the item's summary row)
// into its child rows for editing.
func (cv *ConfigView) ExpandItem(row int) {
	cv.setItemExpanded(row, true)
}

// CollapseItem collapses an expanded list item back to its summary row.
func (cv *ConfigView) CollapseItem(row int) {
	cv.setItemExpanded(row, false)
}

func (cv *ConfigView) setItemExpanded(row int, expanded bool) {
	if row < 0 || row >= len(cv.linePaths) || cv.linePaths[row] == "" {
		return
	}
	// Accept any row of the item (its summary row or a child row): resolve to
	// the item path by dropping trailing map-key segments.
	item := itemPathFrom(cv.linePaths[row])
	if cv.itemExpanded == nil {
		cv.itemExpanded = map[string]bool{}
	}
	cv.itemExpanded[item] = expanded
	cv.refreshLines()
}

// itemPathFrom resolves a dotted path to its owning list-item path by
// dropping trailing map-key segments ("playlists.0.name" -> "playlists.0").
func itemPathFrom(p string) string {
	parts := strings.Split(p, ".")
	for len(parts) > 1 {
		if _, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			return strings.Join(parts, ".")
		}
		parts = parts[:len(parts)-1]
	}
	return p
}

// isExpandedItemHeading reports whether a content row is the heading row of
// an expanded list item: it is a full-row (non-value) row whose owning item
// path — itself included — is currently expanded.
func (cv *ConfigView) isExpandedItemHeading(row int) bool {
	if row < 0 || row >= len(cv.linePaths) || cv.lineIsValue[row] || cv.lineIsSummary[row] {
		return false
	}
	path := cv.linePaths[row]
	if path == "" {
		return false
	}
	// The heading row of item "playlists.0" carries exactly that path (its
	// children carry longer paths).
	if itemPathFrom(path) != path {
		return false
	}
	return cv.itemExpanded[path]
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
		if cv.lineIsValue[i] || cv.lineIsSummary[i] {
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

// RenderTable renders the tree as a bordered two-column table: a header row,
// a separator line before each section heading and each numbered item, and
// none between the scalar rows inside an item (user feedback 2026-10-09).
// tableRows carries the rendered lines; the index-mapping helpers translate
// between content rows (editing) and table rows (display).
func (cv *ConfigView) RenderTable(width int) []string {
	lines := cv.Lines()
	if len(lines) == 0 {
		return []string{"(empty)"}
	}
	keyW := 0
	for i := range cv.keyCols {
		if cv.lineIsValue[i] || cv.lineIsSummary[i] {
			if w := lipgloss.Width(cv.keyCols[i]); w > keyW {
				keyW = w
			}
		}
	}
	if keyW < len("Key") {
		keyW = len("Key")
	}
	// Cap the key column at 40% of the available inner width so the value
	// column keeps at least ~60% of the inner width (user feedback: key
	// column was too wide, squeezing values).
	maxKeyW := (width - 7) * 40 / 100
	if keyW > maxKeyW {
		keyW = maxKeyW
	}
	if keyW < len("Key") {
		keyW = len("Key")
	}
	valW := width - keyW - 7
	if valW < len("Value") {
		valW = len("Value")
	}
	bar := strings.Repeat("─", keyW+2) + "┬" + strings.Repeat("─", valW+2)

	out := make([]string, 0, len(lines)+len(cv.separators)+2)
	tableRowToContent := make([]int, 0, len(lines)+4)
	sepFlags := make([]bool, 0, len(lines)+4)
	contentToTableRow := make([]int, len(lines))
	addRow := func(row string, content int, isSep bool) {
		out = append(out, row)
		tableRowToContent = append(tableRowToContent, content)
		sepFlags = append(sepFlags, isSep)
	}
	addRow("┌"+bar+"┐", -1, false)
	addRow("│ "+padToWidth("Key", keyW)+" │ "+padToWidth("Value", valW)+" │", -1, false)
	addRow("├"+bar+"┤", -1, false)
	sepPending := false
	for i, l := range lines {
		if cv.separators[i] {
			sepPending = true
		}
		if sepPending {
			addRow("├"+bar+"┤", -1, true)
			sepPending = false
		}
		var row string
		if cv.lineIsValue[i] || cv.lineIsSummary[i] {
			val := padToWidth(truncateANSI(cv.values[i], valW), valW)
			key := truncateANSI(cv.keyCols[i], keyW)
			row = "│ " + padToWidth(key, keyW) + " │ " + val + " │"
		} else {
			// heading: spans both columns, truncated to the inner width
			inner := width - 4 // "│ " ... " │"
			head := truncateANSI(l, inner)
			row = "│ " + padToWidth(head, inner) + " │"
		}
		addRow(row, i, false)
		contentToTableRow[i] = len(out) - 1
	}
	addRow("└"+strings.Repeat("─", keyW+2)+"┴"+strings.Repeat("─", valW+2)+"┘", -1, false)
	cv.tableRowToContent = tableRowToContent
	cv.contentToTableRow = contentToTableRow
	cv.sepFlags = sepFlags
	return out
}

// padToWidth right-pads s to width display columns (ANSI-aware: escape
// sequences count as zero columns).
func padToWidth(s string, width int) string {
	w := visibleWidth(s)
	if w < width {
		return s + strings.Repeat(" ", width-w)
	}
	return s
}

// visibleWidth counts the display columns of s, ignoring ANSI escapes.
func visibleWidth(s string) int {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return runewidth.StringWidth(b.String())
}

// ContentTableIndex maps a content row (keyCols index) to its table row.
func (cv *ConfigView) ContentTableIndex(contentRow int) int {
	if contentRow < 0 || contentRow >= len(cv.contentToTableRow) {
		return -1
	}
	return cv.contentToTableRow[contentRow]
}

// TableContentIndex maps a table row back to its content row.
func (cv *ConfigView) TableContentIndex(tableRow int) int {
	if tableRow < 0 || tableRow >= len(cv.tableRowToContent) {
		return -1
	}
	return cv.tableRowToContent[tableRow]
}

// IsTableSeparator reports whether a table row is a separator line.
func (cv *ConfigView) IsTableSeparator(tableRow int) bool {
	if tableRow < 0 || tableRow >= len(cv.sepFlags) {
		return false
	}
	return cv.sepFlags[tableRow]
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
	cv.lineIsSummary = nil
	cv.separators = nil
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

// ItemAtPos returns the index of the list item at the given screen coordinates,
// or -1 if not over any item. Coordinates are relative to the top-left of
// the rendered config table.
func (cv *ConfigView) ItemAtPos(screenX, screenY int) int {
	// Convert screen coordinates to table coordinates
	// The table starts after the header (3 rows: top border, header, separator)
	tableY := screenY - 3
	if tableY < 0 {
		return -1
	}
	
	// Each row is approximately 1 line (plus separators)
	// We need to count how many lines we've passed including separators
	row := 0
	for i := 0; i < len(cv.sepFlags); i++ {
		if i >= tableY {
			break
		}
		if cv.sepFlags[i] {
			tableY++ // separator takes an extra line
		}
		row++
	}
	
	if row < 0 || row >= len(cv.lineIsValue) {
		return -1
	}
	
	// Check if this row is a value row (editable)
	if !cv.lineIsValue[row] {
		return -1
	}
	
	// Verify the column is within bounds (simplified check)
	if screenX < 0 || screenX >= 80 {
		return -1
	}
	
	return row
}

// PosToRow converts a Y coordinate (screenY) to a content row index
func (cv *ConfigView) PosToRow(screenY int) (int, bool) {
	tableY := screenY - 3 // Account for table header
	if tableY < 0 {
		return 0, false
	}
	
	row := 0
	for i := 0; i < len(cv.sepFlags); i++ {
		if i >= tableY {
			break
		}
		if cv.sepFlags[i] {
			tableY++
		}
		row++
	}
	
	if row < 0 || row >= len(cv.lineIsValue) {
		return 0, false
	}
	
	if !cv.lineIsValue[row] {
		return 0, false
	}
	
	return row, true
}

// IsLineEditable checks if a line (by content index) is editable
func (cv *ConfigView) IsLineEditable(line int) bool {
	if line < 0 || line >= len(cv.lineIsValue) {
		return false
	}
	return cv.lineIsValue[line] && !cv.lineIsSummary[line]
}

// startEditAtRow begins editing at the specified content row
func (cv *ConfigView) startEditAtRow(row int) error {
	if !cv.IsLineEditable(row) {
		return fmt.Errorf("line %d is not editable", row)
	}
	
	path, value, ok := cv.EditTargetForLine(row)
	if !ok {
		return fmt.Errorf("could not get edit target for line %d", row)
	}
	
	return cv.SetValue(path, value) // This will set the value to itself to start editing
}

// showItemEditPopupInfo returns information needed to show the item edit popup
func (cv *ConfigView) showItemEditPopupInfo(itemIdx int) (string, error) {
	if itemIdx < 0 || itemIdx >= len(cv.lineIsValue) {
		return "", fmt.Errorf("invalid item index: %d", itemIdx)
	}
	
	if !cv.lineIsValue[itemIdx] {
		return "", fmt.Errorf("item %d is not a value line", itemIdx)
	}
	
	// Get the key path for this item
	path, _, ok := cv.EditTargetForLine(itemIdx)
	if !ok {
		return "", fmt.Errorf("could not get path for item %d", itemIdx)
	}
	
	return path, nil
}

// RectForItem returns the screen rectangle for the given item index
func (cv *ConfigView) RectForItem(itemIdx int, offsetX int, offsetY int, cellWidth int, cellHeight int) image.Rectangle {
	if itemIdx < 0 || itemIdx >= len(cv.lineIsValue) {
		return image.Rectangle{}
	}
	
	y := offsetY + (itemIdx * cellHeight)
	return image.Rect(offsetX, y, offsetX+cellWidth, y+cellHeight)
}
