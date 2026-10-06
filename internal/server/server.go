package server

import (
	"embed"
	"io/fs"
	"strings"

	"github.com/bvalat/simple-iphone-explorer/internal/mtp"
	"github.com/bvalat/simple-iphone-explorer/internal/scanner"
	"github.com/gin-gonic/gin"
)

// Server holds dependencies for HTTP handlers.
type Server struct {
	Scanner      *scanner.Scanner
	MTP          mtp.Client
	FullImageDir string
	TemplateFS   embed.FS // must contain "templates/index.html"
	indexHTML    []byte   // raw index.html bytes (served as-is, no template engine)
}

// NewRouter creates a gin.Engine with all routes registered.
func NewRouter(s *Server) *gin.Engine {
	r := gin.Default()

	// Read index.html as raw bytes — serve as-is to avoid html/template escaping JS.
	raw, err := fs.ReadFile(s.TemplateFS, "templates/index.html")
	if err != nil {
		panic("failed to read embedded template: " + err.Error())
	}
	s.indexHTML = raw

	// Immutable cache for hash-named thumbnails.
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/static/thumbnails/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})
	r.Static("/static", "./static")

	// Routes.
	r.GET("/", s.handleIndex)
	r.GET("/api/files", s.handleFiles)
	r.GET("/api/folders", s.handleFolders)
	r.GET("/api/stats", s.handleStats)
	r.GET("/api/progress", s.handleProgress)
	r.GET("/api/thumbnail-progress", s.handleThumbnailProgress)
	r.GET("/api/rescan", s.handleRescan)
	r.GET("/api/filter", s.handleFiles) // alias

	r.GET("/api/browse-folder", s.handleBrowseFolder)
	r.POST("/api/move-files", s.handleMoveFiles)
	r.GET("/api/move-progress/:id", s.handleCopyProgress) // same format as copy progress

	r.GET("/api/thumbnail/:filename", s.handleThumbnail)
	r.GET("/api/generate-thumbnail/:folder/:filename", s.handleGenerateThumbnail)
	r.GET("/api/fullimage/:folder/:filename", s.handleFullImage)

	r.POST("/api/copy-files", s.handleCopyFiles)
	r.GET("/api/copy-progress/:id", s.handleCopyProgress)
	r.POST("/api/verify-files", s.handleVerifyFiles)

	return r
}
