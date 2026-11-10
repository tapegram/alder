package names

import "testing"

func TestCollection(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"Entry", "entries"},
		{"Reply", "replies"},
		{"BlogPost", "blog-posts"},
		{"Status", "statuses"},
		{"Box", "boxes"},
		{"Quiz", "quizes"},
		{"Match", "matches"},
		{"Wish", "wishes"},
		{"Day", "days"},
		{"Post", "posts"},
		{"HTTPLog", "http-logs"},
		{"Person", "persons"},
	} {
		if got := Collection(c.in); got != c.want {
			t.Errorf("Collection(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
