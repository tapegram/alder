package std

import (
	"hash/fnv"
	"strconv"
	"testing"
)

// Text.hash is FNV-1a 64 over the UTF-8 bytes with the top bit cleared; these values are
// pinned forever (a seeded run must reproduce across versions).
func TestTextHash_Vectors(t *testing.T) {
	for s, want := range map[string]int64{
		"":      5472609002491880229,
		"a":     3414815163700866188,
		"héllo": 2549027629147767008,
	} {
		if got := TextHash(s); got != want {
			t.Errorf("Text.hash %q = %d, want %d", s, got, want)
		}
	}
}

func TestTextHash_NonNegative(t *testing.T) {
	for i := range 1000 {
		for _, s := range []string{strconv.Itoa(i), "k/" + strconv.Itoa(i), strconv.Itoa(i * 7919)} {
			if h := TextHash(s); h < 0 {
				t.Fatalf("Text.hash %q = %d < 0", s, h)
			}
		}
	}
}

func TestTextHash_UTF8(t *testing.T) {
	if TextHash("héllo") == TextHash("hello") {
		t.Fatal(`"héllo" and "hello" hash alike`)
	}
	f := fnv.New64a()
	f.Write([]byte("héllo"))
	if want := int64(f.Sum64() &^ (1 << 63)); TextHash("héllo") != want {
		t.Fatalf("Text.hash héllo = %d, want FNV-1a of its UTF-8 bytes %d", TextHash("héllo"), want)
	}
}
