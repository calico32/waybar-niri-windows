package lib

import (
	"fmt"
	"time"
	"unsafe"
	"wnw/lib/state"
	"wnw/log"
	"wnw/module"
	"wnw/niri"

	"github.com/gotk3/gotk3/glib"
	"github.com/gotk3/gotk3/gtk"
)

/*
#cgo CFLAGS: -DGDK_DISABLE_DEPRECATION_WARNINGS
#cgo pkg-config: gtk+-3.0
#include "waybar_cffi_module.h"
#include <stdio.h>
typedef const wbcffi_init_info wbcffi_init_info_t;
typedef const wbcffi_config_entry wbcffi_config_entry_t;
typedef const char const_char_t;
static inline GtkContainer *GetRootWidget(GtkContainer *(*get_root_widget)(wbcffi_module *obj), wbcffi_module *obj) {
	return get_root_widget(obj);
}
static inline void QueueUpdate(void (*queue_update)(wbcffi_module *), wbcffi_module *obj) {
	queue_update(obj);
}
*/
import "C"

var global = state.New()

// wbcffi_init initializes a Waybar module instance from C and returns a pointer to its instance id or nil on failure.
// 
// It ensures a connection to the NIRI service, creates and registers a new module instance, wraps the provided
// root GTK container and calls the module's Preinit. It attaches a realize handler that will call the module's
// Init with monitor name and dimensions once the widget is realized, and applies the provided configuration
// entries. On any initialization or configuration error the new instance is removed and nil is returned.
func wbcffi_init(init_info *C.wbcffi_init_info_t,
	config_entries *C.wbcffi_config_entry_t,
	config_entries_len C.size_t) unsafe.Pointer {

	global.Locked(func(g *state.State) {
		if global.GetNiriState() == nil {
			var err error
			log.Debugf("connecting to niri socket")
			niriState, niriSocket, err := niri.Init()
			if err != nil {
				log.Errorf("connecting to niri socket: %s", err)
				return
			}
			global.SetNiriState(niriState)
			global.SetNiriSocket(niriSocket)
		}
	})

	if global.GetNiriState() == nil {
		// niri didn't connect (error already logged above), exit
		return nil
	}

	queueUpdate := init_info.queue_update
	waybarModule := init_info.obj

	i := module.New(global.GetNiriState(), global.GetNiriSocket(), func() {
		C.QueueUpdate(queueUpdate, waybarModule)
	})
	global.AddInstance(i)
	id := i.Id()

	root := wrapContainer(C.GetRootWidget(init_info.get_root_widget, init_info.obj))

	err := i.Preinit(root)
	if err != nil {
		global.RemoveInstance(id)
		log.Errorf("preinit: %s", err)
		return nil
	}

	root.Connect("realize", func(obj *glib.Object) {
		go func() {
			// let waybar settle
			time.Sleep(time.Millisecond * 100)
			i := global.GetInstance(id)
			if i == nil {
				log.Errorf("realize: instance %x not found", id)
				return
			}

			root := gtk.Widget{glib.InitiallyUnowned{obj}}
			monitor, screenWidth, screenHeight, err := getMonitorInfo(&root)
			if err != nil {
				log.Errorf("realize: %s", err)
				return
			}

			log.Debugf("got monitor! id=%x name=%s", id, monitor)
			i.Init(monitor, screenWidth, screenHeight)
		}()
	})

	log.Debugf("init from go! id=%x", id)
	for _, entry := range unsafe.Slice(config_entries, config_entries_len) {
		key, value := C.GoString(entry.key), C.GoString(entry.value)
		log.Tracef("config %s = %s", key, value)
		err := i.ApplyConfig(key, value)
		if err != nil {
			global.RemoveInstance(id)
			log.Errorf("%s config: %s", key, err)
			return nil
		}
	}

	return unsafe.Pointer(id)
}

