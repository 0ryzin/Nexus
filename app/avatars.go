package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Profile icon library: System/Avatars/
//   firmware/*.png                       console icons read from the user's firmware
//   packs/<category>/characters|backgrounds|frames|mii/*.png   imported icon packs
//   packs/<category>/*.png               simple packs (all images are characters)
// NEXUS ships no Nintendo artwork; packs are imported from files the user supplies
// (for example a local copy of an icon collection they are entitled to use).

type AvatarCategory struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Source      string   `json:"source"` // firmware | pack
	Characters  []string `json:"characters"`
	Backgrounds []string `json:"backgrounds"`
	Frames      []string `json:"frames"`
}

func (a *App) avatarRoot() string { return filepath.Join(a.Paths.System, "Avatars") }
func (a *App) packRoot() string   { return filepath.Join(a.avatarRoot(), "packs") }

var avatarImageExt = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

// iconGroup derives a series name from an icon file name, e.g. "chr_mario_01.png" -> "mario".
func iconGroup(name string) string {
	n := strings.ToLower(strings.TrimSuffix(name, filepath.Ext(name)))
	for _, p := range []string{"chr_", "chara_", "icon_", "avatar_"} {
		n = strings.TrimPrefix(n, p)
	}
	parts := strings.FieldsFunc(n, func(r rune) bool { return r == '_' || r == '-' || unicode.IsDigit(r) })
	if len(parts) == 0 {
		return "console"
	}
	return parts[0]
}

func humanize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	words := strings.Fields(s)
	for i, w := range words {
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}

func listImages(dir, urlPrefix string) []string {
	entries, _ := os.ReadDir(dir)
	var out []string
	for _, e := range entries {
		if !e.IsDir() && avatarImageExt[strings.ToLower(filepath.Ext(e.Name()))] {
			out = append(out, urlPrefix+"/"+url.PathEscape(e.Name()))
		}
	}
	sort.Strings(out)
	return out
}

