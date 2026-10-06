package imaging

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/bvalat/simple-iphone-explorer/internal/ffmpeg"
	"github.com/disintegration/imaging"
)

const ffmpegTimeout = 30 * time.Second

// ffmpegConvertToJPEG uses ffmpeg to convert any image/video to a JPEG frame.
// For videos it extracts a frame at 1s; for images it simply decodes and re-encodes.
func ffmpegConvertToJPEG(srcPath, destJPEG string, isVideo bool) error {
	ffmpegBin, err := ffmpeg.BinPath()
	if err != nil {
		return fmt.Errorf("ffmpeg not available: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), ffmpegTimeout)
	defer cancel()

	if isVideo {
		for _, ss := range []string{"1", "0"} {
			args := []string{"-ss", ss, "-i", srcPath, "-frames:v", "1", "-q:v", "2", "-y", destJPEG}
			cmd := exec.CommandContext(ctx, ffmpegBin, args...)
			cmd.CombinedOutput()
			if fi, err := os.Stat(destJPEG); err == nil && fi.Size() > 0 {
				return nil
			}
		}
		return fmt.Errorf("ffmpeg produced no frame for video %s", filepath.Base(srcPath))
	}

	args := []string{"-i", srcPath, "-frames:v", "1", "-q:v", "2", "-y", destJPEG}
	cmd := exec.CommandContext(ctx, ffmpegBin, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("ffmpeg error: %v – %s", err, string(output))
	}
	return nil
}

// resizeToThumbnail opens a JPEG and resizes it to 200x200.
func resizeToThumbnail(srcJPEG, destPath string) error {
	img, err := imaging.Open(srcJPEG, imaging.AutoOrientation(true))
	if err != nil {
		return fmt.Errorf("open for resize: %v", err)
	}
	thumb := imaging.Fit(img, 200, 200, imaging.Lanczos)
	return imaging.Save(thumb, destPath, imaging.JPEGQuality(85))
}

// needsFFmpeg returns true for formats that Go's image stdlib cannot decode.
func needsFFmpeg(srcPath string) bool {
	ext := strings.ToLower(filepath.Ext(srcPath))
	switch ext {
	case ".heic", ".heif", ".avif", ".webp", ".tiff", ".tif":
		return true
	}
	return false
}

// GenerateVideoThumbnail extracts a frame from a video file using ffmpeg
// and creates a 200x200 thumbnail JPEG.
func GenerateVideoThumbnail(srcPath, destPath string) error {
	// destPath is already "hash.jpg", so use hash without extension for the temp frame.
	base := strings.TrimSuffix(filepath.Base(destPath), filepath.Ext(destPath))
	tmpFrame := filepath.Join(filepath.Dir(srcPath), "_frame_"+base+".jpg")
	defer os.Remove(tmpFrame)

	if err := ffmpegConvertToJPEG(srcPath, tmpFrame, true); err != nil {
		return err
	}
	return resizeToThumbnail(tmpFrame, destPath)
}

// GenerateRealThumbnail creates a 200x200 thumbnail from a local image file.
// For standard formats (JPEG, PNG, GIF, BMP) it uses the imaging library directly.
// For HEIC/HEIF/AVIF and other exotic formats, it falls back to ffmpeg for decoding.
func GenerateRealThumbnail(srcPath, destPath string) error {
	if needsFFmpeg(srcPath) {
		// Use ffmpeg to convert to JPEG first, then resize.
		base := strings.TrimSuffix(filepath.Base(destPath), filepath.Ext(destPath))
		tmpJPEG := filepath.Join(filepath.Dir(srcPath), "_conv_"+base+".jpg")
		defer os.Remove(tmpJPEG)

		if err := ffmpegConvertToJPEG(srcPath, tmpJPEG, false); err != nil {
			return err
		}
		return resizeToThumbnail(tmpJPEG, destPath)
	}

	// Standard format: decode directly with imaging library.
	img, err := imaging.Open(srcPath, imaging.AutoOrientation(true))
	if err != nil {
		return err
	}
	thumb := imaging.Fit(img, 200, 200, imaging.Lanczos)
	return imaging.Save(thumb, destPath, imaging.JPEGQuality(85))
}
