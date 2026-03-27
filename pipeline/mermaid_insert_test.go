package pipeline

import (
	"strings"
	"testing"

	"github.com/kouko/youtube-summarize-scraper/summarizer"
)

func TestNormalizeHeading(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"#### 章節標題", "章節標題"},
		{"### 章節標題", "章節標題"},
		{"## 概述", "概述"},
		{"  #### Title  ", "Title"},
		{"no hash", "no hash"},
		{"", ""},
	}
	for _, tt := range tests {
		got := normalizeHeading(tt.input)
		if got != tt.want {
			t.Errorf("normalizeHeading(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestInsertMermaidBlocks_ExactH4Match(t *testing.T) {
	summary := strings.Join([]string{
		"### 概述",
		"概述內容",
		"",
		"### 章節摘要",
		"",
		"#### 第一章",
		"- 要點一",
		"",
		"#### 第二章",
		"- 要點二",
	}, "\n")

	blocks := []summarizer.MermaidBlock{
		{Title: "#### 第一章", Code: "graph LR\nA-->B"},
		{Title: "#### 第二章", Code: "graph LR\nC-->D"},
	}

	result := insertMermaidBlocksAfterFirstHeading(summary, blocks)

	// Mermaid for 第一章 should appear before #### 第二章
	idx1 := strings.Index(result, "A-->B")
	idx2 := strings.Index(result, "#### 第二章")
	if idx1 < 0 || idx2 < 0 || idx1 >= idx2 {
		t.Errorf("第一章 mermaid should appear before 第二章 heading\n%s", result)
	}

	// Mermaid for 第二章 should appear after #### 第二章
	idxM2 := strings.Index(result, "C-->D")
	if idxM2 < 0 || idxM2 <= idx2 {
		t.Errorf("第二章 mermaid should appear after 第二章 heading\n%s", result)
	}
}

func TestInsertMermaidBlocks_H4MermaidVsH3Summary(t *testing.T) {
	// The actual bug: summary uses ### for chapters, mermaid uses ####.
	summary := strings.Join([]string{
		"## 概述",
		"概述內容",
		"",
		"## 章節摘要",
		"",
		"### 第一章",
		"- 要點一",
		"",
		"### 第二章",
		"- 要點二",
		"",
		"### 第三章",
		"- 要點三",
	}, "\n")

	blocks := []summarizer.MermaidBlock{
		{Title: "#### 第一章", Code: "graph LR\nA-->B"},
		{Title: "#### 第二章", Code: "graph LR\nC-->D"},
		{Title: "#### 第三章", Code: "graph LR\nE-->F"},
	}

	result := insertMermaidBlocksAfterFirstHeading(summary, blocks)

	// Each mermaid should appear between its chapter and the next chapter.
	idx1 := strings.Index(result, "### 第一章")
	idxM1 := strings.Index(result, "A-->B")
	idx2 := strings.Index(result, "### 第二章")
	idxM2 := strings.Index(result, "C-->D")
	idx3 := strings.Index(result, "### 第三章")
	idxM3 := strings.Index(result, "E-->F")

	if idx1 < 0 || idxM1 < 0 || idx2 < 0 || idxM2 < 0 || idx3 < 0 || idxM3 < 0 {
		t.Fatalf("missing expected content in result:\n%s", result)
	}

	if !(idx1 < idxM1 && idxM1 < idx2) {
		t.Errorf("第一章 mermaid not between 第一章 and 第二章:\nidx1=%d, idxM1=%d, idx2=%d\n%s", idx1, idxM1, idx2, result)
	}
	if !(idx2 < idxM2 && idxM2 < idx3) {
		t.Errorf("第二章 mermaid not between 第二章 and 第三章:\nidx2=%d, idxM2=%d, idx3=%d\n%s", idx2, idxM2, idx3, result)
	}
	if !(idx3 < idxM3) {
		t.Errorf("第三章 mermaid not after 第三章:\nidx3=%d, idxM3=%d\n%s", idx3, idxM3, result)
	}
}

func TestInsertMermaidBlocks_Unmatched(t *testing.T) {
	summary := strings.Join([]string{
		"### 概述",
		"概述內容",
		"",
		"### 章節摘要",
		"章節內容",
	}, "\n")

	blocks := []summarizer.MermaidBlock{
		{Title: "#### 完全不同的標題", Code: "graph LR\nX-->Y"},
	}

	result := insertMermaidBlocksAfterFirstHeading(summary, blocks)

	// Unmatched block should still appear in the output (fallback position).
	if !strings.Contains(result, "X-->Y") {
		t.Errorf("unmatched mermaid block should appear in output:\n%s", result)
	}

	// Should appear after 概述, before 章節摘要.
	idxOverview := strings.Index(result, "### 概述")
	idxMermaid := strings.Index(result, "X-->Y")
	idxSection := strings.Index(result, "### 章節摘要")
	if !(idxOverview < idxMermaid && idxMermaid < idxSection) {
		t.Errorf("unmatched block should be between overview and section:\n%s", result)
	}
}

func TestInsertMermaidBlocks_MixedHeadingLevels(t *testing.T) {
	// Summary has one #### and one ### chapter.
	summary := strings.Join([]string{
		"### 概述",
		"概述內容",
		"",
		"#### 章節A",
		"- 內容A",
		"",
		"### 章節B",
		"- 內容B",
	}, "\n")

	blocks := []summarizer.MermaidBlock{
		{Title: "#### 章節A", Code: "graph LR\nA-->B"},
		{Title: "#### 章節B", Code: "graph LR\nC-->D"},
	}

	result := insertMermaidBlocksAfterFirstHeading(summary, blocks)

	idxA := strings.Index(result, "#### 章節A")
	idxMA := strings.Index(result, "A-->B")
	idxB := strings.Index(result, "### 章節B")
	idxMB := strings.Index(result, "C-->D")

	if !(idxA < idxMA && idxMA < idxB) {
		t.Errorf("章節A mermaid not between A and B:\n%s", result)
	}
	if !(idxB < idxMB) {
		t.Errorf("章節B mermaid not after B:\n%s", result)
	}
}

func TestInsertMermaidBlocks_NoBlocks(t *testing.T) {
	summary := "### 概述\n內容"
	result := insertMermaidBlocksAfterFirstHeading(summary, nil)
	if result != summary {
		t.Errorf("no blocks should return original summary\ngot: %q\nwant: %q", result, summary)
	}
}
