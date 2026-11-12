package dev.nexus.android

import kotlin.math.hypot
import kotlin.math.min

/**
 * Geometry and state of the on-screen controller, independent of Android views so it can be
 * unit-tested. Positions are in pixels of a landscape screen of the given size; the layout
 * scales with the shorter side and keeps clear of the screen's edges.
 */
class ControllerLayout(val width: Float, val height: Float, scale: Float = 1f) {
    data class Control(val id: String, val cx: Float, val cy: Float, val r: Float, val kind: Kind)
    enum class Kind { BUTTON, STICK, DPAD }

    val unit = min(width, height) / 10f * scale
    val controls: List<Control>

    init {
        val m = unit * 0.9f // margin
        val l = mutableListOf<Control>()
        // left: stick above the D-pad; right: face buttons above the stick
        l += Control("LSTICK", m + unit * 1.6f, height - m - unit * 4.4f, unit * 1.3f, Kind.STICK)
        l += Control("DPAD", m + unit * 3.2f, height - m - unit * 1.5f, unit * 1.4f, Kind.DPAD)
        val fx = width - m - unit * 2.0f
        val fy = height - m - unit * 4.6f
        val d = unit * 1.05f
        l += Control("A", fx + d, fy, unit * 0.62f, Kind.BUTTON)
        l += Control("B", fx, fy + d, unit * 0.62f, Kind.BUTTON)
        l += Control("X", fx, fy - d, unit * 0.62f, Kind.BUTTON)
        l += Control("Y", fx - d, fy, unit * 0.62f, Kind.BUTTON)
        l += Control("RSTICK", width - m - unit * 3.4f, height - m - unit * 1.4f, unit * 1.1f, Kind.STICK)
        // shoulders along the top corners
        l += Control("ZL", m + unit * 0.8f, m + unit * 0.7f, unit * 0.65f, Kind.BUTTON)
        l += Control("L", m + unit * 2.4f, m + unit * 0.7f, unit * 0.65f, Kind.BUTTON)
        l += Control("R", width - m - unit * 2.4f, m + unit * 0.7f, unit * 0.65f, Kind.BUTTON)
        l += Control("ZR", width - m - unit * 0.8f, m + unit * 0.7f, unit * 0.65f, Kind.BUTTON)
        // minus, Home (NEXUS Home) and plus along the top centre
        l += Control("MINUS", width / 2 - unit * 1.4f, m + unit * 0.5f, unit * 0.45f, Kind.BUTTON)
        l += Control("PLUS", width / 2 + unit * 1.4f, m + unit * 0.5f, unit * 0.45f, Kind.BUTTON)
        l += Control("HOME", width / 2, m + unit * 0.5f, unit * 0.45f, Kind.BUTTON)
        controls = l
    }

    /** The control under a touch point (sticks and the D-pad accept touches a little outside). */
    fun hit(x: Float, y: Float): Control? = controls.minByOrNull { hypot(x - it.cx, y - it.cy) / it.r }
        ?.takeIf { hypot(x - it.cx, y - it.cy) <= it.r * if (it.kind == Kind.BUTTON) 1.15f else 1.4f }

    /** D-pad direction(s) for a point on the D-pad (diagonals press two directions). */
    fun dpad(c: Control, x: Float, y: Float): List<String> {
        val dx = (x - c.cx) / c.r
        val dy = (y - c.cy) / c.r
        val out = mutableListOf<String>()
        val dead = 0.3f
        if (dy < -dead) out += "DUP"
        if (dy > dead) out += "DDOWN"
        if (dx < -dead) out += "DLEFT"
        if (dx > dead) out += "DRIGHT"
        return out
    }

    /** Stick deflection -1…1 (y up is negative, as on the Switch: up = -1 here, flipped later). */
    fun stick(c: Control, x: Float, y: Float): Pair<Float, Float> {
        var dx = (x - c.cx) / c.r
        var dy = (y - c.cy) / c.r
        val len = hypot(dx, dy)
        if (len > 1f) { dx /= len; dy /= len }
        if (len < 0.08f) return 0f to 0f // dead zone
        return dx to -dy
    }
}

/** What all fingers press together, sent to the engine as TouchState JSON. */
data class ControllerState(
    val buttons: Set<String> = emptySet(),
    val lstick: Pair<Float, Float> = 0f to 0f,
    val rstick: Pair<Float, Float> = 0f to 0f,
) {
    fun toJson(): String {
        val b = buttons.sorted().joinToString(",") { "\"$it\"" }
        fun f(v: Float) = String.format(java.util.Locale.ROOT, "%.3f", v)
        return "{\"buttons\":[$b],\"lstick\":[${f(lstick.first)},${f(lstick.second)}],\"rstick\":[${f(rstick.first)},${f(rstick.second)}]}"
    }

    companion object {
        /** Combines the touches currently down (pointer id -> position). */
        fun from(layout: ControllerLayout, touches: Map<Int, Pair<Float, Float>>, owners: Map<Int, ControllerLayout.Control>): ControllerState {
            val buttons = mutableSetOf<String>()
            var ls = 0f to 0f
            var rs = 0f to 0f
            for ((id, pos) in touches) {
                val c = owners[id] ?: continue
                when (c.kind) {
                    ControllerLayout.Kind.BUTTON -> buttons += c.id
                    ControllerLayout.Kind.DPAD -> buttons += layout.dpad(c, pos.first, pos.second)
                    ControllerLayout.Kind.STICK -> if (c.id == "LSTICK") ls = layout.stick(c, pos.first, pos.second) else rs = layout.stick(c, pos.first, pos.second)
                }
            }
            return ControllerState(buttons, ls, rs)
        }
    }
}
