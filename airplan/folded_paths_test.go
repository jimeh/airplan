package airplan

import (
	"math/rand/v2"
	"strings"
	"testing"
)

// pairwiseDirectoryConflict is the original quadratic check that foldedPaths
// replaces: a conflict exists when either folded path is a directory ancestor
// of the other.
func pairwiseDirectoryConflict(recorded []string, fold string) bool {
	for _, previous := range recorded {
		if strings.HasPrefix(fold, previous+"/") ||
			strings.HasPrefix(previous, fold+"/") {
			return true
		}
	}
	return false
}

func TestFoldedPathsMatchesPairwiseCheck(t *testing.T) {
	t.Parallel()

	random := rand.New(rand.NewPCG(1, 2))
	segments := []string{"a", "b", "A", "ab", "a.b"}
	for round := range 2000 {
		paths := newFoldedPaths(0)
		var recorded []string
		for step := range 8 {
			parts := make([]string, 1+random.IntN(3))
			for index := range parts {
				parts[index] = segments[random.IntN(len(segments))]
			}
			original := strings.Join(parts, "/")
			fold := strings.ToLower(original)

			_, gotDuplicate := paths.get(fold)
			wantDuplicate := false
			for _, previous := range recorded {
				wantDuplicate = wantDuplicate || previous == fold
			}
			if gotDuplicate != wantDuplicate {
				t.Fatalf("round %d step %d: get(%q) = %t, want %t (recorded %q)",
					round, step, fold, gotDuplicate, wantDuplicate, recorded)
			}
			_, gotConflict := paths.directoryConflict(fold)
			if want := pairwiseDirectoryConflict(recorded, fold); gotConflict != want {
				t.Fatalf("round %d step %d: directoryConflict(%q) = %t, want %t (recorded %q)",
					round, step, fold, gotConflict, want, recorded)
			}
			paths.add(fold, original)
			recorded = append(recorded, fold)
		}
	}
}

func TestFoldedPathsNamesConflictingPath(t *testing.T) {
	t.Parallel()

	paths := newFoldedPaths(0)
	paths.add("images", "Images")
	if previous, ok := paths.directoryConflict("images/icon.svg"); !ok || previous != "Images" {
		t.Fatalf("ancestor conflict = %q, %t; want %q", previous, ok, "Images")
	}

	paths = newFoldedPaths(0)
	paths.add("docs/guide/intro.md", "docs/Guide/intro.md")
	if previous, ok := paths.directoryConflict("docs/guide"); !ok || previous != "docs/Guide/intro.md" {
		t.Fatalf("descendant conflict = %q, %t; want %q", previous, ok, "docs/Guide/intro.md")
	}
	if _, ok := paths.directoryConflict("docs/guide.md"); ok {
		t.Fatal("sibling with a shared name prefix reported as a conflict")
	}
}
