package main

import (
	"fmt"
	"os"
	"strings"
)

const None = uint64(0xffffffffffffffff)

type NiriState struct {
	CurrentWorkspaceId uint64
	CurrentWindowId    uint64
	Workspaces         map[uint64]*Workspace
	Windows            map[uint64]*Window

	needsRedraw bool
}

// NewNiriState initializes a new NiriState with empty maps for workspaces and windows.
func NewNiriState() *NiriState {
	return &NiriState{
		CurrentWorkspaceId: None,
		CurrentWindowId:    None,
		Workspaces:         make(map[uint64]*Workspace),
		Windows:            make(map[uint64]*Window),
		needsRedraw:        false,
	}
}

func (s *NiriState) Update(event Event) {
	// fmt.Fprintf(os.Stderr, "Received event: %T\n", event)
	switch event := event.(type) {
	case *WorkspacesChanged:
		s.Workspaces = make(map[uint64]*Workspace)
		for _, wk := range event.Workspaces {
			s.Workspaces[wk.Id] = wk
			if wk.IsFocused && wk.Id != s.CurrentWorkspaceId {
				// fmt.Fprintf(os.Stderr, "  Newly focused workspace: %d\n", wk.Id)
				s.CurrentWorkspaceId = wk.Id
				s.needsRedraw = true
			}
		}
	case *WindowOpenedOrChanged:
		s.Windows[event.Window.Id] = &event.Window
		if event.Window.IsFocused && event.Window.Id != s.CurrentWindowId {
			// fmt.Fprintf(os.Stderr, "  Newly focused window: %d\n", event.Window.Id)
			for _, window := range s.Windows {
				window.IsFocused = false
			}
			event.Window.IsFocused = true
			s.CurrentWindowId = event.Window.Id
			s.needsRedraw = true
		}
	case *WorkspaceActivated:
		wk := s.Workspaces[event.Id]
		for _, workspace := range s.Workspaces {
			if wk.Output == workspace.Output {
				workspace.IsActive = false
			}
		}
		wk.IsActive = true
		if event.Focused {
			// fmt.Fprintf(os.Stderr, "  Workspace activated and focused: %d\n", event.Id)
			for _, wk := range s.Workspaces {
				wk.IsFocused = false
			}
			s.CurrentWorkspaceId = event.Id
			wk.IsFocused = true
		}
	case *WindowFocusChanged:
		s.needsRedraw = true
		if event.Id != nil {
			// fmt.Fprintf(os.Stderr, "  Window focus changed: %d -> %d\n", s.CurrentWindowId, *event.Id)
			// unset focus for all windows
			for _, window := range s.Windows {
				window.IsFocused = false
			}
			// set focus for the new window
			if window, exists := s.Windows[*event.Id]; exists {
				s.CurrentWindowId = *event.Id
				window.IsFocused = true
			} else {
				fmt.Fprintf(os.Stderr, "Warning: focused window %d not found in state\n", s.CurrentWindowId)
			}
			s.needsRedraw = true
		} else {
			s.CurrentWindowId = None
			// fmt.Fprintf(os.Stderr, "  Window focus changed: %d -> None\n", s.CurrentWindowId)
			for _, window := range s.Windows {
				window.IsFocused = false
			}
			s.needsRedraw = true
		}
	case *WindowClosed:
		delete(s.Windows, event.Id)
		if s.CurrentWindowId == event.Id {
			// fmt.Fprintf(os.Stderr, "  Focused window closed: %d\n", event.Id)
			s.needsRedraw = true
			s.CurrentWindowId = None
		}
	case *WindowLayoutsChanged:
		for _, change := range event.Changes {
			window := s.Windows[change.Id]
			window.Location = change.WindowLayout
			if window.WorkspaceId != nil && *window.WorkspaceId == s.CurrentWorkspaceId {
				// fmt.Fprintf(os.Stderr, "  Window layout on current workspace changed: %d\n", change.Id)
				s.needsRedraw = true
			}
		}
	case *WindowsChanged:
		for _, window := range event.Windows {
			s.Windows[window.Id] = &window
			if window.IsFocused && window.Id != s.CurrentWindowId {
				// fmt.Fprintf(os.Stderr, "  Newly focused window: %d\n", window.Id)
				s.CurrentWindowId = window.Id
				s.needsRedraw = true
			}
		}
	default:
		// fmt.Fprintf(os.Stderr, "Ignoring event: %T\n", event)
		return
	}

	// fmt.Fprintf(os.Stderr, "Processed event: %T\n", event)
}

const unfocused = '⋅'
const focused = '⊙'

func (s *NiriState) Redraw() {
	if !s.needsRedraw {
		return
	}

	windowsOnCurrentWorkspace := make([]*Window, 0)
	focusedColumn := -1
	maxColumn := -1
	for _, window := range s.Windows {
		if window.WorkspaceId != nil && *window.WorkspaceId == s.CurrentWorkspaceId {
			windowsOnCurrentWorkspace = append(windowsOnCurrentWorkspace, window)
			col := int(window.Location.TilePosInScrollingLayout.X)
			if window.IsFocused {
				focusedColumn = col
			}
			if col > maxColumn {
				maxColumn = col
			}
		}
	}

	// sort windows by their tile position in the scrolling layout
	// slices.SortFunc(windowsOnCurrentWorkspace, func(a, b *Window) int {
	// 	return int(a.Location.TilePosInScrollingLayout.X) - int(b.Location.TilePosInScrollingLayout.X)
	// })
	// fmt.Println("Collected windows:")
	// for _, window := range windowsOnCurrentWorkspace {
	// 	fmt.Printf("  %s\n", window)
	// }

	var output strings.Builder
	for i := 0; i <= int(maxColumn); i++ {
		if focusedColumn == i {
			output.WriteRune(focused)
		} else {
			output.WriteRune(unfocused)
		}
	}
	write(output.String())
}

func (w *Window) String() string {
	title := "(no title)"
	if w.Title != nil {
		title = *w.Title
	}
	focused := ""
	if w.IsFocused {
		focused = " (focused)"
	}
	wk := "None"
	if w.WorkspaceId != nil {
		wk = fmt.Sprintf("%d", *w.WorkspaceId)
	}
	return fmt.Sprintf("Window(%d) \"%s\"%s at (%d, %d) on wk %s", w.Id, title, focused, w.Location.TilePosInScrollingLayout.X, w.Location.TilePosInScrollingLayout.Y, wk)
}
