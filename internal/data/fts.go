package data

import (
	"github.com/liuminhaw/yatijapp/internal/tokenizer"
	"github.com/yanyiwu/gojieba"
)

// Full Text Search (FTS) struct type
type FTS struct {
	// UUID             uuid.UUID
	TitleToken       *tokenizer.Tokenizer
	DescriptionToken *tokenizer.Tokenizer
	NotesToken       *tokenizer.Tokenizer
	// TitleChineseTSVector       string `json:"title_chinese_tsv"`
	// TitleEnglishTSVector       string `json:"title_english_tsv"`
	// DescriptionChineseTSVector string `json:"description_chinese_tsv"`
	// DescriptionEnglishTSVector string `json:"description_english_tsv"`
}

func GenFTS(title, description, notes string, jieba *gojieba.Jieba) FTS {
	titleTokenizer := tokenizer.New(title, jieba)
	descriptionTokenizer := tokenizer.New(description, jieba)
	notesTokenizer := tokenizer.New(notes, jieba)

	return FTS{
		TitleToken:       titleTokenizer,
		DescriptionToken: descriptionTokenizer,
		NotesToken:       notesTokenizer,
	}
}
