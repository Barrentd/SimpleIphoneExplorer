package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Scanner holds scan state, file data, and copy progress.
type Scanner struct {
	mu            sync.RWMutex
	jsonFile      string
	data          ScanData
	progress      ScanProgress
	thumbnailsDir string

	CopyProgress sync.Map // map[string]*CopyProgress keyed by UUID

	scanning  bool // true while a scan goroutine is running
	thumbProg ThumbnailProgress

	// Cached folder stats — invalidated on data change.
	folderStatsCache []FolderInfo
	folderStatsDirty bool

	// Index: "folder/name" → index in data.Files for O(1) lookups.
	fileIndex map[string]int
}

// New creates a Scanner and ensures thumbnail directories exist.
func New(jsonFile, thumbnailsDir string) *Scanner {
	os.MkdirAll(thumbnailsDir, 0o755)
	os.MkdirAll("static", 0o755)
	return &Scanner{
		jsonFile:      jsonFile,
		thumbnailsDir: thumbnailsDir,
	}
}

// ThumbnailsDir returns the path to the thumbnails directory.
func (s *Scanner) ThumbnailsDir() string { return s.thumbnailsDir }

// rebuildIndex rebuilds the file index. Must be called with mu held.
func (s *Scanner) rebuildIndex() {
	s.fileIndex = make(map[string]int, len(s.data.Files))
	for i := range s.data.Files {
		key := s.data.Files[i].Folder + "/" + s.data.Files[i].Name
		s.fileIndex[key] = i
	}
}

// JSONFile returns the path to the JSON data file.
func (s *Scanner) JSONFile() string { return s.jsonFile }

// ---------- Persistence ----------

// LoadFromDisk loads scan data from the JSON file. Returns false if not found.
func (s *Scanner) LoadFromDisk() bool {
	raw, err := os.ReadFile(s.jsonFile)
	if err != nil {
		return false
	}
	var data ScanData
	if err := json.Unmarshal(raw, &data); err != nil {
		fmt.Printf("Error parsing %s: %v\n", s.jsonFile, err)
		return false
	}

	// Recompute thumbnail presence for every file using parallel workers.
	// Placeholders are ~2KB; treat files under 3KB as "no real thumbnail"
	// so the frontend triggers on-demand generation.
	const numWorkers = 16
	var wg sync.WaitGroup
	ch := make(chan int, len(data.Files))
	for i := range data.Files {
		ch <- i
	}
	close(ch)

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range ch {
				f := &data.Files[i]
				hash := MD5Hash(f.Folder + "_" + f.Name)
				f.ThumbnailHash = hash
				thumbFile := hash + ".jpg"
				thumbPath := filepath.Join(s.thumbnailsDir, thumbFile)
				info, err := os.Stat(thumbPath)
				if err == nil && info.Size() > 3000 {
					f.HasThumbnail = true
					p := "thumbnails/" + thumbFile
					f.ThumbnailPath = &p
				} else {
					f.HasThumbnail = false
					f.ThumbnailPath = nil
				}
			}
		}()
	}
	wg.Wait()

	s.mu.Lock()
	s.data = data
	s.rebuildIndex()
	s.folderStatsDirty = true
	s.mu.Unlock()
	return true
}

// SaveToDisk writes scan data atomically (write tmp then rename).
func (s *Scanner) SaveToDisk() error {
	s.mu.RLock()
	raw, err := json.Marshal(s.data)
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := s.jsonFile + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.jsonFile)
}

// ---------- Accessors ----------

// GetData returns a snapshot copy of the scan data.
func (s *Scanner) GetData() ScanData {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.data
}

// SetData replaces scan data (used by scan worker).
func (s *Scanner) SetData(d ScanData) {
	s.mu.Lock()
	s.data = d
	s.rebuildIndex()
	s.folderStatsDirty = true
	s.mu.Unlock()
}

// GetProgress returns a snapshot of the scan progress.
func (s *Scanner) GetProgress() ScanProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.progress
}

