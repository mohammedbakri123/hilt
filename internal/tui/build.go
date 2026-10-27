package tui

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/mohammedbakri123/hilt/internal/execution"
	"github.com/mohammedbakri123/hilt/internal/process"
	"github.com/mohammedbakri123/hilt/internal/projects"
	"github.com/mohammedbakri123/hilt/internal/state"
)

type buildLineMsg struct {
	run  int
	text string
}

type buildDoneMsg struct {
	run       int
	result    execution.Result
	err       error
	recordErr error
}

type buildTickMsg struct {
	run int
}

type buildStreamWriter struct {
	mu      sync.Mutex
	ch      chan<- tea.Msg
	run     int
	pending string
}

func (w *buildStreamWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.pending += string(data)
	for {
		index := strings.IndexByte(w.pending, '\n')
		if index < 0 {
			break
		}
		line := strings.TrimRight(w.pending[:index], "\r")
		w.pending = w.pending[index+1:]
		if strings.TrimSpace(line) != "" {
			w.ch <- buildLineMsg{run: w.run, text: line}
		}
	}
	return len(data), nil
}

func (w *buildStreamWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	line := strings.TrimRight(w.pending, "\r")
	w.pending = ""
	if strings.TrimSpace(line) != "" {
		w.ch <- buildLineMsg{run: w.run, text: line}
	}
}

func waitForBuild(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		message, ok := <-ch
		if !ok {
			return nil
		}
		return message
	}
}

func buildTick(run int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return buildTickMsg{run: run}
	})
}

func (m *Model) startBuild() tea.Cmd {
	if m.backend == nil {
		m.setStatus("No backend configured.", true)
		return nil
	}
	if m.projectsLoading {
		m.setStatus("Wait for the project scan to finish before building.", true)
		return nil
	}
	project, ok := m.selectedProject()
	if !ok {
		m.setStatus("Select a project before building.", true)
		return nil
	}
	if m.selection == nil || m.selection.DotnetPath == "" {
		if m.selectionErr != nil {
			m.setStatus("No usable .NET SDK: "+m.selectionErr.Error(), true)
		} else {
			m.setStatus("No usable .NET SDK was selected.", true)
		}
		return nil
	}
	workingDirectory := project.Directory
	if workingDirectory == "" {
		workingDirectory = filepath.Dir(project.Path)
	}
	request := execution.BuildRequest{
		DotnetPath:       m.selection.DotnetPath,
		ProjectPath:      project.Path,
		WorkingDirectory: workingDirectory,
		Configuration:    "Debug",
		Environment:      m.selection.Environment(nil),
	}
	return m.startBuildRequest(project, request)
}

func (m *Model) startBuildRequest(project projects.Project, request execution.BuildRequest) tea.Cmd {
	record := state.BuildTarget{
		ProjectPath:   project.Path,
		WorkspacePath: m.workspaceFor(project.Path),
		SDKVersion:    m.selection.SDKVersion,
		Configuration: request.Configuration,
	}
	if project.Kind != projects.KindProject {
		record.WorkspacePath = project.Path
	}

	m.buildRun++
	run := m.buildRun
	ch := make(chan tea.Msg, 512)
	ctx, cancel := context.WithCancel(context.Background())
	m.buildRunning = true
	m.buildCancelling = false
	m.buildCancel = cancel
	m.buildCh = ch
	m.buildTarget = project
	m.hasBuildTarget = true
	m.buildMode = buildRunBuild
	m.buildResult = nil
	m.buildErr = nil
	m.buildStartedAt = time.Now()
	m.buildFinishedAt = time.Time{}
	m.drawer.open = true
	m.drawer.follow = true
	m.drawer.lines = nil
	m.drawer.scroll = 0
	m.drawer.kind = "BUILD LOGS"
	m.drawer.status = "building · " + project.Name
	m.drawer.meta = buildDrawerMeta(project, request, m.selection.SDKVersion)
	m.setStatus("Building "+project.Name+"...", false)

	return tea.Batch(m.buildCmd(ctx, run, request, record, ch), waitForBuild(ch), buildTick(run))
}

func (m *Model) startRunRequest(project projects.Project, request execution.CommandRequest) tea.Cmd {
	m.buildRun++
	run := m.buildRun
	ch := make(chan tea.Msg, 512)
	ctx, cancel := context.WithCancel(context.Background())
	m.buildRunning = true
	m.buildCancelling = false
	m.buildCancel = cancel
	m.buildCh = ch
	m.buildTarget = project
	m.hasBuildTarget = true
	m.buildMode = buildRunRun
	m.buildResult = nil
	m.buildErr = nil
	m.buildStartedAt = time.Now()
	m.buildFinishedAt = time.Time{}
	m.drawer.open = true
	m.drawer.follow = true
	m.drawer.lines = nil
	m.drawer.scroll = 0
	m.drawer.kind = "RUN LOGS"
	m.drawer.status = "running · " + project.Name
	m.drawer.meta = runDrawerMeta(project, request, m.selection.SDKVersion)
	m.setStatus("Running "+project.Name+"...", false)

	return tea.Batch(m.runCmd(ctx, run, request, ch), waitForBuild(ch), buildTick(run))
}

