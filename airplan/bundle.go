package airplan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// DefaultMaxTotalPageSize limits all managed source bytes in one document.
	DefaultMaxTotalPageSize = int64(100 << 20)
	// DefaultMaxGeneratedPageSize limits all generated HTML bytes in one document.
	DefaultMaxGeneratedPageSize = int64(100 << 20)
	// DefaultMaxAssetSize limits one document asset.
	DefaultMaxAssetSize = int64(1 << 30)
	// DefaultMaxDocumentAssetTotalSize limits all document assets.
	DefaultMaxDocumentAssetTotalSize = int64(2 << 30)
)

// PageInput describes one managed page in a document bundle.
type PageInput struct {
	Reader io.Reader
	Path   string
	Format string
	Title  string
	Lang   string
}

// AssetInput describes one byte-for-byte asset in a document bundle.
type AssetInput struct {
	Reader      io.ReadSeeker
	Path        string
	Size        int64
	ContentType string
}

// DocumentInput describes one complete document, including its entry page.
type DocumentInput struct {
	Entry                PageInput
	Pages                []PageInput
	Assets               []AssetInput
	Slug                 string
	Title                string
	MaxPageSize          int64
	MaxTotalPageSize     int64
	MaxAssetSize         int64
	MaxTotalSize         int64
	RepositoryURL        string
	maxGeneratedPageSize int64
}

// PageResult describes one managed page and its optional source object.
type PageResult struct {
	Path        string `json:"path"`
	Format      string `json:"format"`
	Title       string `json:"title,omitempty"`
	URL         string `json:"url"`
	Key         string `json:"key"`
	SourceURL   string `json:"source_url,omitempty"`
	SourceKey   string `json:"source_key,omitempty"`
	Bytes       int64  `json:"bytes"`
	SourceBytes int64  `json:"source_bytes,omitempty"`
}

