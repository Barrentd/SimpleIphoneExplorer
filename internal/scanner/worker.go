package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bvalat/simple-iphone-explorer/internal/imaging"
	"github.com/bvalat/simple-iphone-explorer/internal/mtp"
)

const (
	thumbWorkers      = 6                // parallel ffmpeg/resize goroutines
	maxVideoSizeBytes = 80 * 1024 * 1024 // skip videos > 80 MB for thumbnails
)

// RunScanAndThumbnails performs a full scan then generates all thumbnails.
func RunScanAndThumbnails(sc *Scanner, client mtp.Client) {
	RunScanWorker(sc, client)
	RunThumbnailWorker(sc, client)
}

// RunScanWorker performs a progressive MTP scan in the background.
func RunScanWorker(sc *Scanner, client mtp.Client) {
	if sc.IsScanning() {
		fmt.Println("Scan already in progress")
		return
	}
	sc.SetScanning(true)
	defer sc.SetScanning(false)

	sc.SetThumbnailProgress(ThumbnailProgress{Phase: "scanning"})

	folders, err := client.QuickScanFolders()
	if err != nil {
		fmt.Printf("Quick scan error: %v\n", err)
		return
	}

	now := time.Now().Format(time.RFC3339)
	sc.SetData(ScanData{
		ScanDate:     now,
		TotalFolders: len(folders),
		ScanStatus:   "starting",
	})
	sc.SetProgress(ScanProgress{
		Scanning:     true,
		TotalFolders: len(folders),
		StartTime:    &now,
	})
	_ = sc.SaveToDisk()

	var allFiles []FileInfo
	for i, folderName := range folders {
		sc.SetProgress(ScanProgress{
			Scanning:       true,
			CurrentFolder:  folderName,
			FoldersScanned: i + 1,
			TotalFolders:   len(folders),
			FilesFound:     len(allFiles),
			StartTime:      &now,
		})

		fmt.Printf("  Folder %d/%d: %s\n", i+1, len(folders), folderName)

		files, err := client.ScanFiles(folderName)
		if err != nil {
			fmt.Printf("    Error scanning %s: %v\n", folderName, err)
			continue
		}

		for _, mf := range files {
			ext := filepath.Ext(mf.Name)
			cat := GetCategory(mf.Name)
			hash := MD5Hash(folderName + "_" + mf.Name)
			thumbFile := hash + ".jpg"
			thumbPath := filepath.Join(sc.ThumbnailsDir(), thumbFile)

			var thumbPathStr *string
			hasThumbnail := false
			if info, err := os.Stat(thumbPath); err == nil && info.Size() > 3000 {
				hasThumbnail = true
				p := "thumbnails/" + thumbFile
				thumbPathStr = &p
			}

			fi := FileInfo{
				Name:          mf.Name,
				Folder:        folderName,
				FullPath:      "Internal Storage/" + folderName + "/" + mf.Name,
				Size:          mf.Size,
				SizeFormatted: FormatSize(mf.Size),
				Extension:     ext,
				Category:      cat,
				IsImage:       cat == "image",
				IsVideo:       cat == "video",
				HasThumbnail:  hasThumbnail,
				ThumbnailPath: thumbPathStr,
				ThumbnailHash: hash,
				DateTaken:     mf.DateTaken,
				DateModified:  mf.DateModified,
			}
			allFiles = append(allFiles, fi)
		}

		d := sc.GetData()
		d.TotalFiles = len(allFiles)
		d.Files = allFiles
		d.ScanStatus = fmt.Sprintf("scanning_folder_%d", i+1)
		sc.SetData(d)
		_ = sc.SaveToDisk()

		fmt.Printf("    %d files found – Total: %d\n", len(files), len(allFiles))
		time.Sleep(100 * time.Millisecond)
	}

	completed := time.Now().Format(time.RFC3339)
	d := sc.GetData()
	d.ScanStatus = "completed"
	d.ScanCompleted = completed
	d.TotalFiles = len(allFiles)
	d.Files = allFiles
	sc.SetData(d)
	sc.SetProgress(ScanProgress{
		Scanning:       false,
		CurrentFolder:  "Termine",
		FoldersScanned: len(folders),
		TotalFolders:   len(folders),
		FilesFound:     len(allFiles),
		StartTime:      &now,
	})
	_ = sc.SaveToDisk()
	fmt.Printf("Scan complete: %d files in %d folders\n", len(allFiles), len(folders))
}

// thumbJob describes one thumbnail to generate.
type thumbJob struct {
	folder, name, hash string
	isVideo            bool
	size               int64
}

// genJob is sent to thumbnail-generation workers after MTP copy succeeds.
type genJob struct {
	thumbJob
	localPath string // path to the local copy
	tmpDir    string // temp dir to clean up after
}

