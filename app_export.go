package main

import (
	"errors"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"os"
)

func (a *App) ExportMarkdown(text string) (string, error) {
	if a.ctx == nil {
		return "", errors.New("desktop window is unavailable")
	}
	if len(text) > 4*1024*1024 {
		return "", errors.New("export exceeds 4 MB")
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{DefaultFilename: "snapTrans-cards.md", Filters: []runtime.FileFilter{{DisplayName: "Markdown", Pattern: "*.md"}}})
	if err != nil || path == "" {
		return "", err
	}
	return path, os.WriteFile(path, []byte(text), 0600)
}
