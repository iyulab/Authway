package mail

import (
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

// Locales are the languages a message can be written in: one per file in
// locales/, in alphabetical order.
func Locales() []string {
	entries, err := fs.Glob(files, "locales/*.json")
	if err != nil {
		return []string{DefaultLocale}
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, strings.TrimSuffix(strings.TrimPrefix(e, "locales/"), ".json"))
	}
	sort.Strings(out)
	return out
}

// Pick resolves a language a caller names outright — one tag, "ko" or "ko-KR"
// or "ko_KR" — to the locale it stands for. Unlike Negotiate it does not fall
// back: a tag with no locale, an empty tag, a list or a "*" answers false, so a
// caller that asked for a language is told when there is none instead of being
// answered in another one.
func Pick(tag string) (string, bool) {
	tag = strings.ToLower(strings.TrimSpace(tag))
	if tag == "" || strings.ContainsAny(tag, ",;* ") {
		return "", false
	}
	lang, _, _ := strings.Cut(strings.ReplaceAll(tag, "_", "-"), "-")
	for _, l := range Locales() {
		if l == lang {
			return l, true
		}
	}
	return "", false
}

// Negotiate picks the locale for a language preference: an Accept-Language
// header ("ko-KR,ko;q=0.9,en;q=0.8"), or a single tag a person chose ("ko",
// "en-US"). The best-ranked tag whose language has a locale wins, so a region
// tag (ko-KR) matches its language (ko); a tag of weight 0 is refused, the
// way the header defines it. Nothing usable — an empty or unknown preference,
// or "*" alone — gives DefaultLocale.
func Negotiate(preference string) string {
	supported := map[string]bool{}
	for _, l := range Locales() {
		supported[l] = true
	}
	type ranked struct {
		lang  string
		q     float64
		order int
	}
	var tags []ranked
	for i, part := range strings.Split(preference, ",") {
		fields := strings.Split(part, ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" || tag == "*" {
			continue
		}
		q := 1.0
		for _, param := range fields[1:] {
			name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
			if !ok || strings.ToLower(strings.TrimSpace(name)) != "q" {
				continue
			}
			if parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				q = parsed
			}
		}
		if q <= 0 {
			continue
		}
		lang, _, _ := strings.Cut(strings.ReplaceAll(tag, "_", "-"), "-")
		tags = append(tags, ranked{lang: lang, q: q, order: i})
	}
	// Highest weight first; equal weights keep the order they were written in.
	sort.SliceStable(tags, func(a, b int) bool { return tags[a].q > tags[b].q })
	for _, t := range tags {
		if supported[t.lang] {
			return t.lang
		}
	}
	return DefaultLocale
}
