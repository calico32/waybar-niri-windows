package lib

import (
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"time"
	"unsafe"
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

var logOutput = io.Discard

var instances = make(map[uintptr]*module.Instance)
var niriState *niri.State
var niriSocket net.Conn

func initError(format string, args ...any) {
	log.SetOutput(os.Stderr)
	log.Printf("wbcffi: error initializing module: %s", fmt.Sprintf(format, args...))
	log.SetOutput(logOutput)
}

//export wbcffi_init
func wbcffi_init(init_info *C.wbcffi_init_info_t,
	config_entries *C.wbcffi_config_entry_t,
	config_entries_len C.size_t) unsafe.Pointer {

	log.SetOutput(logOutput)

	if niriState == nil {
		var err error
		log.Printf("wbcffi: connecting to niri socket")
		niriState, niriSocket, err = niri.Init()
		if err != nil {
			initError("connecting to niri socket: %s", err)
			return nil
		}
	}

	queueUpdate := init_info.queue_update
	waybarModule := init_info.obj

	i := module.New(niriState, niriSocket, func() {
		C.QueueUpdate(queueUpdate, waybarModule)
	})
	instances[i.Id] = i

	root := wrapContainer(C.GetRootWidget(init_info.get_root_widget, init_info.obj))

	err := i.Preinit(root)
	if err != nil {
		log.Print(err)
		return nil
	}

	root.Connect("realize", func(obj *glib.Object) {
		// widget is realized, so we can get the monitor
		i, err := getInstanceFromGObject(obj)
		if err != nil {
			log.Println(err)
			return
		}

		go func() {
			// let waybar settle
			time.Sleep(time.Millisecond * 100)

			root := gtk.Widget{glib.InitiallyUnowned{obj}}
			i.Monitor, i.ScreenWidth, i.ScreenHeight, err = getMonitorInfo(&root)
			if err != nil {
				log.Println(err)
				return
			}

			log.Printf("wbcffi: got monitor! id=%x name=%s\n", i.Id, i.Monitor)
			i.Ready = true
			i.Init()
		}()
	})

	log.Printf("wbcffi: init from go! id=%x\n", i.Id)
	for _, entry := range unsafe.Slice(config_entries, config_entries_len) {
		key, value := C.GoString(entry.key), C.GoString(entry.value)
		log.Printf("wbcffi: config %s = %s", key, value)
		err := i.ApplyConfig(key, value)
		if err != nil {
			initError("%s config: %s", key, err)
			return nil
		}
	}

	return unsafe.Pointer(i.Id)
}

//export wbcffi_deinit
func wbcffi_deinit(instanceId unsafe.Pointer) {
	i, ok := instances[uintptr(instanceId)]
	if !ok {
		log.Printf("wbcffi: instance %x not found\n", instanceId)
		return
	}
	log.Printf("wbcffi: deinit id=%x\n", uintptr(instanceId))
	i.Deinit()
	delete(instances, uintptr(instanceId))
}

//export wbcffi_update
func wbcffi_update(instanceId unsafe.Pointer) {
	i, ok := instances[uintptr(instanceId)]
	if !ok {
		log.Printf("wbcffi: instance %x not found\n", instanceId)
		return
	}
	i.Update()
}

//export wbcffi_refresh
func wbcffi_refresh(instanceId unsafe.Pointer, signal C.int) {
	log.Printf("wbcffi: refresh id=%x signal=%d\n", uintptr(instanceId), signal)
	i, ok := instances[uintptr(instanceId)]
	if !ok {
		log.Printf("wbcffi: instance %x not found\n", instanceId)
		return
	}
	i.Refresh(int(signal))
}

//export wbcffi_doaction
func wbcffi_doaction(instanceId unsafe.Pointer, action_name *C.const_char_t) {
	log.Printf("wbcffi: doaction id=%xx action_name=%s\n", uintptr(instanceId), C.GoString(action_name))
	i, ok := instances[uintptr(instanceId)]
	if !ok {
		log.Printf("wbcffi: instance %x not found\n", instanceId)
		return
	}
	i.DoAction(C.GoString(action_name))
}

func wrapContainer(c *C.GtkContainer) *gtk.Container {
	container := &gtk.Container{}
	container.Object = &glib.Object{glib.ToGObject(unsafe.Pointer(c))}
	return container
}

func getMonitorInfo(w *gtk.Widget) (name string, width, height int, err error) {
	// alias gtkmm__GtkWindow to GtkWindow so gotk3 can understand it
	gtk.WrapMap["gtkmm__GtkWindow"] = gtk.WrapMap["GtkWindow"]

	toplevel, err := w.GetToplevel()
	if err != nil {
		err = fmt.Errorf("wbcffi: error getting toplevel: %s", err)
		return
	}
	window, ok := toplevel.(*gtk.Window)
	if !ok {
		err = fmt.Errorf("wbcffi: toplevel is not a window (is a %#T)", toplevel)
		return
	}

	gdkWindow, err := window.GetWindow()
	if err != nil {
		err = fmt.Errorf("wbcffi: error getting gdk window: %s", err)
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

func getInstanceFromGObject(obj *glib.Object) (*module.Instance, error) {
	instanceIdStr, err := obj.GetProperty("name")
	if err != nil {
		return nil, fmt.Errorf("wbcffi: error getting instance_id string: %s", err)
	}
	instanceId, err := strconv.ParseUint(instanceIdStr.(string), 16, 64)
	if err != nil {
		return nil, fmt.Errorf("wbcffi: error parsing instance_id: %s", err)
	}

	i, ok := instances[uintptr(instanceId)]
	if !ok {
		return nil, fmt.Errorf("wbcffi: instance %x not found", instanceId)
	}

	return i, nil
}
