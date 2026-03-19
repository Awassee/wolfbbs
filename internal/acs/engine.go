package acs

import (
	"fmt"
	"strings"

	"wolfbbs/internal/rbac"
)

type Context struct {
	Role       string
	Verified   bool
	Transport  string
	Secure     bool
	AuthFactor int
	Groups     []string
	Attrs      map[string]string
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
	case "secure":
		return ctx.Secure
	case "insecure":
		return !ctx.Secure
	}

	// Comparator expressions:
	// role=sysop, auth_factor>=2, transport=ssh|wss, group~wfc
	if key, op, rhs, ok := splitComparator(lowerToken); ok {
		return evalComparator(key, op, rhs, ctx)
	}

	if value, ok := contextValue(lowerToken, ctx); ok {
		return parseBool(value)
	}
	if ctx.Attrs == nil {
		return false
	}
	raw := strings.TrimSpace(ctx.Attrs[lowerToken])
	if raw == "" {
		return false
	}
	return parseBool(raw)
}

func splitComparator(value string) (key string, op string, rhs string, ok bool) {
	for _, candidate := range []string{">=", "<=", "!=", "==", ">", "<", "~", "=", ":"} {
		if idx := strings.Index(value, candidate); idx > 0 {
			return strings.TrimSpace(value[:idx]), candidate, strings.TrimSpace(value[idx+len(candidate):]), true
		}
	}
	return "", "", "", false
}

func evalComparator(key, op, rhs string, ctx Context) bool {
	key = strings.ToLower(strings.TrimSpace(key))
	rhs = strings.ToLower(strings.TrimSpace(rhs))
	if key == "" || rhs == "" {
		return false
	}
	lhs, ok := contextValue(key, ctx)
	if !ok {
		return false
	}
	lhs = strings.ToLower(strings.TrimSpace(lhs))
	switch op {
	case "=", "==", ":":
		return matchesValue(key, lhs, rhs)
	case "!=":
		return !matchesValue(key, lhs, rhs)
	case "~":
		return strings.Contains(lhs, rhs)
	case ">", "<", ">=", "<=":
		return compareNumeric(lhs, rhs, op)
	default:
		return false
	}
}

func matchesValue(key, lhs, rhs string) bool {
	if rhs == "*" || rhs == "any" {
		return true
	}
	if key == "role" {
		return rbac.NormalizeRole(lhs) == rbac.NormalizeRole(rhs)
	}
	if key == "group" || key == "groups" {
		return listContains(lhs, rhs)
	}
	if strings.Contains(rhs, "|") {
		for _, row := range strings.Split(rhs, "|") {
			if strings.EqualFold(strings.TrimSpace(lhs), strings.TrimSpace(row)) {
				return true
			}
		}
		return false
	}
	if strings.Contains(rhs, ",") {
		for _, row := range strings.Split(rhs, ",") {
			if strings.EqualFold(strings.TrimSpace(lhs), strings.TrimSpace(row)) {
				return true
			}
		}
		return false
	}
	return strings.EqualFold(lhs, rhs)
}

func listContains(csv, value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for _, row := range strings.FieldsFunc(csv, func(r rune) bool {
		return r == ',' || r == ';' || r == '|' || r == ' '
	}) {
		if strings.EqualFold(strings.TrimSpace(row), value) {
			return true
		}
	}
	return false
}

func compareNumeric(lhs, rhs, op string) bool {
	leftVal, leftOK := parseNumber(lhs)
	rightVal, rightOK := parseNumber(rhs)
	if !leftOK || !rightOK {
		return false
	}
	switch op {
	case ">":
		return leftVal > rightVal
	case "<":
		return leftVal < rightVal
	case ">=":
		return leftVal >= rightVal
	case "<=":
		return leftVal <= rightVal
	default:
		return false
	}
}

func parseNumber(value string) (float64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var out float64
	if _, err := fmt.Sscan(value, &out); err != nil {
		return 0, false
	}
	return out, true
}

func contextValue(key string, ctx Context) (string, bool) {
	key = strings.ToLower(strings.TrimSpace(key))
	switch key {
	case "role":
		return rbac.NormalizeRole(ctx.Role), true
	case "verified":
		return boolString(ctx.Verified), true
	case "secure":
		return boolString(ctx.Secure), true
	case "transport":
		return strings.ToLower(strings.TrimSpace(ctx.Transport)), true
	case "auth_factor", "authfactor", "mfa":
		if ctx.AuthFactor > 0 {
			return fmt.Sprintf("%d", ctx.AuthFactor), true
		}
		return "1", true
	case "group", "groups":
		if len(ctx.Groups) == 0 {
			return "", true
		}
		clean := make([]string, 0, len(ctx.Groups))
		for _, row := range ctx.Groups {
			row = strings.ToLower(strings.TrimSpace(row))
			if row == "" {
				continue
			}
			clean = append(clean, row)
		}
		return strings.Join(clean, ","), true
	default:
		if ctx.Attrs == nil {
			return "", false
		}
		value, ok := ctx.Attrs[key]
		return strings.TrimSpace(value), ok
	}
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
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
