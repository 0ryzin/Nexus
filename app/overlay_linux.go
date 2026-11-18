//go:build linux && !android && webkitgtk

package main

// The in-game performance strip on Linux and the Steam Deck (Windows: overlay_windows.go). Before
// 0.8.0 this was not implemented on Linux at all: StartPerfOverlay did nothing (overlay_other.go),
// so the overlay never appeared on a Steam Deck, in Desktop or Gaming Mode.
//
// On Linux the engine draws the game in its own window, which in Gaming Mode gamescope shows full
// screen, alone. The strip is therefore a separate window of NEXUS's own:
//   - Gaming Mode (gamescope): a window with the GAMESCOPE_EXTERNAL_OVERLAY property, which
//     gamescope composites over the game in front: the supported mechanism (Steam's own
//     performance overlay, MangoHud's mangoapp, uses it). It does not take focus or input.
//   - Desktop Mode / other X11 desktops: an override-redirect (unmanaged) window above the game,
//     raised at each refresh, click-through (empty input shape).
//   - A Wayland desktop session: one program cannot put a window over another's there; the strip
//     is not shown and the preview in Settings says so (perfOverlayNote).
// It sits at the top or bottom of the screen (the game is full screen in Gaming Mode; in a window
// on the desktop the strip still uses the screen's edge). Same settings, readings and colours as
// on Windows; refreshed twice a second only while a game is being played.

