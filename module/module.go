package module

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"regexp"
	"slices"
	"strconv"
	"sync"
	"wnw/log"
	"wnw/niri"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type Instance struct {
	mu           sync.RWMutex
	id           uintptr
	queueUpdate  func()
	box          *gtk.Box
	monitor      string
	ready        bool
	niriState    *niri.State
	niriSocket   niri.Socket
	symbols      niri.Symbols
	screenHeight int
	screenWidth  int
	windowRules  []WindowRule
}

func (i *Instance) Id() uintptr {
	// we never change the id, so we can just return it
	return i.id
}

type WindowRuleConfig struct {
	AppId    string `json:"app-id"`
	Title    string `json:"title"`
	Class    string `json:"class"`
	Continue bool   `json:"continue"`
}

type WindowRule struct {
	AppId    *regexp.Regexp
	Title    *regexp.Regexp
	Class    string
	Continue bool
}

// assumes maximum "normal" window height is 95% of screen height
// TODO: compute real value somehow? tile position isn't known for tiled windows;
// see https://github.com/YaLTeR/niri/issues/2381
const screenHeightScale = 0.95

func New(niriState *niri.State, niriSocket niri.Socket, queueUpdate func()) *Instance {
	return &Instance{
		id:          uintptr(rand.Uint64()),
		queueUpdate: queueUpdate,
		niriState:   niriState,
		niriSocket:  niriSocket,
		symbols: niri.Symbols{
			Unfocused:         "⋅",
			Focused:           "⊙",
			UnfocusedFloating: "∗",
			FocusedFloating:   "⊛",
		},
	}
}

func (i *Instance) Preinit(root *gtk.Container) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	root.SetProperty("name", strconv.FormatUint(uint64(i.id), 16))
	style, err := root.GetStyleContext()
	if err != nil {
		return fmt.Errorf("error getting style context: %s", err)
	}
	style.AddClass("cffi-niri-windows")

	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 1)
	if err != nil {
		return fmt.Errorf("error creating box: %s", err)
	}
	root.Add(box)
	i.box = box

	return nil
}

func (i *Instance) ApplyConfig(key, value string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	switch key {
	case "rules":
		var rules []WindowRuleConfig
		err := json.Unmarshal([]byte(value), &rules)
		if err != nil {
			return fmt.Errorf("error unmarshaling rules: %w", err)
		}
		i.windowRules = make([]WindowRule, len(rules))
		for idx, rule := range rules {
			if rule.AppId != "" {
				i.windowRules[idx].AppId, err = regexp.Compile(rule.AppId)
				if err != nil {
					return fmt.Errorf("error compiling regex: %w", err)
				}
			}
			if rule.Title != "" {
				i.windowRules[idx].Title, err = regexp.Compile(rule.Title)
				if err != nil {
					return fmt.Errorf("error compiling regex: %w", err)
				}
			}
			i.windowRules[idx].Class = rule.Class
			i.windowRules[idx].Continue = rule.Continue
		}
	case "module_path", "actions":
		// ignore
	default:
		return fmt.Errorf("unknown config key: %s", key)
	}
	return nil
}

func (i *Instance) Init(monitor string, screenWidth, screenHeight int) {
	i.mu.Lock()
	i.monitor = monitor
	i.screenWidth = screenWidth
	i.screenHeight = screenHeight
	i.ready = true
	i.mu.Unlock()

	i.Notify()
	i.niriState.OnUpdate(uint64(i.id), func(state *niri.State) { i.Notify() })
}

func (i *Instance) Deinit() {
	i.mu.Lock()
	defer i.mu.Unlock()

	i.niriState.RemoveOnUpdate(uint64(i.id))
	i.ready = false
}

func (i *Instance) Notify() {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if !i.ready {
		return
	}
	i.queueUpdate()
}

