package main

import (
	"embed"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/bvalat/simple-iphone-explorer/internal/mtp"
	"github.com/bvalat/simple-iphone-explorer/internal/scanner"
	"github.com/bvalat/simple-iphone-explorer/internal/server"
	"github.com/gin-gonic/gin"
)

//go:embed templates
var templateFS embed.FS

func main() {
	gin.SetMode(gin.ReleaseMode)
	port := flag.Int("port", 5000, "HTTP listen port")
	jsonFile := flag.String("json", "iphone_files_simple.json", "Path to scan data JSON")
	flag.Parse()

	thumbnailsDir := filepath.Join("static", "thumbnails")
	cacheDir := filepath.Join(os.TempDir(), "iphone_explorer_cache")
	os.MkdirAll(cacheDir, 0o755)

	sc := scanner.New(*jsonFile, thumbnailsDir)
	client := mtp.NewClient()

	fmt.Println("iPhone Scanner – Go version")
	fmt.Println("============================================================")

	if sc.LoadFromDisk() {
		stats := sc.GetStats()
		fmt.Printf("Existing data loaded: %v files\n", stats["total_files"])
		fmt.Printf("Last scan: %v\n", stats["scan_date"])
		// Auto-generate missing thumbnails in background.
		go scanner.RunThumbnailWorker(sc, client)
	} else {
		fmt.Println("No existing scan data found.")
		fmt.Println("Starting iPhone scan + thumbnail generation...")
		go scanner.RunScanAndThumbnails(sc, client)
	}

	srv := &server.Server{
		Scanner:      sc,
		MTP:          client,
		FullImageDir: cacheDir,
		TemplateFS:   templateFS,
	}

	r := server.NewRouter(srv)

	fmt.Printf("\nServer: http://localhost:%d\n", *port)
	fmt.Printf("Progress: http://localhost:%d/api/progress\n", *port)
	fmt.Printf("Rescan: http://localhost:%d/api/rescan\n", *port)

	if err := r.Run(fmt.Sprintf("0.0.0.0:%d", *port)); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
