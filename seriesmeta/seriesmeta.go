package main

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	"github.com/geek1011/koboutils/kobo"
	_ "github.com/mattn/go-sqlite3"
	"github.com/mattn/go-zglob"
	"github.com/spf13/pflag"
)

var version = "dev"

func main() {
	help := pflag.BoolP("help", "h", false, "Show this help message")
	pflag.Parse()

	if *help || pflag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: seriesmeta [OPTIONS] [KOBO_PATH]\n\nVersion:\n  seriesmeta %s\n\nOptions:\n", version)
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArguments:\n  KOBO_PATH is the path to the Kobo eReader. If not specified, seriesmeta will try to automatically detect the Kobo.\n")
		os.Exit(2)
	}

	fmt.Println("Finding kobo")
	var kp string
	if pflag.NArg() == 1 {
		kp = pflag.Arg(0)
	} else {
		kobos, err := kobo.Find()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Could not automatically detect a Kobo eReader: %v.\n", err)
			os.Exit(1)
		} else if len(kobos) == 0 {
			fmt.Fprintf(os.Stderr, "Could not automatically detect a Kobo eReader.\n")
			os.Exit(1)
		}
		kp = kobos[0]
	}

	fmt.Println("Opening kobo")
	k, err := OpenKobo(kp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not open Kobo eReader: %v.\n", err)
		os.Exit(1)
	}

	fmt.Println("Updating metadata")
	var nt, nu, ne, nn int
	if err := k.UpdateSeries(func(filename string, i, total int, series string, index float64, err error) {
		fmt.Printf("[%3d/%3d] %-40s %s\n", i+1, total, fmt.Sprintf("(%-34s %3v)", series, index), filename)
		if err != nil {
			fmt.Printf("--------- Error: %v\n", err)
			ne++
		} else if series == "" {
			nn++
		} else {
			nu++
		}
		nt = total
	}); err != nil {
		fmt.Fprintf(os.Stderr, "Could not update metadata: %v.\n", err)
		k.Close()
		os.Exit(1)
	}

	fmt.Printf("%d total: %d updated, %d errored, %d without metadata\n", nt, nu, ne, nn)
	if ne > 0 {
		k.Close()
		os.Exit(1)
	}
	k.Close()
}

// Kobo is a Kobo eReader.
type Kobo struct {
	Path string
	DB   *sql.DB
}

// OpenKobo opens a Kobo eReader device and the database.
func OpenKobo(path string) (*Kobo, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	} else if _, err = os.Stat(filepath.Join(path, ".kobo")); err != nil {
		return nil, fmt.Errorf("could not access .kobo directory, is this a Kobo eReader: %v", err)
	}

	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite3", filepath.Join(path, ".kobo", "KoboReader.sqlite"))
	if err != nil {
		return nil, fmt.Errorf("could not open KoboReader.sqlite: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("could not open KoboReader.sqlite: %w", err)
	}

	return &Kobo{path, db}, nil
}

// Close closes the reader and the database.
func (k *Kobo) Close() error {
	return k.DB.Close()
}

// UpdateSeries updates the series metadata for all epub books on the device. All
// errors from individual books are returned through the log callback.
func (k *Kobo) UpdateSeries(log func(filename string, i, total int, series string, index float64, err error)) error {
	epubs, err := zglob.Glob(filepath.Join(k.Path, "**/*.epub"))
	if err != nil {
		return err
	}

	relEpubs := make([]string, len(epubs))
	for i, epub := range epubs {
		relEpubs[i], err = filepath.Rel(k.Path, epub)
		if err != nil {
			return fmt.Errorf("could not resolve relative path to %#v: %w", epub, err)
		}
	}

	tx, err := k.DB.Begin()
	if err != nil {
		return fmt.Errorf("could not begin db transaction: %w", err)
	}

	for i, epub := range epubs {
		relEpub := relEpubs[i]
		iid := contentIDToImageID(pathToContentID(relEpub))

		series, index, err := readEPUBSeriesInfo(epub)
		if err != nil {
			log(relEpub, i, len(epubs), series, index, err)
			continue
		}

		if res, err := tx.Exec(
			"UPDATE content SET Series=?, SeriesNumber=? WHERE ImageID=?",
			sql.NullString{String: series, Valid: len(series) > 0},
			sql.NullString{String: strconv.FormatFloat(index, 'f', -1, 64), Valid: index > 0},
			iid,
		); err != nil {
			log(relEpub, i, len(epubs), series, index, err)
			continue
		} else if ra, _ := res.RowsAffected(); ra == 0 {
			log(relEpub, i, len(epubs), series, index, fmt.Errorf("no entry in database for book with ImageID %#v", iid))
			continue
		}

		log(relEpub, i, len(epubs), series, index, nil)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit db transaction: %v", err)
	}

	return nil
}

func pathToContentID(relpath string) string {
	return fmt.Sprintf("file:///mnt/onboard/%s", filepath.ToSlash(relpath))
}

func contentIDToImageID(contentID string) string {
	return strings.NewReplacer(
		" ", "_",
		"/", "_",
		":", "_",
		".", "_",
	).Replace(contentID)
}

// readEPUBSeriesInfo reads the series metadata from an epub book.
func readEPUBSeriesInfo(filename string) (series string, index float64, err error) {
	zr, err := zip.OpenReader(filename)
	if err != nil {
		return "", 0, fmt.Errorf("could not open ebook: %w", err)
	}
	defer zr.Close()

	var rootfile string
	for _, f := range zr.File {
		if strings.TrimLeft(strings.ToLower(f.Name), "/") == "meta-inf/container.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", 0, fmt.Errorf("could not open container.xml: %w", err)
			}
			doc := etree.NewDocument()
			_, err = doc.ReadFrom(rc)
			if err != nil {
				rc.Close()
				return "", 0, fmt.Errorf("could not parse container.xml: %w", err)
			}
			if el := doc.FindElement("//rootfiles/rootfile[@full-path]"); el != nil {
				rootfile = el.SelectAttrValue("full-path", "")
			}
			rc.Close()
			break
		}
	}
	if rootfile == "" {
		return "", 0, errors.New("could not open ebook: could not find package document")
	}

	for _, f := range zr.File {
		if strings.TrimLeft(strings.ToLower(f.Name), "/") == strings.TrimLeft(strings.ToLower(rootfile), "/") {
			rc, err := f.Open()
			if err != nil {
				return "", 0, fmt.Errorf("could not open container.xml: %w", err)
			}
			doc := etree.NewDocument()
			_, err = doc.ReadFrom(rc)
			if err != nil {
				rc.Close()
				return "", 0, fmt.Errorf("could not parse container.xml: %w", err)
			}
			if el := doc.FindElement("//meta[@name='calibre:series']"); el != nil {
				series = el.SelectAttrValue("content", "")
			}
			if el := doc.FindElement("//meta[@name='calibre:series_index']"); el != nil {
				index, _ = strconv.ParseFloat(el.SelectAttrValue("content", "0"), 64)
			}
			break
		}
	}
	return series, index, nil
}

/*func readEPUBSeriesInfo(filename string) (series string, index float64, err error) {
	err = epubtransform.New(epubtransform.Transform{
		OPFDoc: func(opf *etree.Document) error {
			if el := opf.FindElement("//meta[@name='calibre:series']"); el != nil {
				series = el.SelectAttrValue("content", "")
			}
			if el := opf.FindElement("//meta[@name='calibre:series_index']"); el != nil {
				index, _ = strconv.ParseFloat(el.SelectAttrValue("content", "0"), 64)
			}
			return nil
		},
	}).Run(epubtransform.FileInput(filename), nil, false)
	return
}*/
