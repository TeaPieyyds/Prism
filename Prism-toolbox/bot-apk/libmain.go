//go:build android

package main

/*
#include <jni.h>

static const char* getJString(JNIEnv *env, jstring str) {
    return (*env)->GetStringUTFChars(env, str, NULL);
}

static void freeJString(JNIEnv *env, jstring str, const char *cstr) {
    (*env)->ReleaseStringUTFChars(env, str, cstr);
}
*/
import "C"
import (
	"fmt"
	"os"
	"runtime/debug"
)

var appDataDir string

//export Java_com_prismtool_box_MainActivity_setDataDir
func Java_com_prismtool_box_MainActivity_setDataDir(env *C.JNIEnv, clazz C.jclass, dir C.jstring) {
	cstr := C.getJString(env, dir)
	appDataDir = C.GoString(cstr)
	C.freeJString(env, dir, cstr)
}

//export Java_com_prismtool_box_MainActivity_setSignatureHash
func Java_com_prismtool_box_MainActivity_setSignatureHash(env *C.JNIEnv, clazz C.jclass, hash C.jstring) {
	cstr := C.getJString(env, hash)
	SetApkSignatureHash(C.GoString(cstr))
	C.freeJString(env, hash, cstr)
}

//export Java_com_prismtool_box_MainActivity_setPackageName
func Java_com_prismtool_box_MainActivity_setPackageName(env *C.JNIEnv, clazz C.jclass, pkg C.jstring) {
	cstr := C.getJString(env, pkg)
	SetApkPackageName(C.GoString(cstr))
	C.freeJString(env, pkg, cstr)
}

//export Java_com_prismtool_box_MainActivity_setApkPath
func Java_com_prismtool_box_MainActivity_setApkPath(env *C.JNIEnv, clazz C.jclass, path C.jstring) {
	cstr := C.getJString(env, path)
	SetApkPath(C.GoString(cstr))
	C.freeJString(env, path, cstr)
}

//export Java_com_prismtool_box_MainActivity_GoMain
func Java_com_prismtool_box_MainActivity_GoMain(env *C.JNIEnv, clazz C.jclass) {
	defer func() {
		if r := recover(); r != nil {
			writeStartupLog(fmt.Sprintf("PANIC: %v\n%s", r, debug.Stack()))
		}
	}()

	port := "8080"
	writeStartupLog(fmt.Sprintf("starting on 127.0.0.1:%s data=%s", port, appDataDir))

	if err := StartServer(port, appDataDir); err != nil {
		writeStartupLog(fmt.Sprintf("ERROR: %v", err))
	}
}

func writeStartupLog(msg string) {
	if appDataDir == "" {
		return
	}
	os.WriteFile(appDataDir+"/startup.log", []byte(msg+"\n"), 0644)
}

func main() {}
