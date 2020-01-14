package kepub

import (
	"io/ioutil"
	"os"
	"path/filepath"
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

// TODO: test unpacking/packing (especially things like path separators in the packed epub)
