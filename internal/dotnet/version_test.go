package dotnet

import "testing"

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    Version
		wantErr bool
	}{
		{input: "10.0.112", want: Version{Major: 10, Minor: 0, Patch: 112}},
		{input: "8.0.100-preview.1.24101.2", want: Version{Major: 8, Minor: 0, Patch: 100, PreRelease: "preview.1.24101.2"}},
		{input: "6.0.36", want: Version{Major: 6, Minor: 0, Patch: 36}},
		{input: "1.2.3.4", want: Version{Major: 1, Minor: 2, Patch: 3, Revision: 4}},
		{input: "", wantErr: true},
		{input: "preview", wantErr: true},
		{input: "1..2", wantErr: true},
		{input: "1.2.3-", wantErr: true},
		{input: "1.2.3.4.5", wantErr: true},
		{input: "-1.0.0", wantErr: true},
	}
	for _, test := range tests {
		got, err := ParseVersion(test.input)
		if test.wantErr {
			if err == nil {
				t.Errorf("ParseVersion(%q) expected error, got %+v", test.input, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseVersion(%q): %v", test.input, err)
			continue
		}
		got.Original = ""
		if got != test.want {
			t.Errorf("ParseVersion(%q) = %+v, want %+v", test.input, got, test.want)
		}
	}
}

func TestVersionCompare(t *testing.T) {
	tests := []struct {
		left, right string
		want        int
	}{
		{"10.0.112", "10.0.111", 1},
		{"10.0.112", "10.0.112", 0},
		{"9.0.300", "10.0.100", -1},
		{"10.0.100", "10.0.100-preview.1", 1},
		{"10.0.100-preview.2", "10.0.100-preview.1", 1},
		{"10.0.100-alpha", "10.0.100-alpha.1", -1},
		{"10.0.100-preview.10", "10.0.100-preview.2", 1},
	}
	for _, test := range tests {
		left, err := ParseVersion(test.left)
		if err != nil {
			t.Fatal(err)
		}
		right, err := ParseVersion(test.right)
		if err != nil {
			t.Fatal(err)
		}
		if got := left.Compare(right); got != test.want {
			t.Errorf("%s.Compare(%s) = %d, want %d", test.left, test.right, got, test.want)
		}
	}
}

func TestVersionString(t *testing.T) {
	version, err := ParseVersion("10.0.112")
	if err != nil {
		t.Fatal(err)
	}
	if got := version.String(); got != "10.0.112" {
		t.Errorf("String = %q", got)
	}
	version, err = ParseVersion("8.0.100-preview.1")
	if err != nil {
		t.Fatal(err)
	}
	if got := version.String(); got != "8.0.100-preview.1" {
		t.Errorf("String = %q", got)
	}
	if !version.IsPrerelease() {
		t.Error("expected prerelease")
	}
}
