//go:build android && cgo

package main

// The Android platform layer of NEXUS: the JNI entry points the Android app (android/ in the
// repository, class dev.nexus.android.NexusCore) calls. The app is a thin shell around the
// shared NEXUS core: it starts the core, shows its interface in a WebView, reports device facts
// and lifecycle, thermal and memory events, and hands over the game surface and touch controls.
// Libraries, users, saves, media, settings and the engine host are the same Go code as on
// Windows.
//
// Built with: GOOS=android GOARCH=arm64 CGO_ENABLED=1 CC=<NDK clang> go build -buildmode=c-shared
// -o libnexus_core.so (android/app/build.gradle.kts does this).

/*
#cgo LDFLAGS: -landroid -llog
#include <jni.h>
#include <stdlib.h>
const char* nx_jstr(JNIEnv* env, jstring s);
void nx_jrel(JNIEnv* env, jstring s, const char* c);
jstring nx_newstr(JNIEnv* env, const char* c);
void* nx_window(JNIEnv* env, jobject surface);
void nx_release_window(void* w);
*/
import "C"

import (
	"encoding/json"
	"path/filepath"
	"sync"
	"unsafe"
)

var (
	androidMu     sync.Mutex
	androidCore   *Core
	androidWindow unsafe.Pointer // ANativeWindow* held for the engine
)

func jstr(env *C.JNIEnv, s C.jstring) string {
	c := C.nx_jstr(env, s)
	if c == nil {
		return ""
	}
	defer C.nx_jrel(env, s, c)
	return C.GoString(c)
}

func jnew(env *C.JNIEnv, s string) C.jstring {
	cs := C.CString(s)
	defer C.free(unsafe.Pointer(cs))
	return C.nx_newstr(env, cs)
}

// nativeStart(filesDir, nativeLibraryDir, gamesDir) starts NEXUS and returns the interface URL,
// or "error:<message>".
//
//export Java_dev_nexus_android_NexusCore_nativeStart
func Java_dev_nexus_android_NexusCore_nativeStart(env *C.JNIEnv, _ C.jclass, filesDir, libDir, gamesDir C.jstring) C.jstring {
	androidMu.Lock()
	defer androidMu.Unlock()
	if androidCore != nil {
		return jnew(env, androidCore.URL)
	}
	files := jstr(env, filesDir)
	androidLibDir = jstr(env, libDir)
	core, err := StartCore(CoreOptions{
		Root:     filepath.Join(files, "nexus"),      // Resources and Licences, extracted from the APK
		Data:     filepath.Join(files, "nexus-data"), // users, saves, settings, engine data
		GamesDir: jstr(env, gamesDir),                // shared storage, visible to the user
	})
	if err != nil {
		return jnew(env, "error:"+err.Error())
	}
	core.App.UI = &noopUI{}
	androidCore = core
	return jnew(env, core.URL)
}

//export Java_dev_nexus_android_NexusCore_nativeStop
func Java_dev_nexus_android_NexusCore_nativeStop(env *C.JNIEnv, _ C.jclass) {
	androidMu.Lock()
	defer androidMu.Unlock()
	if androidCore != nil {
		if st := androidCore.App.Emu.State(); st.Running {
			_ = androidCore.App.Emu.Stop(false)
		}
		androidCore.Close()
		androidCore = nil
	}
}

func withApp(f func(a *App)) {
	androidMu.Lock()
	c := androidCore
	androidMu.Unlock()
	if c != nil {
		f(c.App)
	}
}

// nativeDevice(json): the device profile (DeviceInfo.kt); returns the readiness check as JSON.
//
//export Java_dev_nexus_android_NexusCore_nativeDevice
func Java_dev_nexus_android_NexusCore_nativeDevice(env *C.JNIEnv, _ C.jclass, js C.jstring) C.jstring {
	var d DeviceProfile
	if err := json.Unmarshal([]byte(jstr(env, js)), &d); err != nil {
		return jnew(env, `{"error":"bad device profile"}`)
	}
	d.OS, d.EngineLibrary = "android", androidEngineBundled()
	withApp(func(a *App) { a.SetDevice(d) })
	b, _ := json.Marshal(CheckReadiness(d))
	return jnew(env, string(b))
}

//export Java_dev_nexus_android_NexusCore_nativeLifecycle
func Java_dev_nexus_android_NexusCore_nativeLifecycle(env *C.JNIEnv, _ C.jclass, ev C.jint) {
	withApp(func(a *App) { a.OnLifecycle(LifecycleEvent(ev)) })
}

//export Java_dev_nexus_android_NexusCore_nativeThermal
func Java_dev_nexus_android_NexusCore_nativeThermal(env *C.JNIEnv, _ C.jclass, status C.jint) {
	withApp(func(a *App) { a.OnThermal(int(status)) })
}

//export Java_dev_nexus_android_NexusCore_nativeTrimMemory
func Java_dev_nexus_android_NexusCore_nativeTrimMemory(env *C.JNIEnv, _ C.jclass, level C.jint) {
	withApp(func(a *App) { a.OnTrimMemory(int(level)) })
}

// nativeSurface(surface or null): the game view's drawing surface for the engine.
//
//export Java_dev_nexus_android_NexusCore_nativeSurface
func Java_dev_nexus_android_NexusCore_nativeSurface(env *C.JNIEnv, _ C.jclass, surface C.jobject) C.jboolean {
	w := C.nx_window(env, surface)
	accepted := false
	withApp(func(a *App) {
		a.Emu.mu.Lock()
		h := a.Emu.host
		a.Emu.mu.Unlock()
		if de, ok := h.(*dlEngine); ok {
			accepted = de.SetSurface(w)
		}
	})
	androidMu.Lock()
	old := androidWindow
	androidWindow = w
	androidMu.Unlock()
	C.nx_release_window(old) // the engine has the new surface (or none) by now
	if accepted {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}

// nativeTouch(json): the on-screen controller's state (TouchState).
//
//export Java_dev_nexus_android_NexusCore_nativeTouch
func Java_dev_nexus_android_NexusCore_nativeTouch(env *C.JNIEnv, _ C.jclass, js C.jstring) C.jboolean {
	var t TouchState
	if json.Unmarshal([]byte(jstr(env, js)), &t) != nil {
		return C.JNI_FALSE
	}
	ok := false
	withApp(func(a *App) {
		if me, has := a.Emu.mobileHost(); has {
			ok = me.Touch(t)
		}
	})
	if ok {
		return C.JNI_TRUE
	}
	return C.JNI_FALSE
}
