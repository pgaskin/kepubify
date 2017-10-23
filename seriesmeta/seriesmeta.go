package main

import (
	"archive/zip"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mattn/go-zglob"

	"golang.org/x/tools/godoc/vfs/zipfs"

	"github.com/beevik/etree"
	_ "github.com/mattn/go-sqlite3"
)

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

func pathToContentID(koboPath, path string) (imageID string, err error) {
	relPath, err := filepath.Rel(koboPath, path)
	if err != nil {
		return "", fmt.Errorf("could not get relative path to file: %v", err)
	}

	contentID := fmt.Sprintf("file:///mnt/onboard/%s", relPath)

	return contentID, nil
}

func contentIDToImageID(contentID string) string {
	imageID := contentID

	imageID = strings.Replace(imageID, " ", "_", -1)
	imageID = strings.Replace(imageID, "/", "_", -1)
	imageID = strings.Replace(imageID, ":", "_", -1)
	imageID = strings.Replace(imageID, ".", "_", -1)

	return imageID
}

func updateSeriesMeta(db *sql.DB, imageID, series string, seriesNumber float64) (int64, error) {
	res, err := db.Exec("UPDATE content SET Series=?, SeriesNumber=? WHERE ImageID=?", sql.NullString{
		String: series,
		Valid:  series != "",
	}, sql.NullString{
		String: fmt.Sprintf("%v", seriesNumber),
		Valid:  seriesNumber > 0,
	}, imageID)

	if err != nil {
		return 0, err
	}

	return res.RowsAffected()
}

func getEPUBMeta(path string) (string, float64, error) {
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

func updateSeriesMetaFromEPUB(db *sql.DB, koboPath, epubPath string) (int64, error) {
	series, seriesNumber, err := getEPUBMeta(epubPath)
	if err != nil {
		return 0, err
	}

	cid, err := pathToContentID(koboPath, epubPath)
	if err != nil {
		return 0, err
	}

	iid := contentIDToImageID(cid)

	fmt.Printf("INFO: UPDATE %s => [%s %v]\n", iid, series, seriesNumber)

	return updateSeriesMeta(db, iid, series, seriesNumber)
}

func loadKoboDB(koboPath string) (*sql.DB, error) {
	koboDBPath := filepath.Join(koboPath, ".kobo/KoboReader.sqlite")
	koboDBBackupPath := filepath.Join(koboPath, "KoboReader.sqlite.bak")

	if _, err := os.Stat(koboDBPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Kobo database %s does not exist", koboDBPath)
	}

	copyFile(koboDBPath, koboDBBackupPath)

	return sql.Open("sqlite3", koboDBPath)
}

func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		fmt.Printf("USAGE: %s KOBO_ROOT_PATH [EPUB_PATH]\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	koboPath, err := filepath.Abs(os.Args[1])
	if err != nil {
		fmt.Printf("FATAL: Could resolve Kobo path %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}

	if _, err := os.Stat(filepath.Join(koboPath, ".kobo")); os.IsNotExist(err) {
		fmt.Printf("FATAL: %s is not a valid path to a Kobo eReader.\n", os.Args[1])
		fmt.Printf("USAGE: %s KOBO_ROOT_PATH [EPUB_PATH]\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	if len(os.Args) == 3 {
		epubPath, err := filepath.Abs(os.Args[2])
		if err != nil {
			fmt.Printf("FATAL: Could resolve ePub path %s: %v\n", os.Args[2], err)
			os.Exit(1)
		}

		if !strings.HasPrefix(epubPath, koboPath) {
			fmt.Printf("FATAL: ePub file not in the specified Kobo path.\n")
			os.Exit(1)
		}

		db, err := loadKoboDB(koboPath)
		if err != nil {
			fmt.Printf("FATAL: Could not open Kobo database: %v\n", err)
			os.Exit(1)
		}

		ra, err := updateSeriesMetaFromEPUB(db, koboPath, epubPath)
		if err != nil {
			fmt.Printf("ERROR: Could not update series metadata: %v\n", err)
			os.Exit(1)
		} else if ra < 1 {
			fmt.Printf("ERROR: Could not update series metadata: no database entry for book. Please let the kobo import the book before using this tool.\n")
		} else if ra > 1 {
			fmt.Printf("WARN: More than 1 match for book in database.\n")
		}
	} else {
		db, err := loadKoboDB(koboPath)
		if err != nil {
			fmt.Printf("FATAL: Could not open Kobo database: %v\n", err)
			os.Exit(1)
		}

		matches, err := zglob.Glob(filepath.Join(koboPath, "**/*.epub"))
		if err != nil {
			fmt.Printf("FATAL: Error searching for epub files: %v\n", err)
			os.Exit(1)
		}

		epubs := []string{}
		for _, match := range matches {
			if strings.HasPrefix(filepath.Base(match), ".") {
				continue
			}
			epubs = append(epubs, match)
		}

		fmt.Printf("INFO: Found %v epub files\n", len(epubs))

		errcount := 0
		for _, epub := range epubs {
			ra, err := updateSeriesMetaFromEPUB(db, koboPath, epub)
			if err != nil {
				fmt.Printf("ERROR: Could not update series metadata: %v\n", err)
				errcount++
			} else if ra < 1 {
				fmt.Printf("ERROR: Could not update series metadata: no entry in database for book. Please let the kobo import the book before using this tool.\n")
				errcount++
			} else if ra > 1 {
				fmt.Printf("WARN: More than 1 match for book in database.\n")
			}
			fmt.Println()
		}

		fmt.Printf("INFO: Finished updating metadata. %v books processed. %v errors.\n", len(epubs), errcount)
	}
}
