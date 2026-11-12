//go:build android

// JNI helpers for platform_android.go (cgo forbids C definitions in a file that uses //export).

#include <jni.h>
#include <stdlib.h>
#include <android/native_window_jni.h>

const char* nx_jstr(JNIEnv* env, jstring s) { return s ? (*env)->GetStringUTFChars(env, s, 0) : 0; }
void nx_jrel(JNIEnv* env, jstring s, const char* c) { if (s && c) (*env)->ReleaseStringUTFChars(env, s, c); }
jstring nx_newstr(JNIEnv* env, const char* c) { return (*env)->NewStringUTF(env, c); }
void* nx_window(JNIEnv* env, jobject surface) { return surface ? (void*)ANativeWindow_fromSurface(env, surface) : 0; }
void nx_release_window(void* w) { if (w) ANativeWindow_release((ANativeWindow*)w); }
