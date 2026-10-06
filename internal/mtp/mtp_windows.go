//go:build windows

package mtp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// WinClient implements Client using Shell.Application COM automation.
// It caches the MTP storage folder and per-folder dispatches to avoid
// re-navigating the full MTP tree on every call.
type WinClient struct {
	mu            sync.Mutex
	comInit       bool
	storageDisp   *ole.IDispatch
	releaseChain  func()
	folderCache   map[string]*ole.IDispatch // folderName → Folder dispatch
}

func NewClient() Client {
	return &WinClient{
		folderCache: make(map[string]*ole.IDispatch),
	}
}

// ensureStorage lazily initialises COM and caches the Internal Storage dispatch.
func (c *WinClient) ensureStorage() error {
	if c.storageDisp != nil {
		return nil
	}

	if !c.comInit {
		ole.CoInitialize(0)
		c.comInit = true
	}

	storage, release, err := getStorageFolder()
	if err != nil {
		return err
	}
	c.storageDisp = storage
	c.releaseChain = release
	return nil
}

// getFolder returns a cached Folder dispatch for the given folderName.
func (c *WinClient) getFolder(folderName string) (*ole.IDispatch, error) {
	if disp, ok := c.folderCache[folderName]; ok {
		return disp, nil
	}

	var found *ole.IDispatch
	err := forEachItem(c.storageDisp, func(itemDisp *ole.IDispatch) bool {
		isFolder, _ := oleutil.GetProperty(itemDisp, "IsFolder")
		if isFolder.Val == 0 {
			return false
		}
		nameProp, _ := oleutil.GetProperty(itemDisp, "Name")
		if nameProp.ToString() == folderName {
			folderProp, err := oleutil.GetProperty(itemDisp, "GetFolder")
			if err == nil && folderProp.ToIDispatch() != nil {
				found = folderProp.ToIDispatch()
				itemDisp.AddRef()
				return true
			}
		}
		return false
	})
	if err != nil || found == nil {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("folder %s not found", folderName)
	}

	c.folderCache[folderName] = found
	return found, nil
}

func (c *WinClient) QuickScanFolders() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureStorage(); err != nil {
		return nil, err
	}

	var folders []string
	err := forEachItem(c.storageDisp, func(itemDisp *ole.IDispatch) bool {
		isFolder, _ := oleutil.GetProperty(itemDisp, "IsFolder")
		if isFolder.Val != 0 {
			nameProp, _ := oleutil.GetProperty(itemDisp, "Name")
			folders = append(folders, nameProp.ToString())
		}
		return false
	})
	if err != nil {
		return nil, err
	}
	return folders, nil
}

func (c *WinClient) ScanFiles(folderName string) ([]MTPFile, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureStorage(); err != nil {
		return nil, err
	}

	folderDisp, err := c.getFolder(folderName)
	if err != nil {
		return nil, err
	}

	var files []MTPFile
	err = forEachItem(folderDisp, func(itemDisp *ole.IDispatch) bool {
		isFolder, _ := oleutil.GetProperty(itemDisp, "IsFolder")
		if isFolder.Val != 0 {
			return false
		}

		nameProp, _ := oleutil.GetProperty(itemDisp, "Name")
		name := nameProp.ToString()

		var size int64
		sizeProp, err := oleutil.CallMethod(itemDisp, "ExtendedProperty", "System.Size")
		if err == nil && sizeProp.Value() != nil {
			switch v := sizeProp.Value().(type) {
			case int64:
				size = v
			case int32:
				size = int64(v)
			case int:
				size = int64(v)
			default:
				size = sizeProp.Val
			}
		}

		var dateTaken, dateMod *string
		dtProp, err := oleutil.CallMethod(itemDisp, "ExtendedProperty", "System.Photo.DateTaken")
		if err == nil && dtProp.Value() != nil {
			s := fmt.Sprintf("%v", dtProp.Value())
			dateTaken = &s
		}
		dmProp, err := oleutil.CallMethod(itemDisp, "ExtendedProperty", "System.DateModified")
		if err == nil && dmProp.Value() != nil {
			s := fmt.Sprintf("%v", dmProp.Value())
			dateMod = &s
		}

		files = append(files, MTPFile{
			Name:         name,
			Size:         size,
			DateTaken:    dateTaken,
			DateModified: dateMod,
		})
		return false
	})
	return files, err
}

