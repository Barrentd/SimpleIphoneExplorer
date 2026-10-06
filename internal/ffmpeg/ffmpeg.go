package ffmpeg

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	// BtbN releases – essentials build, small (~30MB zip).
	windowsURL = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-win64-gpl.zip"
	linuxURL   = "https://github.com/BtbN/FFmpeg-Builds/releases/download/latest/ffmpeg-master-latest-linux64-gpl.tar.xz"
)

var (
	resolvedPath string
	once         sync.Once
	resolveErr   error
)

// BinPath returns the path to a working ffmpeg binary.
// It checks, in order:
//  1. A cached copy next to the running executable.
//  2. ffmpeg in the system PATH.
//  3. Downloads a static build and caches it.
func BinPath() (string, error) {
	once.Do(func() {
		resolvedPath, resolveErr = locate()
	})
	return resolvedPath, resolveErr
}

func locate() (string, error) {
	// 1. Check cached copy next to executable.
	exeDir, _ := os.Executable()
	if exeDir != "" {
		exeDir = filepath.Dir(exeDir)
	} else {
		exeDir = "."
	}

	cached := filepath.Join(exeDir, ffmpegBin())
	if fileExists(cached) {
		fmt.Printf("[ffmpeg] Using cached binary: %s\n", cached)
		return cached, nil
	}

	// 2. Check system PATH.
	if p, err := lookPath(); err == nil {
		fmt.Printf("[ffmpeg] Found in PATH: %s\n", p)
		return p, nil
	}

	// 3. Download and cache.
	fmt.Println("[ffmpeg] Not found locally or in PATH. Downloading static build...")
	if err := download(exeDir); err != nil {
		return "", fmt.Errorf("ffmpeg download failed: %w", err)
	}
	if fileExists(cached) {
		fmt.Printf("[ffmpeg] Downloaded and cached: %s\n", cached)
		return cached, nil
	}
	return "", fmt.Errorf("ffmpeg binary not found after download")
}

func ffmpegBin() string {
	if runtime.GOOS == "windows" {
		return "ffmpeg.exe"
	}
	return "ffmpeg"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func download(destDir string) error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("auto-download only supported on Windows; install ffmpeg manually")
	}

	url := windowsURL
	fmt.Printf("[ffmpeg] Downloading from %s ...\n", url)

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	// Save zip to temp file.
	tmpFile, err := os.CreateTemp("", "ffmpeg-*.zip")
	if err != nil {
		return err
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	written, err := io.Copy(tmpFile, resp.Body)
	tmpFile.Close()
	if err != nil {
		return err
	}
	fmt.Printf("[ffmpeg] Downloaded %.1f MB\n", float64(written)/1024/1024)

	// Extract ffmpeg.exe from zip.
	return extractFFmpegFromZip(tmpPath, destDir)
}

func extractFFmpegFromZip(zipPath, destDir string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Look for bin/ffmpeg.exe inside the zip.
		name := filepath.Base(f.Name)
		if !strings.EqualFold(name, "ffmpeg.exe") {
			continue
		}
		if f.FileInfo().IsDir() {
			continue
		}

		fmt.Printf("[ffmpeg] Extracting %s ...\n", f.Name)

		src, err := f.Open()
		if err != nil {
			return err
		}
		defer src.Close()

		destPath := filepath.Join(destDir, "ffmpeg.exe")
		dst, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer dst.Close()

		if _, err := io.Copy(dst, src); err != nil {
			return err
		}
		fmt.Println("[ffmpeg] Extraction complete.")
		return nil
	}

	return fmt.Errorf("ffmpeg.exe not found in zip archive")
}
