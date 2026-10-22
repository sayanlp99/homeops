// Command invdump prints the positioned words the invoice parser sees in a PDF, and the
// parse result. Development aid for supporting new invoice layouts.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"homeops/internal/invoice"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: invdump file.pdf [-json]")
		os.Exit(2)
	}
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	pages, err := invoice.ReadPDF(data)
	if err != nil {
		panic(err)
	}
	if len(os.Args) > 2 && os.Args[2] == "-json" {
		json.NewEncoder(os.Stdout).Encode(pages)
		return
	}
	if len(os.Args) > 2 && os.Args[2] == "-lines" {
		for i, p := range pages {
			fmt.Printf("--- page %d\n", i+1)
			for _, l := range p.Lines {
				fmt.Printf("%6.1f ", l.Y)
				for _, w := range l.Words {
					fmt.Printf("[%.0f]%s ", w.X, w.Text)
				}
				fmt.Println()
			}
		}
		return
	}
	inv := invoice.Parse(pages)
	b, _ := json.MarshalIndent(inv, "", "  ")
	fmt.Println(string(b))
}
