package util

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestTreeBudgets(t *testing.T) {
	for _, scenario := range []string{"file", "total", "count"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			count, size := 1, int64((32<<20)+1)
			if scenario == "total" {
				count, size = 9, 32<<20
			}
			if scenario == "count" {
				count, size = 10001, 0
			}
			for i := 0; i < count; i++ {
				f, err := os.Create(filepath.Join(root, fmt.Sprint(i)))
				if err != nil {
					t.Fatal(err)
				}
				err = f.Truncate(size)
				f.Close()
				if err != nil {
					t.Fatal(err)
				}
			}
			if CheckTree(root) == nil {
				t.Fatal("oversized input accepted")
			}
		})
	}
}
