package cppbridge

// GetFlavor returns recognized architecture flavor
func GetFlavor() string {
	return getFlavor()
}

// MemInfo stats from C++ allocator
type MemInfo struct {
	InUse uint64
}

// GetMemInfo returns current C++ allocator stats
func GetMemInfo() MemInfo {
	return memInfo()
}
