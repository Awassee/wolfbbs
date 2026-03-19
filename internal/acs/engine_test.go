package acs

import "testing"

func TestEvaluateRoleAndVerified(t *testing.T) {
	ctx := Context{
		Role:     "moderator",
		Verified: true,
		Attrs: map[string]string{
			"board": "general",
			"staff": "true",
		},
	}
	ok, err := Evaluate("role=moderator and verified", ctx)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if !ok {
		t.Fatal("expected expression to allow moderator verified user")
	}
	ok, err = Evaluate("role=admin or board=general", ctx)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if !ok {
		t.Fatal("expected attribute fallback to allow")
	}
	ok, err = Evaluate("not role=admin and staff", ctx)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if !ok {
		t.Fatal("expected expression with not/attr to allow")
	}
}

func TestEvaluateComparatorsAndContextFields(t *testing.T) {
	ctx := Context{
		Role:       "sysop",
		Verified:   true,
		Transport:  "wss",
		Secure:     true,
		AuthFactor: 2,
		Groups:     []string{"wfc", "staff"},
		Attrs: map[string]string{
			"term_width":  "132",
			"term_height": "43",
		},
	}
	for _, tc := range []struct {
		expr string
		want bool
	}{
		{expr: "secure and auth_factor>=2", want: true},
		{expr: "transport=wss|ssh", want: true},
		{expr: "group=wfc", want: true},
		{expr: "groups=users", want: false},
		{expr: "term_width>=120 and term_height>25", want: true},
		{expr: "term_width<80", want: false},
		{expr: "role!=user", want: true},
		{expr: "role=user", want: false},
	} {
		got, err := Evaluate(tc.expr, ctx)
		if err != nil {
			t.Fatalf("evaluate %q: %v", tc.expr, err)
		}
		if got != tc.want {
			t.Fatalf("evaluate %q = %v, want %v", tc.expr, got, tc.want)
		}
	}
}

func TestEvaluateParentheses(t *testing.T) {
	ctx := Context{Role: "user", Verified: false}
	ok, err := Evaluate("(role=admin or role=user) and not verified", ctx)
	if err != nil {
		t.Fatalf("evaluate error: %v", err)
	}
	if !ok {
		t.Fatal("expected grouped expression to be true")
	}
}

func TestEvaluateErrors(t *testing.T) {
	if _, err := Evaluate("(role=admin", Context{}); err == nil {
		t.Fatal("expected parse error for missing close paren")
	}
}
