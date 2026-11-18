//go:build linux && !android && webkitgtk

package main

// The NEXUS window on Linux (desktop PCs and the Steam Deck, Desktop and Gaming Mode): a GTK 3
// window with a WebKitGTK view showing the interface, as WebView2 does on Windows. Built with
// `-tags webkitgtk` (release builds, docs/LINUX.md); without the tag NEXUS builds without GTK
// for development and tests (app_other.go).
//
// Gaming Mode (gamescope) is recognised from the environment Steam gives non-Steam games
// (SteamGamepadUI / SteamDeck / gamescope): the window is then always full screen and
// undecorated, and the game window the engine opens takes the screen over until it closes.

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1
#include <stdlib.h>
#include <gtk/gtk.h>
#include <webkit2/webkit2.h>

extern void goFocus(int in);
extern void goFullscreenChanged(int on);
extern void goRunQueue(void);
extern int goKey(unsigned int keyval);
extern void goOpenURI(char *uri);

static GtkWidget *nx_win;
static WebKitWebView *nx_view;

static gboolean nx_focus_in(GtkWidget *w, GdkEvent *e, gpointer d) { goFocus(1); return FALSE; }
static gboolean nx_focus_out(GtkWidget *w, GdkEvent *e, gpointer d) { goFocus(0); return FALSE; }
static gboolean nx_state(GtkWidget *w, GdkEventWindowState *e, gpointer d) {
	if (e->changed_mask & GDK_WINDOW_STATE_FULLSCREEN) goFullscreenChanged((e->new_window_state & GDK_WINDOW_STATE_FULLSCREEN) != 0);
	return FALSE;
}
static gboolean nx_key(GtkWidget *w, GdkEventKey *e, gpointer d) { return goKey(e->keyval) ? TRUE : FALSE; }
static gboolean nx_menu(WebKitWebView *v, WebKitContextMenu *m, GdkEvent *e, WebKitHitTestResult *h, gpointer dev) { return GPOINTER_TO_INT(dev) ? FALSE : TRUE; }
static gboolean nx_delete(GtkWidget *w, GdkEvent *e, gpointer d) { gtk_main_quit(); return TRUE; }
static gboolean nx_idle(gpointer d) { goRunQueue(); return G_SOURCE_REMOVE; }
static void nx_wake(void) { g_idle_add(nx_idle, NULL); }

// a new window (or a link with target=_blank) opens in the user's browser, never inside NEXUS
static gboolean nx_policy(WebKitWebView *v, WebKitPolicyDecision *dec, WebKitPolicyDecisionType t, gpointer d) {
	if (t != WEBKIT_POLICY_DECISION_TYPE_NEW_WINDOW_ACTION) return FALSE;
	WebKitNavigationAction *a = webkit_navigation_policy_decision_get_navigation_action(WEBKIT_NAVIGATION_POLICY_DECISION(dec));
	const char *uri = webkit_uri_request_get_uri(webkit_navigation_action_get_request(a));
	if (uri) goOpenURI((char*)uri);
	webkit_policy_decision_ignore(dec);
	return TRUE;
}

