package main

import (
	"fmt"
	"os"

	"github.com/ralabarta/agentproof/internal/app"
)

var version = "dev"

func main() {
	code, err := app.Run(os.Args[1:], version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "agentproof:", err)
	}
	os.Exit(code)
}
