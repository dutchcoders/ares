package main

import (
	"github.com/dutchcoders/ares/cmd"
)

func main() {
	app := cmd.New()
	app.RunAndExitOnError()
}
