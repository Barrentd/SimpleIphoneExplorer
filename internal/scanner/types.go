package scanner

// FileInfo represents a single file on the iPhone.
type FileInfo struct {
	Name          string  `json:"name"`
	Folder        string  `json:"folder"`
	FullPath      string  `json:"full_path"`
	Size          int64   `json:"size"`
	SizeFormatted string  `json:"size_formatted"`
	Extension     string  `json:"extension"`
	Category      string  `json:"category"`
	IsImage       bool    `json:"is_image"`
	IsVideo       bool    `json:"is_video"`
	HasThumbnail  bool    `json:"has_thumbnail"`
	ThumbnailPath *string `json:"thumbnail_path"`
	ThumbnailHash string  `json:"thumbnail_hash"`
	DateTaken     *string `json:"date_taken"`
	DateModified  *string `json:"date_modified"`
	BackedUp      bool    `json:"backed_up"`
	BackedUpPath  string  `json:"backed_up_path,omitempty"`
}

// FolderInfo represents a folder and its aggregate statistics.
type FolderInfo struct {
	Name          string         `json:"name"`
	FileCount     int            `json:"file_count"`
	TotalSize     string         `json:"total_size"`
	Categories    map[string]int `json:"categories"`
	HasThumbnails bool           `json:"has_thumbnails"` // true if at least some files have real thumbnails
}

// PaginatedResponse wraps a page of files with pagination metadata.
type PaginatedResponse struct {
	Files      []FileInfo `json:"files"`
	Total      int        `json:"total"`
	Page       int        `json:"page"`
	PageSize   int        `json:"page_size"`
	TotalPages int        `json:"total_pages"`
}

// ScanData is the top-level JSON structure persisted to disk.
type ScanData struct {
	ScanDate      string     `json:"scan_date"`
	TotalFiles    int        `json:"total_files"`
	TotalFolders  int        `json:"total_folders"`
	Files         []FileInfo `json:"files"`
	ScanStatus    string     `json:"scan_status"`
	ScanCompleted string     `json:"scan_completed,omitempty"`
}

// ScanProgress tracks an in-flight scan.
type ScanProgress struct {
	Scanning       bool    `json:"scanning"`
	CurrentFolder  string  `json:"current_folder"`
	FoldersScanned int     `json:"folders_scanned"`
	TotalFolders   int     `json:"total_folders"`
	FilesFound     int     `json:"files_found"`
	StartTime      *string `json:"start_time"`
}

// ThumbnailProgress tracks batch thumbnail generation.
type ThumbnailProgress struct {
	Running       bool   `json:"running"`
	Total         int    `json:"total"`
	Completed     int    `json:"completed"`
	Skipped       int    `json:"skipped"`
	Errors        int    `json:"errors"`
	CurrentFile   string `json:"current_file"`
	CurrentFolder string `json:"current_folder"`
	Phase         string `json:"phase"` // "scanning", "generating", "done"
}

// CopyProgress tracks a background copy operation.
type CopyProgress struct {
	Status      string       `json:"status"`
	Total       int          `json:"total"`
	Completed   int          `json:"completed"`
	CurrentFile string       `json:"current_file"`
	Results     []CopyResult `json:"results"`
	Errors      []string     `json:"errors"`
}

// CopyResult is the outcome for a single file copy.
type CopyResult struct {
	Name         string        `json:"name"`
	Folder       string        `json:"folder"`
	Success      bool          `json:"success"`
	DestPath     string        `json:"dest_path,omitempty"`
	Error        string        `json:"error,omitempty"`
	Verification *VerifyResult `json:"verification"`
}

// VerifyResult is the SHA-256 verification outcome.
type VerifyResult struct {
	Name       string  `json:"name"`
	NameMatch  bool    `json:"name_match"`
	SizeMatch  bool    `json:"size_match"`
	HashMatch  bool    `json:"hash_match"`
	SourceHash *string `json:"source_hash"`
	DestHash   *string `json:"dest_hash"`
	SourceSize *int64  `json:"source_size"`
	DestSize   *int64  `json:"dest_size"`
	Verified   bool    `json:"verified"`
	Error      string  `json:"error,omitempty"`
}
