package dev.nexus.android

import android.annotation.SuppressLint
import android.app.Activity
import android.content.ComponentCallbacks2
import android.content.Intent
import android.media.AudioAttributes
import android.media.AudioFocusRequest
import android.media.AudioManager
import android.net.Uri
import android.os.Bundle
import android.os.Environment
import android.os.Handler
import android.os.Looper
import android.os.PowerManager
import android.provider.Settings
import android.view.KeyEvent
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.View
import android.view.WindowInsets
import android.view.WindowInsetsController
import android.webkit.JavascriptInterface
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import android.widget.TextView
import java.io.File
import java.util.concurrent.Executors

/**
 * NEXUS for Android: one activity. Layers, back to front:
 *  1. the game surface (SurfaceView) the engine draws into,
 *  2. the on-screen controller (only while a game is in front),
 *  3. the NEXUS interface (WebView on the core's local server), hidden while a game is in front.
 * The interface is the same as on Windows; it tells this activity when a game is in front
 * through the NexusAndroid JavaScript bridge.
 */
class MainActivity : Activity() {
    private lateinit var web: WebView
    private lateinit var surface: SurfaceView
    private lateinit var controller: OnScreenController
    private val main = Handler(Looper.getMainLooper())
    private val worker = Executors.newSingleThreadExecutor()
    private var started = false
    private var gameInFront = false
    private var audioFocus: AudioFocusRequest? = null
    private val thermalListener = PowerManager.OnThermalStatusChangedListener { NexusCore.nativeThermal(it) }

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        surface = SurfaceView(this)
        controller = OnScreenController(this) { runJs("suspendGame()") } // HOME: NEXUS Home
        controller.visibility = View.GONE
        web = WebView(this).apply {
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.mediaPlaybackRequiresUserGesture = false
            setBackgroundColor(0xFF1D1E21.toInt())
            webViewClient = object : WebViewClient() {
                // only the core's own pages open inside NEXUS
                override fun shouldOverrideUrlLoading(view: WebView, req: android.webkit.WebResourceRequest): Boolean =
                    req.url.host != "127.0.0.1"
            }
            addJavascriptInterface(Bridge(), "NexusAndroid")
        }
        val root = FrameLayout(this)
        root.addView(surface)
        root.addView(controller)
        root.addView(web)
        setContentView(root)
        surface.holder.addCallback(object : SurfaceHolder.Callback {
            override fun surfaceCreated(h: SurfaceHolder) { NexusCore.nativeSurface(h.surface) }
            override fun surfaceChanged(h: SurfaceHolder, format: Int, w: Int, hh: Int) { NexusCore.nativeSurface(h.surface) }
            override fun surfaceDestroyed(h: SurfaceHolder) { NexusCore.nativeSurface(null) }
        })
        immersive()
        startCore()
    }

    private fun startCore() {
        val status = TextView(this).apply { text = "Starting NEXUS…"; setTextColor(0xFFFFFFFF.toInt()); textSize = 18f; setPadding(48, 48, 48, 48) }
        addContentView(status, FrameLayout.LayoutParams(-1, -1))
        worker.execute {
            val gamesDir = gamesFolder()
            val root = BundledFiles.install(this)
            val url = NexusCore.nativeStart(filesDir.absolutePath, applicationInfo.nativeLibraryDir, gamesDir?.absolutePath ?: File(root, "Games").absolutePath)
            NexusCore.nativeDevice(DeviceInfo.collect(this).toString())
            main.post {
                (status.parent as? FrameLayout)?.removeView(status)
                if (url.startsWith("error:")) {
                    showError(url.removePrefix("error:"))
                } else {
                    started = true
                    web.loadUrl(url)
                    if (!DeviceInfo.hasAllFilesAccess()) askForFileAccess()
                }
            }
        }
    }

    /** Games are read from <shared storage>/NEXUS/Games once file access is allowed. */
    private fun gamesFolder(): File? {
        if (!DeviceInfo.hasAllFilesAccess()) return null
        return File(Environment.getExternalStorageDirectory(), "NEXUS/Games").also { it.mkdirs() }
    }

    private fun askForFileAccess() {
        android.app.AlertDialog.Builder(this)
            .setTitle("Allow access to your game folders")
            .setMessage("NEXUS reads your games from the NEXUS/Games folder on your device, and from other folders you add. Android asks you to allow access to all files for this.")
            .setPositiveButton("Open settings") { _, _ ->
                startActivity(Intent(Settings.ACTION_MANAGE_APP_ALL_FILES_ACCESS_PERMISSION, Uri.parse("package:$packageName")))
            }
            .setNegativeButton("Not now", null)
            .show()
    }

    private fun showError(message: String) {
        android.app.AlertDialog.Builder(this).setTitle("NEXUS could not start").setMessage(message)
            .setPositiveButton("Close") { _, _ -> finish() }.setCancelable(false).show()
    }

    private fun runJs(js: String) = main.post { web.evaluateJavascript(js, null) }

    /** Called by the interface (app.js) when a game comes to the front or goes behind NEXUS. */
    inner class Bridge {
        @JavascriptInterface
        fun gameVisible(visible: Boolean) = main.post { setGameInFront(visible) }

        @JavascriptInterface
        fun platform(): String = "android"
    }

    private fun setGameInFront(visible: Boolean) {
        if (visible == gameInFront) return
        gameInFront = visible
        web.visibility = if (visible) View.GONE else View.VISIBLE
        controller.visibility = if (visible) View.VISIBLE else View.GONE
        val am = getSystemService(AudioManager::class.java)
        if (visible) {
            audioFocus = AudioFocusRequest.Builder(AudioManager.AUDIOFOCUS_GAIN)
                .setAudioAttributes(AudioAttributes.Builder().setUsage(AudioAttributes.USAGE_GAME).setContentType(AudioAttributes.CONTENT_TYPE_UNKNOWN).build())
                .build().also { am.requestAudioFocus(it) }
        } else {
            audioFocus?.let { am.abandonAudioFocusRequest(it) }
            audioFocus = null
        }
    }

    // ---- lifecycle: the core pauses a running game while NEXUS is not on screen

    override fun onResume() {
        super.onResume()
        immersive()
        getSystemService(PowerManager::class.java).addThermalStatusListener(mainExecutor, thermalListener)
        if (started) {
            NexusCore.nativeLifecycle(NexusCore.LIFECYCLE_RESUME)
            worker.execute { NexusCore.nativeDevice(DeviceInfo.collect(this).toString()) } // file access may have changed
        }
    }

    override fun onPause() {
        super.onPause()
        getSystemService(PowerManager::class.java).removeThermalStatusListener(thermalListener)
        if (started) NexusCore.nativeLifecycle(NexusCore.LIFECYCLE_PAUSE)
    }

    override fun onStop() {
        super.onStop()
        if (started) NexusCore.nativeLifecycle(NexusCore.LIFECYCLE_STOP)
    }

    override fun onDestroy() {
        if (isFinishing && started) NexusCore.nativeStop()
        worker.shutdown()
        super.onDestroy()
    }

    override fun onTrimMemory(level: Int) {
        super.onTrimMemory(level)
        if (started) NexusCore.nativeTrimMemory(level)
    }

    override fun onLowMemory() {
        super.onLowMemory()
        if (started) NexusCore.nativeTrimMemory(ComponentCallbacks2.TRIM_MEMORY_COMPLETE)
    }

    override fun onWindowFocusChanged(hasFocus: Boolean) {
        super.onWindowFocusChanged(hasFocus)
        if (hasFocus) immersive()
    }

    private fun immersive() {
        window.insetsController?.let {
            it.hide(WindowInsets.Type.systemBars())
            it.systemBarsBehavior = WindowInsetsController.BEHAVIOR_SHOW_TRANSIENT_BARS_BY_SWIPE
        }
    }

    // ---- controllers: in the interface, a controller's buttons become NEXUS's keys

    override fun dispatchKeyEvent(e: KeyEvent): Boolean {
        if (gameInFront) {
            // the controller's Home / Guide button returns to NEXUS; other input is the engine's
            if (e.keyCode == KeyEvent.KEYCODE_BUTTON_MODE && e.action == KeyEvent.ACTION_UP) { runJs("suspendGame()"); return true }
            return super.dispatchKeyEvent(e)
        }
        val mapped = GamepadKeys.toInterfaceKey(e.keyCode) ?: return super.dispatchKeyEvent(e)
        web.dispatchKeyEvent(KeyEvent(e.downTime, e.eventTime, e.action, mapped, e.repeatCount, e.metaState))
        return true
    }

    @Deprecated("Back is NEXUS's own Back in the interface")
    override fun onBackPressed() {
        if (gameInFront) { runJs("suspendGame()"); return }
        // Esc is "Back" in the NEXUS interface; on Home it opens the power menu
        web.dispatchKeyEvent(KeyEvent(KeyEvent.ACTION_DOWN, KeyEvent.KEYCODE_ESCAPE))
        web.dispatchKeyEvent(KeyEvent(KeyEvent.ACTION_UP, KeyEvent.KEYCODE_ESCAPE))
    }
}

