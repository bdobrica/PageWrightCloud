package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bdobrica/PageWrightCloud/compiler/internal/assets"
	"github.com/bdobrica/PageWrightCloud/compiler/internal/config"
)

func TestMetadataAndOutputLimits(t *testing.T) {
	source := seed(t)
	put(t, source, "site.json", `{"site_name":"`+strings.Repeat("a", 64<<10)+`"}`)
	out := filepath.Join(t.TempDir(), "output")
	if _, err := config.Load(starter(t), source, out, ""); err == nil {
		t.Fatal("oversized config accepted")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatal("output created before validation")
	}
	path := filepath.Join(t.TempDir(), "sentinel")
	if err := os.WriteFile(path, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if assets.WriteFile(path, make([]byte, (32<<20)+1)) == nil {
		t.Fatal("oversized output accepted")
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "keep" {
		t.Fatal("existing output changed")
	}
}
