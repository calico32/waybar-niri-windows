//go:build debug

package lib

import (
	"os"
)

func init() {
	logOutput = os.Stdout
}
