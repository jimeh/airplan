package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jimeh/airplan/airplan"
)

type openedDocument struct {
	input   airplan.DocumentInput
	files   []*os.File
	rootDir string
}

func (o *openedDocument) close() {
	for _, file := range o.files {
		_ = file.Close()
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
	rootReal, err := filepath.EvalSymlinks(filepath.Dir(entryReal))
	if err != nil {
		return nil, fmt.Errorf("airplan: resolve bundle root: %w", err)
	}
	opened := &openedDocument{rootDir: rootReal}
	fail := func(err error) (*openedDocument, error) {
		opened.close()
		return nil, err
	}
	entryFile, entryInfo, err := openRegularInput(entryReal, "bundle entry")
	if err != nil {
		return fail(err)
	}
	_ = entryInfo
	opened.files = append(opened.files, entryFile)
	opened.input.Entry = airplan.PageInput{Reader: entryFile, Path: filepath.Base(entryReal)}
	for _, value := range pages {
		file, info, logical, openErr := openBundleMember(rootReal, value, "page")
		if openErr != nil {
			return fail(openErr)
		}
		_ = info
		opened.files = append(opened.files, file)
		opened.input.Pages = append(opened.input.Pages, airplan.PageInput{Reader: file, Path: logical})
	}
	for _, value := range assets {
		file, info, logical, openErr := openBundleMember(rootReal, value, "asset")
		if openErr != nil {
			return fail(openErr)
		}
		opened.files = append(opened.files, file)
		opened.input.Assets = append(opened.input.Assets, airplan.AssetInput{Reader: file, Path: logical, Size: info.Size()})
	}
	return opened, nil
}

func openBundleMember(rootReal, value, role string) (*os.File, os.FileInfo, string, error) {
	if value == "" || value == "-" {
		return nil, nil, "", fmt.Errorf("airplan: bundle %s requires a named file", role)
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return nil, nil, "", fmt.Errorf("airplan: resolve bundle %s %q: %w", role, value, err)
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, nil, "", fmt.Errorf("airplan: resolve bundle %s %q: %w", role, value, err)
	}
	relative, err := filepath.Rel(rootReal, real)
	if err != nil {
		return nil, nil, "", fmt.Errorf("airplan: resolve bundle %s %q beneath entry directory: %w", role, value, err)
	}
	if relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return nil, nil, "", fmt.Errorf("airplan: bundle %s %q resolves outside the entry directory", role, value)
	}
	logical := filepath.ToSlash(relative)
	if err := airplan.ValidateBundlePath(logical); err != nil {
		return nil, nil, "", err
	}
	file, info, err := openRegularInput(real, "bundle "+role)
	if err != nil {
		return nil, nil, "", err
	}
	return file, info, logical, nil
}
