// Package cli implements the airplan command-line interface. It
// contains no business logic: it parses flags, calls the core airplan
// package, and formats output per SPEC.md §1 — upload URLs or fetched
// bytes on stdout, everything else on stderr, non-zero exit on failure.
// Local preview output is defined separately in SPEC.md §6.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"unicode/utf8"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// version is stamped by the release build via ldflags.
var version = "dev"

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	cmd := newRootCmd()
	if err := cmd.Execute(); err != nil {
		msg := err.Error()
		// Core library errors already carry the "airplan:" prefix,
		// stdlib style; only bare errors (cobra's own, wrapping) need
		// it added.
		if !strings.HasPrefix(msg, "airplan:") {
			msg = "airplan: " + msg
		}
		fmt.Fprintln(os.Stderr, msg)
		return 1
	}
	return 0
}

// rootOptions holds the root command's flag values.
type rootOptions struct {
	format             string
	lang               string
	slug               string
	title              string
	noSource           bool
	indexable          bool
	noExternalAssets   bool
	mermaidURL         string
	repository         string
	maxSize            string
	maxTotalSize       string
	template           string
	collectionTemplate string
	files              bool
	timeout            string
	json               bool
	open               bool
	profile            string
	config             string
	manifest           string

	// Connection overrides for one-off use (SPEC.md §6).
	endpoint      string
	bucket        string
	region        string
	publicBaseURL string
	keyPrefix     string
}

func newRootCmd() *cobra.Command {
	opts := &rootOptions{}

	cmd := &cobra.Command{
		Use:   "airplan [flags] [file ...]",
		Short: "Upload documents or files and print shareable URLs",
		Long: "airplan uploads documents and generic file collections " +
			"to S3-compatible object storage " +
			"under a randomized, unguessable URL path and prints the " +
			"resulting URL.",
		Args:          cobra.ArbitraryArgs,
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return run(cmd, args, opts)
		},
		PersistentPreRunE: validatePersistentOptions,
	}
	cmd.PersistentFlags().StringVar(&opts.manifest, "manifest", "",
		"local manifest path (default: AIRPLAN_MANIFEST or platform state dir)")

	f := cmd.Flags()
	f.StringVar(&opts.format, "format", "",
		"input format: md, html, or txt (default: auto-detect)")
	f.StringVar(&opts.lang, "lang", "",
		"highlight language for text input (default: from filename)")
	f.StringVarP(&opts.slug, "slug", "s", "",
		"filename portion of the URL (default: from filename)")
	f.StringVarP(&opts.title, "title", "t", "",
		"page title (default: from content)")
	f.StringVar(&opts.maxSize, "max-size", "10MiB",
		"per-input limit (10MiB documents, 1GiB collections); 0 = no limit")
	f.StringVar(&opts.maxTotalSize, "max-total-size", "2GiB",
		"collection total size limit; 0 = no limit")
	f.BoolVar(&opts.files, "files", false,
		"upload named inputs as one file collection")
	f.BoolVarP(&opts.json, "json", "j", false,
		"print a single JSON object instead of the URL")
	f.BoolVarP(&opts.open, "open", "o", false,
		"open the resulting URL in the default browser")
	addConfigResolutionFlags(f, opts)

	cmd.AddCommand(newConfigCmd())
	cmd.AddCommand(newSkillCmd())
	cmd.AddCommand(newTemplateCmd())
	cmd.AddCommand(newPreviewCmd())
	cmd.AddCommand(newListCmd())
	cmd.AddCommand(newShowCmd())
	cmd.AddCommand(newGetCmd())
	cmd.AddCommand(newDeleteCmd())
	cmd.AddCommand(newPurgeCmd())
	cmd.AddCommand(newSyncCmd())
	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newMCPCmd())
	return cmd
}

