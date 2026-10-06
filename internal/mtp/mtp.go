package mtp

import "errors"

// ErrNotSupported is returned on platforms without MTP/COM support.
var ErrNotSupported = errors.New("MTP not supported on this platform")

// MTPFile is the minimal file metadata returned by a scan.
type MTPFile struct {
	Name         string
	Size         int64
	DateTaken    *string
	DateModified *string
}

// Client abstracts MTP device operations.
type Client interface {
	// QuickScanFolders returns the list of folder names under Internal Storage.
	QuickScanFolders() ([]string, error)

	// ScanFiles returns all files in the given folder.
	ScanFiles(folderName string) ([]MTPFile, error)

	// CopyFileToDir copies a file from the device to a local directory.
	// Returns the local path of the copied file.
	CopyFileToDir(folderName, fileName, destDir string) (string, error)
}
