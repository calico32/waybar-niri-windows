package niri

import (
	"encoding/json"
	"fmt"
)

type Event interface {
	Name() string
}

// A compositor event.
type NiriEvent struct {
	Ok any
	// The workspace configuration has changed.
	WorkspacesChanged *WorkspacesChanged
	// The workspace urgency changed.
	WorkspaceUrgencyChanged *WorkspaceUrgencyChanged
	// A workspace was activated on an output.
	//
	// This doesn't always mean the workspace became focused, just that it's now the
	// active workspace on its output. All other workspaces on the same output
	// become inactive.
	WorkspaceActivated *WorkspaceActivated
	// An active window changed on a workspace.
	WorkspaceActiveWindowChanged *WorkspaceActiveWindowChanged
	// The window configuration has changed.
	WindowsChanged *WindowsChanged
	// A new toplevel window was opened, or an existing toplevel window changed.
	WindowOpenedOrChanged *WindowOpenedOrChanged
	// A toplevel window was closed.
	WindowClosed *WindowClosed
	// Window focus changed.
	//
	// All other windows are no longer focused.
	WindowFocusChanged *WindowFocusChanged
	// Window urgency changed.
	WindowUrgencyChanged *WindowUrgencyChanged

	// Apply changes to the tile location and/or size of one or more
	// tiles/windows.
	//
	// Note that this does not trigger for a window’s physical location
	// changing.
	WindowLayoutsChanged *WindowLayoutsChanged

	// The configured keyboard layouts have changed.
	KeyboardLayoutsChanged *KeyboardLayoutsChanged
	// The keyboard layout switched.
	KeyboardLayoutSwitched *KeyboardLayoutSwitched
	// The overview was opened or closed.
	OverviewOpenedOrClosed *OverviewOpenedOrClosed
	// The configuration was reloaded.
	//
	// You will always receive this event when connecting to the event stream, indicating the last config load attempt.
	ConfigLoaded *ConfigLoaded
	// A screenshot was captured.
	ScreenshotCaptured *ScreenshotCaptured
}

// The workspace configuration has changed.
type WorkspacesChanged struct {
	// The new workspace configuration.
	//
	// This configuration completely replaces the previous configuration. i.e.
	// if any workspaces are missing from here, then they were deleted.
	Workspaces []*Workspace `json:"workspaces"`
}

// The workspace urgency changed.
type WorkspaceUrgencyChanged struct {
	// Id of the workspace.
	Id uint64 `json:"id"`
	// Whether this workspace has an urgent window.
	Urgent bool `json:"urgent"`
}

// A workspace was activated on an output.
//
// This doesn't always mean the workspace became focused, just that it's now the
// active workspace on its output. All other workspaces on the same output
// become inactive.
type WorkspaceActivated struct {
	// Id of the newly active workspace.
	Id uint64 `json:"id"`
	// Whether this workspace also became focused.
	//
	// If true, this is now the single focused workspace. All other workspaces
	// are no longer focused, but they may remain active on their respective
	// outputs.
	Focused bool `json:"focused"`
}

// An active window changed on a workspace.
type WorkspaceActiveWindowChanged struct {
	// Id of the workspace on which the active window changed.
	WorkspaceId uint64 `json:"workspace_id"`

	// Id of the new active window, if any.
	ActiveWindowId *uint64 `json:"active_window_id"`
}

// The window configuration has changed.
type WindowsChanged struct {
	// The new window configuration.
	//
	// This configuration completely replaces the previous configuration. i.e.
	// if any windows are missing from here, then they were closed.
	Windows []Window `json:"windows"`
}

// A new toplevel window was opened, or an existing toplevel window changed.
type WindowOpenedOrChanged struct {
	// The new or updated window.
	//
	// If the window is focused, all other windows are no longer focused.
	Window Window `json:"window"`
}

// A toplevel window was closed.
type WindowClosed struct {
	// Id of the removed window.
	Id uint64 `json:"id"`
}

