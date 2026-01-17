package module

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"wnw/jsonc"
	"wnw/log"
	"wnw/niri"

	"github.com/gotk3/gotk3/gdk"
	"github.com/gotk3/gotk3/gtk"
)

type Instance struct {
	mu              sync.RWMutex
	id              uintptr
	queueUpdate     func()
	box             *gtk.Box
	label           *gtk.Label // only set in text mode
	floatingView    *gtk.Box
	floatingFixed   *gtk.Fixed
	monitor         string
	ready           bool
	niriState       *niri.State
	niriSocket      niri.Socket
	screenHeight    int
	screenWidth     int
	allocatedHeight int
	config          Config
}

func (i *Instance) Id() uintptr {
	// we never change the id, so we can just return it
	return i.id
}

// assumes maximum "normal" window height is 90% of screen height
// TODO: compute real value somehow? tile position isn't known for tiled windows;
// see https://github.com/YaLTeR/niri/issues/2381
const screenHeightScale = 0.90

const floatingViewName = "floating"

func New(niriState *niri.State, niriSocket niri.Socket, queueUpdate func()) *Instance {
	return &Instance{
		id:          uintptr(rand.Uint64()),
		queueUpdate: queueUpdate,
		niriState:   niriState,
		niriSocket:  niriSocket,
		config: Config{
			Mode:              GraphicalMode,
			ShowFloating:      ShowFloatingAuto,
			FloatingPosition:  FloatingPositionRight,
			MinimumSize:       1,
			Spacing:           1,
			ColumnBorders:     0,
			FloatingBorders:   0,
			OnTileClick:       "FocusWindow",
			OnTileMiddleClick: "CloseWindow",
			OnTileRightClick:  "",
			Symbols: niri.Symbols{
				Unfocused:         "⋅",
				Focused:           "⊙",
				UnfocusedFloating: "∗",
				FocusedFloating:   "⊛",
			},
			WindowRules: []WindowRule{},
		},
	}
}

const defaultStylesheet = `
.cffi-niri-windows .tile {
	background-color: rgba(255, 255, 255, 0.250);
	transition: background-color 75ms ease-in-out;
}

.cffi-niri-windows .tile:hover {
	background-color: rgba(255, 255, 255, 0.375);
}

.cffi-niri-windows .tile:active {
	background-color: rgba(255, 255, 255, 0.600);
}

.cffi-niri-windows .tile.urgent {
	background-color: rgba(251, 44, 54, 0.5);
	border: 1px solid rgba(251, 44, 54, 0.8);
}

.cffi-niri-windows .tile.urgent:hover {
	background-color: rgba(251, 44, 54, 0.625);
}
`

func (i *Instance) Preinit(root *gtk.Container) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	root.SetProperty("name", strconv.FormatUint(uint64(i.id), 16))
	style, err := root.GetStyleContext()
	if err != nil {
		return fmt.Errorf("error getting style context: %s", err)
	}
	style.AddClass("cffi-niri-windows")

	cssProvider, _ := gtk.CssProviderNew()
	err = cssProvider.LoadFromData(defaultStylesheet)
	if err != nil {
		return fmt.Errorf("error loading default stylesheet: %w", err)
	}
	screen, _ := root.GetScreen()
	gtk.AddProviderForScreen(screen, cssProvider, gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)

	box, err := gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, i.config.Spacing)
	if err != nil {
		return fmt.Errorf("error creating box: %w", err)
	}
	root.Add(box)
	i.box = box

	return nil
}

