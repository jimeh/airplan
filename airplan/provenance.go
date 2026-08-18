package airplan

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"runtime/debug"
	"strconv"
	"strings"
)

var producerSemverPattern = regexp.MustCompile(
	`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?$`,
)

type producerSemver struct {
	core       [3]uint64
	prerelease []string
}

func producerVersion(configured string) string {
	if configured != "" {
		return strings.TrimPrefix(configured, "v")
	}
	info, ok := debug.ReadBuildInfo()
	if !ok || info == nil || info.Main.Version == "" ||
		info.Main.Version == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

// compareProducerVersions compares current with target when both are strict
// semantic versions. An optional leading v is ignored consistently. Unknown
// build strings such as "dev" deliberately have no ordering.
func compareProducerVersions(current, target string) (int, bool) {
	left, ok := parseProducerSemver(current)
	if !ok {
		return 0, false
	}
	right, ok := parseProducerSemver(target)
	if !ok {
		return 0, false
	}
	for index := range left.core {
		if left.core[index] < right.core[index] {
			return -1, true
		}
		if left.core[index] > right.core[index] {
			return 1, true
		}
	}
	return comparePrerelease(left.prerelease, right.prerelease), true
}

func parseProducerSemver(value string) (producerSemver, bool) {
	match := producerSemverPattern.FindStringSubmatch(strings.TrimPrefix(value, "v"))
	if match == nil {
		return producerSemver{}, false
	}
	var parsed producerSemver
	for index := range parsed.core {
		number, err := strconv.ParseUint(match[index+1], 10, 64)
		if err != nil {
			return producerSemver{}, false
		}
		parsed.core[index] = number
	}
	if match[4] != "" {
		parsed.prerelease = strings.Split(match[4], ".")
		for _, identifier := range parsed.prerelease {
			if len(identifier) > 1 && identifier[0] == '0' &&
				numericPrerelease(identifier) {
				return producerSemver{}, false
			}
		}
	}
	return parsed, true
}

func comparePrerelease(left, right []string) int {
	if len(left) == 0 && len(right) == 0 {
		return 0
	}
	if len(left) == 0 {
		return 1
	}
	if len(right) == 0 {
		return -1
	}
	for index := 0; index < len(left) && index < len(right); index++ {
		if left[index] == right[index] {
			continue
		}
		leftNumeric := numericPrerelease(left[index])
		rightNumeric := numericPrerelease(right[index])
		switch {
		case leftNumeric && rightNumeric:
			if len(left[index]) < len(right[index]) ||
				(len(left[index]) == len(right[index]) && left[index] < right[index]) {
				return -1
			}
			return 1
		case leftNumeric:
			return -1
		case rightNumeric:
			return 1
		case left[index] < right[index]:
			return -1
		default:
			return 1
		}
	}
	if len(left) < len(right) {
		return -1
	}
	if len(left) > len(right) {
		return 1
	}
	return 0
}

func numericPrerelease(value string) bool {
	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}
	return true
}

func contentSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func documentRenderRecipe(cfg *Config, customDigest string) *RenderRecipe {
	template := RenderTemplate{Kind: "builtin"}
	if customDigest != "" {
		template = RenderTemplate{Kind: "custom", SHA256: customDigest}
	}
	return &RenderRecipe{
		Generation: RendererGeneration, Template: template,
		Indexable: cfg.Indexable, NoExternalAssets: cfg.NoExternalAssets,
		MermaidURL: cfg.MermaidURL,
		Themes:     themeRecipePtr(cfg.ThemeBundle),
	}
}

func collectionRenderRecipe(cfg *Config, customDigest string) *RenderRecipe {
	template := RenderTemplate{Kind: "builtin_collection"}
	if customDigest != "" {
		template = RenderTemplate{Kind: "custom_collection", SHA256: customDigest}
	}
	return &RenderRecipe{
		Generation: RendererGeneration, Template: template,
		Indexable: cfg.Indexable,
		Themes:    themeRecipePtr(cfg.ThemeBundle),
	}
}
