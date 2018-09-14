package main

import (
	"flag"
	"fmt"
	"ftpSync"
	"os"
)

var optionConfigFile = flag.String("config", "./config.xml", "configure xml file")
var version = flag.Bool("version", false, "print current version")

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

	ftpSync.Run()
}
