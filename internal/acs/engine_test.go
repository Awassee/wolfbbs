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
