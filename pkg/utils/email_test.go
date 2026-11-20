package utils

import "testing"

func TestNormalizeEmail(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{name: "already lowercase", in: "alice@example.com", want: "alice@example.com"},
		{name: "mixed case", in: "Alice.Smith@Example.COM", want: "alice.smith@example.com"},
		{name: "leading and trailing spaces", in: "  User@Domain.Org \t\n", want: "user@domain.org"},
		{name: "empty string", in: "", want: ""},
		{name: "whitespace only", in: "   ", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NormalizeEmail(tc.in); got != tc.want {
				t.Fatalf("NormalizeEmail(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestNormalizeEmailPtr(t *testing.T) {
	if got := NormalizeEmailPtr(nil); got != nil {
		t.Fatalf("NormalizeEmailPtr(nil) = %v, want nil", *got)
	}

	raw := "  Bob@Example.COM "
	got := NormalizeEmailPtr(&raw)
	if got == nil {
		t.Fatal("NormalizeEmailPtr returned nil for non-nil input")
	}
	if *got != "bob@example.com" {
		t.Fatalf("*NormalizeEmailPtr(%q) = %q, want %q", raw, *got, "bob@example.com")
	}
	// 不应原地修改调用方的原始字符串。
	if raw != "  Bob@Example.COM " {
		t.Fatalf("input string mutated to %q", raw)
	}
}
