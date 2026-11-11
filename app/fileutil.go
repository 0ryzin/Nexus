package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

func parseHexColor(s string) (color.RGBA, bool) {
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return color.RGBA{}, false
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return color.RGBA{}, false
	}
	return color.RGBA{uint8(v >> 16), uint8(v >> 8), uint8(v), 255}, true
}

// scaleInto draws src over dst (nearest/box sampling, alpha-blended).
func scaleInto(dst *image.RGBA, src image.Image) {
	sb, db := src.Bounds(), dst.Bounds()
	for y := 0; y < db.Dy(); y++ {
		for x := 0; x < db.Dx(); x++ {
			sx := sb.Min.X + x*sb.Dx()/db.Dx()
			sy := sb.Min.Y + y*sb.Dy()/db.Dy()
			r, g, b, a := src.At(sx, sy).RGBA()
			if a == 0 {
				continue
			}
			o := dst.RGBAAt(x, y)
			ia := 0xffff - a
			dst.SetRGBA(x, y, color.RGBA{
				uint8((r + uint32(o.R)*0x101*ia/0xffff) >> 8),
				uint8((g + uint32(o.G)*0x101*ia/0xffff) >> 8),
				uint8((b + uint32(o.B)*0x101*ia/0xffff) >> 8), 255})
		}
	}
}

func copyFileOverwrite(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

var verNum = regexp.MustCompile(`\d+`)

// CompareVersions compares "1.2.0", "v2.0.1", "build-150" style versions numerically.
func CompareVersions(a, b string) int {
	na, nb := verNum.FindAllString(a, -1), verNum.FindAllString(b, -1)
	for i := 0; i < len(na) || i < len(nb); i++ {
		var x, y int64
		if i < len(na) {
			x, _ = strconv.ParseInt(na[i], 10, 64)
		}
		if i < len(nb) {
			y, _ = strconv.ParseInt(nb[i], 10, 64)
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func sha256File(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// httpDownload saves url to dst and returns its sha256.
func httpDownload(url, dst string, progress func(int64)) (string, error) {
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", "NEXUS/"+AppVersion)
	resp, err := (&http.Client{Timeout: 30 * time.Minute}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("server returned %s", resp.Status)
	}
	f, err := os.Create(dst)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	var n int64
	buf := make([]byte, 256<<10)
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			if _, werr := f.Write(buf[:k]); werr != nil {
				f.Close()
				return "", werr
			}
			h.Write(buf[:k])
			n += int64(k)
			if progress != nil {
				progress(n)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return "", rerr
		}
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractZipTo extracts every file (verifying CRCs), refusing path traversal.
func extractZipTo(zipPath, dst string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("not a valid zip archive: %w", err)
	}
	defer zr.Close()
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if name == "" || strings.Contains(name, "..") || strings.HasPrefix(name, "/") || strings.Contains(name, ":") {
			continue
		}
		target := filepath.Join(dst, filepath.FromSlash(name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		w, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(w, rc) // zip.ErrChecksum on corruption
		rc.Close()
		if cerr := w.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

// checkPE confirms path is a 64-bit (x86-64) Windows executable or DLL.
func checkPE(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	hdr := make([]byte, 4096)
	n, _ := io.ReadFull(f, hdr)
	hdr = hdr[:n]
	if len(hdr) < 0x40 || hdr[0] != 'M' || hdr[1] != 'Z' {
		return errors.New("not a Windows program file")
	}
	off := int(binary.LittleEndian.Uint32(hdr[0x3c:]))
	if off+6 > len(hdr) || string(hdr[off:off+4]) != "PE\x00\x00" {
		return errors.New("damaged program header")
	}
	if m := binary.LittleEndian.Uint16(hdr[off+4:]); m != 0x8664 {
		return fmt.Errorf("not a 64-bit x86 program (machine %#x)", m)
	}
	return nil
}

// sameDir: two paths name the same folder.
func sameDir(a, b string) bool {
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return strings.EqualFold(filepath.Clean(aa), filepath.Clean(bb))
}