func (s *AvatarSet) List() []string {
	entries, _ := os.ReadDir(filepath.Join(s.app.avatarDir(), "characters"))
	var out []string
	for _, e := range entries {
		if strings.HasSuffix(strings.ToLower(e.Name()), ".png") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// Categories indexes everything available for the icon picker.
func (s *AvatarSet) Categories() []AvatarCategory {
	var cats []AvatarCategory
	fwDir := s.app.avatarDir()
	if fw := listImages(filepath.Join(fwDir, "characters"), "/avatarlib/firmware/characters"); len(fw) > 0 {
		var bgs []string
		subs, _ := os.ReadDir(fwDir)
		for _, d := range subs {
			if d.IsDir() && d.Name() != "characters" {
				bgs = append(bgs, listImages(filepath.Join(fwDir, d.Name()), "/avatarlib/firmware/"+d.Name())...)
			}
		}
		if bgs == nil {
			bgs = []string{}
		}
		cats = append(cats, AvatarCategory{ID: "firmware", Name: "All console icons", Source: "firmware", Characters: fw, Backgrounds: bgs, Frames: []string{}})
		// group by file-name prefix when the firmware names its images by series
		groups := map[string][]string{}
		for _, u := range fw {
			groups[iconGroup(pathBase(u))] = append(groups[iconGroup(pathBase(u))], u)
		}
		if len(groups) > 1 && len(groups) < len(fw) {
			keys := make([]string, 0, len(groups))
			for k := range groups {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				cats = append(cats, AvatarCategory{ID: "firmware-" + k, Name: humanize(k), Source: "firmware", Characters: groups[k], Backgrounds: bgs, Frames: []string{}})
			}
		}
	}
	type packSrc struct{ root, url, source string }
	for _, ps := range []packSrc{
		{filepath.Join(s.app.Paths.Resources, "ProfileIcons"), "/avatarres", "bundled"}, // shipped with NEXUS
		{s.app.packRoot(), "/avatarlib/packs", "pack"},                                  // imported by the user
	} {
		packs, _ := os.ReadDir(ps.root)
		for _, p := range packs {
			if !p.IsDir() {
				continue
			}
			dir := filepath.Join(ps.root, p.Name())
			prefix := ps.url + "/" + url.PathEscape(p.Name())
			name := p.Name()
			if !strings.ContainsAny(name, " ") && strings.ToLower(name) == name {
				name = humanize(name) // "super-mario" -> "Super Mario"; "The Legend of Zelda" stays as written
			}
			c := AvatarCategory{ID: ps.source + "-" + p.Name(), Name: name, Source: ps.source,
				Characters:  append(listImages(filepath.Join(dir, "characters"), prefix+"/characters"), listImages(filepath.Join(dir, "mii"), prefix+"/mii")...),
				Backgrounds: listImages(filepath.Join(dir, "backgrounds"), prefix+"/backgrounds"),
				Frames:      listImages(filepath.Join(dir, "frames"), prefix+"/frames")}
			c.Characters = append(c.Characters, listImages(dir, prefix)...)
			if c.Backgrounds == nil {
				c.Backgrounds = []string{}
			}
			if c.Frames == nil {
				c.Frames = []string{}
			}
			if len(c.Characters)+len(c.Backgrounds)+len(c.Frames) > 0 {
				cats = append(cats, c)
			}
		}
	}
	// optional order.txt (one category folder name per line) in each pack root
	order := map[string]int{}
	for _, root := range []string{filepath.Join(s.app.Paths.Resources, "ProfileIcons"), s.app.packRoot()} {
		if b, err := os.ReadFile(filepath.Join(root, "order.txt")); err == nil {
			for i, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
				if l = strings.TrimSpace(l); l != "" {
					order[strings.ToLower(l)] = i + 1
				}
			}
		}
	}
	rankName := func(c AvatarCategory) int {
		if o, ok := order[strings.ToLower(strings.TrimPrefix(strings.TrimPrefix(c.ID, "bundled-"), "pack-"))]; ok {
			return o
		}
		return 1000
	}
	sort.SliceStable(cats, func(i, j int) bool {
		rank := func(c AvatarCategory) int {
			if c.ID == "firmware" {
				return -1
			}
			switch c.Source {
			case "firmware":
				return 0
			case "bundled":
				return 1
			}
			return 2
		}
		if rank(cats[i]) != rank(cats[j]) {
			return rank(cats[i]) < rank(cats[j])
		}
		if rankName(cats[i]) != rankName(cats[j]) {
			return rankName(cats[i]) < rankName(cats[j])
		}
		return cats[i].Name < cats[j].Name
	})
	return cats
}

var layerDirs = map[string]bool{"characters": true, "backgrounds": true, "frames": true, "mii": true}

func hasLayerDirs(dir string) bool {
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.IsDir() && layerDirs[strings.ToLower(e.Name())] {
			return true
		}
	}
	return false
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "icons"
	}
	return out
}

// copyImages copies image files (non-recursive) from src to dst, skipping existing ones.
func copyImages(src, dst string) int {
	entries, _ := os.ReadDir(src)
	n := 0
	for _, e := range entries {
		if e.IsDir() || !avatarImageExt[strings.ToLower(filepath.Ext(e.Name()))] {
			continue
		}
		if fi, err := e.Info(); err != nil || fi.Size() > 8<<20 {
			continue
		}
		target := filepath.Join(dst, e.Name())
		if fileExists(target) {
			continue
		}
		_ = os.MkdirAll(dst, 0o755)
		if copyFile(filepath.Join(src, e.Name()), target) == nil {
			n++
		}
	}
	return n
}

func importCategory(src, dst string) int {
	n := 0
	if hasLayerDirs(src) {
		entries, _ := os.ReadDir(src)
		for _, e := range entries {
			if e.IsDir() && layerDirs[strings.ToLower(e.Name())] {
				n += copyImages(filepath.Join(src, e.Name()), filepath.Join(dst, strings.ToLower(e.Name())))
			}
		}
	}
	return n + copyImages(src, dst)
}

