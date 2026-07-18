package skills

import (
	"os"
	"testing"
)

func TestAirplanMatchesCanonicalSkill(t *testing.T) {
	want, err := os.ReadFile("airplan/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	if got := Airplan(); got != string(want) {
		t.Fatalf("Airplan() length = %d, want %d", len(got), len(want))
	}
}
