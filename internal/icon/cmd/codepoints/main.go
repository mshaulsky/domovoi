// Command codepoints prints the hexadecimal code points of every icon the
// package uses, one per line, for the font subsetting script.
package main

import (
	"fmt"

	"github.com/mshaulsky/domovoi/internal/icon"
)

func main() {
	for _, ic := range icon.All() {
		fmt.Printf("%X\n", int(ic))
	}
}
