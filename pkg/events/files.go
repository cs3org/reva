package events

// FileUploaded is emitted when a file is uploaded
type FileUploaded struct {
	FileID string
}

//FileDownloaded is emitted when a file is downloaded
type FileDownloaded struct {
	FileID string
}

// ItemTrashed is emitted when a file or folder is trashed
type ItemTrashed struct {
	FileID string
}

// ItemMoved is emitted when a file or folder is moved
type ItemMoved struct {
	FileID string
}

// ItemPurged is emitted when a file or folder is removed from trashbin
type ItemPurged struct {
	FileID string
}

// ItemRestored is emitted when a file or folder is restored from trashbin
type ItemRestored struct {
	FileID string
}

// FileVersionRestored is emitted when a file version is restored
type FileVersionRestored struct {
	FileID string
}
