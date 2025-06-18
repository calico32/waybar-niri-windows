package main

import (
	"encoding/json"
	"fmt"
)

// Toplevel window.
type Window struct {
	// Id of the window.
	Id uint64 `json:"id"`
	// Title, if set.
	Title *string `json:"title"`
	// Application ID, if set.
	AppId *string `json:"app_id"`
	// Process ID that created the Wayland connection for this window, if known.
	//
	// Currently, windows created by xdg-desktop-portal-gnome will have a None
	// PID, but this may change in the future.
	Pid *int32 `json:"pid"`
	// Id of the workspace this window is on, if any.
	WorkspaceId *uint64 `json:"workspace_id"`
	// Whether this window is currently focused.
	//
	// There can be either one focused window or zero (e.g. when a layer-shell
	// surface has focus).
	IsFocused bool `json:"is_focused"`
	// Whether this window is currently floating.
	//
	// If the window isn’t floating then it is in the tiling layout.
	IsFloating bool `json:"is_floating"`
	// Whether this window requests your attention.
	IsUrgent bool `json:"is_urgent"`
	// Position and size related properties of the Window.
	Location WindowLayout `json:"location"`
}

// Position and size related properties of a Window.
type WindowLayout struct {
	// Location of the window within a workspace in terms of (column index, tile
	// index in column).
	//
	// Unset for floating windows.
	TilePosInScrollingLayout *Vec2[uint32] `json:"tile_pos_in_scrolling_layout"`
	// Size of the tile this window is in.
	TileSize Vec2[float64] `json:"tile_size"`
	// Size of the window itself.
	WindowSize Vec2[int32] `json:"window_size"`
	// set for floating windows (“workspace view” is also used for gradients
	// relative-to in the config)
	TilePosInWorkspaceView *Vec2[float64] `json:"tile_pos_in_workspace_view"`
	// same but windows themselves
	WindowPosInWorkspaceView *Vec2[float64] `json:"window_pos_in_workspace_view"`
}

// A workspace.
type Workspace struct {
	// Unique id of this workspace.
	//
	// This id remains constant regardless of the workspace moving around and
	// across monitors.
	//
	// Do not assume that workspace ids will always increase without wrapping,
	// or start at 1. That is an implementation detail subject to change. For
	// example, ids may change to be randomly generated for each new workspace.
	Id uint64 `json:"id"`
	// Index of the workspace on its monitor.
	//
	// This is the same index you can use for requests like niri msg action
	// focus-workspace.
	//
	// This index will change as you move and re-order workspace. It is merely
	// the workspace’s current position on its monitor. Workspaces on different
	// monitors can have the same index.
	//
	// If you need a unique workspace id that doesn’t change, see [Workspace.Id].
	Index uint8 `json:"idx"`
	// Optional name of the workspace.
	Name *string `json:"name"`
	// Name of the output that the workspace is on.
	//
	// Can be None if no outputs are currently connected.
	Output *string `json:"output"`
	// Whether the workspace currently has an urgent window in its output.
	IsUrgent bool `json:"is_urgent"`
	// Whether the workspace is currently active on its output.
	//
	// Every output has one active workspace, the one that is currently visible
	// on that output.
	IsActive bool `json:"is_active"`
	// Whether the workspace is currently focused.
	//
	// There’s only one focused workspace across all outputs.
	IsFocused bool `json:"is_focused"`
	// Id of the active window on this workspace, if any.
	ActiveWindowId *uint64 `json:"active_window_id"`
}

// Configured keyboard layouts.
type KeyboardLayouts struct {
	// XKB names of the configured layouts.
	Names []string `json:"names"`
	// Index of the currently active layout in names.
	CurrentIdx uint8 `json:"current_idx"`
}

// Numeric is a type constraint for numeric types.
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64 |
		~complex64 | ~complex128
}

// Vec2 is a 2D vector with generic types for its components. It c.an be
// marshaled to JSON as a 2-element array.
type Vec2[T Numeric] struct {
	// X component of the vector.
	X T
	// Y component of the vector.
	Y T
}

// MarshalJSON implements [json.Marshaler] for Vec2.
func (v *Vec2[T]) MarshalJSON() ([]byte, error) {
	return json.Marshal([]T{v.X, v.Y})
}

// UnmarshalJSON implements [json.Unmarshaler] for Vec2.
func (v *Vec2[T]) UnmarshalJSON(data []byte) error {
	var arr []T
	if err := json.Unmarshal(data, &arr); err != nil {
		return err
	}
	if len(arr) != 2 {
		return fmt.Errorf("expected array of length 2, got %d", len(arr))
	}
	v.X = arr[0]
	v.Y = arr[1]
	return nil
}

// First is an alias for [Vec2.X].
func (v *Vec2[T]) First() T { return v.X }

// Second is an alias for [Vec2.Y].
func (v *Vec2[T]) Second() T { return v.Y }

// Left is an alias for [Vec2.X].
func (v *Vec2[T]) Left() T { return v.X }

// Right is an alias for [Vec2.Y].
func (v *Vec2[T]) Right() T { return v.Y }
