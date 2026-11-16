// Package i18n serves the interface strings from JSON catalogs embedded in
// the binary, one flat file per language.
package i18n

import (
	"embed"
	"encoding/json"
	"path"
	"sort"
	"strings"
)

//go:embed locales/*.json
var locales embed.FS

const fallback = "en"

// catalogs holds every embedded catalog, keyed by language code.
var catalogs = loadCatalogs()

func loadCatalogs() map[string]map[string]string {
	entries, err := locales.ReadDir("locales")
	if err != nil {
		panic(err)
	}
	out := make(map[string]map[string]string, len(entries))
	for _, e := range entries {
		data, err := locales.ReadFile(path.Join("locales", e.Name()))
		if err != nil {
			panic(err)
		}
		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			panic("i18n: " + e.Name() + ": " + err.Error())
		}
		out[strings.TrimSuffix(e.Name(), ".json")] = m
	}
	return out
}

// Supported returns the sorted codes of the embedded languages.
func Supported() []string {
	out := make([]string, 0, len(catalogs))
	for lang := range catalogs {
		out = append(out, lang)
	}
	sort.Strings(out)
	return out
}

// normalize reduces a locale such as "fr_FR.UTF-8@euro" to "fr".
func normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, "_.@"); i >= 0 {
		s = s[:i]
	}
	return s
}

// Detect returns the first supported language among the override, DCTOP_LANG,
// each entry of LANGUAGE, LC_ALL, LC_MESSAGES and LANG. Empty, C and POSIX
// values are skipped. English is the default.
func Detect(override string, getenv func(string) string) string {
	candidates := []string{override, getenv("DCTOP_LANG")}
	candidates = append(candidates, strings.Split(getenv("LANGUAGE"), ":")...)
	candidates = append(candidates, getenv("LC_ALL"), getenv("LC_MESSAGES"), getenv("LANG"))
	for _, c := range candidates {
		lang := normalize(c)
		if lang == "" || lang == "c" || lang == "posix" {
			continue
		}
		if _, ok := catalogs[lang]; ok {
			return lang
		}
	}
	return fallback
}

// Translator looks up interface strings in one language.
type Translator struct {
	lang     string
	catalogs map[string]map[string]string
}

// New returns a translator for lang; an unsupported language gives English.
func New(lang string) *Translator {
	return newFromCatalogs(lang, catalogs)
}

func newFromCatalogs(lang string, cats map[string]map[string]string) *Translator {
	if _, ok := cats[lang]; !ok {
		lang = fallback
	}
	return &Translator{lang: lang, catalogs: cats}
}

// Lang returns the language code in use.
func (t *Translator) Lang() string { return t.lang }

// T returns the string for key with each {name} replaced by its value; kv
// holds name/value pairs. A key missing from the language falls back to
// English, then to the key itself.
func (t *Translator) T(key string, kv ...string) string {
	s, ok := t.catalogs[t.lang][key]
	if !ok {
		if s, ok = t.catalogs[fallback][key]; !ok {
			s = key
		}
	}
	for i := 0; i+1 < len(kv); i += 2 {
		s = strings.ReplaceAll(s, "{"+kv[i]+"}", kv[i+1])
	}
	return s
}
