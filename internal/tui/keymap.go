package tui

import (
	"fmt"
	"sort"
)

type keyAction string

const (
	actionQuit            keyAction = "quit"
	actionCancel          keyAction = "cancel"
	actionCancelOperation keyAction = "cancel-operation"
	actionCloseHelp       keyAction = "close-help"
	actionHelp            keyAction = "help"
	actionNavigate        keyAction = "navigate"
	actionJump            keyAction = "jump"
	actionMove            keyAction = "move"
	actionSelect          keyAction = "select"
	actionRefresh         keyAction = "refresh"
	actionDoctor          keyAction = "doctor"
	actionInstall         keyAction = "install"
	actionRemove          keyAction = "remove"
	actionUpdates         keyAction = "updates"
	actionSwitchFocus     keyAction = "switch-focus"
	actionScroll          keyAction = "scroll"
	actionConfirm         keyAction = "confirm"
	actionStay            keyAction = "stay"
	actionCancelScan      keyAction = "cancel-scan"
	actionSwitchInput     keyAction = "switch-input"
	actionFramework       keyAction = "framework"
	actionResolve         keyAction = "resolve"
	actionFilter          keyAction = "filter"
	actionProjectTab      keyAction = "project-tab"
	actionBuild           keyAction = "build"
	actionToggleOutput    keyAction = "toggle-output"
	actionFollowOutput    keyAction = "follow-output"
	actionOpenDirectory   keyAction = "open-directory"
	actionChooseRoot      keyAction = "choose-root"
	actionToggleHidden    keyAction = "toggle-hidden"
)

type keyContext string

const (
	ctxQuitConfirm     keyContext = "quit-confirm"
	ctxHelp            keyContext = "help"
	ctxOperation       keyContext = "operation"
	ctxUpdates         keyContext = "updates"
	ctxPromptSDKInput  keyContext = "prompt-sdk-input"
	ctxPromptSDKList   keyContext = "prompt-sdk-list"
	ctxPromptRuntime   keyContext = "prompt-runtime"
	ctxConfirm         keyContext = "confirm"
	ctxHome            keyContext = "home"
	ctxSDKs            keyContext = "sdks"
	ctxDiagnostics     keyContext = "diagnostics"
	ctxProjectsLoading keyContext = "projects-loading"
	ctxProjects        keyContext = "projects"
	ctxProjectSplit    keyContext = "project-split"
	ctxProjectFilter   keyContext = "project-filter"
	ctxProjectImport   keyContext = "project-import"
	ctxProjectRoot     keyContext = "project-root"
	ctxSettings        keyContext = "settings"
	ctxOutput          keyContext = "output"
	ctxBuild           keyContext = "build"
	ctxBuildOutput     keyContext = "build-output"
	ctxNarrow          keyContext = "narrow"
	ctxHelpNavigation  keyContext = "help-navigation"
	ctxHelpManaged     keyContext = "help-managed"
	ctxHelpView        keyContext = "help-view"
)

type keyBinding struct {
	Key     string
	Action  keyAction
	Label   string
	Help    string
	Compact bool
}

type keyContextSpec struct {
	CompactWidth int
	Bindings     []keyBinding
}