func addConfigResolutionFlags(f *pflag.FlagSet, opts *rootOptions) {
	f.BoolVar(&opts.noSource, "no-source", false,
		"don't upload the original source alongside the page")
	f.BoolVar(&opts.indexable, "indexable", false,
		"omit the noindex robots meta tag")
	f.BoolVar(&opts.noExternalAssets, "no-external-assets", false,
		"disable airplan-managed external assets in rendered pages")
	f.StringVar(&opts.mermaidURL, "mermaid-url", "",
		"Mermaid ECMAScript module URL")
	f.StringVar(&opts.repository, "repo", "",
		"repository context: auto, none, or URL (default: auto)")
	f.StringVar(&opts.timeout, "timeout", "",
		"operation timeout, e.g. 30s, 1m30s; 0 = none (default 30s)")
	f.StringVarP(&opts.profile, "profile", "p", "",
		"config profile name (default: config default)")
	f.StringVar(&opts.config, "config", "",
		"config file path (default: XDG config dir)")
	f.StringVar(&opts.endpoint, "endpoint", "", "S3 endpoint URL")
	f.StringVar(&opts.bucket, "bucket", "", "bucket name")
	f.StringVar(&opts.region, "region", "", "region (default: auto)")
	f.StringVar(&opts.publicBaseURL, "public-base-url", "",
		"base URL public links are assembled from")
	f.StringVar(&opts.template, "template", "",
		"custom page template file (md and text input)")
	f.StringVar(&opts.collectionTemplate, "collection-template", "",
		"custom collection overview template file")
	f.StringVar(&opts.keyPrefix, "key-prefix", "",
		"prefix prepended to object keys")
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	return resolveVersion(version, info, ok)
}

