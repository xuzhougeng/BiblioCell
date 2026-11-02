package agent

import (
	"context"
	"errors"
	"strings"
)

const translateSystem = `你是翻译助手。把用户给出的文本翻译成简体中文，保留术语、基因名、物种学名和数字原样。
只输出译文，不加解释、不加引号、不加标题。`

// maxTranslateRunes caps one translation request; a screening page's abstract
// or a selected passage is far shorter.
const maxTranslateRunes = 20_000

// Translate renders text in Chinese with a single completion.
func Translate(ctx context.Context, provider Provider, text string) (string, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("没有要翻译的文字")
	}
	response, err := provider.Complete(ctx, Request{System: translateSystem, Messages: []Message{{Role: "user", Content: truncateRunes(text, maxTranslateRunes)}}})
	if err != nil {
		return "", err
	}
	translated := strings.TrimSpace(response.Message.Content)
	if translated == "" {
		return "", errors.New("模型没有返回译文")
	}
	return translated, nil
}
