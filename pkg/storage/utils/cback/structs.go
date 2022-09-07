package cback

import "time"

type Group struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Backup struct {
	ID         int    `json:"id"`
	Group      Group  `json:"group"`
	Repository string `json:"repository"`
	Username   string `json:"username"`
	Name       string `json:"name"`
	Source     string `json:"source"`
}

type Snapshot struct {
	ID    string    `json:"id"`
	Time  time.Time `json:"time"`
	Paths []string  `json:"paths"`
}

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

type Restore struct {
	ID           int    `json:"id"`
	BackupID     int    `json:"backup_id"`
	SnapshotID   string `json:"snapshot"`
	Destionation string `json:"destination"`
	Pattern      string `json:"pattern"`
	Status       int    `json:"status"`
}

func (r *Resource) IsDir() bool {
	return r.Type == "dir"
}

func (r *Resource) IsFile() bool {
	return r.Type == "file"
}
