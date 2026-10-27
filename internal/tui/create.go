package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

func (m *Model) startProjectCreateWizard() {
	m.clearConfirm()
	destination := m.projectScanRoot()
	if destination == "" && m.projectResult != nil {
		destination = m.projectResult.Root
	}
	m.createWizard = projectCreateWizard{
		open:        true,
		destination: destination,
	}
	m.setStatus("", false)
}

func (m *Model) closeProjectCreateWizard() {
	m.createWizard = projectCreateWizard{}
}

func (m Model) selectedCreateTemplate() projects.Template {
	templates := projects.Templates()
	if len(templates) == 0 {
		return projects.Template{}
	}
	index := m.createWizard.templateCursor
	if index < 0 {
		index = 0
	}
	if index >= len(templates) {
		index = len(templates) - 1
	}
	return templates[index]
}

func (m *Model) moveCreateTemplate(delta int) {
	templates := projects.Templates()
	if len(templates) == 0 {
		return
	}
	m.createWizard.templateCursor += delta
	if m.createWizard.templateCursor < 0 {
		m.createWizard.templateCursor = len(templates) - 1
	}
	if m.createWizard.templateCursor >= len(templates) {
		m.createWizard.templateCursor = 0
	}
}

func (m Model) handleProjectCreateKey(message tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := message.String()
	if key == "esc" || key == "ctrl+c" {
		m.closeProjectCreateWizard()
		m.setStatus("Project creation cancelled.", false)
		return m, nil
	}

	switch m.createWizard.stage {
	case createStageTemplate:
		switch key {
		case "up", "k":
			m.moveCreateTemplate(-1)
		case "down", "j":
			m.moveCreateTemplate(1)
		case "enter":
			m.createWizard.stage = createStageName
		}
		return m, nil
	case createStageName:
		if key == "enter" {
			if strings.TrimSpace(m.createWizard.name) == "" {
				m.createWizard.errorMessage = "Enter a project name."
				return m, nil
			}
			m.createWizard.errorMessage = ""
			m.createWizard.stage = createStageDestination
			return m, nil
		}
		m.updateCreateInput(key, message.Text, &m.createWizard.name)
		return m, nil
	case createStageDestination:
		if key == "enter" {
			if strings.TrimSpace(m.createWizard.destination) == "" {
				m.createWizard.errorMessage = "Enter an absolute destination."
				return m, nil
			}
			m.createWizard.errorMessage = ""
			m.createWizard.stage = createStageSolution
			return m, nil
		}
		m.updateCreateInput(key, message.Text, &m.createWizard.destination)
		return m, nil
	case createStageSolution:
		switch key {
		case "left", "h":
			m.createWizard.solutionMode--
			if m.createWizard.solutionMode < 0 {
				m.createWizard.solutionMode = 2
			}
			return m, nil
		case "right", "l", "tab":
			m.createWizard.solutionMode = (m.createWizard.solutionMode + 1) % 3
			return m, nil
		case "enter":
			if m.createWizard.solutionMode == 2 && strings.TrimSpace(m.createWizard.solutionPath) == "" {
				m.createWizard.errorMessage = "Enter the existing solution path."
				return m, nil
			}
			return m, m.submitProjectCreateWizard()
		}
		if m.createWizard.solutionMode == 2 {
			m.updateCreateInput(key, message.Text, &m.createWizard.solutionPath)
		}
		return m, nil
	}
	return m, nil
}

func (m *Model) updateCreateInput(key, text string, target *string) {
	switch key {
	case "backspace", "ctrl+h":
		runes := []rune(*target)
		if len(runes) > 0 {
			*target = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		*target = ""
	default:
		if text != "" {
			*target += text
		}
	}
}

func (m *Model) submitProjectCreateWizard() tea.Cmd {
	template := m.selectedCreateTemplate()
	request := projects.CreateRequest{
		TemplateID:  template.ID,
		Name:        m.createWizard.name,
		Destination: m.createWizard.destination,
	}
	switch m.createWizard.solutionMode {
	case 1:
		request.CreateSolution = true
	case 2:
		request.ExistingSolution = m.createWizard.solutionPath
	}
	plan, err := projects.PlanCreate(request)
	if err != nil {
		m.createWizard.errorMessage = err.Error()
		return nil
	}
	m.closeProjectCreateWizard()
	m.pendingCreatePlan = &plan
	m.confirm = confirmCreateProjects
	m.confirmSummary = createPlanSummary(plan)
	return nil
}

func createPlanSummary(plan projects.CreatePlan) []string {
	summary := []string{"Create using " + plan.Template.Name, "destination " + plan.Root}
	if plan.Solution != "" {
		summary = append(summary, "solution "+plan.Solution)
	} else {
		summary = append(summary, "independent project")
	}
	for _, projectPath := range plan.Projects {
		summary = append(summary, "create "+projectPath)
	}
	summary = append(summary, fmt.Sprintf("%d commands will run; existing files are not overwritten by Hilt directly", len(plan.Steps)))
	return summary
}

func (m Model) createProjectsCmd(ctx context.Context, plan projects.CreatePlan, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		err := backend.CreateProjects(ctx, plan, progressWriter{ch}, progressWriter{ch})
		return operationDoneMsg{kind: opCreateProjects, err: err}
	}
}

func (m Model) renderProjectCreateWizard(width int) string {
	boxWidth := 78
	if boxWidth > width-4 {
		boxWidth = width - 4
	}
	if boxWidth < 28 {
		boxWidth = 28
	}
	templates := projects.Templates()
	content := []string{styleMuted.Render("Create a project or architecture preset. Nothing runs until you confirm the preview."), ""}
	switch m.createWizard.stage {
	case createStageTemplate:
		content = append(content, styleLabel.Render("TEMPLATE"))
		for index, template := range templates {
			marker := "  "
			if index == m.createWizard.templateCursor {
				marker = styleAccent.Render("> ")
			}
			content = append(content, marker+styleValue.Render(template.Name)+"  "+styleMuted.Render(template.Description))
		}
		content = append(content, "", styleFaint.Render("↑/↓ select · enter continue"))
	case createStageName:
		content = append(content, styleLabel.Render("PROJECT NAME"), "  "+valueOr(m.createWizard.name, styleFaint.Render("Orders"))+styleAccent.Render("▌"), "", styleFaint.Render("enter continue · esc cancel"))
	case createStageDestination:
		content = append(content, styleLabel.Render("DESTINATION"), "  "+valueOr(m.createWizard.destination, styleFaint.Render("/home/me/projects"))+styleAccent.Render("▌"), "", styleFaint.Render("enter continue · esc cancel"))
	case createStageSolution:
		modes := []string{"Independent project", "Create new solution", "Add to existing solution"}
		content = append(content, styleLabel.Render("SOLUTION"), "  "+styleSelected.Render(modes[m.createWizard.solutionMode]), styleFaint.Render("  ←/→ change"))
		if m.createWizard.solutionMode == 2 {
			content = append(content, "", styleLabel.Render("solution path")+"  "+valueOr(m.createWizard.solutionPath, styleFaint.Render("/home/me/App.sln"))+styleAccent.Render("▌"))
		}
		content = append(content, "", styleFaint.Render("enter preview · esc cancel"))
	}
	if m.createWizard.errorMessage != "" {
		content = append(content, "", styleError.Render(m.createWizard.errorMessage))
	}
	return styleCallout.Width(boxWidth).Render(strings.Join(content, "\n"))
}