func resolveVersion(
	stamped string,
	info *debug.BuildInfo,
	ok bool,
) string {
	if stamped != "dev" {
		return stamped
	}
	if !ok || info == nil || info.Main.Version == "" ||
		info.Main.Version == "(devel)" {
		return "dev"
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

// run executes the upload pipeline for the root command, honoring the
// output contract of SPEC.md §1: the final URL is the only thing on
// stdout; warnings and errors go to stderr.
func run(cmd *cobra.Command, args []string, opts *rootOptions) error {
	stderr := cmd.ErrOrStderr()
	collection, err := selectCollectionMode(args, opts)
	if err != nil {
		return err
	}
	if err := validateModeFlags(cmd, collection); err != nil {
		return err
	}
	if collection && !cmd.Flags().Changed("max-size") {
		opts.maxSize = "1GiB"
	}

	maxSize, err := airplan.ParseSize(opts.maxSize)
	if err != nil {
		return fmt.Errorf("--max-size: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if maxSize == 0 {
		maxSize = -1 // 0 on the CLI means unlimited (SPEC.md §2)
	}
	maxTotalSize, err := airplan.ParseSize(opts.maxTotalSize)
	if err != nil {
		return fmt.Errorf("--max-total-size: %s",
			strings.TrimPrefix(err.Error(), "airplan: "))
	}
	if maxTotalSize == 0 {
		maxTotalSize = -1
	}

	cfg, err := airplan.LoadConfig(airplan.ConfigOptions{
		Path:      opts.config,
		Profile:   opts.profile,
		Overrides: flagOverrides(cmd, opts),
	})
	if err != nil {
		return err
	}
	if err := applyManifestSelection(cmd, cfg); err != nil {
		return err
	}
	if cfg.EffectiveBackend() == airplan.BackendAirplan {
		for _, name := range []string{
			"endpoint", "bucket", "region", "public-base-url", "key-prefix",
			"template", "collection-template", "no-source",
			"indexable", "no-external-assets", "mermaid-url",
		} {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf(
					"--%s is controlled by the Airplan server and cannot "+
						"be overridden by an airplan backend client",
					name,
				)
			}
		}
	}

	// The resolved timeout bounds the upload operation after config
	// resolution (SPEC.md §6).
	ctx := cmd.Context()
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	for _, w := range cfg.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}

	client, err := airplan.New(ctx, cfg)
	if err != nil {
		return err
	}

	if collection {
		return runCollection(cmd, ctx, client, args, opts, maxSize, maxTotalSize)
	}

	in := airplan.Input{
		Format:  opts.format,
		Slug:    opts.slug,
		Title:   opts.title,
		MaxSize: maxSize,
		Lang:    opts.lang,
	}
	if len(args) == 0 || args[0] == "-" {
		in.Reader = cmd.InOrStdin()
	} else {
		f, _, err := openRegularInput(args[0], "input")
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		in.Reader = f
		in.Name = args[0]
	}

	res, err := client.Upload(ctx, in)
	if err != nil {
		if errors.Is(err, airplan.ErrInputTooLarge) {
			return fmt.Errorf(
				"%w (raise or remove the limit with --max-size)", err,
			)
		}
		return err
	}

	for _, w := range res.Warnings {
		fmt.Fprintf(stderr, "airplan: warning: %s\n", w)
	}
	if err := printResult(cmd.OutOrStdout(), res, opts.json); err != nil {
		return err
	}
	if opts.open {
		if err := openBrowser(res.URL); err != nil {
			fmt.Fprintf(stderr,
				"airplan: warning: could not open browser: %s\n", err)
		}
	}
	return nil
}

type jsonResult struct {
	URL         string `json:"url"`
	Key         string `json:"key"`
	SourceURL   string `json:"source_url,omitempty"`
	Bucket      string `json:"bucket"`
	Bytes       int64  `json:"bytes"`
	ContentType string `json:"content_type"`
}

func printResult(w io.Writer, res *airplan.Result, jsonOutput bool) error {
	if !jsonOutput {
		_, err := fmt.Fprintln(w, res.URL)
		return err
	}

	out := jsonResult{
		URL:         res.URL,
		Key:         res.Key,
		SourceURL:   res.SourceURL,
		Bucket:      res.Bucket,
		Bytes:       res.Bytes,
		ContentType: res.ContentType,
	}
	return json.NewEncoder(w).Encode(out)
}

var openBrowser = defaultOpenBrowser

func defaultOpenBrowser(url string) error {
	var name string
	var args []string

	switch runtime.GOOS {
	case "darwin":
		name = "open"
		args = []string{url}
	case "linux":
		name = "xdg-open"
		args = []string{url}
	case "windows":
		name = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		return fmt.Errorf("unsupported platform %s", runtime.GOOS)
	}

	return exec.Command(name, args...).Start()
}

// flagOverrides packs explicitly-passed flags into a Settings overlay
// for LoadConfig — the top of the precedence order in SPEC.md §7,
// where it also counts toward profile-resolution completeness.
// Changed() guards let an explicit false override a config-file true
// for the bool flags; string flags use "" as "not set".
func flagOverrides(cmd *cobra.Command, opts *rootOptions) airplan.Settings {
	ov := airplan.Settings{
		Endpoint:           opts.endpoint,
		Bucket:             opts.bucket,
		Region:             opts.region,
		PublicBaseURL:      opts.publicBaseURL,
		KeyPrefix:          opts.keyPrefix,
		Template:           opts.template,
		CollectionTemplate: opts.collectionTemplate,
		Timeout:            opts.timeout,
		MermaidURL: airplan.ResolveMermaidURLOverride(
			opts.mermaidURL,
			cmd.Flags().Changed("mermaid-url"),
		),
		Repository: opts.repository,
	}
	f := cmd.Flags()
	if f.Changed("no-source") {
		ov.NoSource = &opts.noSource
	}
	if f.Changed("indexable") {
		ov.Indexable = &opts.indexable
	}
	if f.Changed("no-external-assets") {
		ov.NoExternalAssets = &opts.noExternalAssets
	}
	return ov
}

func selectCollectionMode(args []string, opts *rootOptions) (bool, error) {
	if opts.files {
		if len(args) == 0 || (len(args) == 1 && args[0] == "-") {
			return false, errors.New("airplan: --files requires one or more named files")
		}
		return true, nil
	}
	if opts.format != "" {
		if len(args) > 1 {
			return false, errors.New("airplan: --format accepts only one input")
		}
		return false, nil
	}
	if len(args) > 1 {
		return true, nil
	}
	if len(args) == 0 || args[0] == "-" {
		return false, nil
	}
	ext := strings.ToLower(filepath.Ext(args[0]))
	for _, e := range []string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg", ".mp4", ".webm", ".mov", ".mp3", ".m4a", ".ogg", ".wav", ".pdf", ".zip", ".gz", ".tar", ".bin", ".dmg", ".exe", ".wasm", ".7z"} {
		if ext == e {
			return true, nil
		}
	}
	f, _, err := openRegularInput(args[0], "input")
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	return sniffInputIsBinary(f)
}

const inputSniffSize = 8192

