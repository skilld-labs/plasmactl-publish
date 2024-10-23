// Package executes Launchr application.
package main

import (
	"github.com/launchrctl/launchr"

	_ "github.com/skilld-labs/plasmactl-publish"
)

func main() {
	launchr.RunAndExit()
}
