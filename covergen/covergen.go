package main

import (
	"fmt"
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

	var nt, nu, ne, ns, nn int
	fmt.Println("Generating covers")
	nt = len(epubs)
	for i, epub := range epubs {
		fmt.Printf("[%3d/%3d] %s\n", i+1, nt, epub)
		//fmt.Printf("--------- Could not extract cover image\n")
		//nn++

		rel, err := filepath.Rel(kp, epub)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: Could not resolve relative path to epub: %v.\n", err)
			os.Exit(1) // fatal error, should not ever occur
		}

		cid := kobo.PathToContentID(rel)
		iid := kobo.ContentIDToImageID(cid)
		for _, ct := range kobo.CoverTypes() {
			fmt.Println(ct, ct.GeneratePath(false, iid))
		}

		// TODO
		_, _, _ = regenerate, filter, epubs
	}

	fmt.Printf("%d total: %d updated, %d errored, %d skipped, %d without covers\n", nt, nu, ne, ns, nn)
	os.Exit(1)
}

var filters = map[string]rez.Filter{
	"bilinear": rez.NewBilinearFilter(),
	"bicubic":  rez.NewBicubicFilter(),
	"lanczos2": rez.NewLanczosFilter(2),
	"lanczos3": rez.NewLanczosFilter(3),
}
