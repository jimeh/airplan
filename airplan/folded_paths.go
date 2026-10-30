package airplan

// foldedPaths records case-folded logical paths and every directory prefix
// beneath which a path lies. Duplicate and file-versus-directory checks then
// cost one lookup per path segment instead of a comparison with every earlier
// path, which keeps marker and bundle validation linear for large inventories.
type foldedPaths struct {
	paths map[string]string // folded path -> latest original path
	dirs  map[string]string // folded directory -> first original path beneath it
}

func newFoldedPaths(capacity int) *foldedPaths {
	return &foldedPaths{
		paths: make(map[string]string, capacity),
		dirs:  make(map[string]string, capacity),
	}
}

// get returns the original path recorded for fold.
func (f *foldedPaths) get(fold string) (string, bool) {
	previous, ok := f.paths[fold]
	return previous, ok
}

// directoryConflict returns a recorded path that is a directory ancestor of
// fold, or that lies beneath fold as though fold were a directory.
func (f *foldedPaths) directoryConflict(fold string) (string, bool) {
	if previous, ok := f.dirs[fold]; ok {
		return previous, true
	}
	for index, char := range fold {
		if char != '/' {
			continue
		}
		if previous, ok := f.paths[fold[:index]]; ok {
			return previous, true
		}
	}
	return "", false
}

// add records original under fold and indexes its directory prefixes.
func (f *foldedPaths) add(fold, original string) {
	f.paths[fold] = original
	for index, char := range fold {
		if char != '/' {
			continue
		}
		if _, ok := f.dirs[fold[:index]]; !ok {
			f.dirs[fold[:index]] = original
		}
	}
}