/*
#cgo pkg-config: gtk+-3.0
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <gtk/gtk.h>
#include <pango/pangocairo.h>

static GtkWidget *hud;
#define HUD_MAX 16
static char *hud_lab[HUD_MAX], *hud_val[HUD_MAX], *hud_unit[HUD_MAX];
static double hud_col[HUD_MAX][3];
static int hud_n, hud_fit, hud_h;
static double hud_alpha;

static gboolean hud_draw(GtkWidget *w, cairo_t *cr, gpointer d) {
	int W = gtk_widget_get_allocated_width(w), H = gtk_widget_get_allocated_height(w);
	cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
	cairo_set_source_rgba(cr, 0, 0, 0, 0);
	cairo_paint(cr);
	cairo_set_operator(cr, CAIRO_OPERATOR_OVER);
	double r = hud_fit ? H / 4.0 : 0;
	cairo_new_sub_path(cr);
	cairo_arc(cr, W - r, r, r, -G_PI / 2, 0);
	cairo_arc(cr, W - r, H - r, r, 0, G_PI / 2);
	cairo_arc(cr, r, H - r, r, G_PI / 2, G_PI);
	cairo_arc(cr, r, r, r, G_PI, 3 * G_PI / 2);
	cairo_close_path(cr);
	cairo_set_source_rgba(cr, 0.06, 0.06, 0.07, hud_alpha);
	cairo_fill(cr);
	PangoLayout *pl = pango_cairo_create_layout(cr);
	char font[64];
	double x = H * 0.5;
	for (int i = 0; i < hud_n; i++) {
		const char *parts[3] = {hud_lab[i], hud_val[i], hud_unit[i]};
		for (int k = 0; k < 3; k++) {
			if (!parts[k] || !*parts[k]) continue;
			snprintf(font, sizeof font, "Sans %s %dpx", k == 1 ? "Bold" : "Bold", (int)(k == 1 ? H * 0.52 : H * 0.34));
			PangoFontDescription *fd = pango_font_description_from_string(font);
			pango_layout_set_font_description(pl, fd);
			pango_font_description_free(fd);
			pango_layout_set_text(pl, parts[k], -1);
			int tw, th;
			pango_layout_get_pixel_size(pl, &tw, &th);
			if (k == 0) cairo_set_source_rgb(cr, hud_col[i][0], hud_col[i][1], hud_col[i][2]);
			else if (k == 1) cairo_set_source_rgb(cr, 1, 1, 1);
			else cairo_set_source_rgb(cr, 0.62, 0.62, 0.66);
			cairo_move_to(cr, x, (H - th) / 2.0);
			pango_cairo_show_layout(cr, pl);
			x += tw + H * 0.14;
		}
		x += H * 0.42;
	}
	g_object_unref(pl);
	return TRUE;
}

// the width the segments need (for the compact layout)
static int hud_measure(int H) {
	PangoFontMap *fm = pango_cairo_font_map_get_default();
	PangoContext *pc = pango_font_map_create_context(fm);
	PangoLayout *pl = pango_layout_new(pc);
	char font[64];
	double x = H * 0.5;
	for (int i = 0; i < hud_n; i++) {
		const char *parts[3] = {hud_lab[i], hud_val[i], hud_unit[i]};
		for (int k = 0; k < 3; k++) {
			if (!parts[k] || !*parts[k]) continue;
			snprintf(font, sizeof font, "Sans Bold %dpx", (int)(k == 1 ? H * 0.52 : H * 0.34));
			PangoFontDescription *fd = pango_font_description_from_string(font);
			pango_layout_set_font_description(pl, fd);
			pango_font_description_free(fd);
			pango_layout_set_text(pl, parts[k], -1);
			int tw, th;
			pango_layout_get_pixel_size(pl, &tw, &th);
			x += tw + H * 0.14;
		}
		x += H * 0.42;
	}
	g_object_unref(pl);
	g_object_unref(pc);
	return (int)(x + H * 0.1);
}

// 0: shown; 1: Wayland session (cannot overlay another program); 2: no display
static int hud_ensure(int gamescope) {
	if (hud) return 0;
	GdkDisplay *d = gdk_display_get_default();
	if (!d) return 2;
	if (strcmp(G_OBJECT_TYPE_NAME(d), "GdkX11Display") != 0) return 1;
	hud = gtk_window_new(GTK_WINDOW_POPUP);
	gtk_window_set_title(GTK_WINDOW(hud), "NEXUS performance");
	gtk_widget_set_app_paintable(hud, TRUE);
	GdkScreen *s = gtk_widget_get_screen(hud);
	GdkVisual *v = gdk_screen_get_rgba_visual(s);
	if (v) gtk_widget_set_visual(hud, v);
	gtk_window_set_accept_focus(GTK_WINDOW(hud), FALSE);
	gtk_window_set_focus_on_map(GTK_WINDOW(hud), FALSE);
	gtk_window_set_keep_above(GTK_WINDOW(hud), TRUE);
	g_signal_connect(hud, "draw", G_CALLBACK(hud_draw), NULL);
	gtk_widget_realize(hud);
	GdkWindow *gw = gtk_widget_get_window(hud);
	cairo_region_t *empty = cairo_region_create();
	gdk_window_input_shape_combine_region(gw, empty, 0, 0); // click-through
	cairo_region_destroy(empty);
	if (gamescope) {
		long one = 1;
		gdk_property_change(gw, gdk_atom_intern_static_string("GAMESCOPE_EXTERNAL_OVERLAY"),
			gdk_atom_intern_static_string("CARDINAL"), 32, GDK_PROP_MODE_REPLACE, (const guchar *)&one, 1);
	}
	return 0;
}

static void hud_clear(void) {
	for (int i = 0; i < hud_n; i++) { free(hud_lab[i]); free(hud_val[i]); free(hud_unit[i]); }
	hud_n = 0;
}

static void hud_add(const char *l, const char *v, const char *u, double r, double g, double b) {
	if (hud_n >= HUD_MAX) return;
	hud_lab[hud_n] = strdup(l); hud_val[hud_n] = strdup(v); hud_unit[hud_n] = strdup(u);
	hud_col[hud_n][0] = r; hud_col[hud_n][1] = g; hud_col[hud_n][2] = b;
	hud_n++;
}

// the primary monitor's geometry (application pixels)
static void hud_monitor(int *x, int *y, int *w, int *h) {
	GdkDisplay *d = gdk_display_get_default();
	GdkMonitor *m = d ? gdk_display_get_primary_monitor(d) : NULL;
	if (!m && d) m = gdk_display_get_monitor(d, 0);
	GdkRectangle r = {0, 0, 1280, 800};
	if (m) gdk_monitor_get_geometry(m, &r);
	*x = r.x; *y = r.y; *w = r.width; *h = r.height;
}

static void hud_show(int x, int y, int w, int h, int fit, double alpha) {
	if (!hud) return;
	hud_fit = fit; hud_h = h; hud_alpha = alpha;
	if (fit) w = hud_measure(h);
	gtk_window_resize(GTK_WINDOW(hud), w, h);
	gtk_window_move(GTK_WINDOW(hud), x, y);
	if (!gtk_widget_get_visible(hud)) gtk_widget_show(hud);
	GdkWindow *gw = gtk_widget_get_window(hud);
	if (gw) gdk_window_raise(gw);
	gtk_widget_queue_draw(hud);
}

static void hud_hide(void) { if (hud && gtk_widget_get_visible(hud)) gtk_widget_hide(hud); }
*/
import "C"