static int nx_create(const char *title, int w, int h, int fullscreen, int decorated, const char *icon, const char *data, const char *cache, int dev) {
	if (!gtk_init_check(NULL, NULL)) return 0;
	nx_win = gtk_window_new(GTK_WINDOW_TOPLEVEL);
	gtk_window_set_title(GTK_WINDOW(nx_win), title);
	gtk_window_set_default_size(GTK_WINDOW(nx_win), w, h);
	gtk_window_set_position(GTK_WINDOW(nx_win), GTK_WIN_POS_CENTER);
	gtk_widget_set_size_request(nx_win, 640, 400);
	gtk_window_set_decorated(GTK_WINDOW(nx_win), decorated);
	if (icon && *icon) gtk_window_set_icon_from_file(GTK_WINDOW(nx_win), icon, NULL);
	g_set_prgname("nexus");
	GdkRGBA bg = {0x10 / 255.0, 0x12 / 255.0, 0x14 / 255.0, 1};
	WebKitWebsiteDataManager *dm = webkit_website_data_manager_new("base-data-directory", data, "base-cache-directory", cache, NULL);
	WebKitWebContext *ctx = webkit_web_context_new_with_website_data_manager(dm);
	// interface sounds play without a click first: controller presses are not "user gestures"
	// to WebKit, so with its default autoplay policy a controller-only start stayed silent
	WebKitWebsitePolicies *pol = webkit_website_policies_new_with_policies("autoplay", WEBKIT_AUTOPLAY_ALLOW, NULL);
	nx_view = WEBKIT_WEB_VIEW(g_object_new(WEBKIT_TYPE_WEB_VIEW, "web-context", ctx, "website-policies", pol, NULL));
	webkit_web_view_set_background_color(nx_view, &bg);
	WebKitSettings *s = webkit_web_view_get_settings(nx_view);
	webkit_settings_set_enable_developer_extras(s, dev);
	webkit_settings_set_media_playback_requires_user_gesture(s, FALSE);
	webkit_settings_set_javascript_can_access_clipboard(s, TRUE);
	webkit_settings_set_enable_back_forward_navigation_gestures(s, FALSE);
	webkit_settings_set_hardware_acceleration_policy(s, WEBKIT_HARDWARE_ACCELERATION_POLICY_ALWAYS);
	gtk_container_add(GTK_CONTAINER(nx_win), GTK_WIDGET(nx_view));
	g_signal_connect(nx_win, "focus-in-event", G_CALLBACK(nx_focus_in), NULL);
	g_signal_connect(nx_win, "focus-out-event", G_CALLBACK(nx_focus_out), NULL);
	g_signal_connect(nx_win, "window-state-event", G_CALLBACK(nx_state), NULL);
	g_signal_connect(nx_win, "delete-event", G_CALLBACK(nx_delete), NULL);
	g_signal_connect(nx_view, "key-press-event", G_CALLBACK(nx_key), NULL);
	g_signal_connect(nx_view, "context-menu", G_CALLBACK(nx_menu), GINT_TO_POINTER(dev));
	g_signal_connect(nx_view, "decide-policy", G_CALLBACK(nx_policy), NULL);
	if (fullscreen) gtk_window_fullscreen(GTK_WINDOW(nx_win));
	gtk_widget_show_all(nx_win);
	gtk_widget_grab_focus(GTK_WIDGET(nx_view));
	return 1;
}
static void nx_load(const char *url) { webkit_web_view_load_uri(nx_view, url); }
static void nx_run(void) { gtk_main(); }
static void nx_quit(void) { gtk_main_quit(); }
static void nx_fullscreen(int on) { if (on) gtk_window_fullscreen(GTK_WINDOW(nx_win)); else gtk_window_unfullscreen(GTK_WINDOW(nx_win)); }
static void nx_iconify(void) { gtk_window_iconify(GTK_WINDOW(nx_win)); }
static void nx_present(void) { gtk_window_present(GTK_WINDOW(nx_win)); gtk_widget_grab_focus(GTK_WIDGET(nx_view)); }
static void nx_size(int *w, int *h) { gtk_window_get_size(GTK_WINDOW(nx_win), w, h); }

// file and folder choosers (folder != 0: a folder); returns a g_malloc'd path or NULL
static char *nx_pick(const char *title, int folder, const char *pattern) {
	GtkWidget *d = gtk_file_chooser_dialog_new(title, GTK_WINDOW(nx_win),
		folder ? GTK_FILE_CHOOSER_ACTION_SELECT_FOLDER : GTK_FILE_CHOOSER_ACTION_OPEN,
		"_Cancel", GTK_RESPONSE_CANCEL, folder ? "_Select" : "_Open", GTK_RESPONSE_ACCEPT, NULL);
	if (!folder && pattern && *pattern) {
		GtkFileFilter *f = gtk_file_filter_new();
		gchar **parts = g_strsplit(pattern, ";", -1);
		for (gchar **p = parts; *p; p++) if (**p) gtk_file_filter_add_pattern(f, *p);
		g_strfreev(parts);
		gtk_file_chooser_set_filter(GTK_FILE_CHOOSER(d), f);
	}
	char *out = NULL;
	if (gtk_dialog_run(GTK_DIALOG(d)) == GTK_RESPONSE_ACCEPT) out = gtk_file_chooser_get_filename(GTK_FILE_CHOOSER(d));
	gtk_widget_destroy(d);
	return out;
}
static void nx_free(char *p) { g_free(p); }
*/
import "C"

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

