package airplan

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	DefaultMaxCollectionFileSize  = int64(1 << 30)
	DefaultMaxCollectionTotalSize = int64(2 << 30)
)

//go:embed assets/collection.html.tmpl
var builtinCollectionTemplate string

// CollectionTemplateData is the stable custom collection template contract.
type CollectionTemplateData struct {
	Title         string
	Files         []CollectionTemplateFile
	TotalBytes    int64
	Indexable     bool
	RepositoryURL string
}

// CollectionTemplateFile describes one file exposed to a collection template.
type CollectionTemplateFile struct {
	Name        string
	Path        string
	ContentType string
	Bytes       int64
	MediaKind   string
}

// FileInput is one member of a collection upload.
type FileInput struct {
	// Name supplies the source path or display filename. Only its basename is
	// stored publicly.
	Name string
	// Reader supplies the original bytes and must support rewinding.
	Reader io.ReadSeeker
	// Size is the exact byte count uploaded from Reader.
	Size int64
	// ContentType optionally overrides deterministic detection.
	ContentType string
}

// FilesInput describes one collection upload.
type FilesInput struct {
	// Files are uploaded in order and must have unique basenames.
	Files []FileInput
	// Title overrides the generated collection overview title.
	Title string
	// MaxSize is the per-file limit; zero uses the 1 GiB default and negative
	// values disable the limit.
	MaxSize int64
	// MaxTotalSize limits all member bytes; zero uses the 2 GiB default and
	// negative values disable the limit.
	MaxTotalSize int64
}

