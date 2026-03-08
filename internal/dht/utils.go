package dht

import (
	"regexp"
	"sort"
	"strings"
)

var (
	// Regex to remove non-alphanumeric characters.
	nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9\s]`)
	
	// Expanded set of stop words (English, Spanish, Programming).
	stopWords = map[string]struct{}{
		"the": {}, "and": {}, "for": {}, "with": {}, "this": {}, "that": {}, 
		"from": {}, "into": {}, "near": {}, "over": {}, "some": {}, "your": {}, 
		"here": {}, "there": {}, "which": {}, "about": {}, "after": {}, "been": {},
		"los": {}, "las": {}, "con": {}, "por": {}, "para": {}, "este": {}, 
		"esta": {}, "desde": {}, "entre": {}, "sobre": {}, "hacia": {},
		"func": {}, "class": {}, "return": {}, "import": {}, "package": {}, 
		"type": {}, "struct": {}, "const": {}, "var": {}, "interface": {},
		"void": {}, "public": {}, "private": {}, "static": {}, "string": {},
		"byte": {}, "error": {}, "nil": {}, "true": {}, "false": {},
		"hive": {}, "discovery": {}, "node": {}, "peer": {},
	}
)

// ExtractKeywords normalizes a string and returns a list of unique keywords.
func ExtractKeywords(s string) []string {
	s = strings.ToLower(s)
	s = nonAlphanumericRegex.ReplaceAllString(s, " ")
	words := strings.Fields(s)
	keywordMap := make(map[string]struct{})
	for _, word := range words {
		if len(word) < 3 {
			continue
		}
		if _, isStop := stopWords[word]; isStop {
			continue
		}
		keywordMap[word] = struct{}{}
	}
	var keywords []string
	for k := range keywordMap {
		keywords = append(keywords, k)
	}
	return keywords
}

// ExtractTopKeywords extracts keywords and returns only the most "significant" ones.
func ExtractTopKeywords(s string, limit int) []string {
	keywords := ExtractKeywords(s)
	if len(keywords) <= limit {
		return keywords
	}
	sort.Slice(keywords, func(i, j int) bool {
		return len(keywords[i]) > len(keywords[j])
	})
	return keywords[:limit]
}

// Split divides data into segments of specified size.
func Split(data []byte, size int) [][]byte {
	if size <= 0 {
		return [][]byte{data}
	}
	var chunks [][]byte
	for i := 0; i < len(data); i += size {
		end := i + size
		if end > len(data) {
			end = len(data)
		}
		chunks = append(chunks, data[i:end])
	}
	return chunks
}

// Join concatenates a list of segments into a single byte slice.
func Join(chunks [][]byte) []byte {
	var totalLen int
	for _, c := range chunks {
		totalLen += len(c)
	}
	res := make([]byte, totalLen)
	var pos int
	for _, c := range chunks {
		copy(res[pos:], c)
		pos += len(c)
	}
	return res
}
