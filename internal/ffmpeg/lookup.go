package ffmpeg

import "os/exec"

func lookPath() (string, error) {
	return exec.LookPath(ffmpegBin())
}