// FileResult describes one uploaded collection member.
type FileResult struct {
	Name        string `json:"name"`
	URL         string `json:"url"`
	Key         string `json:"key"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

// FilesResult describes a completed collection upload.
type FilesResult struct {
	Result
	Files []FileResult `json:"files"`
}

// BuiltinCollectionTemplate returns the collection overview template source.
func BuiltinCollectionTemplate() string { return builtinCollectionTemplate }

// LoadCollectionTemplate parses a custom collection overview template.
func LoadCollectionTemplate(path string) (*template.Template, error) {
	return loadTemplateWithFuncs(path, collectionTemplateFuncs())
}

func loadTemplateWithFuncs(path string, funcs template.FuncMap) (*template.Template, error) {
	resolved := path
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("airplan: resolve template path %q: %w", path, err)
		}
		resolved = filepath.Join(home, strings.TrimPrefix(path, "~/"))
	}
	src, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("airplan: read collection template %q: %w", path, err)
	}
	tmpl, err := template.New(filepath.Base(path)).Funcs(funcs).Parse(string(src))
	if err != nil {
		return nil, fmt.Errorf("airplan: parse collection template %q: %w", path, err)
	}
	return tmpl, nil
}

func collectionTemplateFuncs() template.FuncMap {
	return template.FuncMap{"formatBytes": humanBytes}
}

func parseBuiltinCollectionTemplate() (*template.Template, error) {
	return template.New("collection").Funcs(collectionTemplateFuncs()).Parse(
		builtinCollectionTemplate,
	)
}

// RenderCollection renders a collection overview without uploading it.
func RenderCollection(ctx context.Context, in FilesInput, opts CollectionRenderOptions) ([]byte, []PreparedFile, error) {
	files, title, repo, total, err := prepareCollection(ctx, in, opts.Repository)
	if err != nil {
		return nil, nil, err
	}
	tmpl, err := parseBuiltinCollectionTemplate()
	if opts.TemplatePath != "" {
		tmpl, err = LoadCollectionTemplate(opts.TemplatePath)
	}
	if err != nil {
		return nil, nil, err
	}
	body, err := executeCollectionTemplate(tmpl, CollectionTemplateData{Title: title, Files: templateFiles(files), TotalBytes: total, Indexable: opts.Indexable, RepositoryURL: repo})
	prepared := make([]PreparedFile, len(files))
	for i, file := range files {
		prepared[i] = PreparedFile{
			Name: file.Name, ContentType: file.ContentType,
			MediaKind: file.MediaKind, Bytes: file.Size, Path: file.Path,
		}
	}
	return body, prepared, err
}

// CollectionRenderOptions controls collection overview rendering.
type CollectionRenderOptions struct {
	Indexable                bool
	TemplatePath, Repository string
}

// PreparedFile exposes preflight metadata returned by RenderCollection.
type PreparedFile struct {
	Name, ContentType, MediaKind string
	Bytes                        int64
	Path                         string
}

type collectionFile struct {
	FileInput
	MediaKind, Path string
}

func (c *Client) UploadFiles(ctx context.Context, in FilesInput) (*FilesResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	files, title, repo, total, err := prepareCollection(ctx, in, c.cfg.Repository)
	if err != nil {
		return nil, err
	}
	tmpl, err := parseBuiltinCollectionTemplate()
	if c.cfg.CollectionTemplate != "" {
		tmpl, err = LoadCollectionTemplate(c.cfg.CollectionTemplate)
	}
	if err != nil {
		return nil, err
	}
	overview, err := executeCollectionTemplate(tmpl, CollectionTemplateData{Title: title, Files: templateFiles(files), TotalBytes: total, Indexable: c.cfg.Indexable, RepositoryURL: repo})
	if err != nil {
		return nil, err
	}
	dir, err := RandomDirName()
	if err != nil {
		return nil, fmt.Errorf("airplan: generate key: %w", err)
	}
	createdAt := time.Now().UTC().Truncate(time.Second)
	objects := []MarkerObject{{Name: "index.html", Role: MarkerRolePage, Bytes: int64(len(overview)), ContentType: pageContentType}}
	for _, f := range files {
		objects = append(objects, MarkerObject{Name: f.Name, Role: MarkerRoleFile, Bytes: f.Size, ContentType: f.ContentType})
	}
	marker := UploadMarker{Schema: MarkerSchema, Version: MarkerVersion, Directory: dir, CreatedAt: createdAt, Kind: UploadKindCollection, Objects: objects, Title: title, Repo: repo}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		return nil, err
	}
	markerKey := BuildKey(c.cfg.KeyPrefix, dir, CollectionMarkerFilename)
	if err = c.st.put(ctx, object{Key: markerKey, Body: markerBody, ContentType: markerContentType}); err != nil {
		return nil, err
	}
	res := &FilesResult{Result: Result{Bucket: c.cfg.Bucket, Title: title, CreatedAt: createdAt, MarkerVersion: MarkerVersion, MarkerKey: markerKey, RepositoryURL: repo, Kind: string(UploadKindCollection), ContentType: pageContentType}}
	for _, f := range files {
		key := BuildKey(c.cfg.KeyPrefix, dir, f.Name)
		if _, err = f.Reader.Seek(0, io.SeekStart); err != nil {
			return nil, fmt.Errorf("airplan: seek input %q: %w", f.Name, err)
		}
		if err = c.st.putStream(ctx, streamObject{Key: key, Body: f.Reader, Size: f.Size, ContentType: f.ContentType, Metadata: titleMetadata(title)}); err != nil {
			return nil, err
		}
		u, fallback, urlErr := PublicURL(c.cfg, key)
		if urlErr != nil {
			return nil, urlErr
		}
		if fallback && len(res.Warnings) == 0 {
			res.Warnings = append(res.Warnings, PublicURLFallbackWarning)
		}
		res.Files = append(res.Files, FileResult{Name: f.Name, URL: u, Key: key, Bytes: f.Size, ContentType: f.ContentType})
	}
	pageKey := BuildKey(c.cfg.KeyPrefix, dir, "index.html")
	if err = c.st.put(ctx, object{Key: pageKey, Body: overview, ContentType: pageContentType, Metadata: titleMetadata(title)}); err != nil {
		return nil, err
	}
	res.Key = pageKey
	res.Bytes = int64(len(overview))
	res.URL, _, err = PublicURL(c.cfg, pageKey)
	if err != nil {
		return nil, err
	}
	c.recordUpload(ctx, &res.Result)
	return res, nil
}

func prepareCollection(ctx context.Context, in FilesInput, repository string) ([]collectionFile, string, string, int64, error) {
	if len(in.Files) == 0 {
		return nil, "", "", 0, errors.New("airplan: collection requires at least one file")
	}
	if len(in.Files) > MaxCollectionFiles {
		return nil, "", "", 0, fmt.Errorf("airplan: collection has %d files; maximum is %d", len(in.Files), MaxCollectionFiles)
	}
	maxSize := in.MaxSize
	if maxSize == 0 {
		maxSize = DefaultMaxCollectionFileSize
	}
	maxTotal := in.MaxTotalSize
	if maxTotal == 0 {
		maxTotal = DefaultMaxCollectionTotalSize
	}
	seen := map[string]bool{}
	files := make([]collectionFile, 0, len(in.Files))
	var total int64
	for _, input := range in.Files {
		if err := ctx.Err(); err != nil {
			return nil, "", "", 0, err
		}
		name := filepath.Base(input.Name)
		if err := validateCollectionName(name); err != nil {
			return nil, "", "", 0, err
		}
		if seen[name] {
			return nil, "", "", 0, fmt.Errorf("airplan: duplicate collection filename %q", name)
		}
		seen[name] = true
		if input.Reader == nil {
			return nil, "", "", 0, fmt.Errorf("airplan: file %q has nil reader", name)
		}
		if _, err := input.Reader.Seek(0, io.SeekStart); err != nil {
			return nil, "", "", 0, fmt.Errorf(
				"airplan: file %q is not seekable: %w", name, err,
			)
		}
		if input.Size < 0 {
			return nil, "", "", 0, fmt.Errorf("airplan: file %q has invalid size", name)
		}
		if maxSize > 0 && input.Size > maxSize {
			return nil, "", "", 0, fmt.Errorf("%w of %s: %s", ErrInputTooLarge, formatSize(maxSize), name)
		}
		if input.Size > mathMaxInt64-total {
			return nil, "", "", 0, errors.New("airplan: collection total size is out of range")
		}
		total += input.Size
		if maxTotal > 0 && total > maxTotal {
			return nil, "", "", 0, fmt.Errorf("airplan: collection exceeds maximum total size of %s", formatSize(maxTotal))
		}
		contentType, err := normalizeCollectionContentType(
			input.ContentType, name, input.Reader,
		)
		if err != nil {
			return nil, "", "", 0, err
		}
		files = append(files, collectionFile{FileInput: FileInput{Name: name, Reader: input.Reader, Size: input.Size, ContentType: contentType}, MediaKind: mediaKind(contentType), Path: "./" + url.PathEscape(name)})
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		title = files[0].Name
		if len(files) > 1 {
			title = fmt.Sprintf("%s and %d more", files[0].Name, len(files)-1)
		}
	}
	repo, err := resolveRepository(ctx, repository, in.Files[0].Name, "")
	if err != nil {
		return nil, "", "", 0, err
	}
	return files, title, repo, total, nil
}

const mathMaxInt64 = int64(^uint64(0) >> 1)

func validateCollectionName(name string) error {
	if name == "" || name == "." || name == ".." || name == MarkerFilename || name == CollectionMarkerFilename || name == "index.html" {
		return fmt.Errorf("airplan: invalid collection filename %q", name)
	}
	if !utf8.ValidString(name) || strings.ContainsAny(name, "/\\\x00") {
		return fmt.Errorf("airplan: invalid collection filename %q", name)
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return fmt.Errorf("airplan: invalid collection filename %q", name)
		}
	}
	return nil
}

func normalizeCollectionContentType(
	explicit, name string, r io.ReadSeeker,
) (string, error) {
	if explicit != "" {
		if mt, params, err := mime.ParseMediaType(explicit); err == nil {
			return mime.FormatMediaType(strings.ToLower(mt), params), nil
		}
		return "", fmt.Errorf("airplan: invalid content type %q for %q",
			explicit, name)
	}
	if mt := collectionTypeByExtension(strings.ToLower(filepath.Ext(name))); mt != "" {
		return mt, nil
	}
	pos, err := r.Seek(0, io.SeekCurrent)
	if err != nil {
		return "", fmt.Errorf("airplan: inspect file %q: %w", name, err)
	}
	buf := make([]byte, 512)
	n, readErr := r.Read(buf)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return "", fmt.Errorf("airplan: inspect file %q: %w", name, readErr)
	}
	if _, err := r.Seek(pos, io.SeekStart); err != nil {
		return "", fmt.Errorf("airplan: restore file %q: %w", name, err)
	}
	if n > 0 {
		return http.DetectContentType(buf[:n]), nil
	}
	return "application/octet-stream", nil
}

func collectionTypeByExtension(ext string) string {
	return map[string]string{".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".gif": "image/gif", ".webp": "image/webp", ".avif": "image/avif", ".svg": "image/svg+xml", ".mp4": "video/mp4", ".webm": "video/webm", ".mov": "video/quicktime", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".ogg": "audio/ogg", ".wav": "audio/wav", ".pdf": "application/pdf", ".zip": "application/zip", ".gz": "application/gzip", ".tar": "application/x-tar"}[ext]
}

func mediaKind(ct string) string {
	switch {
	case strings.HasPrefix(ct, "image/"):
		return "image"
	case strings.HasPrefix(ct, "video/"):
		return "video"
	case strings.HasPrefix(ct, "audio/"):
		return "audio"
	default:
		return "file"
	}
}

func templateFiles(files []collectionFile) []CollectionTemplateFile {
	out := make([]CollectionTemplateFile, len(files))
	for i, f := range files {
		out[i] = CollectionTemplateFile{Name: f.Name, Path: f.Path, ContentType: f.ContentType, Bytes: f.Size, MediaKind: f.MediaKind}
	}
	return out
}

func executeCollectionTemplate(t *template.Template, data CollectionTemplateData) ([]byte, error) {
	var b bytes.Buffer
	if err := t.Execute(&b, data); err != nil {
		return nil, fmt.Errorf("airplan: execute collection template: %w", err)
	}
	if len(bytes.TrimSpace(b.Bytes())) == 0 {
		return nil, errors.New("airplan: collection template produced empty output")
	}
	return b.Bytes(), nil
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for q := n / unit; q >= unit; q /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
