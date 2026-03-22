package summarizer

import (
	"fmt"
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
				"- 第一行必須是 graph TD\n"+
				"- 每個節點格式：大寫字母[\"節點文字\"]，例如 A[\"開場\"]、B[\"分析\"]\n"+
				"- 連接格式：A --> B\n"+
				"- 節點數量 5-12 個\n"+
				"- 用 ```mermaid 和 ``` 包裹\n"+
				"- 不要輸出任何說明文字\n\n"+
				"範例：\n```mermaid\ngraph TD\nA[\"主題介紹\"] --> B[\"原因分析\"]\nB --> C[\"影響評估\"]\nC --> D[\"結論\"]\n```\n\n"+
				"摘要：\n%s",
			summary,
		)
	case "ja":
		return fmt.Sprintf(
			"以下の動画要約に基づき、Mermaid フローチャートで動画の論理構成を表現してください。\n\n"+
				"厳格ルール：\n"+
				"- 最初の行は graph TD\n"+
				"- ノード形式：大文字[\"テキスト\"]、例：A[\"導入\"]、B[\"分析\"]\n"+
				"- 接続形式：A --> B\n"+
				"- ノード数 5-12 個\n"+
				"- ```mermaid と ``` で囲む\n"+
				"- 説明文不要\n\n"+
				"例：\n```mermaid\ngraph TD\nA[\"テーマ紹介\"] --> B[\"原因分析\"]\nB --> C[\"影響評価\"]\nC --> D[\"結論\"]\n```\n\n"+
				"要約：\n%s",
			summary,
		)
	default:
		return fmt.Sprintf(
			"Based on the video summary below, create a Mermaid flowchart showing the narrative logic.\n\n"+
				"Strict rules:\n"+
				"- First line must be graph TD\n"+
				"- Node format: UPPERCASE[\"text\"], e.g. A[\"Introduction\"], B[\"Analysis\"]\n"+
				"- Connection format: A --> B\n"+
				"- 5-12 nodes\n"+
				"- Wrap in ```mermaid and ```\n"+
				"- No explanation text\n\n"+
				"Example:\n```mermaid\ngraph TD\nA[\"Topic Introduction\"] --> B[\"Root Cause Analysis\"]\nB --> C[\"Impact Assessment\"]\nC --> D[\"Conclusion\"]\n```\n\n"+
				"Summary:\n%s",
			summary,
		)
	}
}

// ValidateMermaid extracts and validates a Mermaid code block from an LLM response.
// It looks for ```mermaid ... ``` blocks, validates basic syntax requirements,
// and returns the cleaned content and a validity flag.
func ValidateMermaid(content string) (string, bool) {
	// Extract mermaid code block
	mermaidCode := extractMermaidBlock(content)
	if mermaidCode == "" {
		return "", false
	}

	// Basic validation: must start with "graph" or "flowchart"
	trimmed := strings.TrimSpace(mermaidCode)
	if !strings.HasPrefix(trimmed, "graph") && !strings.HasPrefix(trimmed, "flowchart") {
		return "", false
	}

	// Must contain at least one arrow
	if !strings.Contains(trimmed, "-->") {
		return "", false
	}

	return trimmed, true
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
