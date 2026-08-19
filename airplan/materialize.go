package airplan

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// MaterializeDocument renders a complete document through private temporary
// payload files and atomically publishes it as a new local directory. It
// returns non-fatal rendering warnings.
func MaterializeDocument(
	ctx context.Context, in DocumentInput, opts DocumentRenderOptions,
	destination string,
) ([]string, error) {
	if ctx == nil {
		return nil, errors.New("airplan: nil context")
	}
	if opts.Template != nil && opts.TemplatePath != "" {
		return nil, errors.New("airplan: materialize document: template and template path are mutually exclusive")
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
	bundle, err := renderDocumentSpooled(ctx, in, opts, tmpl, tmplErr)
	if err != nil {
		return nil, err
	}
	defer bundle.cleanup()
	if err := materializeRenderedDocument(
		ctx, destination, bundle, assets,
	); err != nil {
		return nil, err
	}
	return append([]string(nil), bundle.Warnings...), nil
}

func materializeRenderedDocument(
	ctx context.Context, destination string, bundle *RenderedDocumentBundle,
	assets []preparedAsset,
) error {
	abs, err := filepath.Abs(destination)
	if err != nil {
		return fmt.Errorf("airplan: resolve preview output directory %q: %w", destination, err)
	}
	if _, err := os.Lstat(abs); err == nil {
		return fmt.Errorf("airplan: preview output directory %q already exists", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("airplan: inspect preview output directory %q: %w", destination, err)
	}
	parent := filepath.Dir(abs)
	if info, err := os.Stat(parent); err != nil || !info.IsDir() {
		if err == nil {
			err = errors.New("parent is not a directory")
		}
		return fmt.Errorf("airplan: preview output parent %q: %w", parent, err)
	}
	temporary, err := os.MkdirTemp(parent, ".airplan-preview-*")
	if err != nil {
		return fmt.Errorf("airplan: create preview staging directory: %w", err)
	}
	if err := os.Chmod(temporary, 0o700); err != nil {
		_ = os.RemoveAll(temporary)
		return fmt.Errorf("airplan: secure preview staging directory: %w", err)
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(temporary)
		}
	}()
	publishedFiles := make([]string, 0, len(bundle.Pages)*2+len(assets))
	write := func(relative string, payload spooledPayload, retained []byte) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		target := filepath.Join(temporary, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(
			target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600,
		)
		if err != nil {
			return err
		}
		var copyErr error
		if payload.path != "" {
			input, openErr := payload.verify()
			if openErr != nil {
				_ = file.Close()
				return openErr
			}
			_, copyErr = io.CopyN(file, input, payload.size)
			_ = input.Close()
		} else {
			_, copyErr = file.Write(retained)
		}
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		publishedFiles = append(publishedFiles, target)
		return nil
	}
	for index := range bundle.Pages {
		page := &bundle.Pages[index]
		if err := write(page.PagePath, page.htmlFile, page.HTML); err != nil {
			return fmt.Errorf("airplan: write preview page %q: %w", page.PagePath, err)
		}
		if page.SourcePath != "" {
			if err := write(page.SourcePath, page.sourceFile, page.Source); err != nil {
				return fmt.Errorf("airplan: write preview source %q: %w", page.SourcePath, err)
			}
		}
	}
	for _, prepared := range assets {
		asset := prepared.AssetInput
		if _, err := asset.Reader.Seek(prepared.start, io.SeekStart); err != nil {
			return fmt.Errorf("airplan: seek preview asset %q: %w", asset.Path, err)
		}
		target := filepath.Join(temporary, filepath.FromSlash(asset.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		file, err := os.OpenFile(
			target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600,
		)
		if err != nil {
			return err
		}
		_, copyErr := io.CopyN(file, asset.Reader, asset.Size)
		closeErr := file.Close()
		if copyErr != nil {
			return fmt.Errorf("airplan: copy preview asset %q: %w", asset.Path, copyErr)
		}
		if closeErr != nil {
			return fmt.Errorf("airplan: close preview asset %q: %w", asset.Path, closeErr)
		}
		written, err := os.Open(target)
		if err != nil {
			return err
		}
		digest, hashErr := hashExactReader(written, 0, asset.Size)
		_ = written.Close()
		if hashErr != nil || digest != prepared.digest {
			return fmt.Errorf("airplan: preview asset %q changed after preflight", asset.Path)
		}
		publishedFiles = append(publishedFiles, target)
	}
	for _, target := range publishedFiles {
		if err := os.Chmod(target, 0o644); err != nil {
			return fmt.Errorf("airplan: set preview file permissions: %w", err)
		}
	}
	if err := os.Chmod(temporary, 0o755); err != nil {
		return fmt.Errorf("airplan: set preview directory permissions: %w", err)
	}
	if err := publishDirectoryNoReplace(temporary, abs); err != nil {
		return fmt.Errorf("airplan: publish preview directory %q: %w", destination, err)
	}
	complete = true
	return nil
}
