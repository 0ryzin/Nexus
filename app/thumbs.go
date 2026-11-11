package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"sync"
)

// Thumbnails for library tiles. Artwork can be large (custom images are kept at full quality,
// downloaded icons are often 1024 px); tiles are drawn at most ~256 css px. Serving a cached,
// downscaled copy keeps the home screen and library fast and their memory use low. The
// original files are never changed. Cache key: source path + size + modification time + width.

var thumbMu sync.Mutex

func (a *App) thumbDir() string { return filepath.Join(a.Paths.Artwork, ".thumbs") }

// Thumbnail returns a cached file of src at most w pixels wide, or src itself when it is already
// small enough or cannot be decoded (formats the standard library cannot read are served as-is).
func (a *App) Thumbnail(src string, w int) string {
	fi, err := os.Stat(src)
	if err != nil || w <= 0 {
		return src
	}
	h := sha1.Sum([]byte(fmt.Sprintf("%s|%d|%d|%d", src, fi.Size(), fi.ModTime().UnixNano(), w)))
	key := hex.EncodeToString(h[:12])
	for _, ext := range []string{".jpg", ".png"} {
		if p := filepath.Join(a.thumbDir(), key+ext); fileExists(p) {
			return p
		}
	}
	thumbMu.Lock() // one decode at a time keeps peak memory low during a library scroll
	defer thumbMu.Unlock()
	f, err := os.Open(src)
	if err != nil {
		return src
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return src
	}
	b := img.Bounds()
	if b.Dx() <= w {
		return src
	}
	nh := max(1, b.Dy()*w/b.Dx())
	out := downscale(img, w, nh)
	var buf bytes.Buffer
	ext := ".jpg"
	if hasAlpha(out) {
		ext = ".png"
		err = (&png.Encoder{CompressionLevel: png.BestSpeed}).Encode(&buf, out)
	} else {
		err = jpeg.Encode(&buf, out, &jpeg.Options{Quality: 90})
	}
	if err != nil {
		return src
	}
	_ = os.MkdirAll(a.thumbDir(), 0o755)
	dst := filepath.Join(a.thumbDir(), key+ext)
	tmp := dst + ".tmp"
	if os.WriteFile(tmp, buf.Bytes(), 0o644) != nil || os.Rename(tmp, dst) != nil {
		return src
	}
	return dst
}

// downscale: area-averaging (box) filter, good quality for large reduction factors.
func downscale(src image.Image, w, h int) *image.NRGBA {
	sb := src.Bounds()
	rgba := image.NewNRGBA(image.Rect(0, 0, sb.Dx(), sb.Dy()))
	draw.Draw(rgba, rgba.Bounds(), src, sb.Min, draw.Src)
	sw, sh := sb.Dx(), sb.Dy()
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		y0, y1 := y*sh/h, max(y*sh/h+1, (y+1)*sh/h)
		for x := 0; x < w; x++ {
			x0, x1 := x*sw/w, max(x*sw/w+1, (x+1)*sw/w)
			var r, g, bl, al, n uint64
			for yy := y0; yy < y1; yy++ {
				row := rgba.Pix[yy*rgba.Stride:]
				for xx := x0; xx < x1; xx++ {
					p := row[xx*4 : xx*4+4]
					a := uint64(p[3])
					r += uint64(p[0]) * a
					g += uint64(p[1]) * a
					bl += uint64(p[2]) * a
					al += a
					n++
				}
			}
			o := out.Pix[y*out.Stride+x*4:]
			if al > 0 {
				o[0], o[1], o[2] = uint8(r/al), uint8(g/al), uint8(bl/al)
			}
			o[3] = uint8(al / n)
		}
	}
	return out
}

func hasAlpha(img *image.NRGBA) bool {
	for i := 3; i < len(img.Pix); i += 4 {
		if img.Pix[i] != 255 {
			return true
		}
	}
	return false
}
