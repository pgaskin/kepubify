<h1 align="center">kepubify</h1>

[![](https://img.shields.io/github/v/release/pgaskin/kepubify)](https://github.com/pgaskin/kepubify/releases/latest) [![](https://img.shields.io/drone/build/pgaskin/kepubify/master)](https://cloud.drone.io/pgaskin/kepubify) [![](https://img.shields.io/drone/build/pgaskin/kepubify/master?label=linux%20build)](https://cloud.drone.io/pgaskin/kepubify) [![](https://img.shields.io/appveyor/ci/pgaskin/kepubify/master?label=windows%20build)](https://ci.appveyor.com/project/pgaskin/kepubify/branch/master) [![](https://img.shields.io/travis/com/pgaskin/kepubify/master?label=macOS%20build)](https://travis-ci.com/pgaskin/kepubify) ![](https://img.shields.io/github/go-mod/go-version/pgaskin/kepubify) [![](https://img.shields.io/badge/godoc-reference-blue.svg)](https://pkg.go.dev/mod/github.com/pgaskin/kepubify/v3?tab=versions) [![](https://goreportcard.com/badge/github.com/pgaskin/kepubify)](https://goreportcard.com/report/github.com/pgaskin/kepubify)

Kepubify converts EPUBs to KEPUBS. Kepubify also includes two standalone utilities
which do not depend on kepubify (and don't conflict with Calibre): [covergen](./cmd/covergen)
(which pre-generates cover images), and [seriesmeta](./cmd/seriesmeta) (which updates
Calibre or EPUB3 series metadata).

See the [releases](https://github.com/pgaskin/kepubify/releases/latest) page for
download links, and the [website](https://pgaskin.net/kepubify) for more information.
Kepubify can also be installed via Homebrew (kepubify).

## Usage
```
Usage: kepubify [options] input_path [input_path]...

General Options:
  -v, --verbose   Show extra information in output
      --version   Show the version
  -h, --help      Show this help text

Output Options:
  -u, --update             Don't reconvert files which have already been converted (i.e. don't overwrite output files)
  -i, --inplace            Don't add the _converted suffix to converted files and directories
      --no-preserve-dirs   Flatten the directory structure of the input (an error will be shown if there are conflicts)
  -o, --output string      [>1 inputs || 1 file input with existing dir output]: Directory to place converted files/dirs under; [1 file input with
                           nonexistent output]: Output filename; [1 dir input]: Output directory for contents of input (default: current directory)
      --calibre            Use .kepub instead of .kepub.epub as the output extension (for Calibre compatibility, only use if you know what you are doing)
  -x, --copy strings       Copy files with the specified extension (with a leading period) to the output unchanged (no effect if the filename ends up the
                           same)

Conversion Options:
      --smarten-punctuation        Smarten punctuation (smart quotes, dashes, etc) (excluding pre and code tags)
  -c, --css stringArray            Custom CSS to add to ebook
      --hyphenate                  Force enable hyphenation
      --no-hyphenate               Force disable hyphenation
      --fullscreen-reading-fixes   Enable fullscreen reading bugfixes based on https://www.mobileread.com/forums/showpost.php?p=3113460&postcount=16
  -r, --replace stringArray        Find and replace on all html files (repeat any number of times) (format: find|replace)

Links:
  Website      - https://pgaskin.net/kepubify
  Source Code  - https://github.com/pgaskin/kepubify
  Bugs/Support - https://github.com/pgaskin/kepubify/issues
  MobileRead   - http://mr.gd/forums/showthread.php?t=295287
```

## seriesmeta
Seriesmeta updates series metadata for sideloaded books. **New:** Seriesmeta now
supports updating metadata for unimported books, so you don't have to connect
twice (this is implemented using SQLite triggers). A reboot may be required in
some cases for the updated metadata to appear.

Seriesmeta works on EPUB and KEPUB books, and does not conflict with Calibre
(unless persistence is used, in which case seriesmeta will take precedence). It
will detect Calibre (`meta[name=calibre:series]`) and EPUB3
(`meta[property=belongs-to-collection]`) series metadata.

```
Usage: seriesmeta [options] [kobo_path]

Options:
  -h, --help         Show this help message
  -p, --no-persist   Don't ensure metadata is always set (this will cause series metadata to be lost if opening a book after an import but before a reboot)
  -n, --no-replace   Don't replace existing series metadata (you probably don't want this option)
  -u, --uninstall    Uninstall seriesmeta table and hooks (imported series metadata will be left untouched)

Arguments:
  kobo_path is the path to the Kobo eReader. If not specified, seriesmeta will
  try to automatically detect the Kobo.
```

## covergen
Covergen (re)generates cover images for nickel, with optional stretching to fit
a specific aspect ratio (I use 1.5). This speeds up browsing the library, and if
stretching is used, will also make it more consistent. In addition, covergen is
useful when the automatically generated cover images are not satisfactory (too
small, white margins, etc).

Covergen works on EPUB and KEPUB books, and does not conflict with Calibre or any
other tool. It is also quite lenient about the way the cover image is referenced
by the book. The following methods are supported: `meta[name=cover]` with the path
as the content, `meta[name=cover]` with a manifest id reference as the content, and
`manifest>item[properties=cover-image]` with the image path as the href. Each
detected path can be relative to the epub root or to the package document.
Covergen does not support the external SD on older devices, and will ignore it.

The N3_LIBRARY_FULL, N3_LIBRARY_LIST, and N3_LIBRARY_GRID images are generated
using the same resizing algorithm as nickel (see [koboutils](https://github.com/pgaskin/koboutils/blob/master/kobo/device.go) for more info).

```
Usage: covergen [options] [kobo_path]

Options:
  -a, --aspect-ratio float   Stretch the covers to fit a specific aspect ratio (for example 1.3, 1.5, 1.6)
  -g, --grayscale            Convert images to grayscale
  -h, --help                 Show this help message
  -i, --invert               Invert images
  -m, --method string        Resize algorithm to use (bilinear, bicubic, lanczos2, lanczos3) (default "lanczos3")
  -r, --regenerate           Re-generate all covers

Arguments:
  kobo_path is the path to the Kobo eReader. If not specified, covergen will try
  to automatically detect the Kobo.
```

