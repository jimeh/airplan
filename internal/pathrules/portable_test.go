package pathrules

import "testing"

func TestPortableSegmentRejectsWindowsReservedCharacters(t *testing.T) {
	for _, reserved := range []string{"<", ">", `:`, `"`, "|", "?", "*"} {
		segment := "file" + reserved + "name.txt"
		if PortableSegment(segment) {
			t.Errorf("PortableSegment(%q) = true, want false", segment)
		}
	}
}

func TestPortableSegmentRejectsWindowsDeviceAliases(t *testing.T) {
	for _, segment := range []string{
		"CON", "prn.txt", "Aux.log", "nul",
		"COM1", "com9.data", "LPT1", "lpt9.txt",
		"COM¹", "com².txt", "Com³.log",
		"LPT¹", "lpt².txt", "Lpt³.log",
	} {
		if PortableSegment(segment) {
			t.Errorf("PortableSegment(%q) = true, want false", segment)
		}
	}
}

func TestPortableSegmentAcceptsNonReservedNames(t *testing.T) {
	for _, segment := range []string{
		"document.md", "COM10.txt", "LPT0", "xCOM1", "report¹.txt",
	} {
		if !PortableSegment(segment) {
			t.Errorf("PortableSegment(%q) = false, want true", segment)
		}
	}
}
