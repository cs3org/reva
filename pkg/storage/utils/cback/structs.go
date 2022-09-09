package cback

import "time"

// Group is the group in cback
type Group struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Backup represents the metadata information of a backuo job
type Backup struct {
	ID         int    `json:"id"`
	Group      Group  `json:"group"`
	Repository string `json:"repository"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Source     string `json:"source"`
}

// Snapshot represents the metadata information of a snapshot in a backup
type Snapshot struct {
	ID    string    `json:"id"`
	Time  time.Time `json:"time"`
	Paths []string  `json:"paths"`
}

// Resource represents the metadata information of a file stored in cback
type Resource struct {
	Name  string  `json:"name"`
	Type  string  `json:"type"`
	Mode  uint64  `json:"mode"`
	MTime float64 `json:"mtime"`
	ATime float64 `json:"atime"`
	CTime float64 `json:"ctime"`
	Inode uint64  `json:"inode"`
	Size  uint64  `json:"size"`
}

// Restore represents the metadata information of a restore job
type Restore struct {
	ID           int    `json:"id"`
	BackupID     int    `json:"backup_id"`
	SnapshotID   string `json:"snapshot"`
	Destionation string `json:"destination"`
	Pattern      string `json:"pattern"`
	Status       int    `json:"status"`
}

// IsDir returns true if the resoure is a directory
func (r *Resource) IsDir() bool {
	return r.Type == "dir"
}

// IsFile returns true if the resoure is a file
func (r *Resource) IsFile() bool {
	return r.Type == "file"
}
