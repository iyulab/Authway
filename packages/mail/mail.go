// Package mail renders Authway's messages from the files beside it, which are
// the same files every Authway implementation renders (README.md describes
// them): a message is its subject, an HTML body and a plain-text body.
package mail

import (
	"embed"
	"encoding/json"
	"fmt"
	"html"
	"maps"
	"net/url"
	"path"
	"strings"
)

//go:embed messages locales
var files embed.FS

// DefaultLocale is the language a message is written in when no other is asked
// for, or the one asked for has no strings.
const DefaultLocale = "en"

// Message is one rendered message.
type Message struct {
	Subject string
	HTML    string
	Text    string
}

// Render renders the message `name` in `locale` with the values in vars. An
// empty value counts as absent, so a section on it is skipped. A "link" must
// be an absolute http or https address. A message that fails to render is an
// error, never a partial body: a partial body would still be sent. `locale`
// is a language tag or an Accept-Language value; see Negotiate.
func Render(name, locale string, vars map[string]any) (Message, error) {
	// A region tag (ko-KR) or an unknown language still gets a message: the
	// locale is whatever Negotiate makes of it.
	locale = Negotiate(locale)
	data := map[string]any{}
	for k, v := range vars {
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		data[k] = v
	}
	if link, ok := data["link"].(string); ok {
		if u, err := url.Parse(link); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return Message{}, fmt.Errorf("mail: %s: the link is not an http(s) address", name)
		}
	}

	strs, err := stringsFor(name, locale)
	if err != nil {
		return Message{}, err
	}
	style, err := styleOf(name)
	if err != nil {
		return Message{}, err
	}
	// Strings are templates too (a subject naming the organization, a
	// headline for a new user): rendered as text first, then escaped where the
	// HTML body puts them.
	text := &renderer{escape: func(s string) string { return s }}
	t := map[string]any{}
	for key, src := range strs {
		parsed, err := parseTemplate(src)
		if err != nil {
			return Message{}, fmt.Errorf("mail: %s: string %q: %w", name, key, err)
		}
		if t[key], err = text.render(parsed, []any{data}, 0); err != nil {
			return Message{}, fmt.Errorf("mail: %s: string %q: %w", name, key, err)
		}
	}
	data["t"] = t
	data["style"] = style

	partials, err := partialsFor(name)
	if err != nil {
		return Message{}, err
	}
	htmlBody, err := renderFile("messages/layout.html", &renderer{escape: html.EscapeString, partials: partials}, data)
	if err != nil {
		return Message{}, fmt.Errorf("mail: %s: HTML body: %w", name, err)
	}
	textBody, err := renderFile("messages/"+name+".txt", text, data)
	if err != nil {
		return Message{}, fmt.Errorf("mail: %s: text body: %w", name, err)
	}
	subject, _ := t["subject"].(string)
	if subject == "" {
		return Message{}, fmt.Errorf("mail: %s: no subject", name)
	}
	return Message{Subject: subject, HTML: htmlBody, Text: textBody}, nil
}

func renderFile(file string, r *renderer, data map[string]any) (string, error) {
	src, err := files.ReadFile(file)
	if err != nil {
		return "", err
	}
	parsed, err := parseTemplate(string(src))
	if err != nil {
		return "", err
	}
	return r.render(parsed, []any{data}, 0)
}

// partialsFor names the message's own HTML body "content", beside the shared
// partials in messages/partials/.
func partialsFor(name string) (map[string]string, error) {
	content, err := files.ReadFile("messages/" + name + ".html")
	if err != nil {
		return nil, fmt.Errorf("mail: no message %q", name)
	}
	partials := map[string]string{"content": string(content)}
	entries, err := files.ReadDir("messages/partials")
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		src, err := files.ReadFile("messages/partials/" + e.Name())
		if err != nil {
			return nil, err
		}
		partials[strings.TrimSuffix(e.Name(), path.Ext(e.Name()))] = string(src)
	}
	return partials, nil
}

func stringsFor(name, locale string) (map[string]string, error) {
	all, err := readLocale(locale)
	if err != nil {
		all, err = readLocale(DefaultLocale)
		if err != nil {
			return nil, err
		}
	}
	strs, ok := all[name]
	if !ok {
		return nil, fmt.Errorf("mail: no strings for %q", name)
	}
	merged := maps.Clone(all["common"])
	if merged == nil {
		merged = map[string]string{}
	}
	maps.Copy(merged, strs)
	return merged, nil
}

func readLocale(locale string) (map[string]map[string]string, error) {
	src, err := files.ReadFile("locales/" + locale + ".json")
	if err != nil {
		return nil, fmt.Errorf("mail: no locale %q", locale)
	}
	var all map[string]map[string]string
	if err := json.Unmarshal(src, &all); err != nil {
		return nil, fmt.Errorf("mail: locale %q: %w", locale, err)
	}
	return all, nil
}

func styleOf(name string) (map[string]any, error) {
	src, err := files.ReadFile("messages/styles.json")
	if err != nil {
		return nil, err
	}
	var all map[string]map[string]any
	if err := json.Unmarshal(src, &all); err != nil {
		return nil, fmt.Errorf("mail: styles.json: %w", err)
	}
	style, ok := all[name]
	if !ok {
		return nil, fmt.Errorf("mail: no style for %q", name)
	}
	return style, nil
}
