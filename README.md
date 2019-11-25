<h1 align="center">kepubify</h1>

<a href="https://travis-ci.org/geek1011/kepubify"><img alt="Build Status" src="https://travis-ci.org/geek1011/kepubify.svg?branch=master" /></a>
<a href="https://goreportcard.com/report/github.com/geek1011/kepubify"><img alt="Go Report Card" src="https://goreportcard.com/badge/github.com/geek1011/kepubify" /></a>
<a href="https://godoc.org/github.com/geek1011/kepubify/kepub"><img alt="GoDoc" src="https://img.shields.io/badge/godoc-reference-blue.svg" /></a>

Kepubify converts EPUBs to KEPUBS. Kepubify also includes two standalone utilities
which do not depend on kepubify (and don't conflict with Calibre): [covergen](./covergen)
(which pre-generates cover images), and [seriesmeta](./seriesmeta) (which updates
Calibre or EPUB3 series metadata).

See the [releases](https://github.com/geek1011/kepubify/releases/latest) page for
download links, and the [website](https://pgaskin.net/kepubify) for more information.

## Usage
```
Usage: kepubify [OPTIONS] PATH [PATH]...

Options:
  -c, --css string                 custom CSS to add to ebook
      --fullscreen-reading-fixes   enable fullscreen reading bugfixes based on https://www.mobileread.com/forums/showpost.php?p=3113460&postcount=16
  -h, --help                       show this help text
      --hyphenate                  force enable hyphenation
      --inline-styles              inline all stylesheets (for working around certain bugs)
      --no-hyphenate               force disable hyphenation
  -o, --output string              the directory to place the converted files (default ".")
  -r, --replace stringArray        find and replace on all html files (repeat any number of times) (format: find|replace)
  -u, --update                     don't reconvert files which have already been converted
  -v, --verbose                    show extra information in output
      --version                    show the version

Arguments:
  PATH is the path to an epub file or directory to convert. If it is a directory,
  the converted dir is the name of the dir with the suffix _converted. If the path
  is a file, the converted file has the extension .kepub.epub.
```
