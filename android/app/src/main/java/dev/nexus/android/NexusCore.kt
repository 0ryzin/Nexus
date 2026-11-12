package dev.nexus.android

import android.view.Surface

/**
 * The shared NEXUS core (Go, libnexus_core.so): library, users, saves, media, settings, the
 * engine host and the interface server. These calls are implemented in app/platform_android.go.
 */
object NexusCore {
    init {
        System.loadLibrary("nexus_core")
    }

    /** Starts the core; returns the interface URL or "error:<message>". */
    @JvmStatic external fun nativeStart(filesDir: String, nativeLibraryDir: String, gamesDir: String): String

    @JvmStatic external fun nativeStop()

    /** Reports the device (DeviceInfo JSON); returns the readiness check as JSON. */
    @JvmStatic external fun nativeDevice(json: String): String

    @JvmStatic external fun nativeLifecycle(event: Int)

    @JvmStatic external fun nativeThermal(status: Int)

    @JvmStatic external fun nativeTrimMemory(level: Int)

    /** The game view's surface (null when it is destroyed); true if the engine took it. */
    @JvmStatic external fun nativeSurface(surface: Surface?): Boolean

    /** The on-screen controller's state as JSON; true if the engine accepted it. */
    @JvmStatic external fun nativeTouch(json: String): Boolean

    // LifecycleEvent in app/platform_mobile.go
    const val LIFECYCLE_RESUME = 1
    const val LIFECYCLE_PAUSE = 2
    const val LIFECYCLE_STOP = 3
}