func init() { runtime.LockOSThread() } // GTK runs on the main thread

type gtkUI struct {
	app        *App
	mu         sync.Mutex
	queue      []func()
	fullscreen bool
	gaming     bool
}

var theGTK *gtkUI

// goOpenURI: a link the interface opens in a new window goes to the desktop's browser, started
// with the desktop's environment (platform_env.go hostEnv), not the AppImage's.
//
//export goOpenURI
func goOpenURI(uri *C.char) {
	u := C.GoString(uri)
	if strings.HasPrefix(u, "https://") || strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "steam://") {
		_ = xdgOpen(u)
	}
}

//export goFocus
func goFocus(in C.int) {
	if u := theGTK; u != nil {
		u.app.Events.Emit("app.focus", in != 0)
	}
}

//export goFullscreenChanged
func goFullscreenChanged(on C.int) {
	if u := theGTK; u != nil {
		u.mu.Lock()
		u.fullscreen = on != 0
		u.mu.Unlock()
	}
}

//export goRunQueue
func goRunQueue() {
	u := theGTK
	if u == nil {
		return
	}
	u.mu.Lock()
	q := u.queue
	u.queue = nil
	u.mu.Unlock()
	for _, fn := range q {
		fn()
	}
}

// goKey: F11 switches full screen (and becomes the start-up choice), as on Windows.
//
//export goKey
func goKey(keyval C.uint) C.int {
	const gdkKeyF11 = 0xffc8
	u := theGTK
	if u == nil || keyval != gdkKeyF11 || u.gaming {
		return 0
	}
	on := !u.IsFullscreen()
	C.nx_fullscreen(gtkBool(on))
	go func() {
		if !u.app.Emu.State().Running {
			_, _ = u.app.Settings.Update(func(g *GlobalSettings) { g.Display.Fullscreen = on })
		}
	}()
	return 1
}

func (u *gtkUI) dispatch(fn func()) {
	u.mu.Lock()
	u.queue = append(u.queue, fn)
	u.mu.Unlock()
	C.nx_wake()
}

func (u *gtkUI) dispatchWait(fn func()) {
	done := make(chan struct{})
	u.dispatch(func() { fn(); close(done) })
	<-done
}

func (u *gtkUI) Native() bool          { return true }
func (u *gtkUI) WindowHandle() uintptr { return 0 }
func (u *gtkUI) IsFullscreen() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.fullscreen || u.gaming
}
func (u *gtkUI) SetFullscreen(on bool) {
	if u.gaming {
		return // gamescope shows NEXUS full screen; there is no window to switch to
	}
	u.dispatch(func() { C.nx_fullscreen(gtkBool(on)) })
}
func (u *gtkUI) Minimize() {
	if !u.gaming {
		u.dispatch(func() { C.nx_iconify() })
	}
}
func (u *gtkUI) Restore() { u.dispatch(func() { C.nx_present() }) }
func (u *gtkUI) Quit()    { u.dispatch(func() { C.nx_quit() }) }

func (u *gtkUI) pick(title string, folder bool, filter string) (string, error) {
	var out string
	u.dispatchWait(func() {
		ct, cf := C.CString(title), C.CString(gtkPattern(filter))
		defer C.free(unsafe.Pointer(ct))
		defer C.free(unsafe.Pointer(cf))
		p := C.nx_pick(ct, gtkBool(folder), cf)
		if p != nil {
			out = C.GoString(p)
			C.nx_free(p)
		}
	})
	if out == "" {
		return "", errors.New("cancelled")
	}
	return out, nil
}

func (u *gtkUI) PickFolder(title string) (string, error) { return u.pick(title, true, "") }
func (u *gtkUI) PickFile(title, filter string) (string, error) {
	return u.pick(title, false, filter)
}

