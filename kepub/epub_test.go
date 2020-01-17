package kepub

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestFindOPF(t *testing.T) {
	td, err := ioutil.TempDir("", "kepubify-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	if err := os.Mkdir(filepath.Join(td, "META-INF"), 0755); err != nil {
		panic(err)
	}

	if _, err := FindOPF(td); err == nil {
		t.Error("expected error when container.xml not present")
	}

	if err := ioutil.WriteFile(filepath.Join(td, "META-INF", "container.xml"), []byte(`<?xml version="1.0"`), 0644); err != nil {
		panic(err)
	}

	if _, err := FindOPF(td); err == nil {
		t.Error("expected error when container.xml invalid")
	}

	if err := ioutil.WriteFile(filepath.Join(td, "META-INF", "container.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
	<rootfiles>
	</rootfiles>
</container>`), 0644); err != nil {
		panic(err)
	}

	if _, err := FindOPF(td); err == nil {
		t.Error("expected error when rootfile not present")
	}

	if err := ioutil.WriteFile(filepath.Join(td, "META-INF", "container.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container">
	<rootfiles>
		<rootfile full-path="content.opf" media-type="application/oebps-package+xml"/>
	</rootfiles>
</container>`), 0644); err != nil {
		panic(err)
	}

	if opf, err := FindOPF(td); err != nil {
		t.Error("expected no error for valid container.xml")
	} else if exp := filepath.Join(td, "content.opf"); opf != exp {
		t.Errorf("expected %#v, got %#v", exp, opf)
	}
}

func TestEPUBUnpackPack(t *testing.T) {
	if err := UnpackEPUB("", ""); err == nil {
		t.Errorf("expected error when unpacking to or from an empty string")
	}

	td, err := ioutil.TempDir("", "kepubify-test")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(td)

	oepub := "test.epub"
	if err := UnpackEPUB(oepub, td); err == nil {
		t.Errorf("expected error when unpacking to existing dir")
	}

	dir := filepath.Join(td, "epub")
	if err := UnpackEPUB(oepub, dir); err != nil {
		t.Fatalf("unexpected error unpacking epub: %v", err)
	}

	for _, f := range []string{
		"/",
		"/mimetype",
		"/OEBPS/",
		"/OEBPS/text2.xhtml",
		"/OEBPS/cover.jpg",
		"/OEBPS/toc.ncx",
		"/OEBPS/style.css",
		"/OEBPS/text1.xhtml",
		"/OEBPS/nav.xhtml",
		"/OEBPS/cover.xhtml",
		"/OEBPS/content.opf",
		"/META-INF/",
		"/META-INF/container.xml",
	} {
		fi, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			t.Fatalf("expected %s to exist in unpacked dir", f)
		} else if fi.IsDir() != strings.HasSuffix(f, "/") {
			t.Fatalf("wrong type (dir/file) for %s", f)
		}
	}

	pepub := filepath.Join(td, "packed.epub")
	if err := PackEPUB(dir, pepub); err != nil {
		t.Fatalf("unexpected error packing epub: %v", err)
	}

	if err := PackEPUB(dir, pepub); err != nil {
		t.Fatalf("unexpected error packing epub over existing file: %v", err)
	}

	if _, err := os.Stat(pepub); err != nil {
		t.Fatalf("packed epub %s doesn't exist", pepub)
	}

	oh := zipHash(oepub)
	nh := zipHash(pepub)

	if oh != nh {
		t.Fatalf("content hashes don't match")
	}

	zr, err := zip.OpenReader(pepub)
	if err != nil {
		t.Fatalf("error opening packed epub: %v", err)
	}
	defer zr.Close()

	if zr.File[0].Name != "mimetype" {
		t.Errorf("packed epub: first entry not mimetype")
	}

	if zr.File[0].Method != zip.Store {
		t.Errorf("packed epub: first entry not uncompressed")
	}

	for _, f := range zr.File {
		if strings.Contains(f.Name, `\`) {
			t.Errorf("packed epub: wrong path separator (%#v)!", f.Name)
		}
		if strings.HasPrefix(f.Name, `/`) {
			t.Errorf("packed epub: path starts with / (%#v)", f.Name)
		}
		if filepath.Clean(filepath.FromSlash(f.Name)) != filepath.FromSlash(f.Name) {
			t.Errorf("packed epub: unclean path (%#v)", f.Name)
		}
	}
}

func zipHash(path string) string {
	zr, err := zip.OpenReader(path)
	if err != nil {
		panic(err)
	}
	defer zr.Close()

	sort.Slice(zr.File, func(i, j int) bool {
		return zr.File[i].Name < zr.File[j].Name
	})

	ss := sha256.New()
	for _, f := range zr.File {
		if err := binary.Write(ss, binary.LittleEndian, f.CRC32); err != nil {
			panic(err)
		}
	}

	return hex.EncodeToString(ss.Sum(nil))
}
