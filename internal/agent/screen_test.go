package agent

import (
	"context"
	"strings"
	"testing"
)

// answering builds a provider that replies with one text, like a model's screening answer.
func answering(reply string) *scriptedProvider {
	return &scriptedProvider{responses: []Response{{Message: Message{Role: "assistant", Content: reply}, StopReason: "end_turn", Usage: Usage{InputTokens: 5}}}}
}

func TestScreenReadsFulltextOverAbstractAndParsesTheVerdict(t *testing.T) {
	provider := answering("判断如下：\n```json\n{\"verdict\": \"Included\", \"reason\": \"用 10x 做了拟南芥根的 scRNA-seq。\"}\n```")
	judgement, err := Screen(context.Background(), provider, ScreenInput{
		Requirement: "植物单细胞转录组", Title: "Root atlas", Abstract: "An atlas.", Fulltext: strings.Repeat("全文 ", 10),
	})
	if err != nil {
		t.Fatal(err)
	}
	if judgement.Verdict != "included" || judgement.Basis != "fulltext" || !strings.Contains(judgement.Reason, "10x") || judgement.Usage.InputTokens != 5 {
		t.Fatalf("judgement = %+v", judgement)
	}
	if len(provider.requests) != 1 || len(provider.requests[0].Tools) != 0 || provider.requests[0].System == "" {
		t.Fatalf("request = %+v", provider.requests)
	}
	prompt := provider.requests[0].Messages[0].Content
	for _, want := range []string{"## 筛选要求\n植物单细胞转录组", "- 题目：Root atlas", "## 摘要\nAn atlas.", "## 全文\n全文"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt lacks %q:\n%s", want, prompt)
		}
	}
}

func TestScreenSkipsAPaperWithNothingToRead(t *testing.T) {
	provider := answering(`{"verdict":"excluded","reason":"x"}`)
	judgement, err := Screen(context.Background(), provider, ScreenInput{Requirement: "r", Title: "only a title"})
	if err != nil {
		t.Fatal(err)
	}
	if !judgement.Skipped || len(provider.requests) != 0 {
		t.Fatalf("judgement = %+v, requests = %d", judgement, len(provider.requests))
	}
	if _, err := Screen(context.Background(), provider, ScreenInput{Abstract: "a"}); err == nil {
		t.Fatal("empty requirement accepted")
	}
}

func TestScreenTruncatesLongFulltextAndRejectsUnknownVerdicts(t *testing.T) {
	provider := answering(`{"verdict":"maybe","reason":"?"}`)
	_, err := Screen(context.Background(), provider, ScreenInput{Requirement: "r", Abstract: "a", Fulltext: strings.Repeat("字", maxFulltextRunes+10)})
	if err == nil || !strings.Contains(err.Error(), "maybe") {
		t.Fatalf("err = %v", err)
	}
	prompt := provider.requests[0].Messages[0].Content
	if !strings.Contains(prompt, "已截断") || len([]rune(prompt)) > maxFulltextRunes+500 {
		t.Fatalf("full text not truncated: %d runes", len([]rune(prompt)))
	}
	if _, err := parseJudgement("no json here"); err == nil {
		t.Fatal("prose accepted as a judgement")
	}
}

func TestTranslateReturnsTheModelsText(t *testing.T) {
	provider := answering("  拟南芥根尖的单细胞图谱。\n")
	text, err := Translate(context.Background(), provider, "A single-cell atlas of the Arabidopsis root tip.")
	if err != nil || text != "拟南芥根尖的单细胞图谱。" {
		t.Fatalf("text = %q, err = %v", text, err)
	}
	if provider.requests[0].System == "" || len(provider.requests[0].Tools) != 0 {
		t.Fatalf("request = %+v", provider.requests[0])
	}
	if _, err := Translate(context.Background(), answering("x"), "   "); err == nil {
		t.Fatal("empty text accepted")
	}
}
