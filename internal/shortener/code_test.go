package shortener

import "testing"

func TestRandomCode_LengthAndCharset(t *testing.T) {
	for n := MinCodeLength; n <= MaxCodeLength; n++ {
		for range 200 {
			c, err := RandomCode(n)
			if err != nil {
				t.Fatalf("RandomCode(%d): %v", n, err)
			}
			if len(c) != n || !IsValidCode(c) {
				t.Fatalf("RandomCode(%d) = %q: wrong length or charset", n, c)
			}
		}
	}
}

func TestRandomCode_Distribution(t *testing.T) {
	seen := map[byte]bool{}
	for range 2000 {
		c, _ := RandomCode(8)
		for i := 0; i < len(c); i++ {
			seen[c[i]] = true
		}
	}
	if len(seen) != len(alphabet) {
		t.Errorf("saw %d distinct characters, want %d", len(seen), len(alphabet))
	}
}

func TestRandomCode_InvalidLength(t *testing.T) {
	for _, n := range []int{0, -1} {
		if _, err := RandomCode(n); err == nil {
			t.Errorf("RandomCode(%d): want error", n)
		}
	}
}

func TestIsValidCode(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"abc123", true},
		{"aB3dE5g", true},
		{"ABCDEFGH", true},
		{"abc12", false},
		{"abcdefghi", false},
		{"abc-12", false},
		{"abc_12", false},
		{"abc.12", false},
		{"favicon.ico", false},
		{"abcdé1", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := IsValidCode(tt.in); got != tt.want {
			t.Errorf("IsValidCode(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}
