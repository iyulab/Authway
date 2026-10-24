package mail

import (
	"html"
	"testing"
)

func renderString(t *testing.T, src string, data map[string]any, partials map[string]string) string {
	t.Helper()
	parsed, err := parseTemplate(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	out, err := (&renderer{escape: html.EscapeString, partials: partials}).render(parsed, []any{data}, 0)
	if err != nil {
		t.Fatalf("render %q: %v", src, err)
	}
	return out
}

func TestMustache_TheSubsetRendersAsTheSpecificationSays(t *testing.T) {
	data := map[string]any{
		"name":  "<b>Acme</b>",
		"on":    true,
		"off":   false,
		"empty": "",
		"list":  []any{"a", "b"},
		"style": map[string]any{"color": "#fff"},
		"t":     map[string]any{"greeting": "Hello"},
	}
	cases := map[string]struct{ src, want string }{
		"escaped variable":           {"{{name}}", "&lt;b&gt;Acme&lt;/b&gt;"},
		"triple mustache is raw":     {"{{{name}}}", "<b>Acme</b>"},
		"ampersand is raw":           {"{{& name}}", "<b>Acme</b>"},
		"missing is empty":           {"[{{nope}}]", "[]"},
		"dotted name":                {"{{style.color}}", "#fff"},
		"dotted name, missing part":  {"[{{style.size}}]", "[]"},
		"section on true":            {"{{#on}}yes{{/on}}", "yes"},
		"section on false":           {"{{#off}}yes{{/off}}", ""},
		"section on empty string":    {"{{#empty}}yes{{/empty}}", ""},
		"inverted section":           {"{{^off}}no{{/off}}{{^on}}never{{/on}}", "no"},
		"list section iterates":      {"{{#list}}[{{.}}]{{/list}}", "[a][b]"},
		"outer names inside section": {"{{#name}}{{t.greeting}}{{/name}}", "Hello"},
		"comment":                    {"a{{! anything }}b", "ab"},
		"nested sections":            {"{{#on}}{{^off}}both{{/off}}{{/on}}", "both"},
	}
	for label, c := range cases {
		if got := renderString(t, c.src, data, nil); got != c.want {
			t.Errorf("%s: %q rendered %q, want %q", label, c.src, got, c.want)
		}
	}
}

func TestMustache_PartialsRenderInTheCurrentContext(t *testing.T) {
	got := renderString(t, "{{#user}}{{> card}}{{/user}}", map[string]any{"user": map[string]any{"name": "Kim"}},
		map[string]string{"card": "[{{name}}]"})
	if got != "[Kim]" {
		t.Fatalf("got %q", got)
	}
}

func TestMustache_RefusesWhatItDoesNotRender(t *testing.T) {
	for _, src := range []string{"{{#a}}open", "{{/a}}", "{{#a}}{{/b}}", "{{a", "{{=<% %>=}}", "{{}}"} {
		if _, err := parseTemplate(src); err == nil {
			t.Errorf("%q parsed", src)
		}
	}
	parsed, _ := parseTemplate("{{> missing}}")
	if _, err := (&renderer{escape: html.EscapeString}).render(parsed, []any{map[string]any{}}, 0); err == nil {
		t.Error("an unknown partial rendered")
	}
	parsed, _ = parseTemplate("{{> self}}")
	if _, err := (&renderer{escape: html.EscapeString, partials: map[string]string{"self": "{{> self}}"}}).render(parsed, nil, 0); err == nil {
		t.Error("a partial that includes itself rendered")
	}
}
