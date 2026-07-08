package airplan

import "testing"

func TestRandomDirNameFormat(t *testing.T) {
	t.Parallel()

	for range 1000 {
		name, err := RandomDirName()
		if err != nil {
			t.Fatalf("RandomDirName returned error: %v", err)
		}

		if len(name) != 26 {
			t.Fatalf("RandomDirName length = %d, want 26 for %q",
				len(name), name)
		}

		for _, r := range name {
			if !isBase32LowerNoPadding(r) {
				t.Fatalf("RandomDirName has invalid character %q in %q",
					r, name)
			}
		}
	}
}

func TestRandomDirNameUniqueAcrossSample(t *testing.T) {
	t.Parallel()

	const samples = 10000
	seen := make(map[string]struct{}, samples)
	for range samples {
		name, err := RandomDirName()
		if err != nil {
			t.Fatalf("RandomDirName returned error: %v", err)
		}

		if _, ok := seen[name]; ok {
			t.Fatalf("RandomDirName collision after %d samples: %q",
				len(seen)+1, name)
		}
		seen[name] = struct{}{}
	}
}

func TestSanitizeSlug(t *testing.T) {
	t.Parallel()

	long := "abcdefghijklmnopqrstuvwxyz0123456789" +
		"abcdefghijklmnopqrstuvwxyz01-ignored"

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "mixed case",
			in:   "RefactorAuth",
			want: "refactorauth",
		},
		{
			name: "spaces",
			in:   "Refactor auth plan",
			want: "refactor-auth-plan",
		},
		{
			name: "unicode",
			in:   "Résumé plan",
			want: "r-sum-plan",
		},
		{
			name: "consecutive separators",
			in:   "refactor___auth---plan",
			want: "refactor-auth-plan",
		},
		{
			name: "leading trailing junk",
			in:   "  ./Refactor Auth!!!  ",
			want: "refactor-auth",
		},
		{
			name: "over 64 chars and trailing hyphen",
			in:   long,
			want: "abcdefghijklmnopqrstuvwxyz0123456789abcdefghijklmnopqrstuvwxyz01",
		},
		{
			name: "empty",
			in:   "",
			want: "plan",
		},
		{
			name: "all non ascii",
			in:   "计划",
			want: "plan",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := SanitizeSlug(tt.in); got != tt.want {
				t.Fatalf("SanitizeSlug(%q) = %q, want %q",
					tt.in, got, tt.want)
			}
		})
	}
}

func TestBuildKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		prefix   string
		dir      string
		filename string
		want     string
	}{
		{
			name:     "without prefix",
			dir:      "vq3nhk2p7r4wzt5c6ydjm3xhqd",
			filename: "refactor-auth.html",
			want:     "vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.html",
		},
		{
			name:     "with prefix",
			prefix:   "plans",
			dir:      "vq3nhk2p7r4wzt5c6ydjm3xhqd",
			filename: "refactor-auth.html",
			want:     "plans/vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.html",
		},
		{
			name:     "prefix with stray slashes",
			prefix:   "/team/plans/",
			dir:      "vq3nhk2p7r4wzt5c6ydjm3xhqd",
			filename: "refactor-auth.html",
			want:     "team/plans/vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.html",
		},
		{
			name:     "prefix only slashes",
			prefix:   "///",
			dir:      "vq3nhk2p7r4wzt5c6ydjm3xhqd",
			filename: "refactor-auth.html",
			want:     "vq3nhk2p7r4wzt5c6ydjm3xhqd/refactor-auth.html",
		},
		{
			name:     "joins only non empty parts",
			prefix:   "/plans/",
			filename: "refactor-auth.html",
			want:     "plans/refactor-auth.html",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := BuildKey(tt.prefix, tt.dir, tt.filename)
			if got != tt.want {
				t.Fatalf("BuildKey(%q, %q, %q) = %q, want %q",
					tt.prefix, tt.dir, tt.filename, got, tt.want)
			}
			if len(got) > 0 && got[0] == '/' {
				t.Fatalf("BuildKey(%q, %q, %q) starts with slash: %q",
					tt.prefix, tt.dir, tt.filename, got)
			}
		})
	}
}

func isBase32LowerNoPadding(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= '2' && r <= '7'
}
