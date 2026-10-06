package server

import (
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	img "github.com/bvalat/simple-iphone-explorer/internal/imaging"
	"github.com/bvalat/simple-iphone-explorer/internal/scanner"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

var hashRegex = regexp.MustCompile(`^[a-f0-9]{32}\.jpg$`)

// ---------- Browse Folder (native Windows dialog) ----------

func (s *Server) handleBrowseFolder(c *gin.Context) {
	path, err := browseFolderNative()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": err.Error()})
		return
	}
	if path == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "cancelled"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "path": path})
}
var safeNameRegex = regexp.MustCompile(`^[\w\-. ]+$`)

// ---------- Page ----------

func (s *Server) handleIndex(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", s.indexHTML)
}

// ---------- Files / Folders / Stats ----------

func (s *Server) handleFiles(c *gin.Context) {
	category := c.DefaultQuery("category", "all")
	folders := c.QueryArray("folders")
	if len(folders) == 0 {
		folders = c.QueryArray("folders[]")
	}
	sortBy := c.DefaultQuery("sort_by", "name")
	sortOrder := c.DefaultQuery("sort_order", "asc")

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	if pageSize < 1 {
		pageSize = 50
	}
	if pageSize > 200 {
		pageSize = 200
	}

	result := s.Scanner.GetFilesPaginated(category, folders, sortBy, sortOrder, page, pageSize)
	c.JSON(http.StatusOK, result)
}

func (s *Server) handleFolders(c *gin.Context) {
	c.JSON(http.StatusOK, s.Scanner.GetFoldersWithStats())
}

func (s *Server) handleStats(c *gin.Context) {
	c.JSON(http.StatusOK, s.Scanner.GetStats())
}

func (s *Server) handleProgress(c *gin.Context) {
	c.JSON(http.StatusOK, s.Scanner.GetProgress())
}

// ---------- Rescan ----------

func (s *Server) handleRescan(c *gin.Context) {
	if s.Scanner.IsScanning() {
		c.JSON(http.StatusOK, gin.H{"success": false, "message": "Scan already in progress"})
		return
	}

	go scanner.RunScanAndThumbnails(s.Scanner, s.MTP)
	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Scan progressif demarre"})
}

func (s *Server) handleThumbnailProgress(c *gin.Context) {
	c.JSON(http.StatusOK, s.Scanner.GetThumbnailProgress())
}

// ---------- Move (copy + verify + mark backed up) ----------