func (m Model) buildCmd(ctx context.Context, run int, request execution.BuildRequest, record state.BuildTarget, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		stdout := &buildStreamWriter{ch: ch, run: run}
		stderr := &buildStreamWriter{ch: ch, run: run}
		result, err := backend.BuildProject(ctx, request, stdout, stderr)
		stdout.Flush()
		stderr.Flush()
		var recordErr error
		if result.Started {
			recordErr = backend.RecordBuildTarget(record)
		}
		return buildDoneMsg{run: run, result: result, err: err, recordErr: recordErr}
	}
}

func (m Model) runCmd(ctx context.Context, run int, request execution.CommandRequest, ch chan tea.Msg) tea.Cmd {
	backend := m.backend
	return func() tea.Msg {
		defer close(ch)
		stdout := &buildStreamWriter{ch: ch, run: run}
		stderr := &buildStreamWriter{ch: ch, run: run}
		result, err := backend.RunProjectCommand(ctx, request, stdout, stderr)
		stdout.Flush()
		stderr.Flush()
		return buildDoneMsg{run: run, result: result, err: err}
	}
}

func (m *Model) finishBuild(message buildDoneMsg) {
	if message.run != m.buildRun {
		return
	}
	if m.buildCancel != nil {
		m.buildCancel()
	}
	m.buildRunning = false
	m.buildCancelling = false
	m.buildCancel = nil
	m.buildFinishedAt = time.Now()
	result := message.result
	m.buildResult = &result
	m.buildErr = message.err
	m.drawer.status = buildDrawerStatus(message.result, message.err)

	status := ""
	isError := false
	switch {
	case errors.Is(message.err, context.Canceled) || message.result.Cancelled:
		status = m.buildMode.label() + " cancelled."
	case message.err != nil:
		var exitErr *process.ExitError
		if errors.As(message.err, &exitErr) {
			status = fmt.Sprintf("%s failed: %s exited with code %d.", m.buildMode.label(), m.buildTarget.Name, exitErr.Code)
		} else {
			status = m.buildMode.label() + " failed: " + message.err.Error()
		}
		isError = true
	case message.result.Started:
		verb := "Built"
		if m.buildMode == buildRunRun {
			verb = "Ran"
		}
		status = fmt.Sprintf("%s %s in %s.", verb, m.buildTarget.Name, formatBuildDuration(message.result.Duration))
	default:
		status = m.buildMode.label() + " did not start."
		isError = true
	}
	if message.recordErr != nil {
		status += " Could not save the last build target: " + message.recordErr.Error()
		isError = true
	}
	m.setStatus(status, isError)
	if m.drawer.follow {
		m.followDrawer()
	} else {
		m.clampDrawerScroll()
	}
}

func (m *Model) cancelBuild() {
	if !m.buildRunning {
		return
	}
	if m.buildCancel != nil {
		m.buildCancel()
	}
	m.buildCancelling = true
	m.drawer.status = "cancelling · " + m.buildTarget.Name
	m.setStatus("Cancelling "+strings.ToLower(m.buildMode.label())+"...", false)
}

func formatBuildDuration(duration time.Duration) string {
	if duration < time.Millisecond {
		return "<1ms"
	}
	return duration.Round(time.Millisecond).String()
}

func buildDrawerStatus(result execution.Result, err error) string {
	if result.Cancelled || errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if err != nil {
		var exitErr *process.ExitError
		if errors.As(err, &exitErr) {
			return fmt.Sprintf("exit %d · %s", exitErr.Code, formatBuildDuration(result.Duration))
		}
		return "start failed"
	}
	if !result.Started {
		return "not started"
	}
	return fmt.Sprintf("exit %d · %s", result.ExitCode, formatBuildDuration(result.Duration))
}

func buildDrawerMeta(project projects.Project, request execution.BuildRequest, sdkVersion string) []string {
	meta := []string{project.Name, "SDK " + valueOr(sdkVersion, "unknown")}
	if request.WorkingDirectory != "" {
		meta = append(meta, request.WorkingDirectory)
	}
	if args, err := execution.BuildArgs(request); err == nil {
		meta = append(meta, strings.Join(append([]string{request.DotnetPath}, args...), " "))
	}
	return meta
}

func runDrawerMeta(project projects.Project, request execution.CommandRequest, sdkVersion string) []string {
	meta := []string{project.Name, "SDK " + valueOr(sdkVersion, "unknown")}
	if request.WorkingDirectory != "" {
		meta = append(meta, request.WorkingDirectory)
	}
	meta = append(meta, strings.Join(append([]string{request.DotnetPath}, request.Arguments...), " "))
	return meta
}
