# kepubify
Convert your ePubs into kepubs, with a easy-to-use command-line tool.

Work-in-progress

## Usage
- `kepubify /path/to/my/book.epub` will output to `./book.kepub.epub`
- `kepubify /path/to/my/book.epub /path/to/another/folder/` will output to `/path/to/another/folder/book.kepub.epub`
- `kepubify /path/to/my/book.epub /path/to/another/folder/newname.kepub.epub` will output to `/path/to/another/folder/newname.kepub.epub`

## Features
- Conversion
    - Adds kobo spans to allow notes and highlighting
    - Adds kobo divs
    - Smartens punctuation
    - Cleans up html
        - Removes extra characters
        - Removes MS Word tags
        - Removes ADEPT encryption leftover tags
    - kobo style fixes
- TODO:
    - Conversion
        - Remove extra calibre tags from content opf
<<<<<<< HEAD
        - Clean up epub folder structure=
        - Add kobo style fixes
=======
        - Clean up epub folder structure
        - Set language in content opf
>>>>>>> bb3bd99... Add kobo style fixes
    - Output
        - Automatically find kobo and place book
    - Integration with BookBrowser?

## Why would I use kepubify rather than calibre-kobo-driver
- Works from the command line
- Faster processing
- Standalone
- Does not add extra calibre meta tags