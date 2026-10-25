package install

import "testing"

func TestValidateVersion(t *testing.T) {
	valid := map[string]string{
		"10.0.100":   "10.0.100",
		"10.0.100.0": "10.0.100",
		" 9.0.300 ":  "9.0.300",
		"10.0.100.1": "10.0.100.1",
	}
	for input, want := range valid {
		version, err := ValidateVersion(input)
		if err != nil {
			t.Errorf("ValidateVersion(%q) error = %v", input, err)
			continue
		}
		if version.String() != want {
			t.Errorf("ValidateVersion(%q) = %q, want %q", input, version.String(), want)
		}
	}

	for _, input := range []string{
		"",
		"10",
		"10.0",
		"10.0.100-rc.1",
		"10.0.100/../../escape",
		"latest",
		"10.0.100.1.2",
	} {
		if _, err := ValidateVersion(input); err == nil {
			t.Errorf("ValidateVersion(%q) succeeded", input)
		}
	}
}

func TestRIDFor(t *testing.T) {
	for arch, want := range map[string]string{"amd64": "linux-x64", "arm64": "linux-arm64"} {
		rid, err := RIDFor("linux", arch)
		if err != nil || rid != want {
			t.Errorf("RIDFor(linux, %s) = %q, %v", arch, rid, err)
		}
	}
	if _, err := RIDFor("darwin", "arm64"); err == nil {
		t.Error("darwin should be unsupported")
	}
	if _, err := RIDFor("linux", "386"); err == nil {
		t.Error("linux/386 should be unsupported")
	}
}
