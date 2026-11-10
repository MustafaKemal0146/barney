package people

import (
	"os"
	"strconv"
	"testing"
)

func two(i int) string {
	s := strconv.Itoa(i)
	if len(s) < 2 {
		s = "0" + s
	}
	return s
}

func writeFile(t *testing.T, p string, b []byte) {
	t.Helper()
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
}
