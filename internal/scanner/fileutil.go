package scanner

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// FormatSize formats bytes into a human-readable string matching Python's output.
func FormatSize(sizeBytes int64) string {
	if sizeBytes == 0 {
		return "0 B"
	}
	units := []string{"B", "KB", "MB", "GB"}
	size := float64(sizeBytes)
	i := 0
	for size >= 1024 && i < len(units)-1 {
		size /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", size, units[i])
}

// GetCategory determines the file category from MIME type and extension.
func GetCategory(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	mimeType := mime.TypeByExtension(ext)

	if strings.HasPrefix(mimeType, "image/") {
		return "image"
	}
	if strings.HasPrefix(mimeType, "video/") {
		return "video"
	}

	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".bmp", ".webp", ".heic", ".heif":
		return "image"
	case ".mp4", ".mov", ".avi", ".mkv", ".m4v":
		return "video"
	case ".mp3", ".m4a", ".wav", ".aac":
		return "audio"
	}
	return "other"
}

// MD5Hash computes md5(s) and returns hex string — used for thumbnail filenames.
func MD5Hash(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// ComputeSHA256 computes the SHA-256 hash of a file.
func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
