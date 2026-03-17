package main

import (
	"os"

	"github.com/Cluas/mcporter-go/internal/mcporter"
)

func main() {
	os.Exit(mcporter.Run(os.Args[1:], os.Stdout, os.Stderr))
}
