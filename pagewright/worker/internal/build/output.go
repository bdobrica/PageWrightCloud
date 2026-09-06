package build

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// Static structural/reference checks, not a browser, JS execution, CSS validator
// or external-link availability test. Archive validation applies separately.
func ValidateOutput(root string) error {
	index, err := os.Stat(filepath.Join(root, "index.html"))
	if err != nil || !index.Mode().IsRegular() || index.Size() == 0 {
		return fmt.Errorf("missing nonempty public/index.html")
	}
	return filepath.Walk(root, func(name string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular output")
		}
		if !strings.HasSuffix(name, ".html") {
			return nil
		}
		if info.Size() > 32<<20 {
			return fmt.Errorf("HTML too large")
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if !utf8.Valid(data) {
			return fmt.Errorf("HTML is not UTF-8")
		}
		tokenizer := html.NewTokenizer(strings.NewReader(string(data)))
		tokenizer.SetMaxBuf(1 << 20)
		hasHTML, hasBody := false, false
		for {
			kind := tokenizer.Next()
			if kind == html.ErrorToken {
				if tokenizer.Err() != io.EOF {
					return tokenizer.Err()
				}
				break
			}
			if kind != html.StartTagToken && kind != html.SelfClosingTagToken {
				continue
			}
			token := tokenizer.Token()
			if token.Data == "html" {
				hasHTML = true
			}
			if token.Data == "body" {
				hasBody = true
			}
			if token.Data == "base" {
				return fmt.Errorf("base URL overrides are unsupported")
			}
			for _, attr := range token.Attr {
				if attr.Key != "src" && attr.Key != "href" && attr.Key != "poster" {
					continue
				}
				if err := checkReference(root, name, attr.Val); err != nil {
					return err
				}
			}
		}
		if !hasHTML || !hasBody {
			return fmt.Errorf("HTML document lacks html/body: %s", filepath.Base(name))
		}
		return nil
	})
}

func checkReference(root, page, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.HasPrefix(raw, "#") {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid output reference")
	}
	if u.IsAbs() || u.Host != "" {
		switch strings.ToLower(u.Scheme) {
		case "http", "https", "mailto", "tel", "":
			return nil
		default:
			return fmt.Errorf("unsupported output URL scheme")
		}
	}
	if strings.ContainsAny(u.Path, "\\\x00") {
		return fmt.Errorf("invalid local reference")
	}
	rel, _ := filepath.Rel(root, page)
	base := &url.URL{Path: "/" + filepath.ToSlash(rel)}
	resolved := base.ResolveReference(u)
	target := filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(path.Clean(resolved.Path), "/")))
	info, err := os.Stat(target)
	if err != nil {
		return fmt.Errorf("missing local output reference %q", raw)
	}
	if info.IsDir() {
		info, err = os.Stat(filepath.Join(target, "index.html"))
	}
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("invalid local output reference %q", raw)
	}
	return nil
}
