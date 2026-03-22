package summarizer

import (
	"fmt"
	"regexp"
	"strings"
)

// MermaidPrompt generates a Stage 3 Mermaid flowchart prompt.
// The prompt language is selected by the language parameter.
func MermaidPrompt(summary string, language string) string {
	switch language {
	case "zh-Hant":
		return fmt.Sprintf(
			"請根據以下影片摘要，用 Mermaid 流程圖呈現影片的敘事邏輯或核心概念的關係。\n\n"+
				"嚴格規則：\n"+
				"- 第一行必須是 graph LR\n"+
				"- 節點文字格式：標題<br/>━━━━━━<br/>細節敘述，用 <br/>━━━━━━<br/> 換行\n"+
				"- 節點格式：大寫字母[\"標題<br/>━━━━━━<br/>細節\"]，例如 A[\"開場<br/>━━━━━━<br/>介紹影片主題\"]\n"+
				"- 連接格式：A --> B\n"+
				"- 節點數量 5-12 個\n"+
				"- 用 ```mermaid 和 ``` 包裹\n"+
				"- 不要輸出任何說明文字\n\n"+
				"範例：\n```mermaid\ngraph LR\nA[\"主題介紹<br/>━━━━━━<br/>說明背景與動機\"] --> B[\"原因分析<br/>━━━━━━<br/>拆解三大因素\"]\nB --> C[\"影響評估<br/>━━━━━━<br/>對市場的衝擊\"] --> D[\"結論<br/>━━━━━━<br/>投資建議與展望\"]\n```\n\n"+
				"摘要：\n%s",
			summary,
		)
	case "ja":
		return fmt.Sprintf(
			"以下の動画要約に基づき、Mermaid フローチャートで動画の論理構成を表現してください。\n\n"+
				"厳格ルール：\n"+
				"- 最初の行は graph LR\n"+
				"- ノードテキスト形式：タイトル<br/>━━━━━━<br/>詳細説明、<br/>━━━━━━<br/> で改行\n"+
				"- ノード形式：大文字[\"タイトル<br/>━━━━━━<br/>詳細\"]、例：A[\"導入<br/>━━━━━━<br/>テーマの紹介\"]\n"+
				"- 接続形式：A --> B\n"+
				"- ノード数 5-12 個\n"+
				"- ```mermaid と ``` で囲む\n"+
				"- 説明文不要\n\n"+
				"例：\n```mermaid\ngraph LR\nA[\"テーマ紹介<br/>━━━━━━<br/>背景と動機の説明\"] --> B[\"原因分析<br/>━━━━━━<br/>三大要因の分解\"]\nB --> C[\"影響評価<br/>━━━━━━<br/>市場への影響\"] --> D[\"結論<br/>━━━━━━<br/>今後の展望\"]\n```\n\n"+
				"要約：\n%s",
			summary,
		)
	default:
		return fmt.Sprintf(
			"Based on the video summary below, create a Mermaid flowchart showing the narrative logic.\n\n"+
				"Strict rules:\n"+
				"- First line must be graph LR\n"+
				"- Node text format: Title<br/>━━━━━━<br/>Detail description, use <br/>━━━━━━<br/> for line break\n"+
				"- Node format: UPPERCASE[\"Title<br/>━━━━━━<br/>Detail\"], e.g. A[\"Introduction<br/>━━━━━━<br/>Explain the topic\"]\n"+
				"- Connection format: A --> B\n"+
				"- 5-12 nodes\n"+
				"- Wrap in ```mermaid and ```\n"+
				"- No explanation text\n\n"+
				"Example:\n```mermaid\ngraph LR\nA[\"Topic Introduction<br/>━━━━━━<br/>Background and motivation\"] --> B[\"Root Cause<br/>━━━━━━<br/>Three key factors\"]\nB --> C[\"Impact Assessment<br/>━━━━━━<br/>Market implications\"] --> D[\"Conclusion<br/>━━━━━━<br/>Outlook and recommendations\"]\n```\n\n"+
				"Summary:\n%s",
			summary,
		)
	}
}

// ValidateMermaid extracts, auto-fixes, and validates a Mermaid code block from an LLM response.
// It looks for ```mermaid ... ``` blocks, applies common fixes, validates basic syntax,
// and returns the cleaned content and a validity flag.
func ValidateMermaid(content string) (string, bool) {
	mermaidCode := extractMermaidBlock(content)
	if mermaidCode == "" {
		// Try treating the whole content as mermaid if no code block found
		if strings.Contains(content, "-->") || strings.Contains(content, "--->") {
			mermaidCode = content
		} else {
			return "", false
		}
	}

	fixed := fixMermaid(mermaidCode)

	// Validate: must start with "graph" or "flowchart"
	if !strings.HasPrefix(fixed, "graph") && !strings.HasPrefix(fixed, "flowchart") {
		return "", false
	}

	// Must contain at least one arrow
	if !strings.Contains(fixed, "-->") {
		return "", false
	}

	return fixed, true
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

// fixArrows normalizes arrow syntax to standard -->.
func fixArrows(code string) string {
	// Order matters: longer patterns first
	code = strings.ReplaceAll(code, "===>", "-->")
	code = strings.ReplaceAll(code, "--->", "-->")
	code = strings.ReplaceAll(code, "==>", "-->")
	// Be careful with -> : only replace when not part of -->
	result := strings.Builder{}
	for i := 0; i < len(code); i++ {
		if i+2 < len(code) && code[i] == '-' && code[i+1] == '-' && code[i+2] == '>' {
			result.WriteString("-->")
			i += 2
		} else if i+1 < len(code) && code[i] == '-' && code[i+1] == '>' {
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

// extractMermaidBlock finds and extracts content between ```mermaid and ``` markers.
func extractMermaidBlock(content string) string {
	startMarker := "```mermaid"
	endMarker := "```"

	startIdx := strings.Index(content, startMarker)
	if startIdx == -1 {
		return ""
	}

	// Move past the start marker
	codeStart := startIdx + len(startMarker)
	// Skip any whitespace/newline after the marker
	for codeStart < len(content) && (content[codeStart] == '\n' || content[codeStart] == '\r') {
		codeStart++
	}

	// Find the closing ```
	remaining := content[codeStart:]
	endIdx := strings.Index(remaining, endMarker)
	if endIdx == -1 {
		return ""
	}

	return strings.TrimSpace(remaining[:endIdx])
}