// AssetResult describes one uploaded document asset.
type AssetResult struct {
	Path        string `json:"path"`
	URL         string `json:"url"`
	Key         string `json:"key"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

// DocumentResult describes a complete document upload. Embedded Result fields
// remain aliases for the entry page.
type DocumentResult struct {
	Result
	Pages  []PageResult  `json:"pages,omitempty"`
	Assets []AssetResult `json:"assets,omitempty"`
}

// DocumentTemplatePage is one navigation item exposed to document templates.
type DocumentTemplatePage struct {
	Path    string
	Title   string
	URL     string
	Current bool
}

// DocumentTemplateAsset exposes an asset inventory item to custom templates.
type DocumentTemplateAsset struct {
	Path        string
	URL         string
	ContentType string
}

// DocumentRenderOptions controls local rendering of a complete document.
type DocumentRenderOptions struct {
	RenderInputOptions
	MaxGeneratedPageSize int64
	Revision             int
	RevisionCount        int
	PreviousRevision     int
	VersionsPath         string
	DiffPath             string
	DiffText             string
	RevisionChainID      string
	PageDiffs            map[string]string
	ChangedPages         map[string]bool
	CompleteDiffText     string
}

// RenderedBundlePage is one generated standalone managed page.
type RenderedBundlePage struct {
	Path       string
	PagePath   string
	SourcePath string
	Format     string
	Title      string
	Lang       string
	HTML       []byte
	Source     []byte
	sourceFile spooledPayload
	htmlFile   spooledPayload
}

// RenderedDocumentBundle contains every generated page and declared asset.
type RenderedDocumentBundle struct {
	Entrypoint    string
	Pages         []RenderedBundlePage
	Assets        []AssetInput
	Slug          string
	Title         string
	Format        string
	RepositoryURL string
	Warnings      []string
	spool         *payloadSpool
}

type preparedAsset struct {
	AssetInput
	start  int64
	digest string
}

// InvalidDocumentInputError reports a structurally invalid document bundle.
// Callers may use errors.As to distinguish these client input failures from
// storage and rendering failures.
type InvalidDocumentInputError struct{ err error }

func (e *InvalidDocumentInputError) Error() string { return e.err.Error() }
func (e *InvalidDocumentInputError) Unwrap() error { return e.err }

func invalidDocumentInput(err error) error {
	if err == nil {
		return nil
	}
	var invalid *InvalidDocumentInputError
	if errors.As(err, &invalid) {
		return err
	}
	return &InvalidDocumentInputError{err: err}
}

// ValidateBundlePath validates one portable bundle-relative logical path.
func ValidateBundlePath(value string) error {
	if !validMarkerObjectPath(value) {
		return invalidDocumentInput(
			fmt.Errorf("airplan: invalid bundle path %q", value),
		)
	}
	return nil
}

// RenderDocument detects and renders every managed page without storage I/O.
func RenderDocument(
	ctx context.Context,
	in DocumentInput,
	opts DocumentRenderOptions,
) (*RenderedDocumentBundle, error) {
	if ctx == nil {
		return nil, errors.New("airplan: nil context")
	}
	if opts.Template != nil && opts.TemplatePath != "" {
		return nil, errors.New("airplan: render document: template and template path are mutually exclusive")
	}
	assets, err := prepareAssets(ctx, in)
	if err != nil {
		return nil, err
	}
	for index := range assets {
		in.Assets[index] = assets[index].AssetInput
	}
	tmpl := opts.Template
	var tmplErr error
	if tmpl == nil && opts.TemplatePath != "" {
		tmpl, tmplErr = LoadTemplate(opts.TemplatePath)
	}
	return renderDocument(ctx, in, opts, tmpl, tmplErr)
}

func renderDocument(
	ctx context.Context,
	in DocumentInput,
	opts DocumentRenderOptions,
	tmpl *template.Template,
	tmplErr error,
) (*RenderedDocumentBundle, error) {
	return renderDocumentWithSpool(ctx, in, opts, tmpl, tmplErr, nil)
}

func renderDocumentSpooled(
	ctx context.Context,
	in DocumentInput,
	opts DocumentRenderOptions,
	tmpl *template.Template,
	tmplErr error,
) (*RenderedDocumentBundle, error) {
	spool, err := newPayloadSpool()
	if err != nil {
		return nil, err
	}
	bundle, err := renderDocumentWithSpool(ctx, in, opts, tmpl, tmplErr, spool)
	if err != nil {
		spool.cleanup()
		return nil, err
	}
	bundle.spool = spool
	return bundle, nil
}

func renderDocumentWithSpool(
	ctx context.Context,
	in DocumentInput,
	opts DocumentRenderOptions,
	tmpl *template.Template,
	tmplErr error,
	spool *payloadSpool,
) (*RenderedDocumentBundle, error) {
	if in.Entry.Reader == nil {
		return nil, errors.New("airplan: document entry reader is nil")
	}
	if len(in.Pages)+len(in.Assets)+1 > MaxDocumentItems {
		return nil, fmt.Errorf("airplan: document has %d items; maximum is %d", len(in.Pages)+len(in.Assets)+1, MaxDocumentItems)
	}
	pageLimit := effectiveLimit(in.MaxPageSize, DefaultMaxInputSize)
	totalPageLimit := effectiveLimit(in.MaxTotalPageSize, DefaultMaxTotalPageSize)
	generatedLimit := opts.MaxGeneratedPageSize
	if generatedLimit == 0 {
		generatedLimit = DefaultMaxGeneratedPageSize
	} else if generatedLimit < 0 {
		generatedLimit = 0
	}
	inputs := make([]PageInput, 0, len(in.Pages)+1)
	inputs = append(inputs, in.Entry)
	inputs = append(inputs, in.Pages...)
	seen := make(map[string]string, len(inputs)+len(in.Assets))
	prepared := make([]RenderedBundlePage, len(inputs))
	var sourceTotal int64
	for index, input := range inputs {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if input.Reader == nil {
			return nil, fmt.Errorf("airplan: page %d reader is nil", index)
		}
		logical := input.Path
		if logical == "" && index == 0 {
			logical = "document.md"
		}
		if index == 0 && pathpkg.Base(logical) != logical {
			return nil, invalidDocumentInput(fmt.Errorf(
				"airplan: document entry path %q must be a basename", logical,
			))
		}
		if err := validateAndRecordBundlePath(seen, logical); err != nil {
			return nil, err
		}
		data, err := readInput(ctx, input.Reader, pageLimit)
		if err != nil {
			return nil, fmt.Errorf("airplan: read page %q: %w", logical, err)
		}
		if len(data) == 0 {
			return nil, fmt.Errorf("airplan: page %q: %w", logical, ErrEmptyInput)
		}
		if !utf8.Valid(data) {
			return nil, fmt.Errorf("airplan: page %q: %w", logical, ErrInvalidUTF8)
		}
		if IsBinary(data) {
			return nil, fmt.Errorf("airplan: page %q: %w", logical, ErrBinaryInput)
		}
		if int64(len(data)) > mathMaxInt64-sourceTotal {
			return nil, errors.New("airplan: managed page total size is out of range")
		}
		sourceTotal += int64(len(data))
		if totalPageLimit > 0 && sourceTotal > totalPageLimit {
			return nil, fmt.Errorf("airplan: page %q exceeds maximum total managed source size of %s", logical, formatSize(totalPageLimit))
		}
		format, err := documentPageFormat(input.Format, logical, data)
		if err != nil {
			return nil, fmt.Errorf("airplan: page %q: %w", logical, err)
		}
		if index == 0 && len(in.Pages) != 0 && format != FormatMarkdown {
			if format == FormatHTML {
				return nil, invalidDocumentInput(errors.New(
					"airplan: authored HTML entries cannot declare managed pages",
				))
			}
			return nil, invalidDocumentInput(errors.New(
				"airplan: managed pages require a Markdown entry",
			))
		}
		if index > 0 && format == FormatHTML {
			return nil, invalidDocumentInput(fmt.Errorf(
				"airplan: managed page %q cannot use HTML format", logical,
			))
		}
		title := input.Title
		if title == "" {
			if index == 0 && in.Title != "" {
				title = in.Title
			} else if format == FormatMarkdown {
				frontMatter, parseErr := parseFrontMatter(data)
				if parseErr != nil {
					return nil, parseErr
				}
				title = resolveMarkdownTitle(
					"", frontMatter.title, frontMatter.body, logical,
					filenameStem(logical),
				)
			} else {
				title = pathpkg.Base(logical)
			}
		}
		_, lang, _ := highlightSource(data, logical, input.Lang)
		prepared[index] = RenderedBundlePage{
			Path: logical, Format: format.String(), Source: data,
			Lang: lang, Title: title,
		}
		if spool != nil {
			payload, spoolErr := spool.put("source", data)
			if spoolErr != nil {
				return nil, spoolErr
			}
			prepared[index].sourceFile = payload
			prepared[index].Source = nil
		}
	}
	slug := SanitizeSlug(in.Slug)
	if in.Slug == "" {
		slug = SanitizeSlug(filenameStem(prepared[0].Path))
	}
	entrypoint := slug + ".html"
	for index := range prepared {
		page := &prepared[index]
		format, _ := ParseFormat(page.Format)
		if index == 0 {
			page.PagePath = entrypoint
			if format != FormatHTML && opts.IncludeSource {
				page.SourcePath = entrySourceObjectName(slug, page.Path, format)
			}
		} else {
			page.PagePath = managedPageObjectName(page.Path, format)
			if opts.IncludeSource {
				page.SourcePath = page.Path
			}
		}
		if format != FormatHTML && page.PagePath == page.Path {
			return nil, invalidDocumentInput(fmt.Errorf(
				"airplan: generated page %q collides with its source", page.PagePath,
			))
		}
		if err := validateGeneratedBundlePath(seen, page.PagePath, page.Path); err != nil {
			return nil, err
		}
		if page.SourcePath != "" && page.SourcePath != page.Path {
			if err := validateGeneratedBundlePath(seen, page.SourcePath, page.Path); err != nil {
				return nil, err
			}
		}
	}
	for _, asset := range in.Assets {
		if err := validateAndRecordBundlePath(seen, asset.Path); err != nil {
			return nil, err
		}
	}
	views := make([]DocumentTemplatePage, len(prepared))
	for index := range prepared {
		page := &prepared[index]
		views[index] = DocumentTemplatePage{Path: page.Path, Title: page.Title}
	}
	repository := in.RepositoryURL
	if repository == "" {
		repository = opts.Repository
	}
	resolvedRepository, err := resolveRepository(ctx, repository, in.Entry.Path, opts.WorkingDirectory)
	if err != nil {
		return nil, err
	}
	bundle := &RenderedDocumentBundle{Entrypoint: entrypoint, Pages: prepared, Assets: append([]AssetInput(nil), in.Assets...), Slug: slug, Title: prepared[0].Title, Format: prepared[0].Format, RepositoryURL: resolvedRepository}
	versionsPath := opts.VersionsPath
	if versionsPath == "" {
		versionsPath = VersionsFilename
	}
	var generatedTotal int64
	for index := range bundle.Pages {
		page := &bundle.Pages[index]
		source := page.Source
		if spool != nil {
			source, err = page.sourceFile.read(ctx)
			if err != nil {
				return nil, fmt.Errorf("airplan: page %q: %w", page.Path, err)
			}
		}
		format, _ := ParseFormat(page.Format)
		currentViews := append([]DocumentTemplatePage(nil), views...)
		for viewIndex := range currentViews {
			currentViews[viewIndex].URL = relativeObjectURL(page.PagePath, bundle.Pages[viewIndex].PagePath)
			currentViews[viewIndex].Current = viewIndex == index
		}
		sourcePath := ""
		if page.SourcePath != "" {
			sourcePath = relativeObjectURL(page.PagePath, page.SourcePath)
		}
		currentAssets := make([]DocumentTemplateAsset, len(in.Assets))
		for assetIndex, asset := range in.Assets {
			currentAssets[assetIndex] = DocumentTemplateAsset{Path: asset.Path, URL: relativeObjectURL(page.PagePath, asset.Path), ContentType: asset.ContentType}
		}
		pageDiff := opts.PageDiffs[page.Path]
		pageChanged := opts.ChangedPages[page.Path]
		completeDiff := ""
		if index == 0 {
			completeDiff = opts.CompleteDiffText
		}
		if opts.PageDiffs == nil && opts.DiffText != "" && index == 0 {
			pageDiff = opts.DiffText
			pageChanged = true
			completeDiff = opts.DiffText
		}
		renderOpts := RenderOptions{Title: page.Title, Slug: slug, SourceName: pathpkg.Base(page.Path), SourcePath: sourcePath, Indexable: opts.Indexable, NoExternalAssets: opts.NoExternalAssets, MermaidURL: opts.MermaidURL, RepositoryURL: resolvedRepository, Lang: page.Lang, Template: tmpl, Themes: opts.Themes, Pages: currentViews, CurrentPage: currentViews[index], Entrypoint: relativeObjectURL(page.PagePath, entrypoint), Assets: currentAssets, ManagedPagePaths: managedPageMap(bundle.Pages), CurrentLogicalPath: page.Path, CurrentRenderedPath: page.PagePath, Revision: opts.Revision, RevisionCount: opts.RevisionCount, PreviousRevision: opts.PreviousRevision, RevisionChainID: opts.RevisionChainID, VersionsPath: relativeControlPath(page.PagePath, versionsPath), DiffPath: relativeControlPath(page.PagePath, opts.DiffPath), PageChanged: pageChanged, PageDiffText: pageDiff, CompleteDiffText: completeDiff, HasCompleteDiff: index == 0 && opts.Revision > 1 && opts.DiffPath != "", AllChangesPath: relativeObjectURL(page.PagePath, entrypoint) + "#airplan-all-changes", structuredDiff: opts.PageDiffs != nil}
		if tmplErr != nil && format != FormatHTML {
			return nil, tmplErr
		}
		switch format {
		case FormatMarkdown:
			frontMatter, parseErr := parseFrontMatter(source)
			if parseErr != nil {
				return nil, parseErr
			}
			page.HTML, err = renderMarkdown(source, frontMatter, renderOpts)
		case FormatText:
			page.HTML, err = RenderText(source, page.Path, renderOpts)
		case FormatHTML:
			page.HTML = append([]byte(nil), source...)
			if !opts.Indexable {
				var outcome NoindexResult
				page.HTML, outcome = InjectNoindex(page.HTML)
				if outcome == NoindexNoHead {
					bundle.Warnings = append(bundle.Warnings, "no <head> tag found; HTML left unmodified, without a noindex meta tag")
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("airplan: render page %q: %w", page.Path, err)
		}
		generatedTotal += int64(len(page.HTML))
		if generatedLimit > 0 && generatedTotal > generatedLimit {
			return nil, fmt.Errorf(
				"%w of %s: generated page %q", ErrInputTooLarge,
				formatSize(generatedLimit), page.Path,
			)
		}
		if spool != nil {
			payload, spoolErr := spool.put("page", page.HTML)
			if spoolErr != nil {
				return nil, spoolErr
			}
			page.htmlFile = payload
			page.HTML = nil
		}
	}
	return bundle, nil
}

func (b *RenderedDocumentBundle) cleanup() {
	if b != nil && b.spool != nil {
		b.spool.cleanup()
		b.spool = nil
	}
}

func (p *RenderedBundlePage) sourceSize() int64 {
	if p.sourceFile.path != "" {
		return p.sourceFile.size
	}
	return int64(len(p.Source))
}

func (p *RenderedBundlePage) sourceDigest() string {
	if p.sourceFile.path != "" {
		return p.sourceFile.digest
	}
	return contentSHA256(p.Source)
}

func (p *RenderedBundlePage) pageSize() int64 {
	if p.htmlFile.path != "" {
		return p.htmlFile.size
	}
	return int64(len(p.HTML))
}

func (p *RenderedBundlePage) pageDigest() string {
	if p.htmlFile.path != "" {
		return p.htmlFile.digest
	}
	return contentSHA256(p.HTML)
}

func (p *RenderedBundlePage) sourceBody(ctx context.Context) ([]byte, error) {
	if p.sourceFile.path != "" {
		return p.sourceFile.read(ctx)
	}
	return p.Source, nil
}

func relativeControlPath(from, control string) string {
	if control == "" {
		return ""
	}
	return relativeObjectURL(from, control)
}

func effectiveLimit(value, fallback int64) int64 {
	if value == 0 {
		return fallback
	}
	if value < 0 {
		return 0
	}
	return value
}

func documentPageFormat(explicit, name string, data []byte) (Format, error) {
	if explicit != "" {
		return ParseFormat(explicit)
	}
	return DetectFormat(name, data), nil
}

func entrySourceObjectName(slug, logical string, format Format) string {
	if format == FormatMarkdown {
		return slug + ".md"
	}
	return slug + "." + sourceExt(logical)
}

func managedPageObjectName(logical string, format Format) string {
	if format == FormatMarkdown {
		ext := pathpkg.Ext(logical)
		return strings.TrimSuffix(logical, ext) + ".html"
	}
	return logical + ".html"
}

func validateAndRecordBundlePath(seen map[string]string, value string) error {
	if err := ValidateBundlePath(value); err != nil {
		return err
	}
	fold := strings.ToLower(value)
	if previous, exists := seen[fold]; exists {
		return invalidDocumentInput(fmt.Errorf(
			"airplan: bundle paths %q and %q collide when case-folded", previous, value,
		))
	}
	for previousFold, previous := range seen {
		if strings.HasPrefix(fold, previousFold+"/") ||
			strings.HasPrefix(previousFold, fold+"/") {
			return invalidDocumentInput(fmt.Errorf(
				"airplan: bundle paths %q and %q conflict as file and directory",
				previous, value,
			))
		}
	}
	seen[fold] = value
	return nil
}

func validateGeneratedBundlePath(seen map[string]string, value, source string) error {
	if err := ValidateBundlePath(value); err != nil {
		return err
	}
	fold := strings.ToLower(value)
	if previous, exists := seen[fold]; exists && !strings.EqualFold(previous, source) {
		return invalidDocumentInput(fmt.Errorf(
			"airplan: generated path %q for %q collides with %q", value, source, previous,
		))
	}
	for previousFold, previous := range seen {
		if strings.HasPrefix(fold, previousFold+"/") ||
			strings.HasPrefix(previousFold, fold+"/") {
			return invalidDocumentInput(fmt.Errorf(
				"airplan: generated path %q for %q conflicts with %q as file and directory",
				value, source, previous,
			))
		}
	}
	seen[fold] = value
	return nil
}

func managedPageMap(pages []RenderedBundlePage) map[string]string {
	result := make(map[string]string, len(pages))
	for _, page := range pages {
		result[page.Path] = page.PagePath
	}
	return result
}

func relativeObjectURL(from, to string) string {
	rel, err := urlPathRelative(pathpkg.Dir(from), to)
	if err != nil {
		return ""
	}
	first := strings.SplitN(rel, "/", 2)[0]
	if first != "." && first != ".." && strings.Contains(first, ":") {
		rel = "./" + rel
	}
	parts := strings.Split(rel, "/")
	for index, part := range parts {
		if part != ".." && part != "." {
			parts[index] = url.PathEscape(part)
		}
	}
	return strings.Join(parts, "/")
}

func urlPathRelative(base, target string) (string, error) {
	baseParts := strings.Split(pathpkg.Clean(base), "/")
	if len(baseParts) == 1 && baseParts[0] == "." {
		baseParts = nil
	}
	targetParts := strings.Split(pathpkg.Clean(target), "/")
	common := 0
	for common < len(baseParts) && common < len(targetParts) && baseParts[common] == targetParts[common] {
		common++
	}
	parts := make([]string, 0, len(baseParts)-common+len(targetParts)-common)
	for range baseParts[common:] {
		parts = append(parts, "..")
	}
	parts = append(parts, targetParts[common:]...)
	if len(parts) == 0 {
		return pathpkg.Base(target), nil
	}
	return strings.Join(parts, "/"), nil
}

func prepareAssets(ctx context.Context, in DocumentInput) ([]preparedAsset, error) {
	maxAsset := effectiveLimit(in.MaxAssetSize, DefaultMaxAssetSize)
	maxTotal := effectiveLimit(in.MaxTotalSize, DefaultMaxDocumentAssetTotalSize)
	seen := make(map[string]string, len(in.Assets))
	prepared := make([]preparedAsset, len(in.Assets))
	var total int64
	for index, asset := range in.Assets {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := validateAndRecordBundlePath(seen, asset.Path); err != nil {
			return nil, err
		}
		if asset.Reader == nil {
			return nil, fmt.Errorf("airplan: asset %q reader is nil", asset.Path)
		}
		if asset.Size < 0 {
			return nil, fmt.Errorf("airplan: asset %q has invalid size", asset.Path)
		}
		if maxAsset > 0 && asset.Size > maxAsset {
			return nil, fmt.Errorf("%w of %s: %s", ErrInputTooLarge, formatSize(maxAsset), asset.Path)
		}
		if asset.Size > mathMaxInt64-total {
			return nil, errors.New("airplan: document asset total size is out of range")
		}
		total += asset.Size
		if maxTotal > 0 && total > maxTotal {
			return nil, fmt.Errorf("airplan: document assets exceed maximum total size of %s", formatSize(maxTotal))
		}
		start, err := asset.Reader.Seek(0, io.SeekCurrent)
		if err != nil {
			return nil, fmt.Errorf("airplan: asset %q is not seekable: %w", asset.Path, err)
		}
		contentType, err := normalizeAssetContentType(asset, start)
		if err != nil {
			return nil, err
		}
		digest, err := hashExactReader(asset.Reader, start, asset.Size)
		if err != nil {
			return nil, fmt.Errorf("airplan: hash asset %q: %w", asset.Path, err)
		}
		asset.ContentType = contentType
		prepared[index] = preparedAsset{AssetInput: asset, start: start, digest: digest}
	}
	return prepared, nil
}

func normalizeAssetContentType(asset AssetInput, start int64) (string, error) {
	if asset.ContentType != "" {
		mediaType, params, err := mime.ParseMediaType(asset.ContentType)
		if err != nil {
			return "", invalidDocumentInput(fmt.Errorf(
				"airplan: invalid content type %q for asset %q",
				asset.ContentType, asset.Path,
			))
		}
		return mime.FormatMediaType(strings.ToLower(mediaType), params), nil
	}
	if mediaType := mime.TypeByExtension(strings.ToLower(pathpkg.Ext(asset.Path))); mediaType != "" {
		parsed, params, err := mime.ParseMediaType(mediaType)
		if err == nil {
			return mime.FormatMediaType(strings.ToLower(parsed), params), nil
		}
	}
	buf := make([]byte, min(int64(512), asset.Size))
	if _, err := asset.Reader.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	n, err := io.ReadFull(asset.Reader, buf)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	if _, err := asset.Reader.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	if n == 0 {
		return "application/octet-stream", nil
	}
	return http.DetectContentType(buf[:n]), nil
}

func hashExactReader(reader io.ReadSeeker, start, size int64) (string, error) {
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	hash := sha256.New()
	written, err := io.CopyN(hash, reader, size)
	if err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return "", fmt.Errorf(
				"truncated input: declared size is %d bytes but reader ended after %d bytes",
				size, written,
			)
		}
		return "", err
	}
	if written != size {
		return "", fmt.Errorf("declared size is %d bytes but reader contains %d bytes", size, written)
	}
	var extra [1]byte
	if n, extraErr := reader.Read(extra[:]); n != 0 || (extraErr != nil && !errors.Is(extraErr, io.EOF)) {
		if extraErr != nil && !errors.Is(extraErr, io.EOF) {
			return "", extraErr
		}
		return "", fmt.Errorf("declared size is %d bytes but reader contains more data", size)
	}
	if _, err := reader.Seek(start, io.SeekStart); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// UploadDocument uploads one complete document bundle.
func (c *Client) UploadDocument(ctx context.Context, in DocumentInput) (*DocumentResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.UploadDocument(ctx, in)
	}
	if err := c.ensureStorage(ctx); err != nil {
		return nil, err
	}
	assets, err := prepareAssets(ctx, in)
	if err != nil {
		return nil, err
	}
	for index := range assets {
		in.Assets[index] = assets[index].AssetInput
	}
	repository := c.cfg.Repository
	if in.RepositoryURL != "" {
		repository = in.RepositoryURL
	}
	generatedLimit := in.maxGeneratedPageSize
	if generatedLimit == 0 {
		generatedLimit = DefaultMaxGeneratedPageSize
	}
	bundle, err := renderDocumentSpooled(ctx, in, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Indexable: c.cfg.Indexable, IncludeSource: !c.cfg.NoSource, NoExternalAssets: c.cfg.NoExternalAssets, MermaidURL: c.cfg.MermaidURL, Repository: repository, Themes: c.cfg.ThemeBundle}, MaxGeneratedPageSize: generatedLimit}, c.template, c.templateErr)
	if err != nil {
		return nil, err
	}
	defer bundle.cleanup()
	dir, err := RandomDirName()
	if err != nil {
		return nil, fmt.Errorf("airplan: generate key: %w", err)
	}
	createdAt := time.Now().UTC().Truncate(time.Second)
	marker := UploadMarker{Schema: MarkerSchema, Version: MarkerVersion, Directory: dir, CreatedAt: createdAt, Kind: UploadKindDocument, Slug: bundle.Slug, Format: bundle.Format, Title: bundle.Title, Repo: bundle.RepositoryURL, Producer: Producer{Name: "airplan", Version: producerVersion(c.cfg.ProducerVersion)}, Entrypoint: bundle.Entrypoint}
	if bundle.Format != FormatHTML.String() {
		marker.Render = documentRenderRecipe(c.cfg, c.templateDigest)
	}
	for _, page := range bundle.Pages {
		marker.Objects = append(marker.Objects, MarkerObject{Name: page.PagePath, Role: MarkerRolePage, Bytes: page.pageSize(), ContentType: pageContentType, SHA256: page.pageDigest()})
		if page.SourcePath != "" {
			contentType := textContentType
			if page.Format == FormatMarkdown.String() {
				contentType = sourceContentType
			}
			marker.Objects = append(marker.Objects, MarkerObject{Name: page.SourcePath, Role: MarkerRoleSource, Bytes: page.sourceSize(), ContentType: contentType, SHA256: page.sourceDigest()})
		}
		if page.Format != FormatHTML.String() {
			marker.Pages = append(marker.Pages, MarkerPage{Path: page.Path, Page: page.PagePath, Source: page.SourcePath, Format: page.Format, Title: page.Title, Lang: page.Lang})
		}
	}
	for _, asset := range assets {
		marker.Objects = append(marker.Objects, MarkerObject{Name: asset.Path, Role: MarkerRoleAsset, Bytes: asset.Size, ContentType: asset.ContentType, SHA256: asset.digest})
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		return nil, err
	}
	markerKey := BuildKey(c.cfg.KeyPrefix, dir, MarkerFilename)
	if err := c.st.put(ctx, object{Key: markerKey, Body: markerBody, ContentType: markerContentType}); err != nil {
		return nil, err
	}
	result := &DocumentResult{Result: Result{ID: dir, Bucket: c.cfg.Bucket, Title: bundle.Title, CreatedAt: createdAt, MarkerVersion: MarkerVersion, MarkerKey: markerKey, Format: bundle.Format, Kind: string(UploadKindDocument), Slug: bundle.Slug, RepositoryURL: bundle.RepositoryURL, Warnings: append([]string(nil), bundle.Warnings...)}}
	for _, page := range bundle.Pages {
		pageKey := BuildKey(c.cfg.KeyPrefix, dir, page.PagePath)
		pageURL, fallback, urlErr := PublicURL(c.cfg, pageKey)
		if urlErr != nil {
			return nil, urlErr
		}
		if fallback && len(result.Warnings) == 0 {
			result.Warnings = append(result.Warnings, PublicURLFallbackWarning)
		}
		pageResult := PageResult{Path: page.Path, Format: page.Format, Title: page.Title, URL: pageURL, Key: pageKey, Bytes: page.pageSize()}
		if page.SourcePath != "" {
			pageResult.SourceBytes = page.sourceSize()
			pageResult.SourceKey = BuildKey(c.cfg.KeyPrefix, dir, page.SourcePath)
			pageResult.SourceURL, _, err = PublicURL(c.cfg, pageResult.SourceKey)
			if err != nil {
				return nil, err
			}
			contentType := textContentType
			if page.Format == FormatMarkdown.String() {
				contentType = sourceContentType
			}
			if err := c.putDocumentPayload(ctx, pageResult.SourceKey,
				page.sourceFile, contentType, titleMetadata(page.Title)); err != nil {
				return nil, err
			}
		}
		result.Pages = append(result.Pages, pageResult)
	}
	for _, asset := range assets {
		digest, hashErr := hashExactReader(asset.Reader, asset.start, asset.Size)
		if hashErr != nil {
			return nil, fmt.Errorf("airplan: verify asset %q: %w", asset.Path, hashErr)
		}
		if digest != asset.digest {
			return nil, fmt.Errorf("airplan: asset %q changed after preflight", asset.Path)
		}
		assetKey := BuildKey(c.cfg.KeyPrefix, dir, asset.Path)
		if _, err := asset.Reader.Seek(asset.start, io.SeekStart); err != nil {
			return nil, err
		}
		if err := c.st.putStream(ctx, streamObject{Key: assetKey, Body: asset.Reader, Size: asset.Size, ContentType: asset.ContentType, Metadata: titleMetadata(bundle.Title)}); err != nil {
			return nil, err
		}
		assetURL, fallback, urlErr := PublicURL(c.cfg, assetKey)
		if urlErr != nil {
			return nil, urlErr
		}
		if fallback && len(result.Warnings) == 0 {
			result.Warnings = append(result.Warnings, PublicURLFallbackWarning)
		}
		result.Assets = append(result.Assets, AssetResult{Path: asset.Path, URL: assetURL, Key: assetKey, Bytes: asset.Size, ContentType: asset.ContentType})
	}
	for index := 1; index < len(bundle.Pages); index++ {
		page := bundle.Pages[index]
		if err := c.putDocumentPayload(ctx,
			BuildKey(c.cfg.KeyPrefix, dir, page.PagePath), page.htmlFile,
			pageContentType, titleMetadata(page.Title)); err != nil {
			return nil, err
		}
	}
	entry := bundle.Pages[0]
	entryKey := BuildKey(c.cfg.KeyPrefix, dir, entry.PagePath)
	if err := c.putDocumentPayload(ctx, entryKey, entry.htmlFile,
		pageContentType, titleMetadata(entry.Title)); err != nil {
		return nil, err
	}
	result.Key = entryKey
	result.URL, _, err = PublicURL(c.cfg, entryKey)
	if err != nil {
		return nil, err
	}
	result.Bytes = entry.pageSize()
	result.ContentType = pageContentType
	if len(result.Pages) > 0 {
		result.SourceKey = result.Pages[0].SourceKey
		result.SourceURL = result.Pages[0].SourceURL
	}
	c.recordUpload(ctx, &result.Result, markerDeclaredTotals(marker, markerBody))
	return result, nil
}

func (c *Client) putDocumentPayload(
	ctx context.Context, key string, payload spooledPayload,
	contentType string, metadata map[string]string,
) error {
	return c.putDocumentPayloadConditional(
		ctx, key, payload, contentType, metadata, "",
	)
}

func (c *Client) putDocumentPayloadConditional(
	ctx context.Context, key string, payload spooledPayload,
	contentType string, metadata map[string]string, ifMatch string,
) error {
	file, err := payload.verify()
	if err != nil {
		return fmt.Errorf("airplan: verify payload %q: %w", key, err)
	}
	defer func() { _ = file.Close() }()
	return c.st.putStreamConditional(ctx, streamObject{
		Key: key, Body: file, Size: payload.size,
		ContentType: contentType, Metadata: metadata,
	}, ifMatch)
}