// wbcffi_deinit deinitializes the module instance identified by instanceId and removes it from the global registry.
// If no instance exists for the provided id, it logs an error and returns.
func wbcffi_deinit(instanceId unsafe.Pointer) {
	log.Tracef("deinit id=%x", uintptr(instanceId))
	i := global.GetInstance(uintptr(instanceId))
	if i == nil {
		log.Errorf("instance %x not found", instanceId)
		return
	}
	i.Deinit()
	global.RemoveInstance(uintptr(instanceId))
}

// wbcffi_update requests the module instance identified by instanceId to perform an update.
// If no instance exists for the given id, the call has no effect.
func wbcffi_update(instanceId unsafe.Pointer) {
	log.Tracef("update id=%x", uintptr(instanceId))
	i := global.GetInstance(uintptr(instanceId))
	if i == nil {
		log.Errorf("instance %x not found", instanceId)
		return
	}
	i.Update()
}

// wbcffi_refresh triggers a refresh on the module instance identified by instanceId using the provided signal.
// If no instance matches the given id, the function logs an error and returns without performing a refresh.
func wbcffi_refresh(instanceId unsafe.Pointer, signal C.int) {
	log.Tracef("refresh id=%x signal=%d", uintptr(instanceId), signal)
	i := global.GetInstance(uintptr(instanceId))
	if i == nil {
		log.Errorf("instance %x not found", instanceId)
		return
	}
	i.Refresh(int(signal))
}

// wbcffi_doaction invokes the named action on the module instance identified by instanceId.
// instanceId is the opaque identifier for a module instance; action_name is a C string naming the action to perform.
func wbcffi_doaction(instanceId unsafe.Pointer, action_name *C.const_char_t) {
	log.Tracef("doaction id=%x action_name=%s", uintptr(instanceId), C.GoString(action_name))
	i := global.GetInstance(uintptr(instanceId))
	if i == nil {
		log.Errorf("instance %x not found", instanceId)
		return
	}
	i.DoAction(C.GoString(action_name))
}

// wrapContainer creates a gtk.Container whose underlying glib.Object wraps the provided C GtkContainer pointer.
func wrapContainer(c *C.GtkContainer) *gtk.Container {
	container := &gtk.Container{}
	container.Object = &glib.Object{glib.ToGObject(unsafe.Pointer(c))}
	return container
}

// getMonitorInfo obtains the monitor plug name and the monitor workarea width and height
// for the toplevel window that contains w.
// It returns the monitor plug name, width, height, and a non-nil error if the toplevel
// widget is not a window or if any underlying GTK/GDK queries fail.
func getMonitorInfo(w *gtk.Widget) (name string, width, height int, err error) {
	// alias gtkmm__GtkWindow to GtkWindow so gotk3 can understand it
	gtk.WrapMap["gtkmm__GtkWindow"] = gtk.WrapMap["GtkWindow"]

	toplevel, err := w.GetToplevel()
	if err != nil {
		err = fmt.Errorf("error getting toplevel: %s", err)
		return
	}
	window, ok := toplevel.(*gtk.Window)
	if !ok {
		err = fmt.Errorf("toplevel is not a window (is a %#T)", toplevel)
		return
	}

	gdkWindow, err := window.GetWindow()
	if err != nil {
		err = fmt.Errorf("error getting gdk window: %s", err)
		return
	}

	c_screen := (*C.GdkScreen)(unsafe.Pointer(window.GetScreen().Native()))
	c_gdkWindow := (*C.GdkWindow)(unsafe.Pointer(gdkWindow.Native()))
	monitorNum := C.gdk_screen_get_monitor_at_window(c_screen, c_gdkWindow)
	name = C.GoString(C.gdk_screen_get_monitor_plug_name(c_screen, monitorNum))

	var c_rectangle C.GdkRectangle
	C.gdk_screen_get_monitor_workarea(c_screen, monitorNum, &c_rectangle)
	width = int(c_rectangle.width)
	height = int(c_rectangle.height)

	return
}