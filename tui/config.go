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
	// data is the parsed YAML as map[string]interface{}
	data map[string]interface{}
	// rows are the structured view lines: each row is [key, value]
	rows [][]string
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

// parse unmarshals YAML and flattens it into rows.
func (cv *ConfigView) parse(content string) error {
	if err := yaml.Unmarshal([]byte(content), &cv.data); err != nil {
		return err
	}
	cv.rows = cv.flatten("", cv.data)
	return nil
}

// flatten recursively flattens a map[string]interface{} or []interface{} into key-value rows.
// Nested keys are joined by dots. Arrays are joined by ', '.
func (cv *ConfigView) flatten(prefix string, v interface{}) [][]string {
	var rows [][]string
	switch val := v.(type) {
	case map[string]interface{}:
		for k, val := range val {
			if k == "" {
				continue
			}
			rows = append(rows, cv.flatten(prefix+k+".", val)...)
		}
	case []interface{}:
		var parts []string
		for _, item := range val {
			if item == nil {
				continue
			}
			parts = append(parts, fmt.Sprint(item))
		}
		if len(parts) > 0 {
			rows = append(rows, []string{prefix, strings.Join(parts, ", ")})
		}
	default:
		// scalar value; escape embedded newlines so a value never splits
		// the key-value table into misaligned rows
		s := strings.ReplaceAll(fmt.Sprint(val), "\r", "")
		s = strings.ReplaceAll(s, "\n", "\\n")
		rows = append(rows, []string{prefix, s})
	}
	return rows
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

// Rows returns the structured view rows.
func (cv *ConfigView) Rows() [][]string {
	return cv.rows
}

// Render returns the string representation of the config view.
func (cv *ConfigView) Render() string {
	if cv.Error() != nil {
		return fmt.Sprintf("Error parsing config: %v", cv.Error())
	}
	switch cv.Mode() {
	case ConfigViewStructured:
		return cv.renderStructured()
	case ConfigViewRaw:
		return cv.Raw()
	}
	return ""
}

// renderStructured builds a two-column table.
func (cv *ConfigView) renderStructured() string {
	if len(cv.Rows()) == 0 {
		return "(empty)"
	}
	// Determine max width of key column for alignment
	maxKeyLen := 0
	for _, row := range cv.Rows() {
		if len(row[0]) > maxKeyLen {
			maxKeyLen = len(row[0])
		}
	}
	var s strings.Builder
	for _, row := range cv.Rows() {
		key := row[0]
		val := row[1]
		// Pad key to maxKeyLen
		paddedKey := key
		if len(key) < maxKeyLen {
			paddedKey = key + strings.Repeat(" ", maxKeyLen-len(key))
		}
		s.WriteString(paddedKey)
		s.WriteString("  ")
		s.WriteString(val)
		s.WriteString("\n")
	}
	return s.String()
}
