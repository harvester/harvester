package main

import (
	"log"

	"github.com/rancher/harvester/pkg/console"
)

func main() {
	if err := console.RunConsole(); err != nil {
		log.Panicln(err)
	}
}
