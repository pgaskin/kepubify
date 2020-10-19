module github.com/pgaskin/kepubify/v3

go 1.14

require (
	// kepubify/covergen/seriesmeta
	github.com/bamiaux/rez v0.0.0-20170731184118-29f4463c688b
	github.com/mattn/go-sqlite3 v2.0.3+incompatible
	github.com/pgaskin/koboutils/v2 v2.1.1
	github.com/spf13/pflag v1.0.5
)

require (
	// kepub
	github.com/andybalholm/cascadia v1.2.0
	github.com/beevik/etree v1.1.0
	github.com/kr/smartypants v0.1.0
	golang.org/x/net v0.0.0-20191209160850-c0dbc17a3553
	golang.org/x/sync v0.0.0-20201008141435-b3e1573b7520
)

// tests/utils
require github.com/sergi/go-diff v1.1.0

replace golang.org/x/net => github.com/pgaskin/net v0.0.0-20200714194227-0135d2310fe0 // x/net/html options for kepubify
