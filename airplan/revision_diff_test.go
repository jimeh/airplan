package airplan

import "testing"

func TestRevisionDiffRangeAcceptsBundleDiffs(t *testing.T) {
	for _, test := range []struct {
		name string
		diff string
	}{
		{
			name: "textual page change",
			diff: "# airplan revisions: 3 -> 4\n\n## page: README.md\n--- revision-3/README.md\n+++ revision-4/README.md\n",
		},
		{
			name: "asset only change",
			diff: "# airplan revisions: 3 -> 4\n\nNo textual page changes.\n",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			previous, current, err := revisionDiffRange([]byte(test.diff))
			if err != nil {
				t.Fatal(err)
			}
			if previous != 3 || current != 4 {
				t.Fatalf("range = %d -> %d, want 3 -> 4", previous, current)
			}
		})
	}
}