func (s *Server) handleMoveFiles(c *gin.Context) {
	var body struct {
		Files    []struct {
			Folder string `json:"folder"`
			Name   string `json:"name"`
		} `json:"files"`
		DestPath string `json:"dest_path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid JSON"})
		return
	}
	if len(body.Files) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "Aucun fichier specifie"})
		return
	}
	if body.DestPath == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "Dossier de destination non specifie"})
		return
	}

	destPath := filepath.Clean(body.DestPath)
	moveID := uuid.New().String()

	files := make([]struct{ Folder, Name string }, len(body.Files))
	for i, f := range body.Files {
		files[i] = struct{ Folder, Name string }{f.Folder, f.Name}
	}

	// markBackedUp=true: after SHA-256 verification, files are marked as backed up.
	go scanner.RunCopyWorker(s.Scanner, s.MTP, moveID, files, destPath, true)

	c.JSON(http.StatusOK, gin.H{"success": true, "move_id": moveID})
}

// ---------- Thumbnails ----------

func (s *Server) handleThumbnail(c *gin.Context) {
	filename := c.Param("filename")
	if !hashRegex.MatchString(filename) {
		c.Status(http.StatusNotFound)
		return
	}
	path := filepath.Join(s.Scanner.ThumbnailsDir(), filename)
	if _, err := os.Stat(path); err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(path)
}

func (s *Server) handleGenerateThumbnail(c *gin.Context) {
	folder := c.Param("folder")
	filename := c.Param("filename")

	if !safeNameRegex.MatchString(folder) || !safeNameRegex.MatchString(filename) {
		c.Status(http.StatusBadRequest)
		return
	}

	hash := scanner.MD5Hash(folder + "_" + filename)
	thumbFile := hash + ".jpg"
	thumbPath := filepath.Join(s.Scanner.ThumbnailsDir(), thumbFile)

	// Return existing real thumbnail (skip placeholders < 3KB).
	if info, err := os.Stat(thumbPath); err == nil && info.Size() > 3000 {
		c.JSON(http.StatusOK, gin.H{"success": true, "thumbnail_path": "thumbnails/" + thumbFile})
		return
	}

	// Try to copy from MTP and generate a real thumbnail.
	tmpDir, err := os.MkdirTemp("", "iphone-thumb-*")
	if err != nil {
		fmt.Printf("generate-thumbnail: tmpdir error: %v\n", err)
		c.JSON(http.StatusInternalServerError, gin.H{"success": false, "error": err.Error()})
		return
	}
	defer os.RemoveAll(tmpDir)

	localPath, err := s.MTP.CopyFileToDir(folder, filename, tmpDir)
	if err != nil {
		fmt.Printf("generate-thumbnail: MTP copy error for %s/%s: %v\n", folder, filename, err)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "MTP copy failed"})
		return
	}

	// Use video thumbnail generation for video files.
	cat := scanner.GetCategory(filename)
	var genErr error
	if cat == "video" {
		genErr = img.GenerateVideoThumbnail(localPath, thumbPath)
	} else {
		genErr = img.GenerateRealThumbnail(localPath, thumbPath)
	}
	if genErr != nil {
		fmt.Printf("generate-thumbnail: error for %s: %v\n", localPath, genErr)
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "thumbnail generation failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "thumbnail_path": "thumbnails/" + thumbFile})
}

// ---------- Full Image ----------

func (s *Server) handleFullImage(c *gin.Context) {
	folder := c.Param("folder")
	filename := c.Param("filename")

	if !safeNameRegex.MatchString(folder) || !safeNameRegex.MatchString(filename) {
		c.Status(http.StatusBadRequest)
		return
	}

	cachedPath, err := img.ServeFullImage(s.MTP, s.FullImageDir, folder, filename)
	if err != nil {
		fmt.Printf("fullimage: error for %s/%s: %v\n", folder, filename, err)
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ext := strings.ToLower(filepath.Ext(filename))
	mimeType := mime.TypeByExtension(ext)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	c.Header("Content-Type", mimeType)
	c.File(cachedPath)
}

// ---------- Copy ----------

func (s *Server) handleCopyFiles(c *gin.Context) {
	var body struct {
		Files    []struct {
			Folder string `json:"folder"`
			Name   string `json:"name"`
		} `json:"files"`
		DestPath string `json:"dest_path"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid JSON"})
		return
	}
	if len(body.Files) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "No files specified"})
		return
	}
	if body.DestPath == "" {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "No destination path specified"})
		return
	}

	destPath := filepath.Clean(body.DestPath)
	copyID := uuid.New().String()

	// Convert to the type expected by RunCopyWorker.
	files := make([]struct{ Folder, Name string }, len(body.Files))
	for i, f := range body.Files {
		files[i] = struct{ Folder, Name string }{f.Folder, f.Name}
	}

	go scanner.RunCopyWorker(s.Scanner, s.MTP, copyID, files, destPath)

	c.JSON(http.StatusOK, gin.H{"success": true, "copy_id": copyID})
}

func (s *Server) handleCopyProgress(c *gin.Context) {
	id := c.Param("id")
	val, ok := s.Scanner.CopyProgress.Load(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown copy ID"})
		return
	}
	c.JSON(http.StatusOK, val)
}

// ---------- Verify ----------

func (s *Server) handleVerifyFiles(c *gin.Context) {
	var body struct {
		Files []struct {
			Folder   string `json:"folder"`
			Name     string `json:"name"`
			DestPath string `json:"dest_path"`
		} `json:"files"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "error": "Invalid JSON"})
		return
	}
	if len(body.Files) == 0 {
		c.JSON(http.StatusOK, gin.H{"success": false, "error": "No files specified"})
		return
	}

	var results []scanner.VerifyResult
	for _, f := range body.Files {
		if f.Folder == "" || f.Name == "" || f.DestPath == "" {
			results = append(results, scanner.VerifyResult{
				Name:  f.Name,
				Error: "Missing parameters",
			})
			continue
		}

		vr := verifyLocally(f.Name, f.DestPath)
		results = append(results, vr)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "results": results})
}

// verifyLocally checks destination file integrity without re-downloading from MTP.
func verifyLocally(filename, destPath string) scanner.VerifyResult {
	vr := scanner.VerifyResult{Name: filename}

	info, err := os.Stat(destPath)
	if err != nil {
		vr.Error = "Destination file not found"
		return vr
	}

	vr.NameMatch = filepath.Base(destPath) == filename
	sz := info.Size()
	vr.DestSize = &sz

	hash, err := scanner.ComputeSHA256(destPath)
	if err != nil {
		vr.Error = fmt.Sprintf("hash error: %v", err)
		return vr
	}
	vr.DestHash = &hash
	vr.Verified = vr.NameMatch
	return vr
}
