//go:build windows

package server

import (
	"fmt"
	"runtime"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// browseFolderNative opens the Windows BrowseForFolder dialog via COM.
// Must run on a dedicated OS thread with COM initialized.
func browseFolderNative() (string, error) {
	type result struct {
		path string
		err  error
	}
	ch := make(chan result, 1)

	go func() {
		// Lock this goroutine to an OS thread for COM.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		ole.CoInitialize(0)
		defer ole.CoUninitialize()

		shell, err := oleutil.CreateObject("Shell.Application")
		if err != nil {
			ch <- result{"", fmt.Errorf("Shell.Application: %w", err)}
			return
		}
		shellDisp, _ := shell.QueryInterface(ole.IID_IDispatch)
		defer shellDisp.Release()

		// BrowseForFolder(hwnd, title, options, rootFolder)
		// options: 0x0040 = new style, 0x0010 = edit box
		// rootFolder: 17 = ssfDRIVES (My Computer)
		folderVar, err := oleutil.CallMethod(shellDisp, "BrowseForFolder",
			int32(0),
			"Choisir le dossier de destination",
			int32(0x0040|0x0010),
			int32(17),
		)
		if err != nil {
			ch <- result{"", fmt.Errorf("BrowseForFolder: %w", err)}
			return
		}

		if folderVar.VT == ole.VT_EMPTY || folderVar.VT == ole.VT_NULL || folderVar.ToIDispatch() == nil {
			ch <- result{"", nil} // user cancelled
			return
		}

		folderDisp := folderVar.ToIDispatch()
		defer folderDisp.Release()

		selfVar, err := oleutil.GetProperty(folderDisp, "Self")
		if err != nil {
			ch <- result{"", fmt.Errorf("Self: %w", err)}
			return
		}
		selfDisp := selfVar.ToIDispatch()
		defer selfDisp.Release()

		pathVar, err := oleutil.GetProperty(selfDisp, "Path")
		if err != nil {
			ch <- result{"", fmt.Errorf("Path: %w", err)}
			return
		}

		ch <- result{pathVar.ToString(), nil}
	}()

	r := <-ch
	return r.path, r.err
}
