# covergen
Standalone tool to generate Kobo cover thumbnails. Useful for pre-generating thumbnails during import to save time or for when Kobo automatically generates the cover incorrectly (for example taking an image of the first page, or using a generic cover).

Covergen is quite lenient about detecting cover images. The following methods are supported: meta[name=cover] with the path as the content, meta[name=cover] with a manifest id reference as the content, and manifest>item[properties=cover-image] with the image path as the href. Each detected path can be relative to the epub root or to the package document.

The N3_FULL, N3_LIBRARY_LIST, and N3_LIBRARY_GRID images are generated using the same resizing algorithm as nickel.

A reboot may be necessary for changes to appear.

## Usage

```
Usage: covergen [OPTIONS] [KOBO_PATH]

Version:
  seriesmeta dev

Options:
  -h, --help            Show this help message
  -m, --method string   Resize algorithm to use (bilinear, bicubic, lanczos2, lanczos3) (default "lanczos3")
  -r, --regenerate      Re-generate all covers

Arguments:
  KOBO_PATH is the path to the Kobo eReader. If not specified, covergen will try to automatically detect the Kobo.
```