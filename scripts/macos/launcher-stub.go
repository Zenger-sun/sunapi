package main

/*
#cgo darwin LDFLAGS: -framework AppKit -lobjc
#include <objc/objc.h>
#include <objc/message.h>
#include <objc/runtime.h>

static void hideDockIcon(void) {
	Class nsApplication = (Class)objc_getClass("NSApplication");
	SEL sharedApplication = sel_registerName("sharedApplication");
	id app = ((id (*)(Class, SEL))objc_msgSend)(nsApplication, sharedApplication);
	SEL setActivationPolicy = sel_registerName("setActivationPolicy:");
	((void (*)(id, SEL, long))objc_msgSend)(app, setActivationPolicy, 1);
}
*/
import "C"

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func main() {
	runtime.LockOSThread()
	C.hideDockIcon()
	runtime.UnlockOSThread()

	exe, err := os.Executable()
	if err != nil {
		return
	}
	launcher := filepath.Join(filepath.Dir(exe), "SunAPI.launcher")
	cmd := exec.Command("/bin/sh", launcher)
	cmd.Dir = filepath.Dir(exe)

	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err == nil {
		defer devNull.Close()
		cmd.Stdin = devNull
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	}

	_ = cmd.Run()
}