func (c *WinClient) CopyFileToDir(folderName, fileName, destDir string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if err := c.ensureStorage(); err != nil {
		return "", err
	}

	folderDisp, err := c.getFolder(folderName)
	if err != nil {
		return "", err
	}

	// Find the file item.
	var fileItem *ole.IDispatch
	err = forEachItem(folderDisp, func(itemDisp *ole.IDispatch) bool {
		isFolder, _ := oleutil.GetProperty(itemDisp, "IsFolder")
		if isFolder.Val != 0 {
			return false
		}
		nameProp, _ := oleutil.GetProperty(itemDisp, "Name")
		if nameProp.ToString() == fileName {
			fileItem = itemDisp
			fileItem.AddRef()
			return true
		}
		return false
	})
	if err != nil {
		return "", err
	}
	if fileItem == nil {
		return "", fmt.Errorf("file %s not found in %s", fileName, folderName)
	}
	defer fileItem.Release()

	// Get destination namespace.
	shell, err := oleutil.CreateObject("Shell.Application")
	if err != nil {
		return "", fmt.Errorf("Shell.Application: %w", err)
	}
	shellDisp, _ := shell.QueryInterface(ole.IID_IDispatch)
	defer shellDisp.Release()

	destNS, err := oleutil.CallMethod(shellDisp, "NameSpace", destDir)
	if err != nil || destNS.ToIDispatch() == nil {
		return "", fmt.Errorf("NameSpace(%s): %w", destDir, err)
	}
	destDisp := destNS.ToIDispatch()
	defer destDisp.Release()

	// CopyHere
	_, err = oleutil.CallMethod(destDisp, "CopyHere", fileItem)
	if err != nil {
		return "", fmt.Errorf("CopyHere: %w", err)
	}

	// Poll for completion with adaptive intervals.
	destPath := filepath.Join(destDir, fileName)
	timeout := 120 * time.Second
	start := time.Now()
	pollInterval := 100 * time.Millisecond

	for !fileExists(destPath) {
		if time.Since(start) > timeout {
			return "", fmt.Errorf("timeout waiting for %s", fileName)
		}
		time.Sleep(pollInterval)
		// Back off: 100ms → 200ms → 500ms → 1s
		if pollInterval < time.Second {
			pollInterval = pollInterval * 2
			if pollInterval > time.Second {
				pollInterval = time.Second
			}
		}
	}

	// Wait for size to stabilize (3 stable reads).
	var lastSize int64 = -1
	stableCount := 0
	for stableCount < 3 {
		if time.Since(start) > timeout {
			break
		}
		info, err := os.Stat(destPath)
		if err != nil {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		sz := info.Size()
		if sz == lastSize && sz > 0 {
			stableCount++
		} else {
			stableCount = 0
		}
		lastSize = sz
		time.Sleep(200 * time.Millisecond)
	}

	if !fileExists(destPath) {
		return "", fmt.Errorf("file not found after copy: %s", destPath)
	}
	return destPath, nil
}

// ---------- helpers ----------

func forEachItem(folderDisp *ole.IDispatch, fn func(*ole.IDispatch) bool) error {
	items, err := oleutil.CallMethod(folderDisp, "Items")
	if err != nil {
		return fmt.Errorf("Items(): %w", err)
	}
	itemsDisp := items.ToIDispatch()
	defer itemsDisp.Release()

	countVar, err := oleutil.GetProperty(itemsDisp, "Count")
	if err != nil {
		return fmt.Errorf("Count: %w", err)
	}
	n := int(countVar.Val)

	for i := 0; i < n; i++ {
		itemVar, err := oleutil.CallMethod(itemsDisp, "Item", i)
		if err != nil {
			continue
		}
		d := itemVar.ToIDispatch()
		if d == nil {
			continue
		}
		stop := fn(d)
		if !stop {
			d.Release()
		}
		if stop {
			break
		}
	}
	return nil
}

// getStorageFolder navigates Shell → My Computer → iPhone → Internal Storage
// and returns the storage Folder dispatch. Caller must call release().
func getStorageFolder() (storage *ole.IDispatch, release func(), err error) {
	shell, err := oleutil.CreateObject("Shell.Application")
	if err != nil {
		return nil, nil, fmt.Errorf("Shell.Application: %w", err)
	}
	shellDisp, _ := shell.QueryInterface(ole.IID_IDispatch)

	computer, err := oleutil.CallMethod(shellDisp, "NameSpace", int32(17))
	if err != nil || computer.ToIDispatch() == nil {
		shellDisp.Release()
		return nil, nil, fmt.Errorf("NameSpace(17): %w", err)
	}
	compDisp := computer.ToIDispatch()

	compItems, err := oleutil.CallMethod(compDisp, "Items")
	if err != nil {
		compDisp.Release()
		shellDisp.Release()
		return nil, nil, fmt.Errorf("computer.Items: %w", err)
	}
	compItemsDisp := compItems.ToIDispatch()

	count, _ := oleutil.GetProperty(compItemsDisp, "Count")
	n := int(count.Val)

	fmt.Println("Connecting to iPhone via MTP...")

	for i := 0; i < n; i++ {
		item, err := oleutil.CallMethod(compItemsDisp, "Item", i)
		if err != nil {
			continue
		}
		itemDisp := item.ToIDispatch()
		if itemDisp == nil {
			continue
		}
		nameProp, _ := oleutil.GetProperty(itemDisp, "Name")
		name := nameProp.ToString()

		if strings.Contains(name, "iPhone") || strings.Contains(name, "Apple") {
			fmt.Printf("iPhone found: %s\n", name)

			iphoneFolder, err := oleutil.GetProperty(itemDisp, "GetFolder")
			if err != nil {
				itemDisp.Release()
				continue
			}
			iphoneFolderDisp := iphoneFolder.ToIDispatch()
			if iphoneFolderDisp == nil {
				itemDisp.Release()
				continue
			}

			// Find Internal Storage.
			iphoneItems, err := oleutil.CallMethod(iphoneFolderDisp, "Items")
			if err != nil {
				iphoneFolderDisp.Release()
				itemDisp.Release()
				continue
			}
			iphoneItemsDisp := iphoneItems.ToIDispatch()

			iCount, _ := oleutil.GetProperty(iphoneItemsDisp, "Count")
			iN := int(iCount.Val)

			for j := 0; j < iN; j++ {
				sItem, err := oleutil.CallMethod(iphoneItemsDisp, "Item", j)
				if err != nil {
					continue
				}
				sDisp := sItem.ToIDispatch()
				if sDisp == nil {
					continue
				}
				sName, _ := oleutil.GetProperty(sDisp, "Name")
				sNameStr := sName.ToString()

				if strings.Contains(sNameStr, "Internal Storage") {
					storageFolder, err := oleutil.GetProperty(sDisp, "GetFolder")
					if err != nil {
						sDisp.Release()
						continue
					}
					storageFolderDisp := storageFolder.ToIDispatch()
					if storageFolderDisp == nil {
						sDisp.Release()
						continue
					}

					fmt.Println("MTP connection established and cached.")

					cleanup := func() {
						storageFolderDisp.Release()
						sDisp.Release()
						iphoneItemsDisp.Release()
						iphoneFolderDisp.Release()
						itemDisp.Release()
						compItemsDisp.Release()
						compDisp.Release()
						shellDisp.Release()
					}
					return storageFolderDisp, cleanup, nil
				}
				sDisp.Release()
			}

			iphoneItemsDisp.Release()
			iphoneFolderDisp.Release()
			itemDisp.Release()
		} else {
			itemDisp.Release()
		}
	}

	compItemsDisp.Release()
	compDisp.Release()
	shellDisp.Release()
	return nil, nil, fmt.Errorf("iPhone not found")
}


func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
