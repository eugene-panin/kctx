package main

import (
	"os"

	"github.com/eugene-panin/kctx/internal/app"
)

func main() {
	os.Exit(app.Run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}