import (
	"sync"
	"time"
	"unsafe"
)

// HUD label colours (as on Windows, overlay_windows.go), 0-1.
var hudRGB = map[string][3]float64{"fps": {100, 210, 255}, "frame": {100, 210, 255}, "cpu": {10, 132, 255},
	"gpu": {48, 209, 88}, "mem": {191, 90, 242}, "temp": {255, 159, 10}, "shader": {100, 210, 255}, "shaderbusy": {255, 159, 10}}

type linuxHUD struct {
	mu      sync.Mutex
	started bool
	state   int // hud_ensure's answer: 0 ok, 1 Wayland, 2 no display, -1 not tried
}

var lhud = &linuxHUD{state: -1}

// perfOverlayNote: why the in-game strip cannot show here (empty when it can).
func perfOverlayNote() string {
	lhud.mu.Lock()
	defer lhud.mu.Unlock()
	if lhud.state == 1 {
		return "This desktop runs on Wayland, where one program cannot draw over another's window, so the in-game strip cannot show here. It works in Steam Deck Gaming Mode and on X11 desktops. Steam's own performance overlay works everywhere."
	}
	return ""
}

// StartPerfOverlay starts the refresh loop (once): cheap while no game is played.
func (a *App) StartPerfOverlay() {
	lhud.mu.Lock()
	defer lhud.mu.Unlock()
	if lhud.started {
		return
	}
	lhud.started = true
	go lhud.loop(a)
}

func (o *linuxHUD) loop(a *App) {
	shown := false
	for {
		st := a.Emu.State()
		cfg := a.Settings.Get().PerfMonitor
		playing := st.Running && st.Phase == "playing" && !st.Suspended
		if playing && cfg.For(st.GameKey) {
			segs := perfSegments(perf.Sample(a.Emu), cfg.Metrics)
			if cfg.Layout == "fps" {
				var f []PerfSeg
				for _, s := range segs {
					if s.Kind == "fps" || s.Kind == "frame" {
						f = append(f, s)
					}
				}
				segs = f
			}
			gs := gamingMode()
			a.UI.(*gtkUI).dispatch(func() {
				o.mu.Lock()
				o.state = int(C.hud_ensure(C.int(gtkBool(gs))))
				ok := o.state == 0
				o.mu.Unlock()
				if !ok {
					return
				}
				C.hud_clear()
				for _, s := range segs {
					c := hudRGB[s.Kind]
					l, v, u := C.CString(s.Label), C.CString(s.Value), C.CString(s.Unit)
					C.hud_add(l, v, u, C.double(c[0]/255), C.double(c[1]/255), C.double(c[2]/255))
					C.free(unsafe.Pointer(l))
					C.free(unsafe.Pointer(v))
					C.free(unsafe.Pointer(u))
				}
				var mx, my, mw, mh C.int
				C.hud_monitor(&mx, &my, &mw, &mh)
				img := gameImageRect(pxRect{int32(mx), int32(my), int32(mx + mw), int32(my + mh)}, hudAspect(a, st.GameKey))
				h := hudBarHeight(img, cfg.Scale)
				x, y, w := img.L, img.T, img.W()
				if cfg.Position == "bottom" {
					y = img.B - h
				}
				fit := 0
				if cfg.Layout == "fps" {
					fit = 1
					x, y = x+h/3, y+h/3
				}
				C.hud_show(C.int(x), C.int(y), C.int(w), C.int(h), C.int(fit), C.double(clampF(float64(cfg.Opacity), 30, 100)/100))
			})
			shown = true
			time.Sleep(500 * time.Millisecond)
			continue
		}
		if shown {
			a.UI.(*gtkUI).dispatch(func() { C.hud_hide() })
			shown = false
		}
		time.Sleep(time.Second)
	}
}