// Window focus changed.
//
// All other windows are no longer focused.
type WindowFocusChanged struct {
	// Id of the newly focused window, or None if no window is now focused.
	Id *uint64 `json:"id"`
}

// Apply changes to the tile location and/or size of one or more
// tiles/windows.
//
// Note that this does not trigger for a window’s physical location
// changing.
type WindowLayoutsChanged struct {
	// Pairs consisting of a window id and new position/size information for the window.
	Changes []WindowLayoutChange `json:"changes"`
}

// A WindowLayoutChange is a pair consisting of a window id and new position/size information for the window.
// It marshals to a 2-element JSON array.
type WindowLayoutChange struct {
	Id           uint64
	WindowLayout WindowLayout
}

func (w *WindowLayoutChange) UnmarshalJSON(data []byte) error {
	var arr []json.RawMessage
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) != 2 {
		return fmt.Errorf("expected array of length 2, got %d", len(arr))
	}
	if err := json.Unmarshal(arr[0], &w.Id); err != nil {
		return fmt.Errorf("failed to unmarshal window id: %w", err)
	}
	if err := json.Unmarshal(arr[1], &w.WindowLayout); err != nil {
		return fmt.Errorf("failed to unmarshal window layout: %w", err)
	}
	return nil
}

func (w *WindowLayoutChange) MarshalJSON() ([]byte, error) {
	return json.Marshal([]any{w.Id, w.WindowLayout})
}

// Window urgency changed.
type WindowUrgencyChanged struct {
	// Id of the window.
	Id uint64 `json:"id"`
	// The new urgency state of the window.
	Urgent bool `json:"urgent"`
}

// The configured keyboard layouts have changed.
type KeyboardLayoutsChanged struct {
	// The new keyboard layout configuration.
	KeyboardLayouts *KeyboardLayouts `json:"keyboard_layouts"`
}

// The keyboard layout switched.
type KeyboardLayoutSwitched struct {
	// Index of the newly active layout.
	Idx uint8 `json:"idx"`
}

// The overview was opened or closed.
type OverviewOpenedOrClosed struct {
	// The new state of the overview.
	IsOpen bool `json:"is_open"`
}

// The configuration was reloaded.
//
// You will always receive this event when connecting to the event stream, indicating the last config load attempt.
type ConfigLoaded struct {
	// Whether the loading failed.
	//
	// For example, the config file couldn't be parsed.
	Failed bool `json:"failed"`
}

// A screenshot was captured.
type ScreenshotCaptured struct {
	// The file path where the screenshot was saved, if it was written to disk.
	//
	// If None, the screenshot was either only copied to the clipboard, or the path couldn't be converted to a String (e.g. contained invalid UTF-8 bytes).
	Path *string `json:"path"`
}

func (*WorkspacesChanged) Name() string            { return "WorkspacesChanged" }
func (*WorkspaceUrgencyChanged) Name() string      { return "WorkspaceUrgencyChanged" }
func (*WorkspaceActivated) Name() string           { return "WorkspaceActivated" }
func (*WorkspaceActiveWindowChanged) Name() string { return "WorkspaceActiveWindowChanged" }
func (*WindowsChanged) Name() string               { return "WindowsChanged" }
func (*WindowOpenedOrChanged) Name() string        { return "WindowOpenedOrChanged" }
func (*WindowClosed) Name() string                 { return "WindowClosed" }
func (*WindowFocusChanged) Name() string           { return "WindowFocusChanged" }
func (*WindowLayoutsChanged) Name() string         { return "WindowLayoutsChanged" }
func (*WindowUrgencyChanged) Name() string         { return "WindowUrgencyChanged" }
func (*KeyboardLayoutsChanged) Name() string       { return "KeyboardLayoutsChanged" }
func (*KeyboardLayoutSwitched) Name() string       { return "KeyboardLayoutSwitched" }
func (*OverviewOpenedOrClosed) Name() string       { return "OverviewOpenedOrClosed" }
func (*ConfigLoaded) Name() string                 { return "ConfigLoaded" }
func (*ScreenshotCaptured) Name() string           { return "ScreenshotCaptured" }
