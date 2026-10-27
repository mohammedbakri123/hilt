package projects

import (
	"path/filepath"
	"testing"
)

func TestTemplatesIncludeStandaloneAndArchitecturePresets(t *testing.T) {
	for _, id := range []string{"console", "webapi", "clean-architecture", "vertical-slice"} {
		if _, ok := FindTemplate(id); !ok {
			t.Fatalf("template %q was not found", id)
		}
	}
	architecture, _ := FindTemplate("clean-architecture")
	if len(architecture.Projects) != 5 {
		t.Fatalf("clean architecture projects = %d, want 5", len(architecture.Projects))
	}
}

func TestPlanCreateExistingSolution(t *testing.T) {
	plan, err := PlanCreate(CreateRequest{
		TemplateID:       "webapi",
		Name:             "Orders.Api",
		Destination:      "/work/src",
		ExistingSolution: "/work/Orders.sln",
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Solution != "/work/Orders.sln" || len(plan.Projects) != 1 {
		t.Fatalf("plan = %+v", plan)
	}
	last := plan.Steps[len(plan.Steps)-1]
	want := []string{"dotnet", "sln", "/work/Orders.sln", "add", filepath.Join("/work/src", "Orders.Api")}
	if len(last.Command) != len(want) {
		t.Fatalf("last command = %#v", last.Command)
	}
	for index := range want {
		if last.Command[index] != want[index] {
			t.Errorf("command[%d] = %q, want %q", index, last.Command[index], want[index])
		}
	}
}

func TestPlanCreateArchitectureCreatesSolutionAndProjects(t *testing.T) {
	plan, err := PlanCreate(CreateRequest{
		TemplateID:     "clean-architecture",
		Name:           "Orders",
		Destination:    "/work/orders",
		CreateSolution: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.Solution != "/work/orders/Orders.sln" || len(plan.Projects) != 5 {
		t.Fatalf("plan = %+v", plan)
	}
	if plan.Steps[0].Command[0:4][0] != "dotnet" || plan.Steps[0].Command[2] != "sln" {
		t.Fatalf("first step = %#v", plan.Steps[0].Command)
	}
}

func TestPlanCreateRejectsUnsafeRequests(t *testing.T) {
	for _, request := range []CreateRequest{
		{TemplateID: "missing", Name: "App", Destination: "/work"},
		{TemplateID: "console", Name: "../App", Destination: "/work"},
		{TemplateID: "console", Name: "App", Destination: "relative"},
		{TemplateID: "console", Name: "App", Destination: "/work", ExistingSolution: "/work/App.sln", CreateSolution: true},
	} {
		if _, err := PlanCreate(request); err == nil {
			t.Errorf("request %+v unexpectedly succeeded", request)
		}
	}
}