func (i *Instance) Update() {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if !i.ready {
		return
	}

	i.box.GetChildren().Foreach(func(child any) {
		child.(*gtk.Widget).Destroy()
	})

	tiled, floating := i.niriState.Windows(i.monitor)
	if len(tiled) == 0 && len(floating) == 0 {
		return
	}

	maxHeight := i.box.GetAllocatedHeight()
	scale := float64(maxHeight) / float64(i.screenHeight)

	columns := groupBy(tiled, func(w *niri.Window) uint32 {
		return w.Layout.PosInScrollingLayout.X
	})
	slices.SortFunc(columns, func(a, b []*niri.Window) int {
		return int(a[0].Layout.PosInScrollingLayout.X) - int(b[0].Layout.PosInScrollingLayout.X)
	})

	for _, column := range columns {
		colBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 1)
		colStyle, _ := colBox.GetStyleContext()
		colStyle.AddClass("column")
		i.box.Add(colBox)

		windowHeights, width := i.calculateWindowSizes(column, scale, maxHeight)

		for idx, window := range column {
			if idx > len(windowHeights)-1 {
				// we had to cut this window to fit into the bar, stop here
				break
			}
			height := windowHeights[idx]

			windowBox, _ := gtk.EventBoxNew()
			style, _ := windowBox.GetStyleContext()
			style.AddClass("tile")
			if window.IsFocused {
				style.AddClass("focused")
				colStyle.AddClass("focused")
			}
			windowBox.SetSizeRequest(width, height)

			if window.Title != nil {
				windowBox.SetTooltipText(*window.Title)
			} else if window.AppId != nil {
				windowBox.SetTooltipText(*window.AppId)
			}

			for _, rule := range i.windowRules {
				appIdMatched := rule.AppId == nil
				titleMatched := rule.Title == nil
				if rule.AppId != nil && window.AppId != nil && rule.AppId.MatchString(*window.AppId) {
					appIdMatched = true
				}
				if rule.Title != nil && window.Title != nil && rule.Title.MatchString(*window.Title) {
					titleMatched = true
				}
				if appIdMatched && titleMatched {
					style.AddClass(rule.Class)
					if !rule.Continue {
						break
					}
				}
			}

			windowBox.Connect("realize", func(obj *gtk.EventBox) {
				gdkWindow, _ := windowBox.GetWindow()
				display, _ := windowBox.GetDisplay()
				pointer, _ := gdk.CursorNewFromName(display, "pointer")
				gdkWindow.SetCursor(pointer)
			})

			windowBox.Connect("button-press-event", i.handleButtonPress(window))

			colBox.Add(windowBox)
		}
	}

	i.box.ShowAll()

}

func (i *Instance) calculateWindowSizes(column []*niri.Window, scale float64, maxHeight int) (windowHeights []int, width int) {
	// called when read-lock is held, no need to re-lock

	if len(column) == 1 {
		screenHeight := float64(i.screenHeight) * screenHeightScale
		height := min(
			int(math.Round(float64(column[0].Layout.TileSize.Y)/screenHeight*float64(maxHeight))),
			maxHeight,
		)
		return []int{height}, int(column[0].Layout.TileSize.X * scale)
	}

	var totalTileHeight float64
	for _, window := range column {
		width = int(window.Layout.TileSize.X * scale)
		totalTileHeight += window.Layout.TileSize.Y
	}
	totalWindowHeight := 0
	maxHeight = maxHeight - (len(column) - 1) // remove 1 pixel between each window
	for _, window := range column {
		height := max(
			int(math.Round(float64(maxHeight)*(window.Layout.TileSize.Y/totalTileHeight))),
			1, // minimum height enforced by GTK is 1
		)
		totalWindowHeight += height
		windowHeights = append(windowHeights, height)
	}
	leftoverHeight := maxHeight - totalWindowHeight
	idx := 0
	for leftoverHeight > 0 {
		windowHeights[idx]++
		leftoverHeight--
		for leftoverHeight < 0 {
			iterations := 0
			for leftoverHeight < 0 {
				if windowHeights[idx] > 1 {
					windowHeights[idx]--
					leftoverHeight++
				} else {
					iterations++
				}
				if iterations > 100 {
					// bar must be too small to fit all windows, we'll try removing one.

					// this is an extremely rare case - even a bar 24px tall
					// can accomodate 12 windows in one column, more than
					// anyone would probably ever have.
					log.Warnf("bar too small, dropping window from display (column has %d windows)", len(windowHeights))
					windowHeights = windowHeights[:len(windowHeights)-1]
					leftoverHeight++ // account for removed gap
					break
				}
				idx++
				if idx >= len(windowHeights) {
					idx = 0
				}
			}
		}
	}
	return windowHeights, width
}

func (i *Instance) handleButtonPress(window *niri.Window) func(obj *gtk.EventBox, event *gdk.Event) {
	return func(obj *gtk.EventBox, event *gdk.Event) {
		eventButton := gdk.EventButtonNewFromEvent(event)
		var request map[string]any
		switch eventButton.Button() {
		case gdk.BUTTON_PRIMARY:
			request = map[string]any{
				"Action": map[string]any{
					"FocusWindow": map[string]any{"id": window.Id},
				},
			}
		case gdk.BUTTON_MIDDLE:
			request = map[string]any{
				"Action": map[string]any{
					"CloseWindow": map[string]any{"id": window.Id},
				},
			}
		}
		if request == nil {
			return
		}

		err := i.niriSocket.Request(request)
		if err != nil {
			log.Errorf("error sending action: %s", err)
		}
	}
}

func (i *Instance) Refresh(signal int) {
	// we don't respond to signals
}

func (i *Instance) DoAction(actionName string) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if !i.ready {
		return
	}

	request := map[string]any{
		"Action": map[string]any{
			actionName: map[string]any{},
		},
	}
	err := i.niriSocket.Request(request)
	if err != nil {
		log.Errorf("error sending action: %s", err)
	}
}

func groupBy[T any, K comparable](list []T, key func(T) K) [][]T {
	m := make(map[K][]T)
	for _, item := range list {
		k := key(item)
		m[k] = append(m[k], item)
	}
	var result [][]T
	for _, v := range m {
		result = append(result, v)
	}
	return result
}
