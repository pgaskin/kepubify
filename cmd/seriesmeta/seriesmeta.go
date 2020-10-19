// Command seriesmeta updates series metadata for EPUB/KEPUB books in the Kobo database.
package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/beevik/etree"
	_ "github.com/mattn/go-sqlite3"
	"github.com/pgaskin/koboutils/v2/kobo"
	"github.com/spf13/pflag"
)

var version = "dev"

func main() {
	noPersist := pflag.BoolP("no-persist", "p", false, "Don't ensure metadata is always set (this will cause series metadata to be lost if opening a book after an import but before a reboot)")
	noReplace := pflag.BoolP("no-replace", "n", false, "Don't replace existing series metadata (you probably don't want this option)")
	uninstall := pflag.BoolP("uninstall", "u", false, "Uninstall seriesmeta table and hooks (imported series metadata will be left untouched)")
	help := pflag.BoolP("help", "h", false, "Show this help message")
	pflag.Parse()

	if *help || pflag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: seriesmeta [options] [kobo_path]\n\nVersion:\n  seriesmeta %s\n\nOptions:\n", version)
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArguments:\n  kobo_path is the path to the Kobo eReader. If not specified, seriesmeta will try to automatically detect the Kobo.\n")
		if pflag.NArg() > 1 {
			os.Exit(2)
		} else {
			os.Exit(0)
		}
		return
	}

	fmt.Println("Note: You might be interested in NickelSeries (https://go.pgaskin.net/kobo/ns), which will automatically import series and subtitle metadata along with the book itself.")

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
		return
	}

	fmt.Println("Setting up database")
	if err := k.SeriesConfig(*noReplace, *noPersist, *uninstall); err != nil {
		fmt.Fprintf(os.Stderr, "Could not set up database: %v.\n", err)
		os.Exit(1)
		return
	}

	if *uninstall {
		os.Exit(0)
		return
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
		return
	}

	fmt.Printf("%d total: %d updated, %d errored, %d without metadata\n", nt, nu, ne, nn)
	if ne > 0 {
		k.Close()
		os.Exit(1)
		return
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

// SeriesConfig sets up the table and triggers for seriesmeta. noReplace prevents
// series metadata from seriesmeta from replacing existing metadata, and noPersist
// allows the series metadata to be changed by something else later. If uninstall
// is true, the table and triggers will be removed.
func (k *Kobo) SeriesConfig(noReplace, noPersist, uninstall bool) error {
	buf := bytes.NewBuffer(nil)
	template.Must(template.New("").Parse(`
		{{if .Uninstall}}
		DROP TABLE _seriesmeta;
		{{else}}
		CREATE TABLE IF NOT EXISTS _seriesmeta (
			ImageId      TEXT NOT NULL UNIQUE,
			Series       TEXT,
			SeriesNumber TEXT,
			PRIMARY KEY(ImageId)
		);
		{{end}}

		/* Adding series metadata on import */

		DROP TRIGGER IF EXISTS _seriesmeta_content_insert;
		{{if not .Uninstall}}
		CREATE TRIGGER _seriesmeta_content_insert
			AFTER INSERT ON content WHEN
				{{if .NoReplace}}(new.Series IS NULL) AND{{end}}
				(new.ImageId LIKE "file____mnt_onboard_%") AND
				(SELECT count() FROM _seriesmeta WHERE ImageId = new.ImageId)
			BEGIN
				UPDATE content
				SET
					Series       = (SELECT Series       FROM _seriesmeta WHERE ImageId = new.ImageId),
					SeriesNumber = (SELECT SeriesNumber FROM _seriesmeta WHERE ImageId = new.ImageId),
					/* Get the SeriesID from books from the Kobo Store (WorkId NOT NULL) where the series name matches, otherwise just use the series name as the SeriesID (https://www.mobileread.com/forums/showthread.php?p=3959768) */
					SeriesID     = coalesce((SELECT SeriesID FROM content WHERE Series = (SELECT Series FROM _seriesmeta WHERE ImageId = new.ImageId) AND WorkId NOT NULL AND SeriesID NOT NULL AND WorkId != "" AND SeriesID != "" LIMIT 1), (SELECT Series FROM _seriesmeta WHERE ImageId = new.ImageId))
				WHERE ImageId = new.ImageId;
				{{if .NoPersist}}DELETE FROM _seriesmeta WHERE ImageId = new.ImageId;{{end}}
			END;
		{{end}}

		DROP TRIGGER IF EXISTS _seriesmeta_content_update;
		{{if not .Uninstall}}
		CREATE TRIGGER _seriesmeta_content_update
			AFTER UPDATE ON content WHEN
				{{if .NoReplace}}(new.Series IS NULL) AND{{end}}
				(new.ImageId LIKE "file____mnt_onboard_%") AND
				(SELECT count() FROM _seriesmeta WHERE ImageId = new.ImageId)
			BEGIN
				UPDATE content
				SET
					Series       = (SELECT Series       FROM _seriesmeta WHERE ImageId = new.ImageId),
					SeriesNumber = (SELECT SeriesNumber FROM _seriesmeta WHERE ImageId = new.ImageId),
					/* Get the SeriesID from books from the Kobo Store (WorkId NOT NULL) where the series name matches, otherwise just use the series name as the SeriesID (https://www.mobileread.com/forums/showthread.php?p=3959768) */
					SeriesID     = coalesce((SELECT SeriesID FROM content WHERE Series = (SELECT Series FROM _seriesmeta WHERE ImageId = new.ImageId) AND WorkId NOT NULL AND SeriesID NOT NULL AND WorkId != "" AND SeriesID != "" LIMIT 1), (SELECT Series FROM _seriesmeta WHERE ImageId = new.ImageId))
				WHERE ImageId = new.ImageId;
				{{if .NoPersist}}DELETE FROM _seriesmeta WHERE ImageId = new.ImageId;{{end}}
			END;
		{{end}}

		DROP TRIGGER IF EXISTS _seriesmeta_content_delete;
		{{if not .Uninstall}}
		CREATE TRIGGER _seriesmeta_content_delete
			AFTER DELETE ON content
			BEGIN
				DELETE FROM _seriesmeta WHERE ImageId = old.ImageId;
			END;
		{{end}}

		/* Adding series metadata directly when already imported */

		{{if not .Uninstall}}
		DROP TRIGGER IF EXISTS _seriesmeta_seriesmeta_insert;
		CREATE TRIGGER _seriesmeta_seriesmeta_insert
			AFTER INSERT ON _seriesmeta WHEN
				(SELECT count() FROM content WHERE ImageId = new.ImageId)
				{{if .NoReplace}}AND ((SELECT Series FROM content WHERE ImageId = new.ImageId) IS NULL){{end}}
			BEGIN
				UPDATE content
				SET
					Series       = new.Series,
					SeriesNumber = new.SeriesNumber,
					/* Get the SeriesID from books from the Kobo Store (WorkId NOT NULL) where the series name matches, otherwise just use the series name as the SeriesID (https://www.mobileread.com/forums/showthread.php?p=3959768) */
					SeriesID     = coalesce((SELECT SeriesID FROM content WHERE Series = new.Series AND WorkId NOT NULL AND SeriesID NOT NULL AND WorkId != "" AND SeriesID != "" LIMIT 1), new.Series)
				WHERE ImageId = new.ImageId;
				{{if .NoPersist}}DELETE FROM _seriesmeta WHERE ImageId = new.ImageId;{{end}}
			END;
		{{end}}

		DROP TRIGGER IF EXISTS _seriesmeta_seriesmeta_update;
		{{if not .Uninstall}}
		CREATE TRIGGER _seriesmeta_seriesmeta_update
			AFTER UPDATE ON _seriesmeta WHEN
				(SELECT count() FROM content WHERE ImageId = new.ImageId)
				{{if .NoReplace}}AND ((SELECT Series FROM content WHERE ImageId = new.ImageId) IS NULL){{end}}
			BEGIN
				UPDATE content
				SET
					Series       = new.Series,
					SeriesNumber = new.SeriesNumber,
					/* Get the SeriesID from books from the Kobo Store (WorkId NOT NULL) where the series name matches, otherwise just use the series name as the SeriesID (https://www.mobileread.com/forums/showthread.php?p=3959768) */
					SeriesID     = coalesce((SELECT SeriesID FROM content WHERE Series = new.Series AND WorkId NOT NULL AND SeriesID NOT NULL AND WorkId != "" AND SeriesID != "" LIMIT 1), new.Series)
				WHERE ImageId = new.ImageId;
				{{if .NoPersist}}DELETE FROM _seriesmeta WHERE ImageId = new.ImageId;{{end}}
			END;
		{{end}}
	`)).Execute(buf, map[string]interface{}{
		"NoReplace": noReplace,
		"NoPersist": noPersist,
		"Uninstall": uninstall,
	})
	_, err := k.DB.Exec(buf.String())
	return err
}

// UpdateSeries updates the series metadata for all epub books on the device. All
// errors from individual books are returned through the log callback.
func (k *Kobo) UpdateSeries(log func(filename string, i, total int, series string, index float64, err error)) error {
	var epubs []string
	err := filepath.Walk(k.Path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error scanning %q: %w", path, err)
		}
		if !info.IsDir() && strings.EqualFold(filepath.Ext(path), ".epub") {
			epubs = append(epubs, path)
		}
		return nil
	})
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

		series, index, err := readEPUBSeriesInfo(epub)
		if err != nil {
			log(relEpub, i, len(epubs), series, index, err)
			continue
		}

		if _, err := tx.Exec(
			"INSERT OR REPLACE INTO _seriesmeta (ImageId, Series, SeriesNumber) VALUES (?, ?, ?)",
			kobo.ContentIDToImageID(kobo.PathToContentID(relEpub)),
			sql.NullString{String: series, Valid: len(series) > 0},
			sql.NullString{String: strconv.FormatFloat(index, 'f', -1, 64), Valid: index > 0},
		); err != nil {
			log(relEpub, i, len(epubs), series, index, err)
			continue
		}

		log(relEpub, i, len(epubs), series, index, nil)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("could not commit db transaction: %w", err)
	}

	return nil
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

			// Calibre series metadata
			if el := doc.FindElement("//meta[@name='calibre:series']"); el != nil {
				series = el.SelectAttrValue("content", "")

				if el := doc.FindElement("//meta[@name='calibre:series_index']"); el != nil {
					index, _ = strconv.ParseFloat(el.SelectAttrValue("content", "0"), 64)
				}
			}

			// EPUB3 series metadata
			if series == "" {
				if el := doc.FindElement("//meta[@property='belongs-to-collection']"); el != nil {
					series = strings.TrimSpace(el.Text())

					var ctype string
					if id := el.SelectAttrValue("id", ""); id != "" {
						for _, el := range doc.FindElements("//meta[@refines='#" + id + "']") {
							val := strings.TrimSpace(el.Text())
							switch el.SelectAttrValue("property", "") {
							case "collection-type":
								ctype = val
							case "group-position":
								index, _ = strconv.ParseFloat(val, 64)
							}
						}
					}

					if ctype != "" && ctype != "series" {
						series, index = "", 0
					}
				}
			}
			break
		}
	}
	return series, index, nil
}