/** Controller buttons → the keys the NEXUS interface uses (see the Controls table in README). */
object GamepadKeys {
    fun toInterfaceKey(code: Int): Int? = when (code) {
        KeyEvent.KEYCODE_BUTTON_A -> KeyEvent.KEYCODE_ENTER
        KeyEvent.KEYCODE_BUTTON_B -> KeyEvent.KEYCODE_ESCAPE
        KeyEvent.KEYCODE_BUTTON_X -> KeyEvent.KEYCODE_X
        KeyEvent.KEYCODE_BUTTON_Y -> KeyEvent.KEYCODE_SLASH
        KeyEvent.KEYCODE_BUTTON_L1 -> KeyEvent.KEYCODE_Q
        KeyEvent.KEYCODE_BUTTON_R1 -> KeyEvent.KEYCODE_E
        KeyEvent.KEYCODE_BUTTON_START -> KeyEvent.KEYCODE_ESCAPE
        KeyEvent.KEYCODE_BUTTON_MODE -> KeyEvent.KEYCODE_H
        KeyEvent.KEYCODE_DPAD_UP, KeyEvent.KEYCODE_DPAD_DOWN, KeyEvent.KEYCODE_DPAD_LEFT, KeyEvent.KEYCODE_DPAD_RIGHT -> code
        else -> null
    }
}
