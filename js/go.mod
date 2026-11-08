module kepubify-js

go 1.26.10

require (
	github.com/evanw/esbuild v0.28.2
	github.com/pgaskin/kepubify/v5 v5.0.0
)

require (
	github.com/pgaskin/htmlstream v0.0.4 // indirect
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8 // indirect
	golang.org/x/text v0.43.0 // indirect
)

replace github.com/pgaskin/kepubify/v5 => ../
