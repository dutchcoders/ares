package main

import (
	"github.com/dutchsec/ares/cmd"
)

func main() {
	app := cmd.New()
	app.RunAndExitOnError()
}
