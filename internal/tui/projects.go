package tui

import (
	"context"
	"errors"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/projects"
)

const projectsScanTimeout = 30 * time.Second

func (m *Model) startProjectsScan() tea.Cmd {
	if m.backend == nil {
		return nil
	}
	m.stopProjectScan()
	ctx, cancel := context.WithTimeout(context.Background(), projectsScanTimeout)
	m.projectCancel = cancel
	m.projectRequest++
	request := m.projectRequest
	m.projectsLoading = true
	m.projectsErr = nil
	backend := m.backend
	return func() tea.Msg {
		result, err := backend.ScanProjects(ctx, "", false)
		return projectsMsg{request: request, result: result, err: err}
	}
}

func (m *Model) stopProjectScan() {
	if m.projectCancel != nil {
		m.projectCancel()
		m.projectCancel = nil
	}
}

func (m *Model) cancelProjects() {
	if !m.projectsLoading {
		return
	}
	m.stopProjectScan()
	m.projectsLoading = false
	m.setStatus("Project scan cancelled.", false)
}

func (m Model) handleProjectsMsg(message projectsMsg) (tea.Model, tea.Cmd) {
	if message.request != m.projectRequest {
		return m, nil
	}
	m.stopProjectScan()
	m.projectsLoading = false
	if message.err != nil {
		if errors.Is(message.err, context.Canceled) {
			m.setStatus("Project scan cancelled.", false)
			return m, nil
		}
		m.projectsErr = message.err
		m.setStatus("Project scan failed.", true)
		return m, nil
	}
	previous := ""
	if m.projectResult != nil {
		if selected, ok := m.selectedProject(); ok {
			previous = selected.Path
		}
	}
	result := message.result
	m.projectResult = &result
	m.projectsErr = nil
	m.projectsLoaded = true
	m.restoreProjectSelection(previous)
	m.clampScroll()
	return m, nil
}

func (m *Model) moveProject(delta int) {
	if m.projectResult == nil || len(m.projectResult.Projects) == 0 {
		return
	}
	m.projectCursor = clampIndex(m.projectCursor+delta, len(m.projectResult.Projects))
}

func (m Model) selectedProject() (projects.Project, bool) {
	if m.projectResult == nil || len(m.projectResult.Projects) == 0 {
		return projects.Project{}, false
	}
	index := clampIndex(m.projectCursor, len(m.projectResult.Projects))
	return m.projectResult.Projects[index], true
}

func (m *Model) restoreProjectSelection(path string) {
	m.clampProjectSelection()
	if path == "" || m.projectResult == nil {
		return
	}
	for index, project := range m.projectResult.Projects {
		if project.Path == path {
			m.projectCursor = index
			return
		}
	}
}

func (m *Model) clampProjectSelection() {
	if m.projectResult == nil || len(m.projectResult.Projects) == 0 {
		m.projectCursor = 0
		return
	}
	m.projectCursor = clampIndex(m.projectCursor, len(m.projectResult.Projects))
}
