package airplan

import (
	"os"
	"testing"
)

func TestAgentSkillMatchesCanonicalSource(t *testing.T) {
	want, err := os.ReadFile("../skills/airplan/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	if got := AgentSkill(); got != string(want) {
		t.Fatalf("AgentSkill() length = %d, want %d", len(got), len(want))
	}
}
