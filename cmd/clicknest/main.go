package main

import (
	"flag"
	"io/fs"
	"log"

	"github.com/danielthedm/clicknest/pkg/bootstrap"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	dataDir := flag.String("data", "./data", "data directory for databases")
	devMode := flag.Bool("dev", false, "enable development mode")
	flag.Parse()

	// Prepare embedded filesystems.
	var webFS fs.FS
	var sdkJS []byte
	if !*devMode {
		var err error
		webFS, err = fs.Sub(webBuildFS, "web_build")
		if err != nil {
			log.Fatalf("preparing web filesystem: %v", err)
		}
		sdkJS, err = fs.ReadFile(sdkDistFS, "sdk_dist/clicknest.js")
		if err != nil {
			log.Fatalf("reading sdk.js: %v", err)
		}
	}

	app := bootstrap.Setup(bootstrap.Config{
		Addr:    *addr,
		DataDir: *dataDir,
		DevMode: *devMode,
		WebFS:   webFS,
		SDKJS:   sdkJS,
	})
	defer app.Close()

	log.Printf("ClickNest started on %s (dev=%v, data=%s)", *addr, *devMode, *dataDir)
	app.Run()
}
