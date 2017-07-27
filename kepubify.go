package main

import (
	"os"

	cli "gopkg.in/urfave/cli.v1"
)

var version = "dev"

func convert(c *cli.Context) error {
	return nil
}

func main() {
	app := cli.NewApp()

	app.Name = "kepubify"
	app.Description = "Convert your ePubs into kepubs, with a easy-to-use command-line tool."
	app.Version = version

	app.ArgsUsage = "EPUB_INPUT_PATH [KEPUB_OUTPUT_PATH]"
	app.Action = convert

	app.Run(os.Args)
}
