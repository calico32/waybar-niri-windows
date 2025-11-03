package niri

import (
	"fmt"
	"log"
	"os"
	"slices"
	"strings"
	"sync"
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
	s.onUpdate[id] = f
}

func (s *State) RemoveOnUpdate(id uint64) {
	delete(s.onUpdate, id)
}

func (s *State) Update(event Event) {
	defer func() {
		for _, f := range s.onUpdate {
			f(s)
		}
	}()

	s.mu.Lock()
	defer s.mu.Unlock()

	// fmt.Fprintf(os.Stderr, "Received event: %T\n", event)
	s.needsRedraw = false
	switch event := event.(type) {
	case *WorkspacesChanged:
		s.workspaces = make(map[uint64]*Workspace)
		for _, wk := range event.Workspaces {
			s.workspaces[wk.Id] = wk
			if wk.IsFocused && wk.Id != s.currentWorkspaceId {
				// fmt.Fprintf(os.Stderr, "  Newly focused workspace: %d\n", wk.Id)
				s.currentWorkspaceId = wk.Id
				s.needsRedraw = true
			}
		}
	case *WindowOpenedOrChanged:
		s.needsRedraw = true
		s.windows[event.Window.Id] = &event.Window
		if event.Window.IsFocused && event.Window.Id != s.currentWindowId {
			// fmt.Fprintf(os.Stderr, "  Newly focused window: %d\n", event.Window.Id)
			for _, window := range s.windows {
				window.IsFocused = false
			}
			event.Window.IsFocused = true
			s.currentWindowId = event.Window.Id
			s.needsRedraw = true
		}
	case *WorkspaceActivated:
		s.needsRedraw = true
		wk := s.workspaces[event.Id]
		if wk.Output == nil {
			log.Printf("wbcffi: workspace %d has no output", wk.Id)
			return
		}
		for _, workspace := range s.workspaces {
			if workspace.Output == nil {
				log.Printf("wbcffi: workspace %d has no output", workspace.Id)
				continue
			}
			if *wk.Output == *workspace.Output {
				workspace.IsActive = false
			}
		}
		wk.IsActive = true
		if event.Focused {
			// fmt.Fprintf(os.Stderr, "  Workspace activated and focused: %d\n", event.Id)
			for _, wk := range s.workspaces {
				wk.IsFocused = false
			}
			s.currentWorkspaceId = event.Id
			wk.IsFocused = true
		}
	case *WindowFocusChanged:
		s.needsRedraw = true
		if event.Id != nil {
			// fmt.Fprintf(os.Stderr, "  Window focus changed: %d -> %d\n", s.CurrentWindowId, *event.Id)
			// unset focus for all windows
			for _, window := range s.windows {
				window.IsFocused = false
			}
			// set focus for the new window
			if window, exists := s.windows[*event.Id]; exists {
				s.currentWindowId = *event.Id
				window.IsFocused = true
			} else {
				fmt.Fprintf(os.Stderr, "Warning: focused window %d not found in state\n", s.currentWindowId)
			}
		} else {
			s.currentWindowId = None
			// fmt.Fprintf(os.Stderr, "  Window focus changed: %d -> None\n", s.CurrentWindowId)
			for _, window := range s.windows {
				window.IsFocused = false
			}
		}
	case *WindowClosed:
		delete(s.windows, event.Id)
		if s.currentWindowId == event.Id {
			// fmt.Fprintf(os.Stderr, "  Focused window closed: %d\n", event.Id)
			s.needsRedraw = true
			s.currentWindowId = None
		}
	case *WindowLayoutsChanged:
		s.needsRedraw = true
		for _, change := range event.Changes {
			window := s.windows[change.Id]
			window.Layout = change.WindowLayout
			if window.WorkspaceId != nil && *window.WorkspaceId == s.currentWorkspaceId {
				// fmt.Fprintf(os.Stderr, "  Window layout on current workspace changed: %d\n", change.Id)
				s.needsRedraw = true
			}
		}
	case *WindowsChanged:
		s.needsRedraw = true
		for _, window := range event.Windows {
			s.windows[window.Id] = &window
			if window.IsFocused && window.Id != s.currentWindowId {
				// fmt.Fprintf(os.Stderr, "  Newly focused window: %d\n", window.Id)
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
		// fmt.Fprintf(os.Stderr, "Ignoring event: %T\n", event)
		return
	}

	// fmt.Fprintf(os.Stderr, "Processed event: %T\n", event)
}

const urgentBegin = "<span color=\"#fb2c36\">"
const urgentEnd = "</span>"

type Symbols struct {
	Unfocused         string `json:"unfocused"`
	Focused           string `json:"focused"`
	UnfocusedFloating string `json:"unfocused-floating"`
	FocusedFloating   string `json:"focused-floating"`
}

func (s *State) Draw(monitor string, symbols Symbols) string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if monitor == "" {
		currentOutput := s.workspaces[s.currentWorkspaceId].Output
		if currentOutput != nil {
			monitor = *currentOutput
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
	urgentColumns := make([]bool, len(s.windows))
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
		if i < len(urgentColumns) && urgentColumns[i] {
			output.WriteString(urgentBegin)
		}
		if focusedColumn == i {
			output.WriteString(symbols.Focused)
		} else {
			output.WriteString(symbols.Unfocused)
		}
		if i < len(urgentColumns) && urgentColumns[i] {
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

	return output.String()
}

func (s *State) Windows(monitor string) (tiled []*Window, floating []*Window) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if monitor == "" {
		currentOutput := s.workspaces[s.currentWorkspaceId].Output
		if currentOutput != nil {
			monitor = *currentOutput
		}
	}

	if monitor == "" {
		return nil, nil
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
