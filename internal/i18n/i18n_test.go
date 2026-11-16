package i18n

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func env(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestDetect(t *testing.T) {
	cases := []struct {
		name, override string
		env            map[string]string
		want           string
	}{
		{"override wins", "fr", map[string]string{"LANG": "en_US.UTF-8"}, "fr"},
		{"DCTOP_LANG", "", map[string]string{"DCTOP_LANG": "fr", "LANG": "en_US.UTF-8"}, "fr"},
		{"LANGUAGE list", "", map[string]string{"LANGUAGE": "de:fr:en", "LANG": "en_US.UTF-8"}, "fr"},
		{"LC_ALL beats LANG", "", map[string]string{"LC_ALL": "fr_FR.UTF-8", "LANG": "en_US.UTF-8"}, "fr"},
		{"LC_MESSAGES beats LANG", "", map[string]string{"LC_MESSAGES": "fr_FR.UTF-8", "LANG": "en_US.UTF-8"}, "fr"},
		{"C locale skipped", "", map[string]string{"LC_ALL": "C.UTF-8", "LANG": "fr_FR.UTF-8"}, "fr"},
		{"POSIX skipped", "", map[string]string{"LC_ALL": "POSIX", "LANG": "fr_FR"}, "fr"},
		{"only C locale", "", map[string]string{"LANG": "C.UTF-8"}, "en"},
		{"regional variant", "", map[string]string{"LANG": "fr_CA.UTF-8"}, "fr"},
		{"upper case", "FR", nil, "fr"},
		{"unsupported falls through", "de", map[string]string{"LANG": "fr_FR.UTF-8"}, "fr"},
		{"unsupported everywhere", "", map[string]string{"LANG": "de_DE.UTF-8"}, "en"},
		{"empty environment", "", nil, "en"},
	}
	for _, c := range cases {
		if got := Detect(c.override, env(c.env)); got != c.want {
			t.Errorf("%s: got %q want %q", c.name, got, c.want)
		}
	}
}

func TestSupported(t *testing.T) {
	if got := strings.Join(Supported(), ","); got != "en,fr" {
		t.Errorf("Supported() = %q", got)
	}
}

func TestT(t *testing.T) {
	if got := New("fr").T("state.running"); got != "démarré" {
		t.Errorf("fr: %q", got)
	}
	if got := New("en").T("confirm.remove.container", "name", "web"); got != "Remove container web?" {
		t.Errorf("placeholder: %q", got)
	}
	if got := New("en").T("no.such.key"); got != "no.such.key" {
		t.Errorf("missing key: %q", got)
	}
	if got := New("xx").Lang(); got != "en" {
		t.Errorf("unsupported lang: %q", got)
	}
	tr := newFromCatalogs("fr", map[string]map[string]string{"en": {"a": "A"}, "fr": {}})
	if got := tr.T("a"); got != "A" {
		t.Errorf("fallback to English: %q", got)
	}
}

var placeholderRe = regexp.MustCompile(`\{[a-z_]+\}`)

func placeholders(s string) string {
	ps := placeholderRe.FindAllString(s, -1)
	sort.Strings(ps)
	return strings.Join(ps, ",")
}

func loadCatalog(t *testing.T, lang string) map[string]string {
	t.Helper()
	data, err := locales.ReadFile("locales/" + lang + ".json")
	if err != nil {
		t.Fatalf("read %s: %v", lang, err)
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parse %s: %v", lang, err)
	}
	return m
}

// Every catalog has exactly the keys of en.json, and each entry uses the same {placeholders}.
func TestCatalogsAreConsistent(t *testing.T) {
	en := loadCatalog(t, "en")
	for _, lang := range Supported() {
		if lang == "en" {
			continue
		}
		cat := loadCatalog(t, lang)
		for k, v := range en {
			other, ok := cat[k]
			if !ok {
				t.Errorf("%s.json: missing key %q", lang, k)
				continue
			}
			if placeholders(v) != placeholders(other) {
				t.Errorf("%s.json: key %q uses {%s}, en.json uses {%s}", lang, k, placeholders(other), placeholders(v))
			}
		}
		for k := range cat {
			if _, ok := en[k]; !ok {
				t.Errorf("%s.json: extra key %q", lang, k)
			}
		}
	}
}

// Every literal key passed to .T("…") in the Go sources exists in en.json.
func TestKeysUsedInCodeExist(t *testing.T) {
	en := loadCatalog(t, "en")
	re := regexp.MustCompile(`\.T\("([a-z0-9_.]+)"`)
	root := filepath.Join("..", "..")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if path != root && (strings.HasPrefix(name, ".") || name == "bin" || name == "dist" || name == "docs") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range re.FindAllStringSubmatch(string(src), -1) {
			if _, ok := en[m[1]]; !ok {
				t.Errorf("%s: key %q is not in en.json", path, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
