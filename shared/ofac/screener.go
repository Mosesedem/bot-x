package ofac

import (
	"encoding/xml"
	"io"
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
	for _, entry := range s.entries {
		if strings.Contains(valLower, entry) || strings.Contains(entry, valLower) {
			return true
		}
	}
	return false
}

// Count returns the number of unique SDN identifiers loaded.
func (s *Screener) Count() int {
	return len(s.entries)
}
