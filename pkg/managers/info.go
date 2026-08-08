package managers

import "github.com/alswl/skm/skm/pkg/dal"

// EntryFiles lists the files under an entry's directory (`skm info`).
func (s *Services) EntryFiles(path string) []string {
	return dal.ListEntryFiles(path)
}

// EntryFrontmatter parses an entry's marker-file frontmatter (`skm info`).
func (s *Services) EntryFrontmatter(data []byte) (*dal.Frontmatter, []byte, error) {
	return dal.ParseFrontmatter(data)
}
