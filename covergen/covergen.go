package main

import (
	"fmt"
	"os"

	"github.com/geek1011/koboutils/kobo"
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

	fmt.Println("Generating covers")
	// TODO
	_, _, _, _ = regenerate, method, help, kp
}
