package dev.nexus.android

import android.app.ActivityManager
import android.content.Context
import android.content.pm.PackageManager
import android.opengl.EGL14
import android.opengl.EGLConfig
import android.opengl.GLES20
import android.os.Build
import android.os.Environment
import android.os.PowerManager
import android.os.StatFs
import org.json.JSONArray
import org.json.JSONObject

/**
 * Collects what NEXUS needs to know about the device for its readiness check
 * (DeviceProfile in app/platform_mobile.go): Android version, processor, graphics, Vulkan
 * version, memory, free storage, file access and temperature.
 */
object DeviceInfo {
    fun collect(context: Context): JSONObject {
        val am = context.getSystemService(ActivityManager::class.java)
        val mem = ActivityManager.MemoryInfo().also { am.getMemoryInfo(it) }
        val power = context.getSystemService(PowerManager::class.java)
        val storage = StatFs(context.filesDir.absolutePath)
        return JSONObject()
            .put("apiLevel", Build.VERSION.SDK_INT)
            .put("release", Build.VERSION.RELEASE)
            .put("manufacturer", Build.MANUFACTURER)
            .put("model", Build.MODEL)
            .put("socManufacturer", if (Build.VERSION.SDK_INT >= 31) Build.SOC_MANUFACTURER else "")
            .put("socModel", if (Build.VERSION.SDK_INT >= 31) Build.SOC_MODEL else Build.HARDWARE)
            .put("gpuRenderer", glRenderer())
            .put("vulkanVersion", vulkanVersion(context.packageManager))
            .put("abis", JSONArray(Build.SUPPORTED_ABIS.toList()))
            .put("totalRamMb", mem.totalMem / (1024 * 1024))
            .put("freeStorageMb", storage.availableBytes / (1024 * 1024))
            .put("allFilesAccess", hasAllFilesAccess())
            .put("thermalStatus", power.currentThermalStatus)
    }

    fun hasAllFilesAccess(): Boolean = Environment.isExternalStorageManager()

    /**
     * The highest Vulkan version the device declares (FEATURE_VULKAN_HARDWARE_VERSION is
     * encoded like VK_MAKE_API_VERSION: major << 22 | minor << 12), "" without Vulkan.
     */
    fun vulkanVersion(pm: PackageManager): String {
        val versions = listOf(1 to 4, 1 to 3, 1 to 2, 1 to 1, 1 to 0)
        for ((major, minor) in versions) {
            if (pm.hasSystemFeature(PackageManager.FEATURE_VULKAN_HARDWARE_VERSION, (major shl 22) or (minor shl 12))) {
                return "$major.$minor"
            }
        }
        return ""
    }

    /** GL_RENDERER from a throw-away 1x1 EGL context ("Adreno (TM) 750", "Mali-G715" …). */
    fun glRenderer(): String {
        val display = EGL14.eglGetDisplay(EGL14.EGL_DEFAULT_DISPLAY)
        if (display == EGL14.EGL_NO_DISPLAY) return ""
        val version = IntArray(2)
        if (!EGL14.eglInitialize(display, version, 0, version, 1)) return ""
        try {
            val configs = arrayOfNulls<EGLConfig>(1)
            val count = IntArray(1)
            val attribs = intArrayOf(EGL14.EGL_RENDERABLE_TYPE, EGL14.EGL_OPENGL_ES2_BIT, EGL14.EGL_SURFACE_TYPE, EGL14.EGL_PBUFFER_BIT, EGL14.EGL_NONE)
            if (!EGL14.eglChooseConfig(display, attribs, 0, configs, 0, 1, count, 0) || count[0] == 0) return ""
            val ctx = EGL14.eglCreateContext(display, configs[0], EGL14.EGL_NO_CONTEXT, intArrayOf(EGL14.EGL_CONTEXT_CLIENT_VERSION, 2, EGL14.EGL_NONE), 0)
            val surface = EGL14.eglCreatePbufferSurface(display, configs[0], intArrayOf(EGL14.EGL_WIDTH, 1, EGL14.EGL_HEIGHT, 1, EGL14.EGL_NONE), 0)
            try {
                EGL14.eglMakeCurrent(display, surface, surface, ctx)
                return GLES20.glGetString(GLES20.GL_RENDERER) ?: ""
            } finally {
                EGL14.eglMakeCurrent(display, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_SURFACE, EGL14.EGL_NO_CONTEXT)
                EGL14.eglDestroySurface(display, surface)
                EGL14.eglDestroyContext(display, ctx)
            }
        } finally {
            EGL14.eglTerminate(display)
        }
    }
}
