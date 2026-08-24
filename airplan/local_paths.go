package airplan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// LocalPathPlanOptions describes one local file set before its upload kind and
// document roles have been resolved.
type LocalPathPlanOptions struct {
	Paths           []string
	Entrypoint      string
	PagePaths       []string
	AssetPaths      []string
	ForceCollection bool
	ForceDocument   bool
	EntryFormat     string
}

// LocalPathPlan is the deterministic result of classifying local upload paths.
// Document plans keep the selected entry separate and preserve declaration
// order for supporting pages and assets. CollectionPaths is populated only for
// collection plans.
type LocalPathPlan struct {
	Kind            UploadKind
	Entrypoint      string
	PagePaths       []string
	AssetPaths      []string
	CollectionPaths []string
}

type localPathCandidate struct {
	path string
	abs  string
	real string
}

type localPathRole uint8

const (
	localPathRoleAuto localPathRole = iota
	localPathRoleEntry
	localPathRolePage
	localPathRoleAsset
)

// PlanLocalPaths validates and classifies one set of named local files. It does
// not retain open descriptors or read payloads into memory.
func PlanLocalPaths(opts LocalPathPlanOptions) (*LocalPathPlan, error) {
	if len(opts.Paths) == 0 && opts.Entrypoint == "" {
		return nil, errors.New("airplan: local upload requires one or more named files")
	}
	if opts.ForceCollection && (opts.ForceDocument || opts.Entrypoint != "" ||
		len(opts.PagePaths) != 0 || len(opts.AssetPaths) != 0) {
		return nil, errors.New("airplan: collection mode cannot declare a document entry, page, or asset")
	}
	if opts.EntryFormat != "" {
		if _, err := ParseFormat(opts.EntryFormat); err != nil {
			return nil, err
		}
	}
	for _, value := range opts.Paths {
		if value == "" || value == "-" {
			return nil, errors.New("airplan: local upload requires named files")
		}
	}
	if opts.ForceCollection {
		if len(opts.Paths) == 0 {
			return nil, errors.New("airplan: collection requires one or more named files")
		}
		return &LocalPathPlan{
			Kind:            UploadKindCollection,
			CollectionPaths: append([]string(nil), opts.Paths...),
		}, nil
	}

	candidates := make([]localPathCandidate, 0,
		len(opts.Paths)+len(opts.PagePaths)+len(opts.AssetPaths)+1)
	byAbsolutePath := make(map[string]int)
	roles := make(map[string]localPathRole)
	add := func(value, declaration string, allowExisting bool) (localPathCandidate, error) {
		if value == "" || value == "-" {
			return localPathCandidate{}, fmt.Errorf(
				"airplan: %s requires a named file", declaration,
			)
		}
		candidate, err := inspectLocalPath(value)
		if err != nil {
			return localPathCandidate{}, err
		}
		if index, ok := byAbsolutePath[candidate.abs]; ok {
			if !allowExisting {
				return localPathCandidate{}, fmt.Errorf(
					"airplan: local file %q was supplied more than once", value,
				)
			}
			return candidates[index], nil
		}
		byAbsolutePath[candidate.abs] = len(candidates)
		candidates = append(candidates, candidate)
		return candidate, nil
	}
	setRole := func(candidate localPathCandidate, role localPathRole) error {
		if previous := roles[candidate.abs]; previous != localPathRoleAuto && previous != role {
			return fmt.Errorf(
				"airplan: local file %q has conflicting document roles",
				candidate.path,
			)
		}
		roles[candidate.abs] = role
		return nil
	}

	for _, value := range opts.Paths {
		if _, err := add(value, "local upload", false); err != nil {
			return nil, err
		}
	}
	var explicitEntry localPathCandidate
	if opts.Entrypoint != "" {
		var err error
		explicitEntry, err = add(opts.Entrypoint, "--entrypoint", true)
		if err != nil {
			return nil, err
		}
		if err := setRole(explicitEntry, localPathRoleEntry); err != nil {
			return nil, err
		}
	}
	for _, value := range opts.PagePaths {
		candidate, err := add(value, "--page", true)
		if err != nil {
			return nil, err
		}
		if err := setRole(candidate, localPathRolePage); err != nil {
			return nil, err
		}
	}
	for _, value := range opts.AssetPaths {
		candidate, err := add(value, "--asset", true)
		if err != nil {
			return nil, err
		}
		if err := setRole(candidate, localPathRoleAsset); err != nil {
			return nil, err
		}
	}

	documentIntent := opts.ForceDocument || opts.Entrypoint != "" ||
		len(opts.PagePaths) != 0 || len(opts.AssetPaths) != 0 || opts.EntryFormat != ""
	kind := UploadKindCollection
	switch {
	case opts.ForceCollection:
		kind = UploadKindCollection
	case documentIntent:
		kind = UploadKindDocument
	case len(candidates) == 1:
		text, err := localPathDefaultsToDocument(candidates[0])
		if err != nil {
			return nil, err
		}
		if text {
			kind = UploadKindDocument
		}
	case containsMarkdownOrHTML(candidates):
		kind = UploadKindDocument
	}
	if kind == UploadKindCollection {
		paths := make([]string, len(candidates))
		for index, candidate := range candidates {
			paths[index] = candidate.path
		}
		return &LocalPathPlan{Kind: kind, CollectionPaths: paths}, nil
	}

	entry := explicitEntry
	if entry.path == "" {
		entry = selectLocalEntrypoint(candidates, opts.EntryFormat)
	}
	if entry.path == "" {
		return nil, errors.New("airplan: document upload has no entry file")
	}
	if err := setRole(entry, localPathRoleEntry); err != nil {
		return nil, err
	}
	rootDeclared := filepath.Dir(entry.abs)
	rootReal, err := filepath.EvalSymlinks(rootDeclared)
	if err != nil {
		return nil, fmt.Errorf("airplan: resolve document root: %w", err)
	}
	for _, candidate := range candidates {
		if !localPathWithinRoot(rootDeclared, candidate.abs) {
			return nil, fmt.Errorf(
				"airplan: local file %q is outside the entry directory", candidate.path,
			)
		}
		if !localPathWithinRoot(rootReal, candidate.real) {
			return nil, fmt.Errorf(
				"airplan: local file %q resolves outside the entry directory", candidate.path,
			)
		}
		logical, err := filepath.Rel(rootDeclared, candidate.abs)
		if err != nil {
			return nil, fmt.Errorf("airplan: resolve document path %q: %w", candidate.path, err)
		}
		if err := ValidateBundlePath(filepath.ToSlash(logical)); err != nil {
			return nil, err
		}
	}

	entryFormat, err := localEntrypointFormat(entry, opts.EntryFormat)
	if err != nil {
		return nil, err
	}
	plan := &LocalPathPlan{Kind: kind, Entrypoint: entry.path}
	for _, candidate := range candidates {
		role := roles[candidate.abs]
		if role == localPathRoleEntry {
			continue
		}
		if role == localPathRoleAuto {
			role, err = inferLocalSupportingRole(candidate, entryFormat)
			if err != nil {
				return nil, err
			}
		}
		if role == localPathRolePage && entryFormat != FormatMarkdown {
			return nil, fmt.Errorf(
				"airplan: managed page %q requires a Markdown entry", candidate.path,
			)
		}
		switch role {
		case localPathRolePage:
			plan.PagePaths = append(plan.PagePaths, candidate.path)
		case localPathRoleAsset:
			plan.AssetPaths = append(plan.AssetPaths, candidate.path)
		default:
			return nil, fmt.Errorf("airplan: local file %q has no document role", candidate.path)
		}
	}
	return plan, nil
}

