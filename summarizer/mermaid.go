package summarizer

import (
	"fmt"
	"regexp"
	"strings"
)

// MermaidPrompt generates a Stage 3 Mermaid flowchart prompt.
// The prompt template is loaded from embedded files by language, with variable substitution.
func MermaidPrompt(summary string, language string) (string, error) {
	template, err := loadBuiltinPromptByPrefix("mermaid", language)
	if err != nil {
		return "", fmt.Errorf("loading mermaid prompt: %w", err)
	}

	return strings.ReplaceAll(template, "{{summary}}", summary), nil
}

// MermaidBlock represents a single validated Mermaid diagram with an optional title.
type MermaidBlock struct {
	Title string // Markdown heading (e.g. "#### Overall Flow"), empty if none
	Code  string // Validated Mermaid code (without fences)
}

// ValidateMermaidBlocks extracts all ```mermaid blocks from an LLM response,
// auto-fixes and validates each one, and returns them with their preceding #### titles.
// Invalid blocks are silently skipped.
func ValidateMermaidBlocks(content string) []MermaidBlock {
	raw := extractAllMermaidBlocks(content)
	var blocks []MermaidBlock
	for _, r := range raw {
		fixed := fixMermaid(r.Code)
		if !isValidMermaid(fixed) {
			continue
		}
		blocks = append(blocks, MermaidBlock{Title: r.Title, Code: fixed})
	}
	return blocks
}

// ValidateMermaid extracts, auto-fixes, and validates the first Mermaid code block from an LLM response.
// Kept for backward compatibility.
func ValidateMermaid(content string) (string, bool) {
	blocks := ValidateMermaidBlocks(content)
	if len(blocks) == 0 {
		// Fallback: try treating whole content as mermaid
		if strings.Contains(content, "-->") || strings.Contains(content, "--->") ||
			strings.Contains(content, "-.->") || strings.Contains(content, "==>") {
			fixed := fixMermaid(content)
			if isValidMermaid(fixed) {
				return fixed, true
			}
		}
		return "", false
	}
	return blocks[0].Code, true
}

// isValidMermaid checks basic syntax requirements for a mermaid code string.
func isValidMermaid(code string) bool {
	if !strings.HasPrefix(code, "graph") && !strings.HasPrefix(code, "flowchart") {
		return false
	}
	return strings.Contains(code, "-->") || strings.Contains(code, "-.->") || strings.Contains(code, "==>")
}

// fixMermaid applies common auto-corrections to LLM-generated mermaid code.
func fixMermaid(code string) string {
	code = strings.TrimSpace(code)

	// Fix wrong arrow types: ---> ==> -> to -->
	code = fixArrows(code)

	// Fix Chinese brackets: A【文字】 → A["文字"]
	code = fixChineseBrackets(code)

	// Fix missing quotes in node text: A[text] → A["text"]
	code = fixMissingQuotes(code)

	// Prepend graph LR if missing
	if !strings.HasPrefix(code, "graph") && !strings.HasPrefix(code, "flowchart") {
		code = "graph LR\n" + code
	}

	// Clean up empty lines
	lines := strings.Split(code, "\n")
	var cleaned []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return strings.Join(cleaned, "\n")
}

// fixArrows normalizes invalid arrow syntax while preserving valid Mermaid arrows.
// Valid arrows preserved: --> (solid), -.-> (dashed), ==> (thick).
// Invalid arrows fixed: ---> → -->, ===> → ==>, bare -> → -->.
func fixArrows(code string) string {
	// Fix overly long arrows: ---> to -->, ===> to ==>
	code = strings.ReplaceAll(code, "===>", "==>")
	code = strings.ReplaceAll(code, "--->", "-->")

	// Fix bare -> (not part of --> or -.->)
	result := strings.Builder{}
	for i := 0; i < len(code); i++ {
		if i+3 < len(code) && code[i] == '-' && code[i+1] == '.' && code[i+2] == '-' && code[i+3] == '>' {
			// Preserve -.->
			result.WriteString("-.->")
			i += 3
		} else if i+2 < len(code) && code[i] == '=' && code[i+1] == '=' && code[i+2] == '>' {
			// Preserve ==>
			result.WriteString("==>")
			i += 2
		} else if i+2 < len(code) && code[i] == '-' && code[i+1] == '-' && code[i+2] == '>' {
			// Preserve -->
			result.WriteString("-->")
			i += 2
		} else if i+1 < len(code) && code[i] == '-' && code[i+1] == '>' {
			// Fix bare -> to -->
			result.WriteString("-->")
			i += 1
		} else {
			result.WriteByte(code[i])
		}
	}
	return result.String()
}

// fixChineseBrackets replaces 【】with [""].
func fixChineseBrackets(code string) string {
	re := regexp.MustCompile(`(\w+)【([^】]+)】`)
	return re.ReplaceAllString(code, `$1["$2"]`)
}

// fixMissingQuotes adds quotes to node text that's missing them.
// Matches A[text] where text doesn't start with " and converts to A["text"].
func fixMissingQuotes(code string) string {
	re := regexp.MustCompile(`(\w+)\[([^"\]]+)\]`)
	return re.ReplaceAllStringFunc(code, func(match string) string {
		sub := regexp.MustCompile(`(\w+)\[([^"\]]+)\]`)
		parts := sub.FindStringSubmatch(match)
		if len(parts) == 3 {
			return fmt.Sprintf(`%s["%s"]`, parts[1], parts[2])
		}
		return match
	})
}

// rawMermaidBlock holds a raw extracted block before validation.
type rawMermaidBlock struct {
	Title string
	Code  string
}

// extractAllMermaidBlocks finds all ```mermaid blocks and their preceding #### titles.
func extractAllMermaidBlocks(content string) []rawMermaidBlock {
	const startMarker = "```mermaid"
	const endMarker = "```"

	var blocks []rawMermaidBlock
	lines := strings.Split(content, "\n")

	// Track the last #### heading seen before each mermaid block.
	var lastHeading string

	for i := 0; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])

		if strings.HasPrefix(trimmed, "#### ") {
			lastHeading = trimmed
			continue
		}

		if trimmed != startMarker {
			continue
		}

		// Found a ```mermaid line — collect until closing ```.
		i++ // skip the ```mermaid line
		var codeLines []string
		found := false
		for i < len(lines) {
			if strings.TrimSpace(lines[i]) == endMarker {
				found = true
				break
			}
			codeLines = append(codeLines, lines[i])
			i++
		}
		if !found && len(codeLines) == 0 {
			continue
		}

		code := strings.TrimSpace(strings.Join(codeLines, "\n"))
		if code != "" {
			blocks = append(blocks, rawMermaidBlock{Title: lastHeading, Code: code})
		}
		lastHeading = "" // reset after consuming
	}

	return blocks
}
