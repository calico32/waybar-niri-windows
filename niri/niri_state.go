package niri

import (
	"slices"
	"strings"
	"sync"
	"wnw/log"
)

const None = uint64(0xffffffffffffffff)

type State struct {
	mu sync.RWMutex

	currentWorkspaceId uint64
	currentWindowId    uint64
	workspaces         map[uint64]*Workspace
	windows            map[uint64]*Window
	onUpdate           map[uint64]func(*State)

	needsRedraw bool
}

// NewNiriState initializes a new NiriState with empty maps for workspaces and windows.
func NewNiriState() *State {
	return &State{
		currentWorkspaceId: None,
		currentWindowId:    None,
		workspaces:         make(map[uint64]*Workspace),
		windows:            make(map[uint64]*Window),
		needsRedraw:        false,
		onUpdate:           make(map[uint64]func(*State)),
	}
}

func (s *State) OnUpdate(id uint64, f func(*State)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onUpdate[id] = f
}

func (s *State) RemoveOnUpdate(id uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.onUpdate, id)
}

func (s *State) Update(event Event) {
	defer func() {
		s.mu.RLock()
		defer s.mu.RUnlock()
		callbacks := make([]func(*State), 0, len(s.onUpdate))
		for _, f := range s.onUpdate {
			callbacks = append(callbacks, f)
		}
		defer func() {
			for _, f := range callbacks {
				f(s)
			}
		}()
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	log.Tracef("received event: %T", event)
	s.needsRedraw = false
	switch event := event.(type) {
	case *WorkspacesChanged:
		s.workspaces = make(map[uint64]*Workspace)
		for _, wk := range event.Workspaces {
			s.workspaces[wk.Id] = wk
			if wk.IsFocused && wk.Id != s.currentWorkspaceId {
				log.Tracef("  newly focused workspace: %d", wk.Id)
				s.currentWorkspaceId = wk.Id
				s.needsRedraw = true
			}
		}
	case *WindowOpenedOrChanged:
		s.needsRedraw = true
		window := event.Window
		s.windows[window.Id] = &window
		if window.IsFocused && window.Id != s.currentWindowId {
			log.Tracef("  newly focused window: %d", event.Window.Id)
			for _, w := range s.windows {
				w.IsFocused = false
			}
			window.IsFocused = true
			s.currentWindowId = window.Id
			s.needsRedraw = true
		}
	case *WorkspaceActivated:
		s.needsRedraw = true
		wk, ok := s.workspaces[event.Id]
		if !ok {
			log.Errorf("workspace %d not found", event.Id)
			return
		}
		if wk.Output == nil {
			log.Errorf("workspace %d has no output", wk.Id)
			return
		}
		for _, workspace := range s.workspaces {
			if workspace.Output == nil {
				log.Errorf("workspace %d has no output", workspace.Id)
				continue
			}
			if *wk.Output == *workspace.Output {
				workspace.IsActive = false
			}
		}
		wk.IsActive = true
		if event.Focused {
			log.Tracef("  workspace activated and focused: %d", event.Id)
			for _, wk := range s.workspaces {
				wk.IsFocused = false
			}
			s.currentWorkspaceId = event.Id
			wk.IsFocused = true
		}
	case *WindowFocusChanged:
		s.needsRedraw = true
		if event.Id != nil {
			log.Tracef("  window focus changed: %d -> %d", s.currentWindowId, *event.Id)
			// unset focus for all windows
			for _, window := range s.windows {
				window.IsFocused = false
			}
			// set focus for the new window
			if window, exists := s.windows[*event.Id]; exists {
				s.currentWindowId = *event.Id
				window.IsFocused = true
			} else {
				log.Warnf("focused window %d not found in state", s.currentWindowId)
			}
		} else {
			s.currentWindowId = None
			log.Tracef("  window focus changed: %d -> None", s.currentWindowId)
			for _, window := range s.windows {
				window.IsFocused = false
			}
		}
	case *WindowFocusTimestampChanged:
		win, ok := s.windows[event.Id]
		if !ok {
			log.Warnf("window %d not found in state", event.Id)
			return
		}
		win.FocusTimestamp = event.FocusTimestamp
	case *WindowClosed:
		delete(s.windows, event.Id)
		if s.currentWindowId == event.Id {
			log.Tracef("  focused window closed: %d", event.Id)
			s.currentWindowId = None
		}
		s.needsRedraw = true
	case *WindowLayoutsChanged:
		s.needsRedraw = true
		for _, change := range event.Changes {
			window := s.windows[change.Id]
			window.Layout = change.WindowLayout
			if window.WorkspaceId != nil && *window.WorkspaceId == s.currentWorkspaceId {
				log.Tracef("  window layout on current workspace changed: %d", change.Id)
				s.needsRedraw = true
			}
		}
	case *WindowsChanged:
		s.needsRedraw = true
		for _, window := range event.Windows {
			w := window
			s.windows[window.Id] = &w
			if window.IsFocused && window.Id != s.currentWindowId {
				log.Tracef("  newly focused window: %d", window.Id)
				s.currentWindowId = window.Id
			}
		}
	case *WindowUrgencyChanged:
		window := s.windows[event.Id]
		if window != nil {
			window.IsUrgent = event.Urgent
			s.needsRedraw = true
		}
	case *WorkspaceUrgencyChanged:
		workspace := s.workspaces[event.Id]
		if workspace != nil {
			workspace.IsUrgent = event.Urgent
			s.needsRedraw = true
		}
	default:
		log.Tracef("ignoring event: %T\n", event)
		return
	}

	log.Tracef("processed event: %T\n", event)
}

const urgentBegin = "<span color=\"#fb2c36\">"
const urgentEnd = "</span>"

type Symbols struct {
	Unfocused         string `json:"unfocused"`
	Focused           string `json:"focused"`
	UnfocusedFloating string `json:"unfocused-floating"`
	FocusedFloating   string `json:"focused-floating"`
	Empty             string `json:"empty"`
}

func (s *State) Text(monitor string, symbols Symbols) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if monitor == "" {
		workspace, ok := s.workspaces[s.currentWorkspaceId]
		if !ok {
			log.Errorf("current workspace %d has no output", s.currentWorkspaceId)
			return "couldn't determine monitor"
		}
		if workspace.Output != nil {
			monitor = *workspace.Output
		}
	}

	if monitor == "" {
		return "couldn't determine monitor"
	}

	targetWorkspaceId := None
	for _, workspace := range s.workspaces {
		if workspace.Output != nil && *workspace.Output == monitor && workspace.IsActive {
			targetWorkspaceId = workspace.Id
			break
		}
	}
	if targetWorkspaceId == None {
		return "couldn't determine workspace"
	}

	focusedColumn := -1
	maxColumn := -1
	urgentColumns := make(map[int]bool)
	focusedFloating := uint64(0)
	floatingWindows := make([]*Window, 0, len(s.windows))
	for _, window := range s.windows {
		if window.WorkspaceId != nil && *window.WorkspaceId == targetWorkspaceId {
			location := window.Layout.PosInScrollingLayout
			if location != nil {
				col := int(location.X)
				if window.IsFocused {
					focusedColumn = col
				}
				if col > maxColumn {
					maxColumn = col
				}
				if window.IsUrgent {
					urgentColumns[col] = true
				}
			} else if window.IsFloating {
				if window.IsFocused {
					focusedFloating = window.Id
				}
				floatingWindows = append(floatingWindows, window)
			}
		}
	}

	// sort floating windows left-to-right
	slices.SortFunc(floatingWindows, func(a, b *Window) int {
		return int(a.Layout.TilePosInWorkspaceView.X) - int(b.Layout.TilePosInWorkspaceView.X)
	})

	var output strings.Builder
	for i := 1; i <= int(maxColumn); i++ {
		if urgentColumns[i] {
			output.WriteString(urgentBegin)
		}
		if focusedColumn == i {
			output.WriteString(symbols.Focused)
		} else {
			output.WriteString(symbols.Unfocused)
		}
		if urgentColumns[i] {
			output.WriteString(urgentEnd)
		}
	}
	if len(floatingWindows) > 0 {
		if maxColumn > 0 {
			output.WriteRune(' ')
		}
		for i := 0; i < len(floatingWindows); i++ {
			if floatingWindows[i].Id == focusedFloating {
				output.WriteString(symbols.FocusedFloating)
			} else {
				output.WriteString(symbols.UnfocusedFloating)
			}
		}
	}

	if output.Len() == 0 {
		return symbols.Empty
	}
	return output.String()
}

