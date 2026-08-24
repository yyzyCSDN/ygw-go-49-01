package transfer

// SplitChunks returns the number of chunks a size needs at chunkSize.
func SplitChunks(size, chunkSize int64) int {
	if size <= 0 || chunkSize <= 0 {
		return 0
	}
	return int((size + chunkSize - 1) / chunkSize)
}

// ChunkBounds returns the byte range of chunk index: [start, end).
func ChunkBounds(size, chunkSize int64, index int) (int64, int64) {
	start := int64(index) * chunkSize
	if start >= size {
		return 0, 0
	}
	end := start + chunkSize
	if end > size {
		end = size
	}
	return start, end
}