func (i *Instance) ApplyConfig(key, value string) error {
	i.mu.Lock()
	defer i.mu.Unlock()

	switch key {
	case "config", "options":
		sanitized, err := jsonc.Sanitize([]byte(value))
		if err != nil {
			return fmt.Errorf("error unmarshaling config: %w", err)
		}
		err = json.Unmarshal(sanitized, &i.config)
		if err != nil {
			return fmt.Errorf("error unmarshaling config: %w", err)
		}
		if i.config.MinimumSize < 1 {
			log.Warnf("minimum-size must be at least 1, setting to 1")
			i.config.MinimumSize = 1
		}
		if i.config.Spacing < 0 {
			log.Warnf("spacing must be at least 0, setting to 0")
			i.config.Spacing = 0
		}
		log.Debugf("config: %#+v", i.config)
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
	i.box.SetSpacing(i.config.Spacing)

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
	i.mu.Lock()
	defer i.mu.Unlock()

	if !i.ready {
		return
	}

	if i.config.Mode == TextMode {
		text := i.niriState.Text(i.monitor, i.config.Symbols)

		if text == "" {
			if i.label != nil {
				i.label.Destroy()
				i.label = nil
			}
			return
		}
		if i.label == nil {
			var err error
			i.label, err = gtk.LabelNew("")
			if err != nil {
				log.Errorf("error creating label: %s", err)
				return
			}
			i.box.Add(i.label)
			i.label.Show()
		}
		i.label.SetText(text)
		return
	}

	tiled, floating := i.niriState.Windows(i.monitor)

	i.box.GetChildren().Foreach(func(child any) {
		w := child.(*gtk.Widget)
		if n, err := w.GetName(); err != nil || n != floatingViewName {
			w.Destroy()
		}
	})

	if i.allocatedHeight == 0 {
		i.allocatedHeight = i.box.GetAllocatedHeight()
	}

	maxHeight := i.allocatedHeight
	scale := float64(maxHeight) / float64(i.screenHeight)
	maxWidth := int(math.Round(float64(i.screenWidth) * scale))

	columns := groupBy(tiled, func(w *niri.Window) uint32 {
		return w.Layout.PosInScrollingLayout.X
	})
	slices.SortFunc(columns, func(a, b []*niri.Window) int {
		return int(a[0].Layout.PosInScrollingLayout.X) - int(b[0].Layout.PosInScrollingLayout.X)
	})

	if i.config.MinimumSize > maxHeight {
		log.Warnf("minimum-size is larger than the bar height (%d), setting to bar height", maxHeight)
		i.config.MinimumSize = maxHeight
	}

	if i.config.FloatingPosition == FloatingPositionLeft {
		i.drawFloating(maxWidth, maxHeight, floating, scale)
	}

	var cols *gtk.Box

	if len(tiled) != 0 {
		cols, _ = gtk.BoxNew(gtk.ORIENTATION_HORIZONTAL, i.config.Spacing)
		i.box.Add(cols)

		for _, column := range columns {
			colBox, _ := gtk.BoxNew(gtk.ORIENTATION_VERTICAL, i.config.Spacing)
			colStyle, _ := colBox.GetStyleContext()
			colStyle.AddClass("column")
			cols.Add(colBox)

			windowHeights, width := i.calculateWindowSizes(column, scale, maxHeight-i.config.ColumnBorders)

			for idx, window := range column {
				if idx > len(windowHeights)-1 {
					// we had to cut this window to fit into the bar, stop here
					break
				}
				height := windowHeights[idx]

				windowBox, _ := gtk.EventBoxNew()
				windowBox.SetSizeRequest(width, height)

				style, _ := windowBox.GetStyleContext()
				style.AddClass("tile")
				if window.IsUrgent && !style.HasClass("urgent") {
					style.AddClass("urgent")
				} else if !window.IsUrgent && style.HasClass("urgent") {
					style.RemoveClass("urgent")
				}
				if window.IsFocused {
					windowBox.SetStateFlags(gtk.STATE_FLAG_ACTIVE, false)
					colBox.SetStateFlags(gtk.STATE_FLAG_ACTIVE, false)
				}

				i.connectRealize(windowBox)
				i.connectButtonPress(windowBox, window)
				i.connectTooltip(windowBox, window)
				i.connectHover(windowBox)
				i.applyWindowRules(windowBox, window)

				colBox.Add(windowBox)
			}

		}
	}

	if i.config.FloatingPosition == FloatingPositionRight {
		i.drawFloating(maxWidth, maxHeight, floating, scale)
		if cols != nil {
			i.box.ReorderChild(cols, 0)
		}
	}

	i.box.ShowAll()
}

func (i *Instance) shouldShowFloating(floating []*niri.Window) bool {
	return i.config.ShowFloating == ShowFloatingAlways || (i.config.ShowFloating == ShowFloatingAuto && len(floating) > 0)
}

func (i *Instance) drawFloating(maxWidth int, maxHeight int, floating []*niri.Window, scale float64) {
	if !i.shouldShowFloating(floating) {
		if i.floatingView != nil {
			i.floatingView.Destroy()
			i.floatingView = nil
		}
		return
	}

	maxHeight -= i.config.FloatingBorders
	maxWidth -= i.config.FloatingBorders

	if i.floatingView == nil {
		i.floatingView, _ = gtk.BoxNew(gtk.ORIENTATION_VERTICAL, 1)
		i.floatingView.SetName(floatingViewName)
		i.floatingView.SetSizeRequest(maxWidth, maxHeight)
		i.box.Add(i.floatingView)

		style, _ := i.floatingView.GetStyleContext()
		style.AddClass("floating")

		i.floatingFixed, _ = gtk.FixedNew()
		i.floatingFixed.SetSizeRequest(maxWidth, maxHeight)
		i.floatingView.Add(i.floatingFixed)
	}

	hasFocused := false

	existingWindows := make(map[string]struct{})
	i.floatingFixed.GetChildren().Foreach(func(item any) {
		windowBox := item.(*gtk.Widget)
		id, _ := windowBox.GetName()
		var window *niri.Window
		for _, w := range floating {
			if id == strconv.FormatUint(w.Id, 10) {
				existingWindows[id] = struct{}{}
				window = w
				break
			}
		}

		if window == nil {
			windowBox.Destroy()
			return
		}

		x, y, w, h := i.getFloatingLayout(window, scale, maxWidth, maxHeight)
		i.floatingFixed.Move(windowBox, x, y)
		windowBox.SetSizeRequest(w, h)

		style, _ := windowBox.GetStyleContext()
		if window.IsUrgent && !style.HasClass("urgent") {
			style.AddClass("urgent")
		} else if !window.IsUrgent && style.HasClass("urgent") {
			style.RemoveClass("urgent")
		}

		i.applyWindowRules(windowBox, window)
		if window.IsFocused {
			windowBox.SetStateFlags(gtk.STATE_FLAG_ACTIVE, false)
			hasFocused = true
		} else {
			windowBox.UnsetStateFlags(gtk.STATE_FLAG_ACTIVE)
		}
	})

	for _, window := range floating {
		id := strconv.FormatUint(window.Id, 10)
		if _, ok := existingWindows[id]; ok {
			continue
		}

		windowBox, _ := gtk.EventBoxNew()
		windowBox.SetName(id)

		style, _ := windowBox.GetStyleContext()
		style.AddClass("tile")
		if window.IsUrgent {
			style.AddClass("urgent")
		}

		x, y, w, h := i.getFloatingLayout(window, scale, maxWidth, maxHeight)
		i.floatingFixed.Put(windowBox, x, y)
		windowBox.SetSizeRequest(w, h)
		if window.IsFocused {
			windowBox.SetStateFlags(gtk.STATE_FLAG_ACTIVE, false)
			hasFocused = true
		}

		i.connectRealize(windowBox)
		i.connectButtonPress(windowBox, window)
		i.connectTooltip(windowBox, window)
		i.connectHover(windowBox)
		i.applyWindowRules(windowBox, window)
	}

	if hasFocused {
		i.floatingView.SetStateFlags(gtk.STATE_FLAG_ACTIVE, false)
	} else {
		i.floatingView.UnsetStateFlags(gtk.STATE_FLAG_ACTIVE)
	}

	i.floatingView.ShowAll()
}

func (i *Instance) getFloatingLayout(window *niri.Window, scale float64, maxWidth int, maxHeight int) (x int, y int, w int, h int) {
	x = int(window.Layout.TilePosInWorkspaceView.X * scale)
	y = int(window.Layout.TilePosInWorkspaceView.Y * scale)

	maxW := min(maxWidth-x, maxWidth)
	maxH := min(maxHeight-y, maxHeight)

	w = max(i.config.MinimumSize, min(maxW, int(window.Layout.TileSize.X*scale)))
	h = max(i.config.MinimumSize, min(maxH, int(window.Layout.TileSize.Y*scale)))

	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}

	if x+w > maxWidth {
		w = maxWidth - x
	}
	if y+h > maxHeight {
		h = maxHeight - y
	}

	return x, y, w, h
}

