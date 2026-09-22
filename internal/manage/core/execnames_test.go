package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Linux command names are lower case. "Systemctl" once reached a release
// through a rename of the Go function wrapping it, and every tunnel setup then
// failed with "executable file not found". This walks the source for any
// exec.Command whose program name starts with a capital letter.
func TestNoCapitalisedCommandNames(t *testing.T) {
	re := regexp.MustCompile(`exec\.Command(Context)?\((ctx, )?"[A-Z][^"]*"`)
	root := filepath.Join("..", "..", "..")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAll(b, -1) {
			t.Errorf("%s: %s — command names are lower case on Linux", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
