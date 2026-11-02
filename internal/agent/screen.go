package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Judgement is the model's answer about one paper against a person's
// requirement. It is a suggestion the person adopts or ignores; nothing here
// writes to the library.
type Judgement struct {
	// Verdict is included, excluded, or uncertain.
	Verdict string `json:"verdict"`
	Reason  string `json:"reason"`
	// Basis says what the model read: fulltext or abstract.
	Basis string `json:"basis"`
	// Skipped is set when the paper had neither full text nor abstract, so no
	// model was asked.
	Skipped bool  `json:"skipped"`
	Usage   Usage `json:"usage"`
}

// ScreenInput is everything the model sees about one paper.
type ScreenInput struct {
	Requirement string
	Title       string
	Authors     string
	Year        string
	Journal     string
	Species     string
	Organs      string
	Modalities  string
	Abstract    string
	Fulltext    string
}

// Verdicts the model may answer with.
var Verdicts = []string{"included", "excluded", "uncertain"}

// maxFulltextRunes caps what one screening call sends: a long paper fits in
// most context windows at this size, and the front matter and methods carry
// what a screening decision needs.
const maxFulltextRunes = 120_000

const screenSystem = `你是文献筛选助手。一个研究者正在逐篇筛选文献，会给出筛选要求和一篇文献的信息（题目、元数据、摘要，可能还有全文）。
你的任务：判断这篇文献是否符合筛选要求。
- 只依据给出的内容判断，不要编造文中没有的信息。
- 符合要求答 included；明确不符合答 excluded；信息不足或模棱两可答 uncertain。
- reason 用中文写 1～3 句，引用文中的具体依据（物种、组织、技术、数据类型等）。
只输出一个 JSON 对象，不要输出其他文字：{"verdict": "included" | "excluded" | "uncertain", "reason": "..."}`

// Screen asks the model whether one paper meets the requirement. The caller
// decides what text it can supply; with neither full text nor abstract the
// answer is Skipped and no request is made.
func Screen(ctx context.Context, provider Provider, input ScreenInput) (Judgement, error) {
	if strings.TrimSpace(input.Requirement) == "" {
		return Judgement{}, errors.New("筛选要求不能为空")
	}
	if strings.TrimSpace(input.Fulltext) == "" && strings.TrimSpace(input.Abstract) == "" {
		return Judgement{Skipped: true}, nil
	}
	basis, prompt := screenPrompt(input)
	response, err := provider.Complete(ctx, Request{System: screenSystem, Messages: []Message{{Role: "user", Content: prompt}}})
	if err != nil {
		return Judgement{}, err
	}
	judgement, err := parseJudgement(response.Message.Content)
	if err != nil {
		return Judgement{}, err
	}
	judgement.Basis = basis
	judgement.Usage = response.Usage
	return judgement, nil
}

// screenPrompt lays out the requirement and the paper for the model and says
// which text it rests on.
func screenPrompt(input ScreenInput) (basis string, prompt string) {
	var b strings.Builder
	b.WriteString("## 筛选要求\n")
	b.WriteString(strings.TrimSpace(input.Requirement))
	b.WriteString("\n\n## 文献信息\n")
	field := func(label, value string) {
		if value = strings.TrimSpace(value); value != "" {
			fmt.Fprintf(&b, "- %s：%s\n", label, value)
		}
	}
	field("题目", input.Title)
	field("作者", input.Authors)
	field("年份", input.Year)
	field("期刊", input.Journal)
	field("物种", input.Species)
	field("组织", input.Organs)
	field("测序类型", input.Modalities)
	if abstract := strings.TrimSpace(input.Abstract); abstract != "" {
		b.WriteString("\n## 摘要\n")
		b.WriteString(abstract)
		b.WriteString("\n")
		basis = "abstract"
	}
	if fulltext := strings.TrimSpace(input.Fulltext); fulltext != "" {
		b.WriteString("\n## 全文\n")
		b.WriteString(truncateRunes(fulltext, maxFulltextRunes))
		b.WriteString("\n")
		basis = "fulltext"
	}
	return basis, b.String()
}

// parseJudgement reads the model's JSON answer, tolerating text or a code
// fence around it.
func parseJudgement(text string) (Judgement, error) {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start < 0 || end <= start {
		return Judgement{}, fmt.Errorf("模型没有返回 JSON 判断：%s", truncateRunes(strings.TrimSpace(text), 200))
	}
	var answer struct {
		Verdict string `json:"verdict"`
		Reason  string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(text[start:end+1]), &answer); err != nil {
		return Judgement{}, fmt.Errorf("模型返回的判断无法解析：%w", err)
	}
	verdict := strings.ToLower(strings.TrimSpace(answer.Verdict))
	switch verdict {
	case "included", "excluded", "uncertain":
	case "include", "yes", "keep":
		verdict = "included"
	case "exclude", "no", "drop":
		verdict = "excluded"
	default:
		return Judgement{}, fmt.Errorf("模型返回了未知的判断 %q", answer.Verdict)
	}
	return Judgement{Verdict: verdict, Reason: strings.TrimSpace(answer.Reason)}, nil
}

// truncateRunes keeps the first limit runes and says how much was cut.
func truncateRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit]) + fmt.Sprintf("\n…[已截断：共 %d 字，显示前 %d 字]", len(runes), limit)
}
