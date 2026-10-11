package service

// Wiki says where the service keeps the wiki pages.
type Wiki struct {
	// Dir is the folder of the pages. It is in a git repository: each
	// write of a page is a commit there.
	Dir string
}

// WithWiki sets the wiki and returns s. Without it, each wiki function
// returns an error.
func (s *Service) WithWiki(wiki Wiki) *Service {
	s.wiki = wiki
	return s
}
