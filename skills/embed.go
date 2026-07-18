// Package skills embeds the canonical agent skills shipped with airplan.
package skills

import _ "embed"

// airplanSkill is the canonical airplan agent skill.
//
//go:embed airplan/SKILL.md
var airplanSkill string

// Airplan returns the complete airplan agent skill.
func Airplan() string {
	return airplanSkill
}
