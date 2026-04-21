package main

import (
	"os"

	"go-task-manager/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
