package module

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"net"
	"regexp"
	"slices"
	"strconv"
	"wnw/niri"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type Instance struct {
	Id           uintptr
	queueUpdate  func()
	box          *gtk.Box
	Monitor      string
	Ready        bool
	niriState    *niri.State
	niriSocket   net.Conn
	symbols      niri.Symbols
	ScreenHeight int
	ScreenWidth  int
	WindowRules  []WindowRule
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

func New(niriState *niri.State, niriSocket net.Conn, queueUpdate func()) *Instance {
	var id uintptr
	for id == 0 {
		// for the very slim chance that id is null, generate a new one
		id = uintptr(rand.Uint64())
	}

	i := &Instance{
		Id:          id,
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

	return i
}

func (i *Instance) Preinit(root *gtk.Container) error {
	root.SetProperty("name", strconv.FormatUint(uint64(i.Id), 16))
	style, err := root.GetStyleContext()
	if err != nil {
		return fmt.Errorf("wbcffi: error getting style context: %s", err)
	}
	style.AddClass("cffi-niri-windows")

	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, 1)
	if err != nil {
		return fmt.Errorf("wbcffi: error creating box: %s", err)
	}
	root.Add(box)
	i.box = box

	return nil
}

func (i *Instance) ApplyConfig(key, value string) error {
	switch key {
	case "rules":
		var rules []WindowRuleConfig
		err := json.Unmarshal([]byte(value), &rules)
		if err != nil {
			return fmt.Errorf("wbcffi: error unmarshaling rules: %w", err)
		}
		i.WindowRules = make([]WindowRule, len(rules))
		for idx, rule := range rules {
			i.WindowRules[idx].AppId, err = regexp.Compile(rule.AppId)
			if err != nil {
				return fmt.Errorf("wbcffi: error compiling regex: %w", err)
			}
			i.WindowRules[idx].Title, err = regexp.Compile(rule.Title)
			if err != nil {
				return fmt.Errorf("wbcffi: error compiling regex: %w", err)
			}
			i.WindowRules[idx].Class = rule.Class
			i.WindowRules[idx].Continue = rule.Continue
		}
	case "module_path", "actions":
		// ignore
	default:
		return fmt.Errorf("wbcffi: unknown config key: %s", key)
	}
	return nil
}

func (i *Instance) Init() {
	if !i.Ready {
		return
	}

	i.Notify()
	i.niriState.OnUpdate(uint64(i.Id), func(state *niri.State) { i.Notify() })
}

func (i *Instance) Deinit() {
	i.niriState.RemoveOnUpdate(uint64(i.Id))
}

func (i *Instance) Notify() {
	if !i.Ready {
		return
	}
	i.queueUpdate()
}

func (i *Instance) Update() {
	if !i.Ready {
		return
	}

	i.box.GetChildren().Foreach(func(child any) {
		child.(*gtk.Widget).Destroy()
	})

	tiled, floating := i.niriState.Windows(i.Monitor)
	if len(tiled) == 0 && len(floating) == 0 {
		return
	}

	maxTotalWindowHeight := i.box.GetAllocatedHeight()
	scale := float64(maxTotalWindowHeight) / float64(i.ScreenHeight)

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

		var width int
		var windowHeights []int
		var totalTileHeight float64
		for _, window := range column {
			width = int(window.Layout.TileSize.X * scale)
			totalTileHeight += window.Layout.TileSize.Y
		}
		if len(column) == 1 {
			screenHeight := float64(i.ScreenHeight) * screenHeightScale
			height := min(
				int(math.Round(float64(column[0].Layout.TileSize.Y)/screenHeight*float64(maxTotalWindowHeight))),
				maxTotalWindowHeight,
			)
			windowHeights = append(windowHeights, int(height))
		} else {
			totalWindowHeight := 0
			maxTotalWindowHeight = maxTotalWindowHeight - (len(column) - 1) // remove 1 pixel between each window
			for _, window := range column {
				height := max(
					int(math.Round(float64(maxTotalWindowHeight)*(window.Layout.TileSize.Y/totalTileHeight))),
					1, // minimum height enforced by GTK is 1
				)
				totalWindowHeight += height
				windowHeights = append(windowHeights, height)
			}
			leftoverHeight := maxTotalWindowHeight - totalWindowHeight
			idx := 0
			for leftoverHeight > 0 {
				windowHeights[idx]++
				leftoverHeight--
				idx++
				if idx >= len(windowHeights) {
					idx = 0
				}
			}
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
						// bar must be too small to fit all windows, we'll try removing one
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
			if window.AppId != nil {
				for _, rule := range i.WindowRules {
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
			}

			windowBox.Connect("realize", func(obj *gtk.EventBox) {
				gdkWindow, _ := windowBox.GetWindow()
				display, _ := windowBox.GetDisplay()
				pointer, _ := gdk.CursorNewFromName(display, "pointer")
				gdkWindow.SetCursor(pointer)
			})

			windowBox.Connect("button-press-event", func(obj *gtk.EventBox, event *gdk.Event) {
				eventButton := gdk.EventButtonNewFromEvent(event)
				eventButton.Button()
				var request map[string]any
				switch eventButton.Button() {
				case gdk.BUTTON_PRIMARY:
					request = map[string]any{
						"Action": map[string]any{
							"FocusWindow": map[string]any{"id": window.Id},
						},
					}
				// case gdk.BUTTON_SECONDARY:
				// 	request = map[string]any{
				// 		"Action": map[string]any{
				// 			"CloseWindow": map[string]any{"id": window.Id},
				// 		},
				// 	}
				case gdk.BUTTON_MIDDLE:
					request = map[string]any{
						"Action": map[string]any{
							"ToggleOverview": map[string]any{},
						},
					}
				}

				b, _ := json.Marshal(request)
				log.Printf("wbcffi: niri <- %s", b)
				i.niriSocket.Write(b)
				i.niriSocket.Write([]byte("\n"))
			})

			colBox.Add(windowBox)
		}
	}

	i.box.ShowAll()
}

func (i *Instance) Refresh(signal int) {}

func (i *Instance) DoAction(actionName string) {
	request := map[string]any{
		"Action": map[string]any{
			actionName: map[string]any{},
		},
	}
	b, err := json.Marshal(request)
	if err != nil {
		log.Printf("wbcffi: error marshaling request: %s", err)
		return
	}
	log.Printf("wbcffi: niri <- %s", b)
	i.niriSocket.Write(b)
	i.niriSocket.Write([]byte("\n"))
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