func (s *State) Windows(monitor string) (tiled []*Window, floating []*Window) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if monitor == "" {
		workspace, ok := s.workspaces[s.currentWorkspaceId]
		if !ok {
			log.Errorf("current workspace %d has no output", s.currentWorkspaceId)
			return nil, nil
		}
		if workspace.Output != nil {
			monitor = *workspace.Output
		}
	}

	targetWorkspaceId := None
	for _, workspace := range s.workspaces {
		if workspace.Output != nil && *workspace.Output == monitor && workspace.IsActive {
			targetWorkspaceId = workspace.Id
			break
		}
	}
	if targetWorkspaceId == None {
		return nil, nil
	}

	for _, window := range s.windows {
		if window.WorkspaceId != nil && *window.WorkspaceId == targetWorkspaceId {
			if window.IsFloating {
				floating = append(floating, window)
			} else {
				tiled = append(tiled, window)
			}
		}
	}

	slices.SortFunc(tiled, func(a, b *Window) int {
		x := int(a.Layout.PosInScrollingLayout.X) - int(b.Layout.PosInScrollingLayout.X)
		if x != 0 {
			return x
		}
		return int(a.Layout.PosInScrollingLayout.Y) - int(b.Layout.PosInScrollingLayout.Y)
	})

	slices.SortFunc(floating, func(a, b *Window) int {
		x := int(a.Layout.TilePosInWorkspaceView.X) - int(b.Layout.TilePosInWorkspaceView.X)
		if x != 0 {
			return x
		}
		return int(a.Layout.TilePosInWorkspaceView.Y) - int(b.Layout.TilePosInWorkspaceView.Y)
	})

	return
}
