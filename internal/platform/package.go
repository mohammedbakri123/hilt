package platform

// PackageManager describes the detected native package manager.
type PackageManager struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Privilege string `json:"privilege"`
}

type managerCandidate struct {
	name string
	bin  string
}

var managerCandidates = []managerCandidate{
	{"pacman", "pacman"},
	{"apt", "apt-get"},
	{"dnf", "dnf"},
	{"zypper", "zypper"},
	{"apk", "apk"},
}

var privilegeCandidates = []string{"sudo", "doas", "pkexec"}

// FindPackageManager returns the first supported package manager found. It
// never executes a package command.
func FindPackageManager(lookPath func(string) (string, error)) *PackageManager {
	if lookPath == nil {
		return nil
	}
	for _, candidate := range managerCandidates {
		path, err := lookPath(candidate.bin)
		if err != nil {
			continue
		}
		return &PackageManager{
			Name:      candidate.name,
			Path:      path,
			Privilege: findPrivilege(lookPath),
		}
	}
	return nil
}

func findPrivilege(lookPath func(string) (string, error)) string {
	for _, name := range privilegeCandidates {
		if _, err := lookPath(name); err == nil {
			return name
		}
	}
	return ""
}

// FindTool returns the resolved path of the first available named tool.
func FindTool(lookPath func(string) (string, error), names ...string) (string, bool) {
	if lookPath == nil {
		return "", false
	}
	for _, name := range names {
		if path, err := lookPath(name); err == nil {
			return path, true
		}
	}
	return "", false
}
