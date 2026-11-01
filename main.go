package main

import (
	"flag"
	"log"
	"os"

	"aether-dots/internal/desktopui"
	"aether-dots/internal/store"
)

func main() {
	dir := os.Getenv("AETHER_DOTS_DATA_DIR")
	if dir == "" {
		var err error
		dir, err = store.DefaultDataDir()
		if err != nil {
			log.Fatal(err)
		}
	}
	flag.StringVar(&dir, "data-dir", dir, "local task store folder")
	flag.Parse()
	if err := desktopui.Run(dir); err != nil {
		log.Fatal(err)
	}
}
