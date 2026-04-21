package jxr

import (
	"archive/zip"
	"crypto/sha256"
	"io"
	"path/filepath"
	"testing"

	"github.com/kaikozlov/kindle-koplugin/internal/testutil"
)

func TestParseContainerFixture(t *testing.T) {
	data := readFixtureJXR(t, "resource/rsrc3S4..jxr")

	container, err := ParseContainer(data)
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}

	if container.PixelFormat != "24c3dd6f-034e-fe4b-b185-3d77768dc908" {
		t.Fatalf("ParseContainer() pixel format = %q", container.PixelFormat)
	}
	if container.Width != 921 || container.Height != 522 {
		t.Fatalf("ParseContainer() size = %dx%d", container.Width, container.Height)
	}
	if len(container.ImageData) != 22626 {
		t.Fatalf("ParseContainer() image data length = %d", len(container.ImageData))
	}
}

func TestParseHeaderFixtureNarrowSubset(t *testing.T) {
	data := readFixtureJXR(t, "resource/rsrc3S4..jxr")

	container, err := ParseContainer(data)
	if err != nil {
		t.Fatalf("ParseContainer() error = %v", err)
	}
	header, err := ParseHeader(container.ImageData)
	if err != nil {
		t.Fatalf("ParseHeader() error = %v", err)
	}

	if !header.SupportsFixtureGraySubset() {
		t.Fatalf("fixture header no longer matches the supported grayscale subset: %+v", header)
	}
	if header.ImageWidth != 921 || header.ImageHeight != 522 {
		t.Fatalf("header image size = %dx%d", header.ImageWidth, header.ImageHeight)
	}
	if header.ExtraRight != 7 || header.ExtraBottom != 6 {
		t.Fatalf("header padding = right:%d bottom:%d", header.ExtraRight, header.ExtraBottom)
	}
	if header.MBWidth != 58 || header.MBHeight != 33 {
		t.Fatalf("header macroblocks = %dx%d", header.MBWidth, header.MBHeight)
	}
}

func TestAllMartyrJXRImagesMatchNarrowSubset(t *testing.T) {
	archivePath := filepath.Join("..", "..", "REFERENCE", "martyr_unpack.zip")
	testutil.SkipIfMissing(t, archivePath)
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	expected := map[string][2]int{
		"resource/rsrc3S1..jxr": {1094, 1920},
		"resource/rsrc3S2..jxr": {331, 337},
		"resource/rsrc3S3..jxr": {1875, 202},
		"resource/rsrc3S4..jxr": {921, 522},
		"resource/rsrc3S5..jxr": {1117, 894},
	}
	for _, file := range archive.File {
		want, ok := expected[file.Name]
		if !ok {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("%s Open() error = %v", file.Name, err)
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s ReadAll() error = %v", file.Name, err)
		}
		container, err := ParseContainer(data)
		if err != nil {
			t.Fatalf("%s ParseContainer() error = %v", file.Name, err)
		}
		header, err := ParseHeader(container.ImageData)
		if err != nil {
			t.Fatalf("%s ParseHeader() error = %v", file.Name, err)
		}
		if !header.SupportsFixtureGraySubset() {
			t.Fatalf("%s header no longer matches the supported grayscale subset", file.Name)
		}
		if container.Width != want[0] || container.Height != want[1] {
			t.Fatalf("%s size = %dx%d, want %dx%d", file.Name, container.Width, container.Height, want[0], want[1])
		}
		delete(expected, file.Name)
	}
	if len(expected) != 0 {
		t.Fatalf("missing expected JXR fixtures: %v", expected)
	}
}

func TestDecodeGray8MatchesPythonJXRBaseline(t *testing.T) {
	archivePath := filepath.Join("..", "..", "REFERENCE", "martyr_unpack.zip")
	testutil.SkipIfMissing(t, archivePath)
	rawArchive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("OpenReader(raw) error = %v", err)
	}
	defer rawArchive.Close()

	expected := map[string]struct {
		width  int
		height int
		hash   string
	}{
		"resource/rsrc3S1..jxr": {1094, 1920, "298647df94eb6398df970e42523a6218d386e657d59ad076e528e652dc217f25"},
		"resource/rsrc3S2..jxr": {331, 337, "b0b27aa3ae280e97592fe4dc343495280362c21f02a1be11769e3ef637ed9725"},
		"resource/rsrc3S3..jxr": {1875, 202, "35ca14ffde11435ba8d9dee1631f0c491b48fff1e666fbc5e9ed17d5e32918e9"},
		"resource/rsrc3S4..jxr": {921, 522, "6d94429a3ac40d075bad0a38d9d38477a5fa83245c13cf7aa8f27fcdfff5f618"},
		"resource/rsrc3S5..jxr": {1117, 894, "303c7917c5a5941aef1508113e0002007e8d00ed2c0665ae1cae85a601d26684"},
	}

	for _, file := range rawArchive.File {
		want, ok := expected[file.Name]
		if !ok {
			continue
		}
		rawData := readZipBytes(t, file)
		got, err := DecodeGray8(rawData)
		if err != nil {
			t.Fatalf("%s DecodeGray8() error = %v", file.Name, err)
		}

		if got.Bounds().Dx() != want.width || got.Bounds().Dy() != want.height {
			t.Fatalf("%s decoded size = %dx%d, want %dx%d", file.Name, got.Bounds().Dx(), got.Bounds().Dy(), want.width, want.height)
		}
		gotHash := sha256.Sum256(got.Pix)
		if gotHashHex := stringHex(gotHash[:]); gotHashHex != want.hash {
			t.Fatalf("%s decoded pixel hash = %s, want %s", file.Name, gotHashHex, want.hash)
		}
		delete(expected, file.Name)
	}

	if len(expected) != 0 {
		t.Fatalf("missing decode fixtures: %v", expected)
	}
}

func readFixtureJXR(t *testing.T, name string) []byte {
	t.Helper()

	archivePath := filepath.Join("..", "..", "REFERENCE", "martyr_unpack.zip")
	testutil.SkipIfMissing(t, archivePath)
	archive, err := zip.OpenReader(archivePath)
	if err != nil {
		t.Fatalf("OpenReader() error = %v", err)
	}
	defer archive.Close()

	for _, file := range archive.File {
		if file.Name != name {
			continue
		}
		rc, err := file.Open()
		if err != nil {
			t.Fatalf("%s Open() error = %v", name, err)
		}
		defer rc.Close()
		data, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("%s ReadAll() error = %v", name, err)
		}
		return data
	}

	t.Fatalf("fixture %s not found", name)
	return nil
}

func readZipBytes(t *testing.T, file *zip.File) []byte {
	t.Helper()

	rc, err := file.Open()
	if err != nil {
		t.Fatalf("%s Open() error = %v", file.Name, err)
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("%s ReadAll() error = %v", file.Name, err)
	}
	return data
}

func stringHex(data []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, len(data)*2)
	for i, b := range data {
		out[i*2] = hexdigits[b>>4]
		out[i*2+1] = hexdigits[b&0x0f]
	}
	return string(out)
}
