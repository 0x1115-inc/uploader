package model

type FileRecord struct {
	ID          string
	Filename    string
	ContentType string
	SizeBytes   int64
	Bucket      string
	ObjectKey   string
	CreatedAt   string
}

