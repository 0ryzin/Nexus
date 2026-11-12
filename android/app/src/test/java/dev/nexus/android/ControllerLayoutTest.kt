package dev.nexus.android

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import kotlin.math.hypot

class ControllerLayoutTest {
    private val screens = listOf(2400f to 1080f, 1920f to 1080f, 2560f to 1600f, 1280f to 720f, 2176f to 1812f)

    @Test
    fun controlsStayOnScreenAndDoNotOverlap() {
        for ((w, h) in screens) {
            val l = ControllerLayout(w, h)
            for (c in l.controls) {
                assertTrue("${c.id} off screen on ${w}x$h", c.cx - c.r >= 0 && c.cy - c.r >= 0 && c.cx + c.r <= w && c.cy + c.r <= h)
            }
            for (a in l.controls) for (b in l.controls) if (a !== b) {
                assertTrue("${a.id} overlaps ${b.id} on ${w}x$h", hypot(a.cx - b.cx, a.cy - b.cy) >= a.r + b.r)
            }
        }
    }

    @Test
    fun hitFindsTheControlUnderTheFinger() {
        val l = ControllerLayout(2400f, 1080f)
        for (c in l.controls) assertEquals(c.id, l.hit(c.cx, c.cy)?.id)
        assertNull(l.hit(1200f, 540f)) // the middle of the screen is the game's
    }

    @Test
    fun dpadDiagonalsAndStickDeadZone() {
        val l = ControllerLayout(2400f, 1080f)
        val d = l.controls.first { it.id == "DPAD" }
        assertEquals(listOf("DUP", "DRIGHT"), l.dpad(d, d.cx + d.r * 0.7f, d.cy - d.r * 0.7f))
        assertEquals(emptyList<String>(), l.dpad(d, d.cx, d.cy))
        val s = l.controls.first { it.id == "LSTICK" }
        assertEquals(0f to 0f, l.stick(s, s.cx + 1f, s.cy))
        val (x, y) = l.stick(s, s.cx, s.cy - s.r * 3) // far above: clamped, up is +1
        assertEquals(0f, x, 0.001f)
        assertEquals(1f, y, 0.001f)
    }

    @Test
    fun stateCombinesFingersAndSerialises() {
        val l = ControllerLayout(2400f, 1080f)
        val a = l.controls.first { it.id == "A" }
        val zr = l.controls.first { it.id == "ZR" }
        val ls = l.controls.first { it.id == "LSTICK" }
        val touches = mapOf(1 to (a.cx to a.cy), 2 to (zr.cx to zr.cy), 3 to ((ls.cx + ls.r) to ls.cy))
        val owners = mapOf(1 to a, 2 to zr, 3 to ls)
        val s = ControllerState.from(l, touches, owners)
        assertEquals(setOf("A", "ZR"), s.buttons)
        val json = s.toJson()
        assertTrue(json, json.startsWith("{\"buttons\":[\"A\",\"ZR\"],\"lstick\":[1.000,"))
        assertTrue(json, json.endsWith("\"rstick\":[0.000,0.000]}"))
    }
}