func sniffInputIsBinary(r io.Reader) (bool, error) {
	buf := make([]byte, inputSniffSize+utf8.UTFMax-1)
	n, err := io.ReadFull(r, buf)
	if err != nil && !errors.Is(err, io.EOF) &&
		!errors.Is(err, io.ErrUnexpectedEOF) {
		return false, err
	}
	end := n
	if end > inputSniffSize {
		end = inputSniffSize
	}
	if bytes.IndexByte(buf[:end], 0) >= 0 {
		return true, nil
	}
	// Include only enough lookahead to finish a rune split at the sniff
	// boundary. Invalid UTF-8 earlier in the prefix can never become valid.
	for ; end <= n; end++ {
		if utf8.Valid(buf[:end]) {
			return false, nil
		}
	}
	return true, nil
}

func validateModeFlags(cmd *cobra.Command, collection bool) error {
	docOnly := []string{"format", "lang", "slug", "template", "no-source", "no-external-assets", "mermaid-url"}
	collectionOnly := []string{"files", "collection-template", "max-total-size"}
	if collection {
		for _, name := range docOnly {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("airplan: --%s is only valid for document uploads", name)
			}
		}
	} else {
		for _, name := range collectionOnly {
			if cmd.Flags().Changed(name) {
				return fmt.Errorf("airplan: --%s is only valid for collection uploads", name)
			}
		}
	}
	return nil
}

func runCollection(cmd *cobra.Command, ctx context.Context, client *airplan.Client, args []string, opts *rootOptions, maxSize, maxTotal int64) error {
	inputs, closers, err := openCollectionInputs(args)
	if err != nil {
		return err
	}
	defer func() {
		for _, c := range closers {
			_ = c.Close()
		}
	}()
	res, err := client.UploadFiles(ctx, airplan.FilesInput{Files: inputs, Title: opts.title, MaxSize: maxSize, MaxTotalSize: maxTotal})
	if err != nil {
		return err
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: %s\n", w)
	}
	if opts.json {
		out := struct {
			URL         string               `json:"url"`
			Key         string               `json:"key"`
			Files       []airplan.FileResult `json:"files"`
			Bucket      string               `json:"bucket"`
			Bytes       int64                `json:"bytes"`
			ContentType string               `json:"content_type"`
		}{res.URL, res.Key, res.Files, res.Bucket, res.Bytes, res.ContentType}
		if err := json.NewEncoder(cmd.OutOrStdout()).Encode(out); err != nil {
			return err
		}
	} else {
		for _, f := range res.Files {
			if _, err := fmt.Fprintln(cmd.OutOrStdout(), f.URL); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(cmd.OutOrStdout(), res.URL); err != nil {
			return err
		}
	}
	if opts.open {
		if err := openBrowser(res.URL); err != nil {
			fmt.Fprintf(cmd.ErrOrStderr(), "airplan: warning: could not open browser: %s\n", err)
		}
	}
	return nil
}

func openCollectionInputs(args []string) ([]airplan.FileInput, []*os.File, error) {
	if len(args) == 0 {
		return nil, nil, errors.New("airplan: collections require named files")
	}
	if len(args) > airplan.MaxCollectionFiles {
		return nil, nil, fmt.Errorf(
			"airplan: collection has %d files; maximum is %d",
			len(args), airplan.MaxCollectionFiles,
		)
	}
	for _, name := range args {
		if name == "-" {
			return nil, nil, errors.New(
				"airplan: collections require named files",
			)
		}
	}

	inputs := make([]airplan.FileInput, 0, len(args))
	files := make([]*os.File, 0, len(args))
	closeFiles := func() {
		for _, file := range files {
			_ = file.Close()
		}
	}
	for _, name := range args {
		file, info, err := openRegularInput(name, "collection input")
		if err != nil {
			closeFiles()
			return nil, nil, err
		}
		files = append(files, file)
		inputs = append(inputs, airplan.FileInput{
			Name: name, Reader: file, Size: info.Size(),
		})
	}
	return inputs, files, nil
}

func openRegularInput(
	name, label string,
) (*os.File, os.FileInfo, error) {
	// Reject obvious non-regular inputs before os.Open: opening a FIFO for
	// reading can block before the configured operation timeout applies.
	// After a successful open, descriptor metadata remains authoritative.
	pathInfo, err := os.Stat(name)
	if err != nil {
		return nil, nil, err
	}
	if !pathInfo.Mode().IsRegular() {
		return nil, nil, fmt.Errorf(
			"airplan: %s %q is not a regular file", label, name,
		)
	}
	file, err := os.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, nil, fmt.Errorf(
			"airplan: %s %q is not a regular file", label, name,
		)
	}
	return file, info, nil
}
