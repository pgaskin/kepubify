package main

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/beevik/etree"
	"github.com/mattn/go-zglob"
	"golang.org/x/tools/godoc/vfs/zipfs"

	"github.com/spf13/pflag"

	_ "github.com/mattn/go-sqlite3"
)

var version = "dev"

func helpExit() {
	fmt.Fprintf(os.Stderr, "Usage: seriesmeta [OPTIONS] KOBO_PATH\n\nVersion:\n  seriesmeta %s\n\nOptions:\n", version)
	pflag.PrintDefaults()
	fmt.Fprintf(os.Stderr, "\nArguments:\n  KOBO_PATH is the path to the Kobo eReader.\n")
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second * 2)
	}
	os.Exit(1)
}

func errExit() {
	if runtime.GOOS == "windows" {
		time.Sleep(time.Second * 2)
	}
	os.Exit(1)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Close()
}

// pathToContentID gets the content ID for a book. The path needs to be relative to the root of the kobo.
func pathToContentID(relpath string) string {
	return fmt.Sprintf("file:///mnt/onboard/%s", relpath)
}

func contentIDToImageID(contentID string) string {
	imageID := contentID

	imageID = strings.Replace(imageID, " ", "_", -1)
	imageID = strings.Replace(imageID, "/", "_", -1)
	imageID = strings.Replace(imageID, ":", "_", -1)
	imageID = strings.Replace(imageID, ".", "_", -1)

	return imageID
}

func getMeta(path string) (string, float64, error) {
	zr, err := zip.OpenReader(path)
	if err != nil {
		return "", 0, err
	}

	zfs := zipfs.New(zr, "epub")
	rsk, err := zfs.Open("/META-INF/container.xml")
	if err != nil {
		return "", 0, err
	}
	defer rsk.Close()

	container := etree.NewDocument()
	_, err = container.ReadFrom(rsk)
	if err != nil {
		return "", 0, err
	}

	rootfile := ""
	for _, e := range container.FindElements("//rootfiles/rootfile[@full-path]") {
		rootfile = e.SelectAttrValue("full-path", "")
	}

	if rootfile == "" {
		return "", 0, errors.New("Cannot parse container")
	}

	rrsk, err := zfs.Open("/" + rootfile)
	if err != nil {
		return "", 0, err
	}
	defer rrsk.Close()

	opf := etree.NewDocument()
	_, err = opf.ReadFrom(rrsk)
	if err != nil {
		return "", 0, err
	}

	var series string
	for _, e := range opf.FindElements("//meta[@name='calibre:series']") {
		series = e.SelectAttrValue("content", "")
		break
	}

	var seriesNumber float64
	for _, e := range opf.FindElements("//meta[@name='calibre:series_index']") {
		i, err := strconv.ParseFloat(e.SelectAttrValue("content", "0"), 64)
		if err == nil {
			seriesNumber = i
			break
		}
	}

	return series, seriesNumber, nil
}

func main() {
	help := pflag.BoolP("help", "h", false, "Show this help message")
	pflag.Parse()

	if *help || pflag.NArg() != 1 {
		helpExit()
	}

	log := func(format string, a ...interface{}) {
		fmt.Printf(format, a...)
	}

	logE := func(format string, a ...interface{}) {
		fmt.Fprintf(os.Stderr, format, a...)
	}

	kpath, err := filepath.Abs(pflag.Args()[0])
	if err != nil {
		logE("Fatal: Could not resolve path to kobo\n")
		errExit()
	}

	kpath = strings.Replace(kpath, ".kobo", "", 1)

	log("Looking for kobo at '%s'\n", kpath)

	dbpath := filepath.Join(kpath, ".kobo", "KoboReader.sqlite")
	_, err = os.Stat(dbpath)
	if err != nil {
		if os.IsNotExist(err) {
			logE("Fatal: '%s' is not a kobo eReader (it does not contain .kobo/KoboReader.sqlite)\n", kpath)
		} else if os.IsPermission(err) {
			logE("Fatal: Could not access kobo: %v\n", err)
		} else {
			logE("Fatal: Error reading database: %v\n", err)
		}
		errExit()
	}

	log("Making backup of KoboReader.sqlite\n")
	err = copyFile(dbpath, dbpath+".bak")
	if err != nil {
		logE("Fatal: Could not make copy of KoboReader.sqlite: %v\n", err)
		errExit()
	}

	log("Opening KoboReader.sqlite\n")
	db, err := sql.Open("sqlite3", dbpath)
	if err != nil {
		logE("Fatal: Could not open KoboReader.sqlite: %v\n", err)
		errExit()
	}

	log("Searching for sideloaded epubs and kepubs\n")
	epubs, err := zglob.Glob(filepath.Join(kpath, "**", "*.epub"))
	if err != nil {
		logE("Fatal: Could not search for epubs: %v\n", err)
		errExit()
	}

	log("\nUpdating metadata for %d books\n", len(epubs))
	errcount := 0
	for i, epub := range epubs {
		rpath, err := filepath.Rel(kpath, epub)
		if err != nil {
			log("[%d/%d] Updating '%s'\n", i+1, len(epubs), epub)
			logE("  Error: could not resolve path: %v\n", err)
			errcount++
			continue
		}

		log("[%d/%d] Updating '%s'\n", i+1, len(epubs), rpath)
		series, seriesNumber, err := getMeta(epub)
		if err != nil {
			logE("  Error: could not read metadata: %v\n", err)
			errcount++
			continue
		}

		if series == "" && seriesNumber == 0 {
			log("  No series\n")
			continue
		}

		log("  Series: '%s' Number: %v\n", series, seriesNumber)

		iid := contentIDToImageID(pathToContentID(rpath))

		res, err := db.Exec("UPDATE content SET Series=?, SeriesNumber=? WHERE ImageID=?", sql.NullString{
			String: series,
			Valid:  series != "",
		}, sql.NullString{
			String: fmt.Sprintf("%v", seriesNumber),
			Valid:  seriesNumber > 0,
		}, iid)
		if err != nil {
			logE("  Error: could not update database: %v\n", err)
			errcount++
			continue
		}

		ra, err := res.RowsAffected()
		if err != nil {
			logE("  Error: could not update database: %v\n", err)
			errcount++
			continue
		}

		if ra > 1 {
			logE("  Warn: more than one match in database for ImageID\n")
		} else if ra < 1 {
			logE("  Error: could not update database: no entry in database for book (the kobo may still need to import the book)\n")
			errcount++
			continue
		}
	}

	time.Sleep(time.Second)
	log("\nFinished updating metadata. %v books processed. %v errors.\n", len(epubs), errcount)
}