func inspectLocalPath(value string) (localPathCandidate, error) {
	abs, err := filepath.Abs(value)
	if err != nil {
		return localPathCandidate{}, fmt.Errorf("airplan: resolve local file %q: %w", value, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return localPathCandidate{}, fmt.Errorf("airplan: inspect local file %q: %w", value, err)
	}
	if !info.Mode().IsRegular() {
		return localPathCandidate{}, fmt.Errorf(
			"airplan: local file %q is not a regular file", value,
		)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return localPathCandidate{}, fmt.Errorf("airplan: resolve local file %q: %w", value, err)
	}
	return localPathCandidate{path: value, abs: abs, real: real}, nil
}

func containsMarkdownOrHTML(candidates []localPathCandidate) bool {
	for _, candidate := range candidates {
		ext := strings.ToLower(filepath.Ext(candidate.abs))
		if ext == ".md" || ext == ".markdown" || ext == ".html" || ext == ".htm" {
			return true
		}
	}
	return false
}

func selectLocalEntrypoint(candidates []localPathCandidate, forcedFormat string) localPathCandidate {
	if len(candidates) == 0 {
		return localPathCandidate{}
	}
	preferMarkdown := forcedFormat == "" || forcedFormat == "md"
	preferHTML := forcedFormat == "" || forcedFormat == "html"
	if preferMarkdown {
		for _, preferred := range []string{"readme.md", "index.md"} {
			for _, candidate := range candidates {
				if strings.EqualFold(filepath.Base(candidate.abs), preferred) &&
					candidateDirectoryContainsAll(candidate, candidates) {
					return candidate
				}
			}
		}
		for _, candidate := range candidates {
			ext := strings.ToLower(filepath.Ext(candidate.abs))
			if ext == ".md" || ext == ".markdown" {
				return candidate
			}
		}
	}
	if preferHTML {
		for _, candidate := range candidates {
			base := strings.ToLower(filepath.Base(candidate.abs))
			if (base == "index.html" || base == "index.htm") &&
				candidateDirectoryContainsAll(candidate, candidates) {
				return candidate
			}
		}
		for _, candidate := range candidates {
			ext := strings.ToLower(filepath.Ext(candidate.abs))
			if ext == ".html" || ext == ".htm" {
				return candidate
			}
		}
	}
	return candidates[0]
}

func candidateDirectoryContainsAll(entry localPathCandidate, candidates []localPathCandidate) bool {
	root := filepath.Dir(entry.abs)
	for _, candidate := range candidates {
		if !localPathWithinRoot(root, candidate.abs) {
			return false
		}
	}
	return true
}

func localEntrypointFormat(candidate localPathCandidate, forced string) (Format, error) {
	if forced != "" {
		return ParseFormat(forced)
	}
	ext := strings.ToLower(filepath.Ext(candidate.abs))
	switch ext {
	case ".md", ".markdown":
		return FormatMarkdown, nil
	case ".html", ".htm":
		return FormatHTML, nil
	default:
		return FormatText, nil
	}
}

func inferLocalSupportingRole(candidate localPathCandidate, entryFormat Format) (localPathRole, error) {
	if entryFormat != FormatMarkdown {
		return localPathRoleAsset, nil
	}
	ext := strings.ToLower(filepath.Ext(candidate.abs))
	if ext == ".md" || ext == ".markdown" {
		return localPathRolePage, nil
	}
	if ext == ".html" || ext == ".htm" || knownOpaqueLocalExtension(ext) {
		return localPathRoleAsset, nil
	}
	text, err := localFileIsUTF8Text(candidate.abs)
	if err != nil {
		return localPathRoleAuto, err
	}
	if text {
		return localPathRolePage, nil
	}
	return localPathRoleAsset, nil
}

func localPathDefaultsToDocument(candidate localPathCandidate) (bool, error) {
	ext := strings.ToLower(filepath.Ext(candidate.abs))
	if ext == ".md" || ext == ".markdown" || ext == ".html" || ext == ".htm" {
		return true, nil
	}
	if knownOpaqueLocalExtension(ext) {
		return false, nil
	}
	return localFilePrefixIsUTF8Text(candidate.abs)
}

const localInputSniffSize = 8192

func localFilePrefixIsUTF8Text(value string) (bool, error) {
	file, err := os.Open(value)
	if err != nil {
		return false, fmt.Errorf("airplan: inspect local file %q: %w", value, err)
	}
	defer func() { _ = file.Close() }()
	buffer := make([]byte, localInputSniffSize+utf8.UTFMax-1)
	n, err := io.ReadFull(file, buffer)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return false, fmt.Errorf("airplan: inspect local file %q: %w", value, err)
	}
	end := n
	if end > localInputSniffSize {
		end = localInputSniffSize
	}
	if bytes.IndexByte(buffer[:end], 0) >= 0 {
		return false, nil
	}
	for ; end <= n; end++ {
		if utf8.Valid(buffer[:end]) {
			return true, nil
		}
	}
	return false, nil
}

