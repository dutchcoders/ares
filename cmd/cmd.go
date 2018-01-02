package cmd

import (
	"fmt"

	"github.com/dutchcoders/ares/server"
	"github.com/fatih/color"
	"github.com/minio/cli"
	"github.com/op/go-logging"
)

var Version = "0.1"
var helpTemplate = `NAME:
{{.Name}} - {{.Usage}}

DESCRIPTION:
{{.Description}}

USAGE:
{{.Name}} {{if .Flags}}[flags] {{end}}command{{if .Flags}}{{end}} [arguments...]

COMMANDS:
{{range .Commands}}{{join .Names ", "}}{{ "\t" }}{{.Usage}}
{{end}}{{if .Flags}}
FLAGS:
{{range .Flags}}{{.}}
{{end}}{{end}}
VERSION:
` + Version +
	`{{ "\n"}}`

var log = logging.MustGetLogger("ares/cmd")

var globalFlags = []cli.Flag{
	cli.StringFlag{
		Name:  "p,port",
		Usage: "port",
		Value: "127.0.0.1:8080",
	},
	cli.StringFlag{
		Name:  "tlsport",
		Usage: "port",
		Value: "",
	},
	cli.StringFlag{
		Name:   "cache",
		Usage:  "path to cache ",
		Value:  "./cache/",
		EnvVar: "ARES_CACHE",
	},
	cli.StringFlag{
		Name:  "path",
		Usage: "path to static files",
		Value: "",
	},
	cli.StringFlag{
		Name:  "c,config",
		Usage: "config file",
		Value: "config.toml",
	},
}

type Cmd struct {
	*cli.App
}

func VersionAction(c *cli.Context) {
	fmt.Println(color.YellowString(fmt.Sprintf("Ares: Phishing toolkit.")))
}

func New() *Cmd {
	app := cli.NewApp()
	app.Name = "Ares"
	app.Author = ""
	app.Usage = "ares"
	app.Description = `Phishing toolkit`
	app.Flags = globalFlags
	app.CustomAppHelpTemplate = helpTemplate
	app.Commands = []cli.Command{
		{
			Name:   "version",
			Action: VersionAction,
		},
	}

	app.Before = func(c *cli.Context) error {
		return nil
	}

	app.Action = func(c *cli.Context) {
		options := []func(*server.Server){
			server.Address(c.String("port")),
			server.Cache(c.String("cache")),
			server.Config(c.String("config")),
		}

		if s := c.String("tlsport"); s != "" {
			options = append(options, server.TLSAddress(s))
		}

		srvr := server.New(
			options...,
		)

		srvr.Run()
	}

	return &Cmd{
		App: app,
	}
}