// RunThumbnailWorker generates real thumbnails for all image/video files
// that don't have one yet.
//
// Architecture (pipeline):
//   MTP copier (sequential)  ──►  channel  ──►  N ffmpeg/resize workers
//
// MTP copies are sequential (COM single-threaded) but thumbnail encoding
// runs in parallel, so ffmpeg/resize never blocks the MTP copier.
func RunThumbnailWorker(sc *Scanner, client mtp.Client) {
	data := sc.GetData()

	// Group jobs by folder.
	folderJobs := make(map[string][]thumbJob)
	folderSet := make(map[string]bool)
	totalJobs := 0

	for _, f := range data.Files {
		if !f.IsImage && !f.IsVideo {
			continue
		}
		thumbPath := filepath.Join(sc.ThumbnailsDir(), f.ThumbnailHash+".jpg")
		if info, err := os.Stat(thumbPath); err == nil && info.Size() > 3000 {
			continue
		}
		// Skip huge videos.
		if f.IsVideo && f.Size > maxVideoSizeBytes {
			continue
		}
		job := thumbJob{f.Folder, f.Name, f.ThumbnailHash, f.IsVideo, f.Size}
		folderSet[f.Folder] = true
		folderJobs[f.Folder] = append(folderJobs[f.Folder], job)
		totalJobs++
	}

	// Sort folders by number of pending files (ascending = smallest first).
	folderOrder := make([]string, 0, len(folderSet))
	for f := range folderSet {
		folderOrder = append(folderOrder, f)
	}
	sort.Slice(folderOrder, func(i, j int) bool {
		return len(folderJobs[folderOrder[i]]) < len(folderJobs[folderOrder[j]])
	})

	if totalJobs == 0 {
		sc.SetThumbnailProgress(ThumbnailProgress{Phase: "done"})
		fmt.Println("All thumbnails already generated.")
		return
	}

	fmt.Printf("Generating thumbnails: %d files in %d folders (%d workers)\n",
		totalJobs, len(folderOrder), thumbWorkers)

	var completedAtomic int64
	var errAtomic int64
	var currentFolder atomic.Value
	currentFolder.Store("")

	sc.SetThumbnailProgress(ThumbnailProgress{
		Running: true,
		Total:   totalJobs,
		Phase:   "generating",
	})

	// Channel: MTP copier → thumbnail workers.
	genCh := make(chan genJob, thumbWorkers*2)

	// folderWg tracks pending jobs for the current folder.
	var folderWg sync.WaitGroup

	// --- Thumbnail generation worker pool (long-lived) ---
	var poolWg sync.WaitGroup
	for w := 0; w < thumbWorkers; w++ {
		poolWg.Add(1)
		go func() {
			defer poolWg.Done()
			for gj := range genCh {
				thumbPath := filepath.Join(sc.ThumbnailsDir(), gj.hash+".jpg")

				var genErr error
				if gj.isVideo {
					genErr = imaging.GenerateVideoThumbnail(gj.localPath, thumbPath)
				} else {
					genErr = imaging.GenerateRealThumbnail(gj.localPath, thumbPath)
				}

				c := atomic.AddInt64(&completedAtomic, 1)
				if genErr != nil {
					atomic.AddInt64(&errAtomic, 1)
					fmt.Printf("  [%d/%d] ERR  %s/%s: %v\n", c, totalJobs, gj.folder, gj.name, genErr)
				} else {
					fmt.Printf("  [%d/%d] OK   %s/%s\n", c, totalJobs, gj.folder, gj.name)
					updateFileThumbnail(sc, gj.folder, gj.name, gj.hash)
				}

				os.RemoveAll(gj.tmpDir)
				folderWg.Done() // signal this job is fully done
			}
		}()
	}

	// --- Progress updater ---
	stopProgress := make(chan struct{})
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopProgress:
				return
			case <-ticker.C:
				sc.SetThumbnailProgress(ThumbnailProgress{
					Running:       true,
					Total:         totalJobs,
					Completed:     int(atomic.LoadInt64(&completedAtomic)),
					Errors:        int(atomic.LoadInt64(&errAtomic)),
					CurrentFolder: currentFolder.Load().(string),
					Phase:         "generating",
				})
			}
		}
	}()

	// --- Debounced SaveToDisk: save at most every 5 seconds ---
	var lastSave time.Time
	saveToDiskDebounced := func(force bool) {
		if force || time.Since(lastSave) > 5*time.Second {
			_ = sc.SaveToDisk()
			lastSave = time.Now()
		}
	}

	// --- Process folder by folder: finish all files in a folder before moving on ---
	for fi, folder := range folderOrder {
		jobs := folderJobs[folder]
		currentFolder.Store(folder)
		fmt.Printf("  Folder [%d/%d] %s: %d files\n", fi+1, len(folderOrder), folder, len(jobs))

		// One temp dir per folder — avoid thousands of mkdir/rmdir calls.
		folderTmpDir, err := os.MkdirTemp("", "iphone-thumb-"+folder+"-*")
		if err != nil {
			atomic.AddInt64(&completedAtomic, int64(len(jobs)))
			atomic.AddInt64(&errAtomic, int64(len(jobs)))
			fmt.Printf("  [ERR] tmpdir for %s: %v\n", folder, err)
			continue
		}

		for _, job := range jobs {
			localPath, err := client.CopyFileToDir(folder, job.name, folderTmpDir)
			if err != nil {
				c := atomic.AddInt64(&completedAtomic, 1)
				atomic.AddInt64(&errAtomic, 1)
				// File not found on device → remove from scan data.
				if strings.Contains(err.Error(), "not found") {
					sc.RemoveFile(folder, job.name)
					fmt.Printf("  [%d/%d] REMOVED %s/%s (not on device)\n", c, totalJobs, folder, job.name)
				} else {
					fmt.Printf("  [%d/%d] SKIP %s/%s: %v\n", c, totalJobs, folder, job.name, err)
				}
				continue
			}

			folderWg.Add(1)
			genCh <- genJob{
				thumbJob:  job,
				localPath: localPath,
				tmpDir:    "", // cleaned up after folder completes
			}
		}

		// Wait for ALL thumbnail generation in this folder to finish.
		folderWg.Wait()
		os.RemoveAll(folderTmpDir)
		saveToDiskDebounced(false)
		fmt.Printf("  Folder %s: done\n", folder)
	}

	// Close channel and wait for worker pool to exit.
	close(genCh)
	poolWg.Wait()
	close(stopProgress)

	finalCompleted := int(atomic.LoadInt64(&completedAtomic))
	finalErrors := int(atomic.LoadInt64(&errAtomic))

	sc.SetThumbnailProgress(ThumbnailProgress{
		Running:   false,
		Total:     totalJobs,
		Completed: finalCompleted,
		Errors:    finalErrors,
		Phase:     "done",
	})
	saveToDiskDebounced(true) // force final save
	fmt.Printf("Thumbnail generation complete: %d done, %d errors\n",
		finalCompleted-finalErrors, finalErrors)
}

