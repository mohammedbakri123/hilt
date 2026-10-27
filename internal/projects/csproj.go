package projects

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type csprojDocument struct {
	XMLName        xml.Name              `xml:"Project"`
	Sdk            string                `xml:"Sdk,attr"`
	SDKs           []csprojSDK           `xml:"Sdk"`
	PropertyGroups []csprojPropertyGroup `xml:"PropertyGroup"`
	ItemGroups     []csprojItemGroup     `xml:"ItemGroup"`
}

type csprojSDK struct {
	Name    string `xml:"Name,attr"`
	Version string `xml:"Version,attr"`
}

type csprojPropertyGroup struct {
	TargetFramework  string `xml:"TargetFramework"`
	TargetFrameworks string `xml:"TargetFrameworks"`
	OutputType       string `xml:"OutputType"`
	IsTestProject    string `xml:"IsTestProject"`
}

type csprojItemGroup struct {
	ProjectReferences []csprojReference `xml:"ProjectReference"`
	PackageReferences []csprojReference `xml:"PackageReference"`
}

type csprojReference struct {
	Include        string `xml:"Include,attr"`
	Version        string `xml:"Version,attr"`
	VersionElement string `xml:"Version"`
}

var testPackages = map[string]bool{
	"microsoft.net.test.sdk":              true,
	"mstest.testadapter":                  true,
	"mstest.testframework":                true,
	"nunit":                               true,
	"nunit3testadapter":                   true,
	"xunit":                               true,
	"xunit.runner.visualstudio":           true,
	"xunit.v3":                            true,
	"coverlet.collector":                  true,
	"microsoft.visualstudio.testplatform": true,
}

// ParseProject reads a .csproj file and returns its declared metadata. It does
// not evaluate MSBuild properties or conditions.
func ParseProject(path string) (Project, error) {
	absolute, err := absolutePath(path)
	if err != nil {
		return Project{}, fmt.Errorf("parse project %s: %w", path, err)
	}
	data, err := os.ReadFile(absolute)
	if err != nil {
		return Project{}, fmt.Errorf("parse project %s: %w", absolute, err)
	}

	var document csprojDocument
	if err := xml.NewDecoder(bytes.NewReader(data)).Decode(&document); err != nil {
		return Project{}, fmt.Errorf("parse project %s: %w", absolute, err)
	}
	if !strings.EqualFold(document.XMLName.Local, "Project") {
		return Project{}, fmt.Errorf("parse project %s: root element is %q, want Project", absolute, document.XMLName.Local)
	}

	directory := filepath.Dir(absolute)
	name := strings.TrimSuffix(filepath.Base(absolute), filepath.Ext(absolute))

	var frameworks []string
	outputType := ""
	testProject := false
	for _, group := range document.PropertyGroups {
		frameworks = append(frameworks, splitFrameworks(group.TargetFramework)...)
		frameworks = append(frameworks, splitFrameworks(group.TargetFrameworks)...)
		if value := strings.TrimSpace(group.OutputType); value != "" {
			outputType = value
		}
		if strings.EqualFold(strings.TrimSpace(group.IsTestProject), "true") {
			testProject = true
		}
	}

	var references []string
	var packages []PackageReference
	for _, group := range document.ItemGroups {
		for _, reference := range group.ProjectReferences {
			if resolved := resolvePath(directory, reference.Include); resolved != "" {
				references = append(references, resolved)
			}
		}
		for _, reference := range group.PackageReferences {
			name := strings.TrimSpace(reference.Include)
			version := strings.TrimSpace(reference.Version)
			if version == "" {
				version = strings.TrimSpace(reference.VersionElement)
			}
			if name != "" {
				packages = append(packages, PackageReference{Name: name, Version: version})
			}
			if testPackages[strings.ToLower(strings.TrimSpace(reference.Include))] {
				testProject = true
			}
		}
	}

	sdk := strings.TrimSpace(document.Sdk)
	if sdk == "" {
		for _, element := range document.SDKs {
			if name := strings.TrimSpace(element.Name); name != "" {
				sdk = name
				break
			}
		}
	}

	return Project{
		Path:              absolute,
		Name:              name,
		Directory:         directory,
		Kind:              KindProject,
		TargetFrameworks:  uniqueStrings(frameworks),
		SDK:               sdk,
		OutputType:        outputType,
		TestProject:       testProject,
		ProjectReferences: uniqueStrings(references),
		PackageReferences: uniquePackageReferences(packages),
	}, nil
}

func uniquePackageReferences(packages []PackageReference) []PackageReference {
	seen := make(map[string]bool, len(packages))
	result := make([]PackageReference, 0, len(packages))
	for _, packageReference := range packages {
		key := strings.ToLower(packageReference.Name)
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, packageReference)
	}
	return result
}

func splitFrameworks(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ";")
	frameworks := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			frameworks = append(frameworks, part)
		}
	}
	return frameworks
}
