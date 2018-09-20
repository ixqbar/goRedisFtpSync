package main

import (
	"flag"
	"fmt"
	"ftpSync"
	"net/http"
	_ "net/http/pprof"
	"os"
)

var optionConfigFile = flag.String("config", "./config.xml", "configure xml file")
var version = flag.Bool("version", false, "print current version")
var monitor = flag.Bool("monitor", false, "open pprof to monitor")

func usage() {
	fmt.Printf("Usage: %s options\nOptions:\n", os.Args[0])
	flag.PrintDefaults()
	os.Exit(0)
}

func main() {
	flag.Usage = usage
	flag.Parse()

	if len(os.Args) < 2 {
		usage()
	}

	if *version {
		fmt.Printf("%s\n", ftpSync.VERSION)
		os.Exit(0)
	}

	_, err := ftpSync.ParseXmlConfig(*optionConfigFile)
	if err != nil {
		ftpSync.Logger.Print(err)
		os.Exit(1)
	}

	if *monitor {
		go func() {
			http.ListenAndServe("0.0.0.0:6060", nil)
		}()
	}

	ftpSync.Run()
}