func knownOpaqueLocalExtension(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg",
		".ico", ".mp4", ".webm", ".mov", ".mp3", ".m4a", ".ogg",
		".wav", ".pdf", ".zip", ".gz", ".tar", ".bin", ".dmg",
		".exe", ".wasm", ".7z", ".woff", ".woff2", ".ttf", ".otf":
		return true
	default:
		return false
	}
}

func localFileIsUTF8Text(value string) (bool, error) {
	file, err := os.Open(value)
	if err != nil {
		return false, fmt.Errorf("airplan: inspect local file %q: %w", value, err)
	}
	defer func() { _ = file.Close() }()

	buffer := make([]byte, 64<<10)
	pending := make([]byte, 0, utf8.UTFMax-1)
	for {
		n, readErr := file.Read(buffer)
		data := append(pending, buffer[:n]...)
		pending = pending[:0]
		for len(data) > 0 {
			if data[0] == 0 {
				return false, nil
			}
			if !utf8.FullRune(data) {
				pending = append(pending, data...)
				break
			}
			r, size := utf8.DecodeRune(data)
			if r == utf8.RuneError && size == 1 {
				return false, nil
			}
			data = data[size:]
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return false, fmt.Errorf("airplan: inspect local file %q: %w", value, readErr)
			}
			return len(pending) == 0, nil
		}
	}
}

func localPathWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}
