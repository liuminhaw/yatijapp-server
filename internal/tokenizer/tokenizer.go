package tokenizer

import (
	"regexp"
	"strings"

	"github.com/yanyiwu/gojieba"
)

type Tokenizer struct {
	Chinese string
	English string
}

var (
	reChinese = regexp.MustCompile(`[\p{Han}]+`)
	reEnglish = regexp.MustCompile(`[a-zA-Z0-9]+`)
)

func New(s string, jieba *gojieba.Jieba) *Tokenizer {
	chineseMatches := reChinese.FindAllString(s, -1)
	englishMatches := reEnglish.FindAllString(s, -1)

	rawChinese := strings.Join(chineseMatches, "")
	chineseTokens := jieba.CutForSearch(rawChinese, true)

	return &Tokenizer{
		Chinese: strings.Join(chineseTokens, " "),
		English: strings.Join(englishMatches, " "),
	}
}