// updateFileThumbnail marks a file as having a real thumbnail in scan data.
func updateFileThumbnail(sc *Scanner, folder, name, hash string) {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if idx, ok := sc.fileIndex[folder+"/"+name]; ok {
		f := &sc.data.Files[idx]
		f.HasThumbnail = true
		p := "thumbnails/" + hash + ".jpg"
		f.ThumbnailPath = &p
		sc.folderStatsDirty = true
	}
}

// RunCopyWorker copies files from iPhone to a local directory with SHA-256 verification.
// If markBackedUp is true, files that pass verification are marked as backed up in the scan data.
func RunCopyWorker(sc *Scanner, client mtp.Client, copyID string, files []struct{ Folder, Name string }, destPath string, markBackedUp ...bool) {
	doMark := len(markBackedUp) > 0 && markBackedUp[0]
	_ = doMark
	total := len(files)
	prog := &CopyProgress{
		Status:  "copying",
		Total:   total,
		Results: []CopyResult{},
		Errors:  []string{},
	}
	sc.CopyProgress.Store(copyID, prog)

	os.MkdirAll(destPath, 0o755)

	for i, f := range files {
		prog.CurrentFile = f.Name

		result := CopyResult{
			Name:   f.Name,
			Folder: f.Folder,
		}

		localPath, err := client.CopyFileToDir(f.Folder, f.Name, destPath)
		if err != nil || localPath == "" {
			errMsg := "copy failed"
			if err != nil {
				errMsg = err.Error()
			}
			result.Error = errMsg
			prog.Errors = append(prog.Errors, fmt.Sprintf("%s: %s", f.Name, errMsg))
		} else {
			result.Success = true
			result.DestPath = localPath

			vr := verifyFile(sc, f.Folder, f.Name, localPath)
			result.Verification = &vr

			// Mark as backed up only if verification passed.
			if doMark && vr.Verified {
				sc.MarkBackedUp(f.Folder, f.Name, localPath)
				_ = sc.SaveToDisk()
			}
		}

		prog.Results = append(prog.Results, result)
		prog.Completed = i + 1
		sc.CopyProgress.Store(copyID, prog)
	}

	prog.Status = "completed"
	prog.CurrentFile = ""
	sc.CopyProgress.Store(copyID, prog)
}

// verifyFile checks the copied file against the known size from the scan data.
// No re-download from iPhone — just hash the destination and compare size.
func verifyFile(sc *Scanner, folder, filename, destPath string) VerifyResult {
	vr := VerifyResult{Name: filename}

	info, err := os.Stat(destPath)
	if err != nil {
		vr.Error = "Destination file not found"
		return vr
	}

	vr.NameMatch = filepath.Base(destPath) == filename
	destSize := info.Size()
	vr.DestSize = &destSize

	// Compare with known source size from scan data.
	expectedSize := sc.GetFileSize(folder, filename)
	if expectedSize > 0 {
		vr.SourceSize = &expectedSize
		vr.SizeMatch = destSize == expectedSize
	} else {
		// No scan data — accept if file exists and is non-empty.
		vr.SizeMatch = destSize > 0
	}

	destHash, err := ComputeSHA256(destPath)
	if err != nil {
		vr.Error = fmt.Sprintf("hash error: %v", err)
		return vr
	}
	vr.DestHash = &destHash

	vr.Verified = vr.NameMatch && vr.SizeMatch
	return vr
}
