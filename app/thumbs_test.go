package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestThumbnail(t *testing.T) {
	app, _ := NewApp(t.TempDir())
	src := filepath.Join(t.TempDir(), "big.png")
	img := image.NewNRGBA(image.Rect(0, 0, 1600, 1600))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = 200, 20, 20, 255
	}
	f, _ := os.Create(src)
	png.Encode(f, img)
	f.Close()
	th := app.Thumbnail(src, 320)
	if th == src {
		t.Fatal("no thumbnail made")
	}
	g, _ := os.Open(th)
	defer g.Close()
	ti, _, err := image.Decode(g)
	if err != nil || ti.Bounds().Dx() != 320 || ti.Bounds().Dy() != 320 {
		t.Fatalf("thumb %v %v", err, ti.Bounds())
	}
	if c := color.NRGBAModel.Convert(ti.At(10, 10)).(color.NRGBA); c.R < 190 || c.G > 35 {
		t.Fatalf("colour changed: %v", c)
	}
	if app.Thumbnail(src, 320) != th {
		t.Fatal("thumbnail not cached")
	}
	if app.Thumbnail(src, 4000) != src {
		t.Fatal("small enough images are served as they are")
	}
	if fi, _ := os.Stat(src); fi.Size() == 0 {
		t.Fatal("original changed")
	}
}
