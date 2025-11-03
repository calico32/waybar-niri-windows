package module

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
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

type WindowRule struct {
	AppId string `json:"app-id"`
	Class string `json:"class"`
}

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

func (i *Instance) ApplyConfig(key, value string) {
	switch key {
	case "rules":
		err := json.Unmarshal([]byte(value), &i.WindowRules)
		if err != nil {
			log.Printf("wbcffi: error unmarshaling rules: %s", err)
			return
		}
	}
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

	maxHeight := i.box.GetAllocatedHeight()
	scale := float64(maxHeight) / float64(i.ScreenHeight)

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
		var totalHeight float64
		for _, window := range column {
			width = int(window.Layout.TileSize.X * scale)
			totalHeight += window.Layout.TileSize.Y
		}
		var windowBoxHeights []int
		for _, window := range column {
			height := int(float64(maxHeight-len(column)-1) * (window.Layout.TileSize.Y / totalHeight))
			windowBoxHeights = append(windowBoxHeights, height)
		}
		totalBoxHeight := 0
		for _, height := range windowBoxHeights {
			totalBoxHeight += height
		}
		leftoverHeight := maxHeight - len(column) - 1 - totalBoxHeight
		idx := 0
		for leftoverHeight > 0 {
			windowBoxHeights[idx]++
			leftoverHeight--
			idx++
			if idx >= len(windowBoxHeights) {
				idx = 0
			}
		}

		for idx, window := range column {
			height := windowBoxHeights[idx]

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
					if *window.AppId == rule.AppId {
						style.AddClass(rule.Class)
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
