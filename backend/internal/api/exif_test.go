package api

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
)

// withOrientation inserts an EXIF APP1 segment with the Orientation tag after SOI.
func withOrientation(t *testing.T, jpg []byte, o uint16, bo binary.ByteOrder, mark string) []byte {
	t.Helper()
	var tiff bytes.Buffer
	tiff.WriteString(mark)
	binary.Write(&tiff, bo, uint16(42))
	binary.Write(&tiff, bo, uint32(8))
	binary.Write(&tiff, bo, uint16(1))           // one entry
	binary.Write(&tiff, bo, []uint16{0x0112, 3}) // Orientation, SHORT
	binary.Write(&tiff, bo, uint32(1))           // count
	binary.Write(&tiff, bo, []uint16{o, 0})      // value, padding
	binary.Write(&tiff, bo, uint32(0))           // no next IFD
	seg := append([]byte("Exif\x00\x00"), tiff.Bytes()...)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(seg)+2))
	out = append(out, seg...)
	return append(out, jpg[2:]...)
}

func TestOrientation(t *testing.T) {
	var buf bytes.Buffer
	jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 2)), nil)
	if got := jpegOrientation(buf.Bytes()); got != 1 {
		t.Fatalf("no exif: %d", got)
	}
	for _, c := range []struct {
		bo   binary.ByteOrder
		mark string
	}{{binary.LittleEndian, "II"}, {binary.BigEndian, "MM"}} {
		for o := uint16(1); o <= 8; o++ {
			if got := jpegOrientation(withOrientation(t, buf.Bytes(), o, c.bo, c.mark)); got != int(o) {
				t.Errorf("%s orientation %d read as %d", c.mark, o, got)
			}
		}
	}

	// A 3×2 image with a red top-left pixel; where it lands for each orientation.
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	red := color.RGBA{255, 0, 0, 255}
	src.Set(0, 0, red)
	want := map[int]image.Point{1: {0, 0}, 2: {2, 0}, 3: {2, 1}, 4: {0, 1}, 5: {0, 0}, 6: {1, 0}, 7: {1, 2}, 8: {0, 2}}
	for o, p := range want {
		dst := orient(src, o)
		if o >= 5 && (dst.Bounds().Dx() != 2 || dst.Bounds().Dy() != 3) {
			t.Errorf("orientation %d: size %v", o, dst.Bounds())
		}
		if r, _, _, _ := dst.At(p.X, p.Y).RGBA(); r>>8 != 255 {
			t.Errorf("orientation %d: red pixel not at %v", o, p)
		}
	}
}
