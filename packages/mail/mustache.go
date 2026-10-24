package mail

import (
	"fmt"
	"strings"
)

// The part of Mustache (https://mustache.github.io/mustache.5.html) the
// message files use: variables ({{name}} escaped, {{{name}}} and {{& name}}
// as they are), dotted names, sections ({{#name}}…{{/name}}), inverted
// sections ({{^name}}…{{/name}}), partials ({{> name}}), comments ({{! …}})
// and the implicit iterator ({{.}}). Not here: lambdas, set delimiters, and
// the removal of lines holding only a section tag — the files keep section
// tags inline, so an engine that removes such lines renders the same text.

type nodeKind int

const (
	textNode nodeKind = iota
	escapedNode
	rawNode
	sectionNode
	invertedNode
	partialNode
)

type node struct {
	kind     nodeKind
	text     string // the text, or the name a tag refers to
	children []node
}

const maxPartialDepth = 16

func parseTemplate(src string) ([]node, error) {
	nodes, rest, err := parseUntil(src, "")
	if err != nil {
		return nil, err
	}
	if rest != "" {
		return nil, fmt.Errorf("mustache: text after the template")
	}
	return nodes, nil
}

// parseUntil parses src up to the tag closing section `closing` ("" for the
// end of the template) and returns what follows that tag.
func parseUntil(src, closing string) ([]node, string, error) {
	var nodes []node
	for {
		open := strings.Index(src, "{{")
		if open < 0 {
			if closing != "" {
				return nil, "", fmt.Errorf("mustache: section %q is not closed", closing)
			}
			if src != "" {
				nodes = append(nodes, node{kind: textNode, text: src})
			}
			return nodes, "", nil
		}
		if open > 0 {
			nodes = append(nodes, node{kind: textNode, text: src[:open]})
		}
		src = src[open+2:]

		end := "}}"
		triple := strings.HasPrefix(src, "{")
		if triple {
			end = "}}}"
			src = src[1:]
		}
		closeAt := strings.Index(src, end)
		if closeAt < 0 {
			return nil, "", fmt.Errorf("mustache: a tag is not closed")
		}
		tag := strings.TrimSpace(src[:closeAt])
		src = src[closeAt+len(end):]

		if triple {
			nodes = append(nodes, node{kind: rawNode, text: tag})
			continue
		}
		if tag == "" {
			return nil, "", fmt.Errorf("mustache: an empty tag")
		}
		sigil, name := tag[0], strings.TrimSpace(tag[1:])
		switch sigil {
		case '!':
		case '&':
			nodes = append(nodes, node{kind: rawNode, text: name})
		case '>':
			nodes = append(nodes, node{kind: partialNode, text: name})
		case '#', '^':
			children, rest, err := parseUntil(src, name)
			if err != nil {
				return nil, "", err
			}
			kind := sectionNode
			if sigil == '^' {
				kind = invertedNode
			}
			nodes = append(nodes, node{kind: kind, text: name, children: children})
			src = rest
		case '/':
			if name != closing {
				return nil, "", fmt.Errorf("mustache: {{/%s}} closes no open section", name)
			}
			return nodes, src, nil
		case '=':
			return nil, "", fmt.Errorf("mustache: set delimiters are not supported")
		default:
			nodes = append(nodes, node{kind: escapedNode, text: tag})
		}
	}
}

// renderer renders parsed templates. escape is applied to {{name}}; partials
// are looked up by name and parsed when first used.
type renderer struct {
	escape   func(string) string
	partials map[string]string
	parsed   map[string][]node
}

func (r *renderer) render(nodes []node, stack []any, depth int) (string, error) {
	var b strings.Builder
	for _, n := range nodes {
		switch n.kind {
		case textNode:
			b.WriteString(n.text)
		case escapedNode:
			b.WriteString(r.escape(stringOf(lookup(stack, n.text))))
		case rawNode:
			b.WriteString(stringOf(lookup(stack, n.text)))
		case sectionNode:
			value := lookup(stack, n.text)
			if !truthy(value) {
				continue
			}
			items := []any{value}
			if list, ok := value.([]any); ok {
				items = list
			}
			for _, item := range items {
				s, err := r.render(n.children, append(stack, item), depth)
				if err != nil {
					return "", err
				}
				b.WriteString(s)
			}
		case invertedNode:
			if truthy(lookup(stack, n.text)) {
				continue
			}
			s, err := r.render(n.children, stack, depth)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		case partialNode:
			if depth >= maxPartialDepth {
				return "", fmt.Errorf("mustache: partials nested deeper than %d", maxPartialDepth)
			}
			parsed, err := r.partial(n.text)
			if err != nil {
				return "", err
			}
			s, err := r.render(parsed, stack, depth+1)
			if err != nil {
				return "", err
			}
			b.WriteString(s)
		}
	}
	return b.String(), nil
}

func (r *renderer) partial(name string) ([]node, error) {
	if parsed, ok := r.parsed[name]; ok {
		return parsed, nil
	}
	src, ok := r.partials[name]
	if !ok {
		return nil, fmt.Errorf("mustache: no partial %q", name)
	}
	parsed, err := parseTemplate(src)
	if err != nil {
		return nil, fmt.Errorf("partial %q: %w", name, err)
	}
	if r.parsed == nil {
		r.parsed = map[string][]node{}
	}
	r.parsed[name] = parsed
	return parsed, nil
}

// lookup resolves a name against the context stack: "." is the innermost
// context; the first part of a dotted name is looked up from the innermost
// context outwards, the rest within what it found.
func lookup(stack []any, name string) any {
	if name == "." {
		if len(stack) == 0 {
			return nil
		}
		return stack[len(stack)-1]
	}
	parts := strings.Split(name, ".")
	var value any
	found := false
	for i := len(stack) - 1; i >= 0 && !found; i-- {
		if m, ok := stack[i].(map[string]any); ok {
			value, found = m[parts[0]]
		}
	}
	if !found {
		return nil
	}
	for _, part := range parts[1:] {
		m, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		value = m[part]
	}
	return value
}

func truthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case []any:
		return len(v) > 0
	default:
		return true
	}
}

func stringOf(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	default:
		return fmt.Sprint(v)
	}
}