// ImportFolder imports an icon collection. Recognised layouts:
//
//	<root>/<category>/characters|backgrounds|frames/*.png   (e.g. the nso-icons layout)
//	<root>/characters|backgrounds|frames/*.png             (one category)
//	<root>/<category>/*.png  and  <root>/*.png              (plain images)
func (s *AvatarSet) ImportFolder(src string) (int, []string, error) {
	if !dirExists(src) {
		return 0, nil, fmt.Errorf("Folder not found: %s", src)
	}
	root := s.app.packRoot()
	if strings.HasPrefix(strings.ToLower(filepath.Clean(src)), strings.ToLower(filepath.Clean(s.app.avatarRoot()))) {
		return 0, nil, errors.New("That folder is already inside NEXUS.")
	}
	total := 0
	var cats []string
	if hasLayerDirs(src) {
		id := slug(filepath.Base(src))
		total += importCategory(src, filepath.Join(root, id))
		cats = append(cats, id)
	} else {
		entries, _ := os.ReadDir(src)
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
				continue
			}
			id := slug(e.Name())
			if n := importCategory(filepath.Join(src, e.Name()), filepath.Join(root, id)); n > 0 {
				total += n
				cats = append(cats, id)
			}
		}
		if n := copyImages(src, filepath.Join(root, slug(filepath.Base(src)))); n > 0 {
			total += n
			cats = append(cats, slug(filepath.Base(src)))
		}
	}
	if total == 0 {
		return 0, nil, errors.New("No new PNG, JPG or WEBP icons were found in that folder.")
	}
	s.app.Log.Printf("avatars: imported %d icons into %d categories from %s", total, len(cats), src)
	return total, cats, nil
}

// ImportZip extracts an uploaded .zip into a temporary folder and imports it.
func (s *AvatarSet) ImportZip(r io.Reader, name string) (int, []string, error) {
	tmpDir := filepath.Join(s.app.avatarRoot(), ".import")
	_ = os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return 0, nil, err
	}
	zpath := filepath.Join(tmpDir, "upload.zip")
	f, err := os.Create(zpath)
	if err != nil {
		return 0, nil, err
	}
	if _, err := io.Copy(f, io.LimitReader(r, 4<<30)); err != nil {
		f.Close()
		return 0, nil, err
	}
	f.Close()
	zr, err := zip.OpenReader(zpath)
	if err != nil {
		return 0, nil, errors.New("That file is not a valid .zip archive.")
	}
	base := slug(strings.TrimSuffix(name, filepath.Ext(name)))
	out := filepath.Join(tmpDir, base)
	for _, zf := range zr.File {
		n := filepath.ToSlash(zf.Name)
		if zf.FileInfo().IsDir() || strings.Contains(n, "..") || strings.HasPrefix(n, "/") || zf.UncompressedSize64 > 8<<20 ||
			!avatarImageExt[strings.ToLower(filepath.Ext(n))] || strings.HasPrefix(pathBase(n), ".") {
			continue
		}
		target := filepath.Join(out, filepath.FromSlash(n))
		_ = os.MkdirAll(filepath.Dir(target), 0o755)
		rc, err := zf.Open()
		if err != nil {
			continue
		}
		w, err := os.Create(target)
		if err == nil {
			_, _ = io.Copy(w, rc)
			w.Close()
		}
		rc.Close()
	}
	zr.Close()
	// a zip usually wraps everything in one top folder (e.g. "nso-icons-main")
	src := out
	for {
		entries, _ := os.ReadDir(src)
		if len(entries) == 1 && entries[0].IsDir() && !layerDirs[strings.ToLower(entries[0].Name())] {
			src = filepath.Join(src, entries[0].Name())
			continue
		}
		break
	}
	return s.ImportFolder(src)
}

func (s *AvatarSet) RemoveCategory(id string) error {
	if id == "firmware" {
		return os.RemoveAll(s.app.avatarDir())
	}
	dir := filepath.Join(s.app.packRoot(), filepath.Base(strings.TrimPrefix(id, "pack-")))
	if !dirExists(dir) {
		return errors.New("Category not found.")
	}
	return os.RemoveAll(dir)
}
