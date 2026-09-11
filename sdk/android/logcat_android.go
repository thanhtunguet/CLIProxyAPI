//go:build android && cgo

package android

/*
#include <android/log.h>
#include <stdlib.h>

static void do_android_log_write(int prio, const char* tag, const char* text) {
    __android_log_write(prio, tag, text);
}
*/
import "C"
import "unsafe"

func writeToLogcat(prio int, tag string, text string) {
	cTag := C.CString(tag)
	defer C.free(unsafe.Pointer(cTag))
	cText := C.CString(text)
	defer C.free(unsafe.Pointer(cText))

	C.do_android_log_write(C.int(prio), cTag, cText)
}
