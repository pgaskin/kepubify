package main

import (
	"archive/zip"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/bamiaux/rez"
	"github.com/beevik/etree"
	"github.com/geek1011/koboutils/kobo"
	"github.com/mattn/go-zglob"
	"github.com/spf13/pflag"
)

var version = "dev"

func main() {
	regenerate := pflag.BoolP("regenerate", "r", false, "Re-generate all covers")
	method := pflag.StringP("method", "m", "lanczos3", "Resize algorithm to use (bilinear, bicubic, lanczos2, lanczos3)")
	// TODO: invert, grayscale options
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

			//fmt.Println(ct, origCover.Bounds().Size(), resized.Bounds().Size())

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
	cp := filepath.Join(kp, filepath.FromSlash(ct.GeneratePath(false, iid)))
	if _, err := os.Stat(cp); err == nil {
		return cp, true, nil
	} else if os.IsNotExist(err) {
		return cp, false, nil
	} else {
		return cp, false, err
	}
}

func extract(epub string) (image.Image, error) {
	zr, err := zip.OpenReader(epub)
	if err != nil {
		return nil, err
	}
	defer zr.Close()

	var rootfile string
	for _, f := range zr.File {
		if strings.TrimLeft(strings.ToLower(f.Name), "/") == "meta-inf/container.xml" {
			doc := etree.NewDocument()
			if rc, err := f.Open(); err != nil {
				return nil, fmt.Errorf("could not open container.xml: %w", err)
			} else if _, err = doc.ReadFrom(rc); err != nil {
				rc.Close()
				return nil, fmt.Errorf("could not parse container.xml: %w", err)
			} else {
				rc.Close()
			}

			if el := doc.FindElement("//rootfiles/rootfile[@full-path]"); el != nil {
				rootfile = el.SelectAttrValue("full-path", "")
			}
			break
		}
	}
	if rootfile == "" {
		return nil, errors.New("could not open ebook: could not find package document")
	}

	var covers []string
	for _, f := range zr.File {
		if strings.TrimLeft(strings.ToLower(f.Name), "/") == strings.TrimLeft(strings.ToLower(rootfile), "/") {
			doc := etree.NewDocument()
			if rc, err := f.Open(); err != nil {
				return nil, fmt.Errorf("could not open container.xml: %w", err)
			} else if _, err = doc.ReadFrom(rc); err != nil {
				rc.Close()
				return nil, fmt.Errorf("could not parse container.xml: %w", err)
			} else {
				rc.Close()
			}

			if el := doc.FindElement("//meta[@name='cover']"); el != nil {
				content := el.SelectAttrValue("content", "")
				if content != "" {
					covers = append(covers, content) // some put the path directly in the value
					if iel := doc.FindElement("//manifest/item[@id='" + content + "']"); iel != nil {
						if href := iel.SelectAttrValue("href", ""); href != "" {
							covers = append(covers, href) // most have it as an id for a manifest item
						}
					}
				}
			}

			if el := doc.FindElement("//manifest/item[@properties='cover-image']"); el != nil {
				if href := el.SelectAttrValue("href", ""); href != "" {
					covers = append(covers, href) // pure epub3 books have it as a manifest item
				}
			}

			// most books have it relative to the opf
			for _, cover := range covers {
				covers = append(covers, path.Join(path.Dir(f.Name), cover))
			}

			break
		}
	}

	var img image.Image
	for _, cover := range covers {
		for _, f := range zr.File {
			if strings.TrimLeft(strings.ToLower(f.Name), "/") == strings.TrimLeft(strings.ToLower(cover), "/") {
				if rc, err := f.Open(); err != nil {
					return nil, fmt.Errorf("could not open image %s: %w", cover, err)
				} else if img, _, err = image.Decode(rc); err != nil {
					rc.Close()
					return nil, fmt.Errorf("could not parse image %s: %w", cover, err)
				} else {
					rc.Close()
					break
				}
			}
		}
	}

	return img, nil // img may be nil
}

func resize(dev kobo.Device, ct kobo.CoverType, filter rez.Filter, orig image.Image) (image.Image, error) {
	szo := orig.Bounds().Size()
	szn := dev.CoverSized(ct, szo)

	if szn.Eq(szo) {
		return orig, nil
	}

	origy, ok := orig.(*image.YCbCr)
	if !ok {
		return nil, fmt.Errorf("unsupported image encoding (not YCbCr): %s", reflect.TypeOf(orig))
	}

	new := image.NewYCbCr(image.Rect(0, 0, szn.X, szn.Y), origy.SubsampleRatio)
	if err := rez.Convert(new, orig, filter); err != nil {
		return nil, err
	}

	return new, nil
}

func save(fn string, img image.Image, quality int) error {
	if err := os.MkdirAll(path.Dir(filepath.ToSlash(fn)), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(fn, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	return jpeg.Encode(f, img, &jpeg.Options{Quality: quality})
}