func (i *Instance) applyWindowRules(windowBox gtk.IWidget, window *niri.Window) {
	style, _ := windowBox.ToWidget().GetStyleContext()
	for _, rule := range i.config.WindowRules {
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

			_, h := windowBox.ToWidget().GetSizeRequest()
			if rule.Icon != "" && h > i.allocatedHeight/2 {
				lab, err := gtk.LabelNew(rule.Icon)
				if err != nil {
					log.Errorf("error creating label: %s", err)
					return
				}
				box, ok := windowBox.(*gtk.EventBox)
				if ok {
					box.Add(lab)
				}
			}
			if !rule.Continue {
				break
			}
		} else if style.HasClass(rule.Class) {
			style.RemoveClass(rule.Class)
		}
	}
}

func (*Instance) connectRealize(windowBox gtk.IWidget) {
	windowBox.ToWidget().Connect("realize", func(obj gtk.IWidget) {
		gdkWindow, _ := windowBox.ToWidget().GetWindow()
		display, _ := windowBox.ToWidget().GetDisplay()
		pointer, _ := gdk.CursorNewFromName(display, "pointer")
		gdkWindow.SetCursor(pointer)
	})
}

func (*Instance) connectTooltip(windowBox gtk.IWidget, window *niri.Window) {
	windowBox.ToWidget().SetProperty("has-tooltip", true)
	windowBox.ToWidget().Connect("query-tooltip", func(obj gtk.IWidget, x, y int, keyboardTip bool, tooltip *gtk.Tooltip) bool {
		if window.Title != nil {
			tooltip.SetText(*window.Title)
			return true
		}

		if window.AppId != nil {
			tooltip.SetText(*window.AppId)
			return true
		}

		return false
	})
}

