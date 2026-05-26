package ofac

import (
	"encoding/xml"
	"io"
	"math"
	"os"
	"strings"
)

type Screener struct {
	entries []string
}

// New creates an empty screener.
func New() *Screener {
	return &Screener{entries: make([]string, 0)}
}

type SdnList struct {
	XMLName xml.Name   `xml:"sdnList"`
	Entries []SdnEntry `xml:"sdnEntry"`
}

type SdnEntry struct {
	FirstName   string `xml:"firstName"`
	LastName    string `xml:"lastName"`
	AddressList struct {
		Addresses []Address `xml:"address"`
	} `xml:"addressList"`
}

type Address struct {
	Address1 string `xml:"address1"`
	Address2 string `xml:"address2"`
	City     string `xml:"city"`
	Country  string `xml:"country"`
}

// LoadFromFile parses the OFAC SDN XML file and builds the cache of blocked entries.
func (s *Screener) LoadFromFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	var list SdnList
	if err := xml.Unmarshal(data, &list); err != nil {
		return err
	}

	entries := make([]string, 0)
	for _, entry := range list.Entries {
		fullName := strings.TrimSpace(entry.FirstName + " " + entry.LastName)
		if fullName != "" {
			entries = append(entries, strings.ToLower(fullName))
		}
		for _, addr := range entry.AddressList.Addresses {
			a1 := strings.TrimSpace(addr.Address1)
			if a1 != "" {
				entries = append(entries, strings.ToLower(a1))
			}
			a2 := strings.TrimSpace(addr.Address2)
			if a2 != "" {
				entries = append(entries, strings.ToLower(a2))
			}
		}
	}
	s.entries = entries
	return nil
}

// Screen checks if a query matches any of the blocked SDN names or addresses.
func (s *Screener) Screen(value string) bool {
	valLower := strings.ToLower(strings.TrimSpace(value))
	if valLower == "" {
		return false
	}
	// First try simple substring checks for exact matches/contains.
	for _, entry := range s.entries {
		if strings.Contains(valLower, entry) || strings.Contains(entry, valLower) {
			return true
		}
	}

	// Fuzzy match using Levenshtein distance normalized by length.
	// If normalized distance <= 0.20 (i.e., >=80% similarity) treat as match.
	const threshold = 0.20
	for _, entry := range s.entries {
		d := levenshteinDistance(valLower, entry)
		maxLen := math.Max(float64(len(valLower)), float64(len(entry)))
		if maxLen == 0 {
			continue
		}
		norm := float64(d) / maxLen
		if norm <= threshold {
			return true
		}
	}
	return false
}

// levenshteinDistance computes the Levenshtein edit distance between two strings.
func levenshteinDistance(a, b string) int {
	la := len(a)
	lb := len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		ai := a[i-1]
		for j := 1; j <= lb; j++ {
			cost := 0
			if ai != b[j-1] {
				cost = 1
			}
			insertion := curr[j-1] + 1
			deletion := prev[j] + 1
			substitution := prev[j-1] + cost
			curr[j] = insertion
			if deletion < curr[j] {
				curr[j] = deletion
			}
			if substitution < curr[j] {
				curr[j] = substitution
			}
		}
		copy(prev, curr)
	}
	return prev[lb]
}

// Count returns the number of unique SDN identifiers loaded.
func (s *Screener) Count() int {
	return len(s.entries)
}
