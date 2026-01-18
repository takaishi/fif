package preview

// Preview represents a code preview with context lines
type Preview struct {
	File      string
	StartLine int
	Lines     []string
	HitLine   int // The line number that matched (1-based, relative to file)
}
