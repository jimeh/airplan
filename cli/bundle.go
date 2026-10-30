package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/jimeh/airplan/airplan"
)

// openedDocument holds a bundle whose members were validated up front but are
// opened on demand: pages one at a time while rendering, assets only on the
// library's bounded upload workers.
type openedDocument struct {
	input   airplan.DocumentInput
	readers []io.Closer
	rootDir string
}

func (o *openedDocument) close() {
	for _, reader := range o.readers {
		_ = reader.Close()
	}
}

func openDocumentBundle(entry string, pages, assets []string) (*openedDocument, error) {
	if entry == "" || entry == "-" {
		return nil, errors.New("airplan: document bundles require a named entry file")
	}
	entryAbs, err := filepath.Abs(entry)
	if err != nil {
		return nil, fmt.Errorf("airplan: resolve bundle entry %q: %w", entry, err)
	}
	entryReal, err := filepath.EvalSymlinks(entryAbs)
	if err != nil {
		return nil, fmt.Errorf("airplan: resolve bundle entry %q: %w", entry, err)
	}
	rootDeclared := filepath.Dir(entryAbs)
	rootReal, err := filepath.EvalSymlinks(rootDeclared)
	if err != nil {
		return nil, fmt.Errorf("airplan: resolve bundle root: %w", err)
	}
	if !pathWithinRoot(rootReal, entryReal) {
		return nil, fmt.Errorf(
			"airplan: bundle entry %q resolves outside its declared directory", entry,
		)
	}
	opened := &openedDocument{rootDir: rootReal}
	fail := func(err error) (*openedDocument, error) {
		opened.close()
		return nil, err
	}
	entryFile, err := airplan.StatLocalFile(entryReal, "bundle entry")
	if err != nil {
		return fail(err)
	}
	entryReader := entryFile.LazyReader()
	opened.readers = append(opened.readers, entryReader)
	opened.input.Entry = airplan.PageInput{Reader: entryReader, Path: filepath.Base(entryAbs)}
	for _, value := range pages {
		file, logical, statErr := statBundleMember(rootDeclared, rootReal, value, "page")
		if statErr != nil {
			return fail(statErr)
		}
		reader := file.LazyReader()
		opened.readers = append(opened.readers, reader)
		opened.input.Pages = append(opened.input.Pages, airplan.PageInput{Reader: reader, Path: logical})
	}
	for _, value := range assets {
		file, logical, statErr := statBundleMember(rootDeclared, rootReal, value, "asset")
		if statErr != nil {
			return fail(statErr)
		}
		opened.input.Assets = append(opened.input.Assets, airplan.AssetInput{Open: file.Open, Path: logical, Size: file.Size()})
	}
	return opened, nil
}

func statBundleMember(rootDeclared, rootReal, value, role string) (*airplan.LocalFile, string, error) {
	if value == "" || value == "-" {
		return nil, "", fmt.Errorf("airplan: bundle %s requires a named file", role)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return nil, "", fmt.Errorf("airplan: resolve bundle %s %q: %w", role, value, err)
	}
	relative, err := filepath.Rel(rootDeclared, abs)
	if err != nil {
		return nil, "", fmt.Errorf("airplan: resolve bundle %s %q beneath entry directory: %w", role, value, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, "", fmt.Errorf("airplan: bundle %s %q is declared outside the entry directory", role, value)
	}
	logical := filepath.ToSlash(relative)
	if err := airplan.ValidateBundlePath(logical); err != nil {
		return nil, "", err
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, "", fmt.Errorf("airplan: resolve bundle %s %q: %w", role, value, err)
	}
	if !pathWithinRoot(rootReal, real) {
		return nil, "", fmt.Errorf("airplan: bundle %s %q resolves outside the entry directory", role, value)
	}
	file, err := airplan.StatLocalFile(real, "bundle "+role)
	if err != nil {
		return nil, "", err
	}
	return file, logical, nil
}

func pathWithinRoot(root, target string) bool {
	relative, err := filepath.Rel(root, target)
	return err == nil && relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}
