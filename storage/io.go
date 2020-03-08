package storage

type ReadResult struct {
	Data  []byte
	Error error
}

type WriteResult struct {
	Error error
}
