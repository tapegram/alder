package buildkit

import (
	"testing"

	"alder/internal/testgocache"
)

// The go builds these tests run go into a cache of their own, not the user's (internal/testgocache).
func TestMain(m *testing.M) { testgocache.Main(m) }
