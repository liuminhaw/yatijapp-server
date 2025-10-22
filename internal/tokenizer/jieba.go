package tokenizer

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed "dict"
var jiebaFS embed.FS

func writeEmbeddedFile(fs embed.FS, src, target string) error {
	data, err := fs.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(target, data, 0644); err != nil {
		return err
	}
	return nil
}

func WriteJiebaDictFiles() ([]string, func(), error) {
	dictFiles := []string{
		"dict/jieba.dict.utf8",
		"dict/hmm_model.utf8",
		"dict/user.dict.utf8",
		"dict/idf.utf8",
		"dict/stop_words.utf8",
	}

	tmpDir, err := os.MkdirTemp("", "gojieba-******")
	if err != nil {
		return nil, nil, err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	var files []string
	for _, srcFile := range dictFiles {
		targetPath := filepath.Join(tmpDir, filepath.Base(srcFile))
		if err := writeEmbeddedFile(jiebaFS, srcFile, targetPath); err != nil {
			return nil, nil, err
		}
		files = append(files, targetPath)
	}

	return files, cleanup, nil
}
