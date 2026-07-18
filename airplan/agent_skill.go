package airplan

import "github.com/jimeh/airplan/skills"

// AgentSkill returns the complete canonical airplan agent skill
// (SPEC.md §6).
func AgentSkill() string {
	return skills.Airplan()
}
