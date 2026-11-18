package agentarch

import (
	"io/fs"
	"path"
	"testing"
)

func TestEmbeddedContentHasNoPythonBytecode(t *testing.T) {
	err := fs.WalkDir(Content, "content", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Name() == "__pycache__" || (!entry.IsDir() && path.Ext(name) == ".pyc") {
			t.Errorf("embedded payload includes Python cache %s; run helper tests without writing bytecode and rebuild from clean content", name)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestEmbeddedContentRetainsRequiredDotFiles(t *testing.T) {
	for _, name := range []string{
		"content/blueprints/chatbot-web/.env.example",
		"content/blueprints/chatbot-web/.gitignore",
		"content/blueprints/chatbot-web/.github/workflows/agentarch.yml",
	} {
		body, err := Content.ReadFile(name)
		if err != nil || len(body) == 0 {
			t.Errorf("required blueprint dotfile is missing: %s: %v", name, err)
		}
	}
}
