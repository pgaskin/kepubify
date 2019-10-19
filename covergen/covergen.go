package main

import (
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"

	"github.com/bamiaux/rez"
	"github.com/geek1011/koboutils/kobo"
	"github.com/mattn/go-zglob"
	"github.com/spf13/pflag"
)

var version = "dev"

func main() {
	regenerate := pflag.BoolP("regenerate", "r", false, "Re-generate all covers")
	method := pflag.StringP("method", "m", "lanczos3", "Resize algorithm to use (bilinear, bicubic, lanczos2, lanczos3)")
	help := pflag.BoolP("help", "h", false, "Show this help message")
	pflag.Parse()

	if *help || pflag.NArg() > 1 {
		fmt.Fprintf(os.Stderr, "Usage: covergen [OPTIONS] [KOBO_PATH]\n\nVersion:\n  seriesmeta %s\n\nOptions:\n", version)
		pflag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nArguments:\n  KOBO_PATH is the path to the Kobo eReader. If not specified, covergen will try to automatically detect the Kobo.\n")
		os.Exit(2)
	}

	filter, ok := filters[*method]
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: Unknown resize method %s.\n", *method)
		os.Exit(2)
	}

	fmt.Println("Finding kobo")
	var kp string
	if pflag.NArg() == 1 {
		kp = pflag.Arg(0)
	} else {
		kobos, err := kobo.Find()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not automatically detect a Kobo eReader: %v.\n", err)
			os.Exit(1)
		} else if len(kobos) == 0 {
			fmt.Fprintf(os.Stderr, "Error: Could not automatically detect a Kobo eReader.\n")
			os.Exit(1)
		}
		kp = kobos[0]
	}

	_, _, id, err := kobo.ParseKoboVersion(kp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not parse version info: %v.\n", err)
		os.Exit(1)
	}

	dev, ok := kobo.DeviceByID(id)
	if !ok {
		fmt.Fprintf(os.Stderr, "Error: Unsupported device: %s.\n", id)
		os.Exit(1)
	}

	fmt.Printf("Found %s at %s\n", dev, kp)

	fmt.Println("Finding epubs")
	epubs, err := zglob.Glob(filepath.Join(kp, "**", "*.epub"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Could not find epubs: %v.\n", err)
		os.Exit(1)
	}

	var nt, ntc, nu, ne, ns, nn int
	fmt.Println("Generating covers")
	nt = len(epubs)
	ntc = nt * len(kobo.CoverTypes())
	for i, epub := range epubs {
		fmt.Printf("[%3d/%3d] %s\n", i+1, nt, epub)

		iid, err := imageID(kp, epub)
		if err != nil {
			fmt.Fprintf(os.Stderr, "--------- Could not generate ImageId: %v.\n", err)
			ne += len(kobo.CoverTypes())
			continue
		}

		var origCover image.Image
		for _, ct := range kobo.CoverTypes() {
			cp, exists, err := check(ct, kp, iid)
			if err != nil {
				fmt.Fprintf(os.Stderr, "--------- Could not check if cover exists: %v.\n", err)
				ne++
				continue
			} else if !*regenerate && exists {
				ns++
				continue
			}

			if origCover == nil {
				origCover, err = extract(epub)
				if err != nil {
					fmt.Fprintf(os.Stderr, "--------- Could not extract cover: %v.\n", err)
					ne++
					continue
				} else if origCover == nil {
					nn++
					continue
				}
			}

			resized, err := resize(dev, ct, filter, origCover)
			if err != nil {
				fmt.Fprintf(os.Stderr, "--------- Could not resize cover: %v.\n", err)
				ne++
				continue
			}

			if err := save(cp, resized, jpeg.DefaultQuality); err != nil {
				fmt.Fprintf(os.Stderr, "--------- Could not save cover: %v.\n", err)
				ne++
				continue
			}

			nu++
		}
	}

	fmt.Printf("%d covers (%d books): %d updated, %d errored, %d skipped, %d without covers\n", ntc, nt, nu, ne, ns, nn)
	os.Exit(1)
}

var filters = map[string]rez.Filter{
	"bilinear": rez.NewBilinearFilter(),
	"bicubic":  rez.NewBicubicFilter(),
	"lanczos2": rez.NewLanczosFilter(2),
	"lanczos3": rez.NewLanczosFilter(3),
}

func imageID(kp, book string) (string, error) {
	rel, err := filepath.Rel(kp, book)
	if err != nil {
		return "", fmt.Errorf("could not resolve book path relative to kobo: %w", err)
	}
	return kobo.ContentIDToImageID(kobo.PathToContentID(rel)), nil
}

func check(ct kobo.CoverType, kp, iid string) (string, bool, error) {
	cp := ct.GeneratePath(false, iid)
	if _, err := os.Stat(filepath.Join(kp, cp)); err == nil {
		return cp, true, nil
	} else if os.IsNotExist(err) {
		return cp, false, nil
	} else {
		return cp, false, err
	}
}

func extract(epub string) (image.Image, error) {
	return nil, errors.New("not implemented")
}

func resize(dev kobo.Device, ct kobo.CoverType, filter rez.Filter, orig image.Image) (image.Image, error) {
	return nil, errors.New("not implemented")
}

func save(path string, img image.Image, quality int) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
}
