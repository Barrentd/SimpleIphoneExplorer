//go:build !windows

package mtp

// StubClient is a no-op MTP client for non-Windows platforms.
type StubClient struct{}

func NewClient() Client              { return &StubClient{} }
func (c *StubClient) Available() bool { return false }

func (c *StubClient) QuickScanFolders() ([]string, error) {
	return nil, ErrNotSupported
}

func (c *StubClient) ScanFiles(folderName string) ([]MTPFile, error) {
	return nil, ErrNotSupported
}

func (c *StubClient) CopyFileToDir(folderName, fileName, destDir string) (string, error) {
	return "", ErrNotSupported
}
