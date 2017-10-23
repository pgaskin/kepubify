# SeriesMeta
A standalone command to update the series metadata on Kobo eReaders. Only works with books stored in the internal memory. Currently only tested on linux with firmware 4.6.9995 and 3.19.5761. Works with epub and kepub.

Can update the metadata for all epub files on the kobo, or just a single one.

Examples:
- `./seriesmeta /path/to/KOBOeReader`
- `./seriesmeta /path/to/KOBOeReader /path/to/KOBOeReader/book.epub`