package imaging

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/bvalat/simple-iphone-explorer/internal/mtp"
)

const maxCacheBytes = 2 * 1024 * 1024 * 1024 // 2 GB

var cacheMu sync.Mutex

// ServeFullImage copies a file from MTP (if not cached) and returns the local
// cached path. Evicts oldest files when cache exceeds maxCacheBytes.
func ServeFullImage(client mtp.Client, cacheDir, folder, filename string) (cachedPath string, err error) {
	ext := filepath.Ext(filename)
	cacheKey := md5hex(folder + "_" + filename)
	cachedFile := cacheKey + ext
	cachedPath = filepath.Join(cacheDir, cachedFile)

	// Serve from cache if available.
	if _, err := os.Stat(cachedPath); err == nil {
		return cachedPath, nil
	}

	os.MkdirAll(cacheDir, 0o755)

	// Copy from MTP to a temp dir, then move to cache.
	tmpDir, err := os.MkdirTemp("", "iphone-fullimg-*")
	if err != nil {
		return "", fmt.Errorf("temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	localPath, err := client.CopyFileToDir(folder, filename, tmpDir)
	if err != nil || localPath == "" {
		return "", fmt.Errorf("MTP copy failed: %w", err)
	}

	// Move to cache.
	if err := copyFile(localPath, cachedPath); err != nil {
		return "", fmt.Errorf("cache write: %w", err)
	}

	// Evict old entries if cache too large (async).
	go evictCache(cacheDir)

	return cachedPath, nil
}

// evictCache removes oldest files until cache is under maxCacheBytes.
func evictCache(cacheDir string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}

	type entry struct {
		path    string
		size    int64
		modTime int64
	}

	var files []entry
	var totalSize int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		sz := info.Size()
		files = append(files, entry{filepath.Join(cacheDir, e.Name()), sz, info.ModTime().UnixNano()})
		totalSize += sz
	}

	if totalSize <= maxCacheBytes {
		return
	}

	// Sort oldest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime < files[j].modTime
	})

	for _, f := range files {
		if totalSize <= maxCacheBytes {
			break
		}
		os.Remove(f.path)
		totalSize -= f.size
	}
}

func md5hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
