package dev.nexus.android

import android.content.Context
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.view.HapticFeedbackConstants
import android.view.MotionEvent
import android.view.View

/**
 * The touch controller shown over the game. Each finger belongs to the control it first
 * touched (so sliding off a stick keeps steering it); the combined state is sent to the engine
 * whenever it changes. HOME goes to NEXUS Home instead of the game.
 */
class OnScreenController(context: Context, private val onHome: () -> Unit) : View(context) {
    private var layout: ControllerLayout? = null
    private val touches = mutableMapOf<Int, Pair<Float, Float>>()
    private val owners = mutableMapOf<Int, ControllerLayout.Control>()
    private var last = ControllerState()
    var opacity = 0.55f
        set(v) { field = v; invalidate() }

    private val fill = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.WHITE; style = Paint.Style.FILL }
    private val ring = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.WHITE; style = Paint.Style.STROKE; strokeWidth = 4f }
    private val text = Paint(Paint.ANTI_ALIAS_FLAG).apply { color = Color.WHITE; textAlign = Paint.Align.CENTER; isFakeBoldText = true }

    override fun onSizeChanged(w: Int, h: Int, ow: Int, oh: Int) {
        layout = ControllerLayout(w.toFloat(), h.toFloat())
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val l = layout ?: return false
        when (e.actionMasked) {
            MotionEvent.ACTION_DOWN, MotionEvent.ACTION_POINTER_DOWN -> {
                val i = e.actionIndex
                val id = e.getPointerId(i)
                val c = l.hit(e.getX(i), e.getY(i)) ?: return true
                if (c.id == "HOME") { performHapticFeedback(HapticFeedbackConstants.VIRTUAL_KEY); onHome(); return true }
                owners[id] = c
                touches[id] = e.getX(i) to e.getY(i)
                if (c.kind != ControllerLayout.Kind.STICK) performHapticFeedback(HapticFeedbackConstants.VIRTUAL_KEY)
            }
            MotionEvent.ACTION_MOVE -> for (i in 0 until e.pointerCount) {
                val id = e.getPointerId(i)
                if (owners.containsKey(id)) touches[id] = e.getX(i) to e.getY(i)
            }
            MotionEvent.ACTION_UP, MotionEvent.ACTION_POINTER_UP -> {
                val id = e.getPointerId(e.actionIndex)
                touches.remove(id); owners.remove(id)
            }
            MotionEvent.ACTION_CANCEL -> { touches.clear(); owners.clear() }
        }
        val s = ControllerState.from(l, touches, owners)
        if (s != last) {
            last = s
            NexusCore.nativeTouch(s.toJson())
            invalidate()
        }
        return true
    }

    override fun onDraw(canvas: Canvas) {
        val l = layout ?: return
        val alpha = (opacity * 255).toInt()
        for (c in l.controls) {
            val pressed = c.id in last.buttons || (c.kind == ControllerLayout.Kind.DPAD && last.buttons.any { it.startsWith("D") })
            fill.alpha = if (pressed) alpha else alpha / 4
            ring.alpha = alpha
            canvas.drawCircle(c.cx, c.cy, c.r, fill)
            canvas.drawCircle(c.cx, c.cy, c.r, ring)
            if (c.kind == ControllerLayout.Kind.STICK) {
                val (sx, sy) = if (c.id == "LSTICK") last.lstick else last.rstick
                fill.alpha = alpha
                canvas.drawCircle(c.cx + sx * c.r * 0.6f, c.cy - sy * c.r * 0.6f, c.r * 0.45f, fill)
            } else {
                text.alpha = alpha
                text.textSize = c.r * if (c.kind == ControllerLayout.Kind.DPAD) 0.35f else 0.7f
                val label = when (c.id) { "MINUS" -> "−"; "PLUS" -> "+"; "HOME" -> "⌂"; "DPAD" -> "✚"; else -> c.id }
                canvas.drawText(label, c.cx, c.cy + text.textSize / 3, text)
            }
        }
    }
}
