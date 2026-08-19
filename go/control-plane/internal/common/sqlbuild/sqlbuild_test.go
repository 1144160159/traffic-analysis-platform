package sqlbuild

import "testing"

func TestPlaceholderStyles(t *testing.T) {
	if got := New(PlaceholderDollar).Placeholder(1); got != "$1" {
		t.Fatalf("dollar placeholder = %q, want $1", got)
	}
	if got := New(PlaceholderQuestion).Placeholder(3); got != "?" {
		t.Fatalf("question placeholder = %q, want ?", got)
	}
}

func TestWhereJoin(t *testing.T) {
	cases := []struct {
		name       string
		conditions []string
		want       string
	}{
		{name: "empty", conditions: nil, want: ""},
		{name: "single", conditions: []string{"tenant_id=$1"}, want: " WHERE tenant_id=$1"},
		{name: "multiple", conditions: []string{"tenant_id=$1", "status=$2"}, want: " WHERE tenant_id=$1 AND status=$2"},
	}
	for _, c := range cases {
		if got := Where(c.conditions); got != c.want {
			t.Fatalf("%s: Where = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestCountFromAndEq(t *testing.T) {
	b := New(PlaceholderDollar)
	if got := b.CountFrom("assets"); got != "SELECT COUNT(*) FROM assets" {
		t.Fatalf("CountFrom = %q", got)
	}
	if got := b.Eq("tenant_id", 1); got != "tenant_id = $1" {
		t.Fatalf("Eq = %q", got)
	}
	if got := b.ILike("vendor", 2); got != "vendor ILIKE $2" {
		t.Fatalf("ILike = %q", got)
	}
}