var keymap = map[keyContext]keyContextSpec{
	ctxQuitConfirm: {Bindings: []keyBinding{
		{Key: "y/enter", Action: actionQuit, Label: "quit", Compact: true},
		{Key: "esc/n", Action: actionStay, Label: "stay", Compact: true},
	}},
	ctxHelp: {Bindings: []keyBinding{
		{Key: "esc", Action: actionCloseHelp, Label: "close help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxOperation: {Bindings: []keyBinding{
		{Key: "esc", Action: actionCancelOperation, Label: "cancel operation", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxUpdates: {Bindings: []keyBinding{
		{Key: "esc", Action: actionCancelOperation, Label: "cancel update check", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxPromptSDKInput: {Bindings: []keyBinding{
		{Key: "tab", Action: actionSwitchInput, Label: "select", Compact: true},
		{Key: "enter", Action: actionInstall, Label: "install", Compact: true},
		{Key: "esc", Action: actionCancel, Label: "cancel", Compact: true},
	}},
	ctxPromptSDKList: {Bindings: []keyBinding{
		{Key: "↑/↓", Action: actionSelect, Label: "select", Compact: true},
		{Key: "enter", Action: actionInstall, Label: "install", Compact: true},
		{Key: "tab", Action: actionSwitchInput, Label: "type", Compact: true},
		{Key: "esc", Action: actionCancel, Label: "cancel", Compact: true},
	}},
	ctxPromptRuntime: {Bindings: []keyBinding{
		{Key: "enter", Action: actionResolve, Label: "resolve", Compact: true},
		{Key: "←/→", Action: actionFramework, Label: "framework", Compact: true},
		{Key: "esc", Action: actionCancel, Label: "cancel", Compact: true},
	}},
	ctxConfirm: {Bindings: []keyBinding{
		{Key: "enter/y", Action: actionConfirm, Label: "confirm", Compact: true},
		{Key: "esc/n", Action: actionCancel, Label: "cancel", Compact: true},
	}},
	ctxHome: {CompactWidth: 100, Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "section", Compact: true},
		{Key: "1-5", Action: actionJump, Label: "jump"},
		{Key: "i", Action: actionInstall, Label: "install SDK", Compact: true},
		{Key: "r", Action: actionRefresh, Label: "refresh", Compact: true},
		{Key: "d", Action: actionDoctor, Label: "doctor"},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxSDKs: {CompactWidth: 110, Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "section", Compact: true},
		{Key: "tab", Action: actionSwitchFocus, Label: "switch list", Compact: true},
		{Key: "↑/↓", Action: actionSelect, Label: "select"},
		{Key: "i", Action: actionInstall, Label: "install", Compact: true},
		{Key: "x", Action: actionRemove, Label: "remove", Compact: true},
		{Key: "u", Action: actionUpdates, Label: "updates", Compact: true},
		{Key: "r", Action: actionRefresh, Label: "refresh"},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxDiagnostics: {CompactWidth: 100, Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "section", Compact: true},
		{Key: "pgup/pgdn", Action: actionScroll, Label: "scroll"},
		{Key: "r", Action: actionRefresh, Label: "re-run", Compact: true},
		{Key: "d", Action: actionDoctor, Label: "doctor"},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxProjectsLoading: {Bindings: []keyBinding{
		{Key: "esc", Action: actionCancelScan, Label: "cancel scan", Compact: true},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxProjects: {CompactWidth: 90, Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "nav"},
		{Key: "↑/↓", Action: actionSelect, Label: "select", Compact: true},
		{Key: "enter", Action: actionConfirm, Label: "open", Compact: true},
		{Key: "b", Action: actionBuild, Label: "build", Compact: true},
		{Key: "r", Action: actionRefresh, Label: "rescan", Compact: true},
		{Key: "/", Action: actionFilter, Label: "search", Compact: true},
		{Key: "p", Action: actionChooseRoot, Label: "root", Compact: true},
		{Key: "a", Action: actionChooseRoot, Label: "add"},
		{Key: ", .", Action: actionProjectTab, Label: "tab"},
		{Key: "pgup/pgdn", Action: actionScroll, Label: "scroll"},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxProjectSplit: {CompactWidth: 120, Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "nav"},
		{Key: "tab", Action: actionSwitchFocus, Label: "focus", Compact: true},
		{Key: "↑/↓", Action: actionSelect, Label: "select", Compact: true},
		{Key: "enter", Action: actionConfirm, Label: "open", Compact: true},
		{Key: "b", Action: actionBuild, Label: "build", Compact: true},
		{Key: "/", Action: actionFilter, Label: "search", Compact: true},
		{Key: "p", Action: actionChooseRoot, Label: "root", Compact: true},
		{Key: ", .", Action: actionProjectTab, Label: "tab", Compact: true},
		{Key: "pgup/pgdn", Action: actionScroll, Label: "scroll"},
		{Key: "r", Action: actionRefresh, Label: "rescan", Compact: true},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxBuild: {Bindings: []keyBinding{
		{Key: "esc", Action: actionCancelOperation, Label: "cancel build", Compact: true},
		{Key: "o", Action: actionToggleOutput, Label: "open output", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxBuildOutput: {Bindings: []keyBinding{
		{Key: "esc", Action: actionCancelOperation, Label: "cancel build", Compact: true},
		{Key: "o", Action: actionToggleOutput, Label: "close", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxOutput: {CompactWidth: 72, Bindings: []keyBinding{
		{Key: "o", Action: actionToggleOutput, Label: "close", Compact: true},
		{Key: "f", Action: actionFollowOutput, Label: "follow", Compact: true},
		{Key: "pgup/pgdn", Action: actionScroll, Label: "scroll", Compact: true},
		{Key: "home/end", Action: actionScroll, Label: "top/bottom", Compact: true},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxProjectFilter: {Bindings: []keyBinding{
		{Key: "type", Action: actionFilter, Label: "filter", Compact: true},
		{Key: "enter", Action: actionConfirm, Label: "keep", Compact: true},
		{Key: "esc", Action: actionCancel, Label: "clear", Compact: true},
	}},
	ctxProjectImport: {Bindings: []keyBinding{
		{Key: "enter", Action: actionConfirm, Label: "import", Compact: true},
		{Key: "esc", Action: actionCancel, Label: "cancel", Compact: true},
	}},
	ctxProjectRoot: {CompactWidth: 86, Bindings: []keyBinding{
		{Key: "esc", Action: actionCancel, Label: "cancel", Compact: true},
		{Key: "↑/↓", Action: actionSelect, Label: "select", Compact: true},
		{Key: "enter", Action: actionOpenDirectory, Label: "open", Compact: true},
		{Key: "s", Action: actionChooseRoot, Label: "choose", Compact: true},
		{Key: ".", Action: actionToggleHidden, Label: "hidden", Help: "show or hide dot-directories"},
		{Key: "q", Action: actionQuit, Label: "quit"},
	}},
	ctxSettings: {CompactWidth: 72, Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "section", Compact: true},
		{Key: "1-5", Action: actionJump, Label: "jump"},
		{Key: "r", Action: actionRefresh, Label: "refresh", Compact: true},
		{Key: "d", Action: actionDoctor, Label: "doctor"},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxNarrow: {Bindings: []keyBinding{
		{Key: "[/]", Action: actionNavigate, Label: "section", Compact: true},
		{Key: "?", Action: actionHelp, Label: "help", Compact: true},
		{Key: "q", Action: actionQuit, Label: "quit", Compact: true},
	}},
	ctxHelpNavigation: {Bindings: []keyBinding{
		{Key: "1-5", Action: actionJump, Label: "jump", Help: "jump to a section"},
		{Key: "[ / ]", Action: actionNavigate, Label: "section", Help: "previous / next section"},
		{Key: "tab", Action: actionSwitchFocus, Label: "focus", Help: "switch focus; arrows move the focused list"},
		{Key: "enter", Action: actionConfirm, Label: "open", Help: "activate project / confirm prompt"},
		{Key: "/", Action: actionFilter, Label: "filter", Help: "filter the current list"},
		{Key: ", .", Action: actionProjectTab, Label: "tab", Help: "previous / next project tab"},
	}},
	ctxHelpManaged: {Bindings: []keyBinding{
		{Key: "i", Action: actionInstall, Label: "install", Help: "install SDK or runtime"},
		{Key: "x", Action: actionRemove, Label: "remove", Help: "remove a managed item"},
		{Key: "u", Action: actionUpdates, Label: "updates", Help: "check or apply updates"},
		{Key: "esc", Action: actionCancel, Label: "cancel", Help: "close prompt / cancel operation"},
	}},
	ctxHelpView: {Bindings: []keyBinding{
		{Key: "r", Action: actionRefresh, Label: "refresh", Help: "refresh inventory"},
		{Key: "d", Action: actionDoctor, Label: "doctor", Help: "run diagnostics"},
		{Key: "b / p", Action: actionBuild, Label: "build", Help: "build selected project / choose scan root"},
		{Key: "o", Action: actionToggleOutput, Label: "output", Help: "open or close the output drawer"},
		{Key: "f", Action: actionFollowOutput, Label: "follow", Help: "follow or pause output"},
		{Key: "pgup/pgdn", Action: actionScroll, Label: "scroll", Help: "scroll content or output"},
		{Key: "? / esc", Action: actionCloseHelp, Label: "close", Help: "close help"},
		{Key: "q  ctrl+c", Action: actionQuit, Label: "quit", Help: "quit Hilt"},
	}},
}

func bindingsFor(context keyContext) []keyBinding {
	spec, ok := keymap[context]
	if !ok {
		return nil
	}
	return spec.Bindings
}

func footerBindings(context keyContext, width int) []keyBinding {
	spec, ok := keymap[context]
	if !ok {
		spec = keymap[ctxSettings]
	}
	bindings := spec.Bindings
	if spec.CompactWidth > 0 && width < spec.CompactWidth {
		compact := make([]keyBinding, 0, len(bindings))
		for _, binding := range bindings {
			if binding.Compact {
				compact = append(compact, binding)
			}
		}
		bindings = compact
	}
	if context == ctxOutput && width < 72 {
		narrow := make([]keyBinding, 0, 4)
		for _, binding := range bindings {
			switch binding.Action {
			case actionToggleOutput, actionFollowOutput, actionHelp, actionQuit:
				narrow = append(narrow, binding)
			}
		}
		bindings = narrow
	}
	if context == ctxProjectRoot && width < 56 {
		narrow := make([]keyBinding, 0, 3)
		for _, binding := range bindings {
			switch binding.Action {
			case actionCancel, actionOpenDirectory, actionChooseRoot:
				narrow = append(narrow, binding)
			}
		}
		bindings = narrow
	}
	if width < 56 && len(bindings) > 3 {
		bindings = keymap[ctxNarrow].Bindings
	}
	return bindings
}

func actionFor(context keyContext, key string) (keyAction, bool) {
	for _, binding := range bindingsFor(context) {
		if binding.Key == key {
			return binding.Action, true
		}
	}
	return "", false
}

func keymapConflicts(m map[keyContext]keyContextSpec) []string {
	var conflicts []string
	contexts := make([]string, 0, len(m))
	for context := range m {
		contexts = append(contexts, string(context))
	}
	sort.Strings(contexts)
	for _, raw := range contexts {
		context := keyContext(raw)
		seen := make(map[string]bool)
		for _, binding := range m[context].Bindings {
			switch {
			case binding.Key == "":
				conflicts = append(conflicts, fmt.Sprintf("%s: empty key", context))
			case binding.Action == "":
				conflicts = append(conflicts, fmt.Sprintf("%s: %s has no action", context, binding.Key))
			case binding.Label == "":
				conflicts = append(conflicts, fmt.Sprintf("%s: %s has no label", context, binding.Key))
			}
			if seen[binding.Key] {
				conflicts = append(conflicts, fmt.Sprintf("%s: duplicate key %q", context, binding.Key))
				continue
			}
			seen[binding.Key] = true
		}
	}
	return conflicts
}