// gtkPattern turns a Windows-style filter ("Keys|*.keys;*.zip" or "*.keys") into "*.keys;*.zip".
func gtkPattern(filter string) string {
	if i := strings.LastIndex(filter, "|"); i >= 0 {
		filter = filter[i+1:]
	}
	return strings.ReplaceAll(filter, "\x00", "")
}

func gtkBool(b bool) C.int {
	if b {
		return 1
	}
	return 0
}

// singleInstance: one NEXUS per data folder. A second start asks the first to come to the
// front (over a socket in the abstract namespace, named after the folder) and ends.
func singleInstance(root string) bool {
	sum := sha256.Sum256([]byte(root))
	name := "@nexus-" + hex.EncodeToString(sum[:8])
	if c, err := net.DialTimeout("unix", name, time.Second); err == nil {
		_, _ = c.Write([]byte("activate\n"))
		c.Close()
		return false
	}
	l, err := net.Listen("unix", name)
	if err != nil {
		return true // cannot tell; start anyway
	}
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
			if u := theGTK; u != nil {
				u.Restore()
			}
		}
	}()
	return true
}

// fatalBox shows an error before or instead of the window: a desktop dialog when one of the
// usual tools is there, and always on standard error.
func fatalBox(text string) {
	fmt.Fprintln(os.Stderr, "NEXUS:", text)
	for _, c := range [][]string{{"zenity", "--error", "--title=NEXUS", "--text=" + text}, {"kdialog", "--title", "NEXUS", "--error", text}} {
		if p, err := exec.LookPath(c[0]); err == nil {
			_ = exec.Command(p, c[1:]...).Run()
			return
		}
	}
}

func runUI(a *App, url string) {
	settings := a.Settings.Get()
	u := &gtkUI{app: a, gaming: gamingMode()}
	theGTK = u
	a.UI = u
	w, h := settings.Display.Width, settings.Display.Height
	if w < 960 || h < 540 {
		w, h = 1280, 800
	}
	full := settings.Display.Fullscreen || u.gaming
	u.fullscreen = full
	icon := filepath.Join(a.Paths.Root, "Resources", "Icons", "app-icon-512.png")
	data := filepath.Join(a.Paths.WebView, "data")
	cache := filepath.Join(a.Paths.WebView, "cache")
	_ = os.MkdirAll(data, 0o700)
	_ = os.MkdirAll(cache, 0o700)
	ct, ci, cd, cc := C.CString("NEXUS"), C.CString(icon), C.CString(data), C.CString(cache)
	defer C.free(unsafe.Pointer(ct))
	defer C.free(unsafe.Pointer(ci))
	defer C.free(unsafe.Pointer(cd))
	defer C.free(unsafe.Pointer(cc))
	dev := os.Getenv("NEXUS_DEVTOOLS") == "1"
	if C.nx_create(ct, C.int(w), C.int(h), gtkBool(full), gtkBool(!u.gaming), ci, cd, cc, gtkBool(dev)) == 0 {
		fatalBox("NEXUS could not open its window: no display was found (DISPLAY / WAYLAND_DISPLAY).")
		return
	}
	a.markStartup("window shown", fmt.Sprintf("GTK, full screen %v, Gaming Mode %v", full, u.gaming))
	cu := C.CString(url)
	C.nx_load(cu)
	C.free(unsafe.Pointer(cu))
	C.nx_run()
	// remember the window size for next time (not in Gaming Mode, where it is the screen)
	if !u.gaming && !u.IsFullscreen() {
		var cw, chh C.int
		C.nx_size(&cw, &chh)
		if cw >= 960 && chh >= 540 {
			_, _ = a.Settings.Update(func(g *GlobalSettings) { g.Display.Width, g.Display.Height = int(cw), int(chh) })
		}
	}
}

func diskSpace(path string) (free, total uint64) {
	var st syscall.Statfs_t
	if syscall.Statfs(path, &st) != nil {
		return 0, 0
	}
	return st.Bavail * uint64(st.Bsize), st.Blocks * uint64(st.Bsize)
}

func setHidden(string)   {}
func clearHidden(string) {}