// SetProgress updates scan progress (used by scan worker).
func (s *Scanner) SetProgress(p ScanProgress) {
	s.mu.Lock()
	s.progress = p
	s.mu.Unlock()
}

// IsScanning returns true if a scan is in progress.
func (s *Scanner) IsScanning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.scanning
}

// SetScanning sets the scanning flag.
func (s *Scanner) SetScanning(v bool) {
	s.mu.Lock()
	s.scanning = v
	s.mu.Unlock()
}

// GetThumbnailProgress returns a snapshot of thumbnail generation progress.
func (s *Scanner) GetThumbnailProgress() ThumbnailProgress {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.thumbProg
}

// SetThumbnailProgress updates thumbnail generation progress.
func (s *Scanner) SetThumbnailProgress(p ThumbnailProgress) {
	s.mu.Lock()
	s.thumbProg = p
	s.mu.Unlock()
}

// ---------- Query helpers ----------

// GetFiles returns files filtered by category/folders and sorted.
func (s *Scanner) GetFiles(category string, folders []string, sortBy, sortOrder string) []FileInfo {
	s.mu.RLock()
	files := make([]FileInfo, len(s.data.Files))
	copy(files, s.data.Files)
	s.mu.RUnlock()

	// Filter by category.
	if category != "" && category != "all" {
		filtered := files[:0]
		for _, f := range files {
			if f.Category == category {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	// Filter by folders.
	if len(folders) > 0 {
		folderSet := make(map[string]struct{}, len(folders))
		for _, f := range folders {
			folderSet[f] = struct{}{}
		}
		filtered := files[:0]
		for _, f := range files {
			if _, ok := folderSet[f.Folder]; ok {
				filtered = append(filtered, f)
			}
		}
		files = filtered
	}

	// Sort.
	desc := sortOrder == "desc"
	sort.SliceStable(files, func(i, j int) bool {
		var less bool
		switch sortBy {
		case "size":
			less = files[i].Size < files[j].Size
		case "folder":
			less = strings.ToLower(files[i].Folder) < strings.ToLower(files[j].Folder)
		case "extension":
			less = strings.ToLower(files[i].Extension) < strings.ToLower(files[j].Extension)
		case "date":
			di := ptrOr(files[i].DateTaken, ptrOr(files[i].DateModified, ""))
			dj := ptrOr(files[j].DateTaken, ptrOr(files[j].DateModified, ""))
			less = di < dj
		default: // "name"
			less = strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
		}
		if desc {
			return !less
		}
		return less
	})

	return files
}

// GetFilesPaginated returns a paginated, filtered, and sorted set of files.
func (s *Scanner) GetFilesPaginated(category string, folders []string, sortBy, sortOrder string, page, pageSize int) PaginatedResponse {
	files := s.GetFiles(category, folders, sortBy, sortOrder)

	total := len(files)
	totalPages := 0
	if pageSize > 0 {
		totalPages = (total + pageSize - 1) / pageSize
	}

	offset := (page - 1) * pageSize
	if offset > total {
		offset = total
	}
	end := offset + pageSize
	if end > total {
		end = total
	}

	return PaginatedResponse{
		Files:      files[offset:end],
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	}
}

// GetFoldersWithStats returns folder information with aggregate statistics.
// Results are cached and only recomputed when data changes.
func (s *Scanner) GetFoldersWithStats() []FolderInfo {
	s.mu.RLock()
	if !s.folderStatsDirty && s.folderStatsCache != nil {
		result := make([]FolderInfo, len(s.folderStatsCache))
		copy(result, s.folderStatsCache)
		s.mu.RUnlock()
		return result
	}
	s.mu.RUnlock()

	// Recompute under write lock.
	s.mu.Lock()
	defer s.mu.Unlock()

	// Double-check after acquiring write lock.
	if !s.folderStatsDirty && s.folderStatsCache != nil {
		result := make([]FolderInfo, len(s.folderStatsCache))
		copy(result, s.folderStatsCache)
		return result
	}

	type folderAcc struct {
		totalSize     int64
		fileCount     int
		categories    map[string]int
		hasThumbnails bool
	}

	accMap := make(map[string]*folderAcc)
	for _, f := range s.data.Files {
		acc, ok := accMap[f.Folder]
		if !ok {
			acc = &folderAcc{categories: make(map[string]int)}
			accMap[f.Folder] = acc
		}
		acc.fileCount++
		acc.totalSize += f.Size
		acc.categories[f.Category]++
		if f.HasThumbnail {
			acc.hasThumbnails = true
		}
	}

	result := make([]FolderInfo, 0, len(accMap))
	for name, acc := range accMap {
		result = append(result, FolderInfo{
			Name:          name,
			FileCount:     acc.fileCount,
			TotalSize:     FormatSize(acc.totalSize),
			Categories:    acc.categories,
			HasThumbnails: acc.hasThumbnails,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	s.folderStatsCache = result
	s.folderStatsDirty = false

	out := make([]FolderInfo, len(result))
	copy(out, result)
	return out
}

// GetFolders returns the sorted list of unique folder names.
func (s *Scanner) GetFolders() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	set := make(map[string]struct{})
	for _, f := range s.data.Files {
		set[f.Folder] = struct{}{}
	}
	folders := make([]string, 0, len(set))
	for f := range set {
		folders = append(folders, f)
	}
	sort.Strings(folders)
	return folders
}

// GetStats returns aggregate statistics.
func (s *Scanner) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.data.Files) == 0 && s.data.ScanDate == "" {
		return map[string]interface{}{}
	}

	var totalSize int64
	categories := map[string]int{}
	for _, f := range s.data.Files {
		totalSize += f.Size
		categories[f.Category]++
	}

	return map[string]interface{}{
		"total_files": len(s.data.Files),
		"total_size":  FormatSize(totalSize),
		"scan_date":   s.data.ScanDate,
		"categories":  categories,
	}
}

// UpdateFileLocations changes the folder for the given filenames in the JSON data.
func (s *Scanner) UpdateFileLocations(fileNames []string, targetFolder string) {
	nameSet := make(map[string]struct{}, len(fileNames))
	for _, n := range fileNames {
		nameSet[n] = struct{}{}
	}
	s.mu.Lock()
	for i := range s.data.Files {
		if _, ok := nameSet[s.data.Files[i].Name]; ok {
			s.data.Files[i].Folder = targetFolder
		}
	}
	s.rebuildIndex()
	s.folderStatsDirty = true
	s.mu.Unlock()
	_ = s.SaveToDisk()
}

// GetFileSize returns the known size for a file, or 0 if not found. O(1).
func (s *Scanner) GetFileSize(folder, name string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if idx, ok := s.fileIndex[folder+"/"+name]; ok {
		return s.data.Files[idx].Size
	}
	return 0
}

// MarkBackedUp marks a file as backed up with its destination path.
func (s *Scanner) MarkBackedUp(folder, name, destPath string) {
	s.mu.Lock()
	if idx, ok := s.fileIndex[folder+"/"+name]; ok {
		s.data.Files[idx].BackedUp = true
		s.data.Files[idx].BackedUpPath = destPath
		s.folderStatsDirty = true
	}
	s.mu.Unlock()
}

// RemoveFile removes a file from the scan data (e.g. not found on device).
func (s *Scanner) RemoveFile(folder, name string) {
	s.mu.Lock()
	key := folder + "/" + name
	if idx, ok := s.fileIndex[key]; ok {
		// Swap with last element and shrink slice.
		last := len(s.data.Files) - 1
		if idx != last {
			s.data.Files[idx] = s.data.Files[last]
			// Update index for the moved element.
			movedKey := s.data.Files[idx].Folder + "/" + s.data.Files[idx].Name
			s.fileIndex[movedKey] = idx
		}
		s.data.Files = s.data.Files[:last]
		delete(s.fileIndex, key)
		s.folderStatsDirty = true
	}
	s.mu.Unlock()
}

func ptrOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}
