package core

type StorageProvider interface {
	GetStorage() Storage
}

type PathStrategy interface {
	Resolve(baseDir string, taskID string, title string, fileType string) (string, string)
}
