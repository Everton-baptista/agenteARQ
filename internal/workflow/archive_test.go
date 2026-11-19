package workflow

import (
	"archive/tar"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestSourceArchiveOmitsHostOwnership(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0700); err != nil {
		t.Fatal(err)
	}
	write(t, source, "nested/server.ts", "export const port = 8080;\n")
	archive, err := sourceArchive(source, filepath.Join(root, "source.tar"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	reader := tar.NewReader(archive)
	files := 0
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Uid != 0 || header.Gid != 0 || header.Uname != "" || header.Gname != "" {
			t.Fatalf("host ownership leaked: %+v", header)
		}
		if header.Typeflag == tar.TypeReg {
			files++
		}
	}
	if files != 1 {
		t.Fatalf("archive lost source: %d regular files", files)
	}
}
