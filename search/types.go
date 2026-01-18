package search

// SearchResult represents a single search result from ripgrep
type SearchResult struct {
	File   string // 相対パス
	Line   int    // 1-based
	Column int
	Text   string // マッチ行
}
