module github.com/geek1011/kepubify/v3

go 1.13

require (
	// kepubify/covergen/seriesmeta
	github.com/bamiaux/rez v0.0.0-20170731184118-29f4463c688b
	github.com/geek1011/koboutils/v2 v2.0.0
	github.com/mattn/go-sqlite3 v2.0.2+incompatible
	github.com/mattn/go-zglob v0.0.1
	github.com/spf13/pflag v1.0.5
)

require (
	// kepub
	github.com/andybalholm/cascadia v1.1.0
	github.com/beevik/etree v1.1.0
	github.com/kr/smartypants v0.1.0
	golang.org/x/net v0.0.0-20191209160850-c0dbc17a3553
	golang.org/x/sync v0.0.0-20190911185100-cd5d95a43a6e
)

// tests/utils
require github.com/sergi/go-diff v1.1.0

replace golang.org/x/net => github.com/geek1011/net v0.0.0-20200112223105-747a32f770f8 // x/net/html options for kepubify
