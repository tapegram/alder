package format

import "testing"

func TestSource_MinInt64Literal(t *testing.T) {
	src := "module T\n\nf : Int -> Int\nf n = match n {\n  -9223372036854775808 -> -9223372036854775808,\n  _ -> 9223372036854775807\n}\n"
	out, diags, err := Source("T.alder", []byte(src))
	if err != nil || len(diags) != 0 {
		t.Fatalf("format: %v %v", diags, err)
	}
	again, _, _ := Source("T.alder", out)
	if string(again) != string(out) {
		t.Errorf("not idempotent:\n%s\n%s", out, again)
	}
	if want := "-9223372036854775808 -> -9223372036854775808"; !contains(string(out), want) {
		t.Errorf("lost MinInt64:\n%s", out)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
