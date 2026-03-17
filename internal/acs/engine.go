package acs

import (
	"fmt"
	"strings"

	"wolfbbs/internal/rbac"
)

type Context struct {
	Role     string
	Verified bool
	Attrs    map[string]string
}

func Evaluate(expr string, ctx Context) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}
	tokens := tokenize(expr)
	if len(tokens) == 0 {
		return true, nil
	}
	p := parser{tokens: tokens}
	value, err := p.parseExpr(ctx)
	if err != nil {
		return false, err
	}
	if p.pos != len(p.tokens) {
		return false, fmt.Errorf("unexpected token %q", p.tokens[p.pos])
	}
	return value, nil
}

type parser struct {
	tokens []string
	pos    int
}

func (p *parser) parseExpr(ctx Context) (bool, error) {
	left, err := p.parseAnd(ctx)
	if err != nil {
		return false, err
	}
	for p.accept("or", "||") {
		right, err := p.parseAnd(ctx)
		if err != nil {
			return false, err
		}
		left = left || right
	}
	return left, nil
}

func (p *parser) parseAnd(ctx Context) (bool, error) {
	left, err := p.parseUnary(ctx)
	if err != nil {
		return false, err
	}
	for p.accept("and", "&&") {
		right, err := p.parseUnary(ctx)
		if err != nil {
			return false, err
		}
		left = left && right
	}
	return left, nil
}

func (p *parser) parseUnary(ctx Context) (bool, error) {
	if p.accept("not", "!") {
		v, err := p.parseUnary(ctx)
		if err != nil {
			return false, err
		}
		return !v, nil
	}
	return p.parsePrimary(ctx)
}

func (p *parser) parsePrimary(ctx Context) (bool, error) {
	if p.accept("(") {
		v, err := p.parseExpr(ctx)
		if err != nil {
			return false, err
		}
		if !p.accept(")") {
			return false, fmt.Errorf("missing closing )")
		}
		return v, nil
	}
	if p.pos >= len(p.tokens) {
		return false, fmt.Errorf("unexpected end of expression")
	}
	token := p.tokens[p.pos]
	p.pos++
	return evalPredicate(token, ctx), nil
}

func (p *parser) accept(values ...string) bool {
	if p.pos >= len(p.tokens) {
		return false
	}
	current := strings.ToLower(strings.TrimSpace(p.tokens[p.pos]))
	for _, value := range values {
		if current == strings.ToLower(strings.TrimSpace(value)) {
			p.pos++
			return true
		}
	}
	return false
}

func evalPredicate(token string, ctx Context) bool {
	token = strings.TrimSpace(token)
	lowerToken := strings.ToLower(token)
	switch lowerToken {
	case "true", "any", "allow", "*":
		return true
	case "false", "deny":
		return false
	case "verified":
		return ctx.Verified
	case "unverified":
		return !ctx.Verified
	}

	if strings.HasPrefix(lowerToken, "role=") {
		return rbac.NormalizeRole(strings.TrimPrefix(lowerToken, "role=")) == rbac.NormalizeRole(ctx.Role)
	}
	if strings.HasPrefix(lowerToken, "role:") {
		return rbac.NormalizeRole(strings.TrimPrefix(lowerToken, "role:")) == rbac.NormalizeRole(ctx.Role)
	}

	key, value, hasKV := splitPredicate(lowerToken)
	if hasKV {
		switch key {
		case "role":
			return rbac.NormalizeRole(value) == rbac.NormalizeRole(ctx.Role)
		case "verified":
			want := parseBool(value)
			return ctx.Verified == want
		default:
			return strings.EqualFold(strings.TrimSpace(ctx.Attrs[key]), value)
		}
	}

	if ctx.Attrs == nil {
		return false
	}
	raw := strings.ToLower(strings.TrimSpace(ctx.Attrs[lowerToken]))
	if raw == "" {
		return false
	}
	return parseBool(raw)
}

func splitPredicate(value string) (key string, rhs string, ok bool) {
	if idx := strings.Index(value, "="); idx > 0 {
		return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+1:]), true
	}
	if idx := strings.Index(value, ":"); idx > 0 {
		return strings.TrimSpace(value[:idx]), strings.TrimSpace(value[idx+1:]), true
	}
	return "", "", false
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "allow":
		return true
	default:
		return false
	}
}

func tokenize(expr string) []string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil
	}
	tokens := make([]string, 0, 16)
	var current strings.Builder
	flush := func() {
		if current.Len() == 0 {
			return
		}
		tokens = append(tokens, current.String())
		current.Reset()
	}
	for _, r := range expr {
		switch r {
		case ' ', '\t', '\n', '\r':
			flush()
		case '(', ')':
			flush()
			tokens = append(tokens, string(r))
		default:
			current.WriteRune(r)
		}
	}
	flush()
	return tokens
}
