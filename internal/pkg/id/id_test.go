package id

import "testing"

func TestNewUUIDIsLowercaseAndValid(t *testing.T) {
	for i := 0; i < 10; i++ {
		u := NewUUID()
		if !IsUUID(u) {
			t.Fatalf("NewUUID produced non-canonical UUID: %q", u)
		}
	}
}

func TestIsUUID(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"00000000-0000-0000-0000-000000000001", true},
		{"64b72193-8b2a-484c-9728-6b703fa90ca9", true},
		{"64B72193-8B2A-484C-9728-6B703FA90CA9", false}, // uppercase rejected — callers must lowercase first
		{"not-a-uuid", false},
		{"", false},
		{"64b72193-8b2a-484c-9728-6b703fa90ca", false}, // too short
	}
	for _, tc := range cases {
		if got := IsUUID(tc.in); got != tc.want {
			t.Errorf("IsUUID(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestNewUUIDv5IsDeterministic(t *testing.T) {
	const namespace = "64b72193-8b2a-484c-9728-6b703fa90ca9"

	a, err := NewUUIDv5(namespace, "authentik|user-42")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewUUIDv5(namespace, "authentik|user-42")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("NewUUIDv5 not deterministic: %q != %q", a, b)
	}
	if !IsUUID(a) {
		t.Errorf("NewUUIDv5 produced non-canonical UUID: %q", a)
	}

	c, err := NewUUIDv5(namespace, "authentik|user-43")
	if err != nil {
		t.Fatal(err)
	}
	if a == c {
		t.Error("different names must not collide")
	}
}

func TestNewUUIDv5RejectsBadNamespace(t *testing.T) {
	if _, err := NewUUIDv5("not-a-uuid", "whatever"); err == nil {
		t.Fatal("expected an error for an invalid namespace UUID")
	}
}