func (*Instance) connectHover(windowBox gtk.IWidget) {
	windowBox.ToWidget().AddEvents(int(gdk.POINTER_MOTION_MASK | gdk.ENTER_NOTIFY_MASK | gdk.LEAVE_NOTIFY_MASK))

	windowBox.ToWidget().Connect("enter-notify-event", func(obj gtk.IWidget, event *gdk.Event) {
		windowBox.ToWidget().SetStateFlags(gtk.STATE_FLAG_PRELIGHT, false)
	})
	windowBox.ToWidget().Connect("leave-notify-event", func(obj gtk.IWidget, event *gdk.Event) {
		windowBox.ToWidget().UnsetStateFlags(gtk.STATE_FLAG_PRELIGHT)
	})
}

func (i *Instance) connectButtonPress(windowBox gtk.IWidget, window *niri.Window) {
	windowBox.ToWidget().AddEvents(int(gdk.BUTTON_PRESS_MASK))

	windowBox.ToWidget().Connect("button-press-event", func(obj gtk.IWidget, event *gdk.Event) {
		eventButton := gdk.EventButtonNewFromEvent(event)
		var request map[string]any
		switch eventButton.Button() {
		case gdk.BUTTON_PRIMARY:
			if i.config.OnTileClick != "" {
				request = map[string]any{
					"Action": map[string]any{
						i.config.OnTileClick: map[string]any{"id": window.Id},
					},
				}
			}
		case gdk.BUTTON_MIDDLE:
			if i.config.OnTileMiddleClick != "" {
				request = map[string]any{
					"Action": map[string]any{
						i.config.OnTileMiddleClick: map[string]any{"id": window.Id},
					},
				}
			}
		case gdk.BUTTON_SECONDARY:
			if i.config.OnTileRightClick != "" {
				request = map[string]any{
					"Action": map[string]any{
						i.config.OnTileRightClick: map[string]any{"id": window.Id},
					},
				}
			}
		}
		if request == nil {
			return
		}

		err := i.niriSocket.Request(request)
		if err != nil {
			log.Errorf("error sending action: %s", err)
		}
	})
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
	maxHeight = maxHeight - (len(column)-1)*i.config.Spacing // remove spacing between each window
	for _, window := range column {
		height := max(
			int(math.Round(float64(maxHeight)*(window.Layout.TileSize.Y/totalTileHeight))),
			i.config.MinimumSize,
		)
		totalWindowHeight += height
		windowHeights = append(windowHeights, height)
	}
	remainingHeight := maxHeight - totalWindowHeight
	idx := 0
	iterations := 0
outer:
	for remainingHeight < 0 {
		inner := 0
		for remainingHeight < 0 {
			if windowHeights[idx] > i.config.MinimumSize {
				windowHeights[idx]--
				remainingHeight++
			} else {
				inner++
			}
			if inner > 50 && i.config.Spacing > 1 {
				// try decreasing spacing
				log.Warnf("bar too small, decreasing spacing to fit all windows; decrease minimum-size or spacing to fit more")
				i.config.Spacing--
			} else if inner > 50 && i.config.MinimumSize > 1 {
				// try decreasing minimum-size
				log.Warnf("bar too small, decreasing minimum-size to fit all windows; decrease minimum-size or spacing to fit more")
				i.config.MinimumSize--
			} else if inner > 100 {
				if len(windowHeights) == 1 {
					log.Errorf("bar too small, giving up on fitting windows")
					break outer
				}
				// bar must be too small to fit all windows, we'll try removing one.
				log.Warnf("bar too small, dropping window from display (column has %d windows); decrease minimum-size or spacing to fit more", len(windowHeights))
				h := windowHeights[len(windowHeights)-1]
				windowHeights = windowHeights[:len(windowHeights)-1]
				remainingHeight += i.config.Spacing // account for removed gap
				remainingHeight += h                // account for removed window
				// reset idx if needed
				if idx >= len(windowHeights) {
					idx = 0
				}
				break
			}
			idx++
			if idx >= len(windowHeights) {
				idx = 0
			}
		}

		iterations++
		if iterations > 100 {
			log.Errorf("bar too small, giving up on fitting windows")
			break
		}
	}
	for remainingHeight > 0 {
		windowHeights[idx]++
		remainingHeight--
		idx++
		if idx >= len(windowHeights) {
			idx = 0
		}
	}

	return windowHeights, width
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
