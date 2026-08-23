package airplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"time"
)

func (c *Client) createBundleRevision(
	ctx context.Context, input CreateDocumentRevisionInput, maxDiffSize int,
) (revisionResult *DocumentRevisionResult, returnErr error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if input.Target == "" {
		return nil, errors.New("airplan: document revision target is required")
	}
	if input.Document.Entry.Reader == nil {
		return nil, errors.New("airplan: document revision entry reader is nil")
	}
	if err := validateDocumentItemCount(input.Document); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.CreateDocumentRevision(ctx, input)
	}
	if err := c.ensureStorage(ctx); err != nil {
		return nil, err
	}
	input.Document.Assets = append([]AssetInput(nil), input.Document.Assets...)
	assets, err := prepareAssets(ctx, input.Document)
	if err != nil {
		return nil, err
	}
	for index := range assets {
		input.Document.Assets[index] = assets[index].AssetInput
	}
	preflight, err := renderDocumentSpooled(ctx, input.Document,
		DocumentRenderOptions{RenderInputOptions: RenderInputOptions{
			IncludeSource: true, Repository: "none", Themes: c.cfg.ThemeBundle,
		}, MaxGeneratedPageSize: input.Document.maxGeneratedPageSize}, c.template, c.templateErr)
	if err != nil {
		return nil, err
	}
	defer preflight.cleanup()
	if preflight.Format != FormatMarkdown.String() {
		return nil, errors.New(
			"airplan: document revision entry must use Markdown format; " +
				"supports Markdown input only",
		)
	}
	_, latest, err := c.prepareRevisionTarget(ctx, input.Target)
	if err != nil {
		return nil, err
	}
	if latest.marker.Format != "md" || len(latest.marker.Pages) == 0 {
		return nil, errors.New("airplan: document revisions require a source-backed Markdown entry")
	}
	if input.Document.Slug != "" && SanitizeSlug(input.Document.Slug) != latest.marker.Slug {
		return nil, errors.New("airplan: document revision cannot change the slug")
	}
	if input.Document.Entry.Path != "" &&
		SanitizeSlug(filenameStem(input.Document.Entry.Path)) != latest.marker.Slug {
		return nil, errors.New("airplan: document revision entry name must match the existing document slug")
	}
	input.Document.Slug = latest.marker.Slug
	input.Document.RepositoryURL = latest.marker.Repo
	validatedInput, closeValidated, err := reopenedDocumentInput(
		preflight, input.Document,
	)
	if err != nil {
		return nil, err
	}
	defer closeValidated()
	if input.Document.Entry.Path == "" {
		validatedInput.Entry.Path = latest.marker.Pages[0].Path
		if input.Document.Entry.Title == "" && input.Document.Title == "" {
			// The preflight used its anonymous-input fallback name. Let the
			// target's preserved logical name participate in title resolution.
			validatedInput.Entry.Title = ""
		}
	}
	if len(input.Document.Pages) == 0 && len(latest.marker.Pages) == 1 &&
		input.Document.Title == "" && input.Document.Entry.Title == "" {
		validatedInput.Title = latest.marker.Title
		validatedInput.Entry.Title = latest.marker.Title
	}
	previousRevision, newRevision, chainID := 1, 2, ""
	if latest.versions != nil {
		previousRevision = latest.versions.CurrentRevision
		newRevision = latest.versions.LastAssignedRevision + 1
		chainID = latest.versions.ChainID
	} else {
		chainID, err = RandomDirName()
		if err != nil {
			return nil, fmt.Errorf("airplan: generate revision chain ID: %w", err)
		}
	}
	recipe := cloneRenderRecipe(latest.marker.Render)
	if recipe == nil || !c.upgradeTemplateMatches(recipe) {
		return nil, errors.New("airplan: latest revision's rendering template cannot be reproduced")
	}
	if !themeRecipeMatches(recipe.Themes, c.cfg.ThemeBundle) {
		return nil, errors.New("airplan: latest revision's themes do not match the configured catalog; run airplan upgrade --force first")
	}
	// Render once without revision chrome to compare the complete logical input.
	comparison, err := renderDocumentSpooled(ctx, validatedInput, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Indexable: recipe.Indexable, IncludeSource: true, NoExternalAssets: recipe.NoExternalAssets, MermaidURL: recipe.MermaidURL, Repository: latest.marker.Repo, Themes: c.cfg.ThemeBundle}, MaxGeneratedPageSize: input.Document.maxGeneratedPageSize}, c.template, c.templateErr)
	if err != nil {
		return nil, err
	}
	defer comparison.cleanup()
	if bundleLogicalStateMatches(latest.marker, comparison, assets) {
		if err := c.reconcileRevisionMetadata(ctx, latest); err != nil {
			return nil, err
		}
		result := bundleRevisionResultFromExisting(c.cfg, latest)
		result.Unchanged = true
		return result, nil
	}
	diffBody, err := c.generateBundleRevisionDiff(ctx, latest, comparison, assets, previousRevision, newRevision, maxDiffSize)
	if err != nil {
		return nil, err
	}
	diffReport, err := parseRevisionDiffReport(diffBody, comparison.Pages[0].Path)
	if err != nil {
		return nil, fmt.Errorf("airplan: parse generated revision diff: %w", err)
	}
	pageDiffs, changedPages := inlinePageDiffs(diffReport, comparison.Pages)
	revisionInput, closeSources, err := reopenedDocumentInput(comparison, validatedInput)
	if err != nil {
		return nil, err
	}
	defer closeSources()
	bundle, err := renderDocumentSpooled(ctx, revisionInput, DocumentRenderOptions{RenderInputOptions: RenderInputOptions{Indexable: recipe.Indexable, IncludeSource: true, NoExternalAssets: recipe.NoExternalAssets, MermaidURL: recipe.MermaidURL, Repository: latest.marker.Repo, Themes: c.cfg.ThemeBundle}, MaxGeneratedPageSize: input.Document.maxGeneratedPageSize, Revision: newRevision, RevisionCount: newRevision, PreviousRevision: previousRevision, RevisionChainID: chainID, VersionsPath: VersionsFilename, DiffPath: DiffFilename, PageDiffs: pageDiffs, ChangedPages: changedPages, CompleteDiffText: inlineRevisionDiff(diffBody)}, c.template, c.templateErr)
	if err != nil {
		return nil, err
	}
	defer bundle.cleanup()
	dir, err := RandomDirName()
	if err != nil {
		return nil, fmt.Errorf("airplan: generate revision upload key: %w", err)
	}
	createdAt := time.Now().UTC().Truncate(time.Second)
	pageKey := BuildKey(c.cfg.KeyPrefix, dir, bundle.Entrypoint)
	pageURL, fallback, err := PublicURL(c.cfg, pageKey)
	if err != nil {
		return nil, err
	}
	diffKey := BuildKey(c.cfg.KeyPrefix, dir, DiffFilename)
	diffURL, _, err := PublicURL(c.cfg, diffKey)
	if err != nil {
		return nil, err
	}
	marker := UploadMarker{Schema: MarkerSchema, Version: MarkerVersion, Directory: dir, CreatedAt: createdAt, Kind: UploadKindDocument, Slug: bundle.Slug, Format: bundle.Format, Title: bundle.Title, Repo: bundle.RepositoryURL, Producer: Producer{Name: "airplan", Version: producerVersion(c.cfg.ProducerVersion)}, Render: recipe, Revision: &RevisionDescriptor{ChainID: chainID, Number: newRevision, PreviousURL: latest.pageURL}, Entrypoint: bundle.Entrypoint}
	for _, page := range bundle.Pages {
		marker.Objects = append(marker.Objects, MarkerObject{Name: page.PagePath, Role: MarkerRolePage, Bytes: page.pageSize(), ContentType: pageContentType, SHA256: page.pageDigest()})
		contentType := textContentType
		if page.Format == "md" {
			contentType = sourceContentType
		}
		marker.Objects = append(marker.Objects, MarkerObject{Name: page.SourcePath, Role: MarkerRoleSource, Bytes: page.sourceSize(), ContentType: contentType, SHA256: page.sourceDigest()})
		marker.Pages = append(marker.Pages, MarkerPage{Path: page.Path, Page: page.PagePath, Source: page.SourcePath, Format: page.Format, Title: page.Title, Lang: page.Lang})
	}
	for _, asset := range assets {
		marker.Objects = append(marker.Objects, MarkerObject{Name: asset.Path, Role: MarkerRoleAsset, Bytes: asset.Size, ContentType: asset.ContentType, SHA256: asset.digest})
	}
	marker.Objects = append(marker.Objects, MarkerObject{Name: DiffFilename, Role: MarkerRoleDiff, Bytes: int64(len(diffBody)), ContentType: diffContentType, SHA256: contentSHA256(diffBody)})
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		return nil, err
	}
	markerKey := BuildKey(c.cfg.KeyPrefix, dir, MarkerFilename)
	if err := c.st.put(ctx, object{Key: markerKey, Body: markerBody, ContentType: markerContentType}); err != nil {
		return nil, err
	}
	candidateKeys := make([]string, 0, len(marker.Objects))
	for _, object := range marker.Objects {
		candidateKeys = append(
			candidateKeys, BuildKey(c.cfg.KeyPrefix, dir, object.Name),
		)
	}
	rollback := func() error {
		cleanupCtx, cancel := revisionCleanupContext(ctx, c.cfg.Timeout)
		defer cancel()
		if err := c.st.deleteKeys(cleanupCtx, candidateKeys); err != nil {
			return err
		}
		return c.st.deleteMarker(cleanupCtx, markerKey)
	}
	candidatePending := true
	defer func() {
		if !candidatePending {
			return
		}
		if cleanupErr := rollback(); cleanupErr != nil {
			if returnErr == nil {
				returnErr = fmt.Errorf(
					"airplan: candidate rollback failed: %w", cleanupErr,
				)
				return
			}
			returnErr = fmt.Errorf(
				"%w; candidate rollback failed: %v", returnErr, cleanupErr,
			)
		}
	}()
	pages := make([]PageResult, 0, len(bundle.Pages))
	for _, page := range bundle.Pages {
		sourceKey := BuildKey(c.cfg.KeyPrefix, dir, page.SourcePath)
		contentType := textContentType
		if page.Format == "md" {
			contentType = sourceContentType
		}
		if err := c.putDocumentPayload(ctx, sourceKey, page.sourceFile,
			contentType, titleMetadata(page.Title)); err != nil {
			return nil, err
		}
		result, resultErr := c.bundlePageResult(dir, page)
		if resultErr != nil {
			return nil, resultErr
		}
		pages = append(pages, result)
	}
	assetResults := make([]AssetResult, 0, len(assets))
	for _, asset := range assets {
		digest, hashErr := hashExactReader(asset.Reader, asset.start, asset.Size)
		if hashErr != nil {
			return nil, fmt.Errorf(
				"airplan: verify asset %q after preflight: %w", asset.Path, hashErr,
			)
		}
		if digest != asset.digest {
			return nil, fmt.Errorf("airplan: asset %q changed after preflight", asset.Path)
		}
		if _, err := asset.Reader.Seek(asset.start, io.SeekStart); err != nil {
			return nil, err
		}
		key := BuildKey(c.cfg.KeyPrefix, dir, asset.Path)
		if err := c.st.putStream(ctx, streamObject{Key: key, Body: asset.Reader, Size: asset.Size, ContentType: asset.ContentType, Metadata: titleMetadata(bundle.Title)}); err != nil {
			return nil, err
		}
		assetURL, _, urlErr := PublicURL(c.cfg, key)
		if urlErr != nil {
			return nil, urlErr
		}
		assetResults = append(assetResults, AssetResult{Path: asset.Path, URL: assetURL, Key: key, Bytes: asset.Size, ContentType: asset.ContentType})
	}
	if err := c.st.put(ctx, object{Key: diffKey, Body: diffBody, ContentType: diffContentType}); err != nil {
		return nil, err
	}
	for index := 1; index < len(bundle.Pages); index++ {
		page := bundle.Pages[index]
		if err := c.putDocumentPayload(ctx,
			BuildKey(c.cfg.KeyPrefix, dir, page.PagePath), page.htmlFile,
			pageContentType, titleMetadata(page.Title)); err != nil {
			return nil, err
		}
	}
	if err := c.putDocumentPayload(ctx, pageKey, bundle.Pages[0].htmlFile,
		pageContentType, titleMetadata(bundle.Title)); err != nil {
		return nil, err
	}
	metadata := c.nextVersionsMetadata(latest, chainID, newRevision, pageURL, diffURL, createdAt)
	memberBodies, err := c.encodeMemberMetadata(metadata)
	if err != nil {
		return nil, err
	}
	latestMetadataKey := latest.dirPrefix + VersionsFilename
	newMetadataKey := BuildKey(c.cfg.KeyPrefix, dir, VersionsFilename)
	if latest.versions == nil {
		err = c.st.putIfAbsent(ctx, object{Key: latestMetadataKey, Body: memberBodies[previousRevision], ContentType: markerContentType})
	} else {
		err = c.st.putConditional(ctx, object{Key: latestMetadataKey, Body: memberBodies[previousRevision], ContentType: markerContentType}, latest.versionsETag)
	}
	if err != nil {
		originalErr := err
		reconcileCtx, cancel := revisionCleanupContext(ctx, c.cfg.Timeout)
		observed, committed, reconcileErr := c.reconcileRevisionAppendCommit(
			reconcileCtx, latestMetadataKey, latest.pageKey,
			memberBodies[previousRevision], newMetadataKey, pageKey, metadata,
			newRevision, pageURL, latest.versions == nil, originalErr,
		)
		cancel()
		if reconcileErr != nil {
			candidatePending = false
			return nil, fmt.Errorf(
				"%w; revision candidate state is uncertain: %v",
				originalErr, reconcileErr,
			)
		}
		if !committed {
			if errors.Is(originalErr, ErrConflict) {
				originalErr = &revisionAppendConflictError{err: originalErr}
			}
			return nil, originalErr
		}
		metadata = *observed
		memberBodies, err = c.encodeMemberMetadata(metadata)
		if err != nil {
			return nil, err
		}
	}
	candidatePending = false
	postCommitCtx, postCommitCancel := revisionCleanupContext(ctx, c.cfg.Timeout)
	defer postCommitCancel()
	if latest.versions == nil {
		if err := c.promoteStandaloneRevision(postCommitCtx, latest, chainID, metadata); err != nil {
			return nil, err
		}
		promoted, loadErr := c.loadRevisionDocumentForUpdate(
			postCommitCtx, latest.pageURL,
		)
		if loadErr != nil || promoted.needsPromotion || promoted.needsPageRepair ||
			promoted.marker.Revision == nil ||
			promoted.marker.Revision.ChainID != chainID ||
			promoted.marker.Revision.Number != 1 {
			if loadErr != nil {
				return nil, fmt.Errorf(
					"airplan: revision committed but predecessor verification failed: %w",
					loadErr,
				)
			}
			return nil, errors.New(
				"airplan: revision committed but predecessor verification failed",
			)
		}
		latest = promoted
	}
	if err := c.st.putIfAbsent(postCommitCtx, object{Key: newMetadataKey, Body: memberBodies[newRevision], ContentType: markerContentType}); err != nil {
		actual, readErr := c.st.getBytes(
			postCommitCtx, newMetadataKey, MaxVersionsMetadataSize,
		)
		if !errors.Is(err, ErrConflict) || readErr != nil ||
			!c.revisionMetadataAtLeast(actual, memberBodies[newRevision], pageKey) {
			return nil, fmt.Errorf(
				"airplan: revision committed but metadata propagation failed: %w", err,
			)
		}
	}
	if err := c.propagateVersionsMetadata(postCommitCtx, metadata, memberBodies, latestMetadataKey, newMetadataKey); err != nil {
		return nil, fmt.Errorf(
			"airplan: revision committed but metadata propagation failed: %w", err,
		)
	}
	if err := c.verifyRevisionChain(postCommitCtx, metadata, memberBodies); err != nil {
		return nil, err
	}
	result := &DocumentRevisionResult{DocumentResult: DocumentResult{Result: Result{ID: dir, URL: pageURL, Key: pageKey, SourceURL: pages[0].SourceURL, SourceKey: pages[0].SourceKey, Bucket: c.cfg.Bucket, Bytes: bundle.Pages[0].pageSize(), ContentType: pageContentType, Title: bundle.Title, CreatedAt: createdAt, MarkerVersion: MarkerVersion, MarkerKey: markerKey, Format: bundle.Format, Kind: string(UploadKindDocument), Slug: bundle.Slug, RepositoryURL: bundle.RepositoryURL, RevisionChainID: chainID, Revision: newRevision, LatestRevision: metadata.LatestRevision}, Pages: pages, Assets: assetResults}, PreviousURL: latest.pageURL, DiffURL: diffURL}
	if fallback {
		result.Warnings = append(result.Warnings, PublicURLFallbackWarning)
	}
	compatibility := &UpdateDocumentResult{Result: result.Result, PreviousURL: result.PreviousURL, DiffURL: result.DiffURL}
	c.recordRevisionAppend(postCommitCtx, compatibility, markerBody, chainID, metadata)
	return result, nil
}

func revisionCleanupContext(
	ctx context.Context, timeout time.Duration,
) (context.Context, context.CancelFunc) {
	cleanupCtx := context.WithoutCancel(ctx)
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return context.WithTimeout(cleanupCtx, timeout)
}

func (c *Client) bundlePageResult(dir string, page RenderedBundlePage) (PageResult, error) {
	key := BuildKey(c.cfg.KeyPrefix, dir, page.PagePath)
	u, _, err := PublicURL(c.cfg, key)
	if err != nil {
		return PageResult{}, err
	}
	sourceKey := BuildKey(c.cfg.KeyPrefix, dir, page.SourcePath)
	sourceURL, _, err := PublicURL(c.cfg, sourceKey)
	if err != nil {
		return PageResult{}, err
	}
	return PageResult{Path: page.Path, Format: page.Format, Title: page.Title, URL: u, Key: key, SourceURL: sourceURL, SourceKey: sourceKey, Bytes: page.pageSize(), SourceBytes: page.sourceSize()}, nil
}

func reopenedDocumentInput(
	bundle *RenderedDocumentBundle, original DocumentInput,
) (DocumentInput, func(), error) {
	opened := make([]io.Closer, 0, len(bundle.Pages))
	closeOpened := func() {
		for _, closer := range opened {
			_ = closer.Close()
		}
	}
	pages := make([]PageInput, len(bundle.Pages))
	for index := range bundle.Pages {
		page := &bundle.Pages[index]
		file, err := page.sourceFile.open()
		if err != nil {
			closeOpened()
			return DocumentInput{}, func() {}, err
		}
		opened = append(opened, file)
		pages[index] = PageInput{
			Reader: file, Path: page.Path, Format: page.Format,
			Title: page.Title, Lang: page.Lang,
		}
	}
	input := original
	input.Entry = pages[0]
	input.Pages = pages[1:]
	return input, closeOpened, nil
}

func bundleLogicalStateMatches(marker *UploadMarker, bundle *RenderedDocumentBundle, assets []preparedAsset) bool {
	if marker == nil || marker.Title != bundle.Title || marker.Slug != bundle.Slug || marker.Format != bundle.Format || len(marker.Pages) != len(bundle.Pages) {
		return false
	}
	objects := make(map[string]MarkerObject, len(marker.Objects))
	for _, object := range marker.Objects {
		objects[object.Name] = object
	}
	for index, page := range bundle.Pages {
		descriptor := marker.Pages[index]
		if descriptor.Path != page.Path || descriptor.Page != page.PagePath || descriptor.Source != page.SourcePath || descriptor.Format != page.Format || descriptor.Title != page.Title || descriptor.Lang != page.Lang {
			return false
		}
		source, ok := objects[page.SourcePath]
		if !ok || source.Role != MarkerRoleSource || source.SHA256 != page.sourceDigest() {
			return false
		}
	}
	markerAssets := make([]MarkerObject, 0, len(assets))
	for _, object := range marker.Objects {
		if object.Role == MarkerRoleAsset {
			markerAssets = append(markerAssets, object)
		}
	}
	if len(markerAssets) != len(assets) {
		return false
	}
	for index, asset := range assets {
		object := markerAssets[index]
		if object.Name != asset.Path || object.Bytes != asset.Size || object.ContentType != asset.ContentType || object.SHA256 != asset.digest {
			return false
		}
	}
	return true
}

func bundleRevisionResultFromExisting(cfg *Config, doc *revisionDocument) *DocumentRevisionResult {
	result := cResultFromMarker(cfg, doc.marker, doc.dirPrefix)
	result.URL = doc.pageURL
	result.Key = doc.pageKey
	result.SourceKey = doc.sourceKey
	result.SourceURL, _, _ = PublicURL(cfg, doc.sourceKey)
	result.Bytes = int64(len(doc.pageBody))
	result.ContentType = pageContentType
	var previousURL, diffURL string
	if doc.marker.Revision != nil {
		previousURL = doc.marker.Revision.PreviousURL
	}
	if doc.versions != nil {
		result.LatestRevision = doc.versions.LatestRevision
		if current := liveVersionsRevision(
			doc.versions, doc.versions.CurrentRevision,
		); current != nil {
			diffURL = current.DiffURL
		}
	}
	return &DocumentRevisionResult{
		DocumentResult: DocumentResult{
			Result: result,
			Pages:  pageResultsFromMarker(cfg, doc.marker, doc.dirPrefix),
			Assets: assetResultsFromMarker(cfg, doc.marker, doc.dirPrefix),
		},
		PreviousURL: previousURL,
		DiffURL:     diffURL,
	}
}

func cResultFromMarker(cfg *Config, marker *UploadMarker, dirPrefix string) Result {
	result := Result{ID: marker.Directory, Bucket: cfg.Bucket, Title: marker.Title, CreatedAt: marker.CreatedAt, MarkerVersion: marker.Version, MarkerKey: dirPrefix + MarkerFilename, Format: marker.Format, Kind: string(marker.Kind), Slug: marker.Slug, RepositoryURL: marker.Repo}
	if marker.Revision != nil {
		result.RevisionChainID = marker.Revision.ChainID
		result.Revision = marker.Revision.Number
	}
	return result
}

func pageResultsFromMarker(cfg *Config, marker *UploadMarker, dirPrefix string) []PageResult {
	objects := make(map[string]MarkerObject, len(marker.Objects))
	for _, object := range marker.Objects {
		objects[object.Name] = object
	}
	results := make([]PageResult, 0, len(marker.Pages))
	for _, page := range marker.Pages {
		pageObject := objects[page.Page]
		result := PageResult{Path: page.Path, Format: page.Format, Title: page.Title, Key: dirPrefix + page.Page, Bytes: pageObject.Bytes}
		result.URL, _, _ = PublicURL(cfg, result.Key)
		if page.Source != "" {
			result.SourceKey = dirPrefix + page.Source
			result.SourceURL, _, _ = PublicURL(cfg, result.SourceKey)
			result.SourceBytes = objects[page.Source].Bytes
		}
		results = append(results, result)
	}
	return results
}

func assetResultsFromMarker(
	cfg *Config, marker *UploadMarker, dirPrefix string,
) []AssetResult {
	results := make([]AssetResult, 0)
	for _, object := range marker.Objects {
		if object.Role != MarkerRoleAsset {
			continue
		}
		result := AssetResult{
			Path: object.Name, Key: dirPrefix + object.Name,
			Bytes: object.Bytes, ContentType: object.ContentType,
		}
		result.URL, _, _ = PublicURL(cfg, result.Key)
		results = append(results, result)
	}
	return results
}

func (c *Client) generateBundleRevisionDiff(ctx context.Context, previous *revisionDocument, next *RenderedDocumentBundle, assets []preparedAsset, previousRevision, newRevision, maxSize int) ([]byte, error) {
	oldSources := make(map[string]MarkerPage, len(previous.marker.Pages))
	for _, page := range previous.marker.Pages {
		if page.Source == "" {
			continue
		}
		oldSources[page.Path] = page
	}
	newSources := make(map[string]*RenderedBundlePage, len(next.Pages))
	for index := range next.Pages {
		newSources[next.Pages[index].Path] = &next.Pages[index]
	}
	oldAssets := make(map[string]MarkerObject)
	for _, object := range previous.marker.Objects {
		if object.Role == MarkerRoleAsset {
			oldAssets[object.Name] = object
		}
	}
	newAssets := make(map[string]preparedAsset, len(assets))
	for _, asset := range assets {
		newAssets[asset.Path] = asset
	}
	paths := make(map[string]struct{}, len(oldSources)+len(newSources)+len(oldAssets)+len(newAssets))
	for value := range oldSources {
		paths[value] = struct{}{}
	}
	for value := range newSources {
		paths[value] = struct{}{}
	}
	for value := range oldAssets {
		paths[value] = struct{}{}
	}
	for value := range newAssets {
		paths[value] = struct{}{}
	}
	ordered := make([]string, 0, len(paths))
	for value := range paths {
		ordered = append(ordered, value)
	}
	sort.Strings(ordered)
	var out bytes.Buffer
	fmt.Fprintf(&out, "# airplan revisions: %d -> %d\n", previousRevision, newRevision)
	out.WriteString(revisionDiffFormatHeader)
	baseSize := out.Len()
	sizeError := func() error {
		return fmt.Errorf("airplan: revision diff exceeds maximum size of %s", formatSize(int64(maxSize)))
	}
	checkSize := func() error {
		if maxSize > 0 && out.Len() > maxSize {
			return sizeError()
		}
		return nil
	}
	if err := checkSize(); err != nil {
		return nil, err
	}
	oldPageOrder := make([]string, 0, len(previous.marker.Pages))
	for _, page := range previous.marker.Pages {
		oldPageOrder = append(oldPageOrder, page.Path)
	}
	newPageOrder := make([]string, 0, len(next.Pages))
	for _, page := range next.Pages {
		newPageOrder = append(newPageOrder, page.Path)
	}
	if !revisionPathOrderEqual(oldPageOrder, newPageOrder) {
		before, _ := json.Marshal(oldPageOrder)
		after, _ := json.Marshal(newPageOrder)
		fmt.Fprintf(&out, "# airplan page order\nbefore: %s\nafter: %s\n", before, after)
	}
	oldAssetOrder := make([]string, 0)
	for _, object := range previous.marker.Objects {
		if object.Role == MarkerRoleAsset {
			oldAssetOrder = append(oldAssetOrder, object.Name)
		}
	}
	newAssetOrder := make([]string, 0, len(assets))
	for _, asset := range assets {
		newAssetOrder = append(newAssetOrder, asset.Path)
	}
	if !revisionPathOrderEqual(oldAssetOrder, newAssetOrder) {
		before, _ := json.Marshal(oldAssetOrder)
		after, _ := json.Marshal(newAssetOrder)
		fmt.Fprintf(&out, "# airplan asset order\nbefore: %s\nafter: %s\n", before, after)
	}
	if previous.marker.Slug != next.Slug || previous.marker.Title != next.Title ||
		previous.marker.Format != next.Format {
		before, _ := json.Marshal(struct {
			Format string `json:"format"`
			Title  string `json:"title"`
			Slug   string `json:"slug"`
		}{previous.marker.Format, previous.marker.Title, previous.marker.Slug})
		after, _ := json.Marshal(struct {
			Format string `json:"format"`
			Title  string `json:"title"`
			Slug   string `json:"slug"`
		}{next.Format, next.Title, next.Slug})
		fmt.Fprintf(&out, "# airplan metadata\nbefore: %s\nafter: %s\n", before, after)
	}
	if err := checkSize(); err != nil {
		return nil, err
	}
	for _, logical := range ordered {
		oldPage, oldOK := oldSources[logical]
		newPage, newOK := newSources[logical]
		if oldOK || newOK {
			if err := c.appendBundlePageRevisionDiff(
				ctx, &out, previous, oldPage, oldOK, newPage, newOK,
				logical, previousRevision, newRevision, maxSize,
			); err != nil {
				return nil, err
			}
		}
		oldAsset, oldAssetOK := oldAssets[logical]
		newAsset, newAssetOK := newAssets[logical]
		if oldAssetOK || newAssetOK {
			if err := appendBundleAssetRevisionDiff(
				&out, logical, oldAsset, oldAssetOK, newAsset, newAssetOK,
				maxSize,
			); err != nil {
				return nil, err
			}
		}
	}
	if out.Len() == baseSize {
		out.WriteString("No textual changes.\n")
	}
	if err := checkSize(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func (c *Client) appendBundlePageRevisionDiff(
	ctx context.Context, out *bytes.Buffer, previous *revisionDocument,
	oldPage MarkerPage, oldOK bool, newPage *RenderedBundlePage, newOK bool,
	logical string, previousRevision, newRevision, maxSize int,
) error {
	var oldBody, newBody []byte
	if oldOK {
		object, ok := markerObjectNamed(previous.marker, oldPage.Source)
		if !ok {
			return fmt.Errorf("airplan: previous page %q source is missing", oldPage.Path)
		}
		var err error
		oldBody, err = c.st.getBytes(
			ctx, previous.dirPrefix+oldPage.Source, object.Bytes,
		)
		if err != nil {
			return err
		}
	}
	if newOK {
		var err error
		newBody, err = newPage.sourceBody(ctx)
		if err != nil {
			return err
		}
	}
	metadataLine := ""
	switch {
	case !oldOK:
		metadataLine = "page added: " +
			pageDescriptorJSON(newPage.Format, newPage.Title, newPage.Lang)
	case !newOK:
		metadataLine = "page removed: " +
			pageDescriptorJSON(oldPage.Format, oldPage.Title, oldPage.Lang)
	default:
		type changedValue struct {
			Before string `json:"before"`
			After  string `json:"after"`
		}
		changes := struct {
			Format *changedValue `json:"format,omitempty"`
			Title  *changedValue `json:"title,omitempty"`
			Lang   *changedValue `json:"lang,omitempty"`
		}{}
		if oldPage.Format != newPage.Format {
			changes.Format = &changedValue{oldPage.Format, newPage.Format}
		}
		if oldPage.Title != newPage.Title {
			changes.Title = &changedValue{oldPage.Title, newPage.Title}
		}
		if oldPage.Lang != newPage.Lang {
			changes.Lang = &changedValue{oldPage.Lang, newPage.Lang}
		}
		if changes.Format != nil || changes.Title != nil || changes.Lang != nil {
			encoded, _ := json.Marshal(changes)
			metadataLine = "page metadata changed: " + string(encoded)
		}
	}
	sourceChanged := !oldOK || !newOK || !bytes.Equal(oldBody, newBody)
	if !sourceChanged && metadataLine == "" {
		return nil
	}
	writeRevisionDiffSectionHeader(out, "page", logical)
	if metadataLine != "" {
		fmt.Fprintln(out, metadataLine)
	}
	if sourceChanged {
		sectionLimit := maxSize
		if maxSize > 0 {
			sectionLimit = maxSize - out.Len()
			if sectionLimit <= 0 {
				return bundleRevisionDiffSizeError(maxSize)
			}
		}
		section, err := generateRevisionDiffForPath(
			oldBody, newBody, previousRevision, newRevision, logical, sectionLimit,
		)
		if err != nil {
			return err
		}
		out.Write(section)
		if len(section) == 0 || section[len(section)-1] != '\n' {
			out.WriteByte('\n')
		}
	}
	if maxSize > 0 && out.Len() > maxSize {
		return bundleRevisionDiffSizeError(maxSize)
	}
	return nil
}

func appendBundleAssetRevisionDiff(
	out *bytes.Buffer, logical string,
	oldAsset MarkerObject, oldOK bool, newAsset preparedAsset, newOK bool,
	maxSize int,
) error {
	if oldOK && newOK && oldAsset.Bytes == newAsset.Size &&
		oldAsset.ContentType == newAsset.ContentType &&
		oldAsset.SHA256 == newAsset.digest {
		return nil
	}
	writeRevisionDiffSectionHeader(out, "asset", logical)
	switch {
	case !oldOK:
		fmt.Fprintf(out, "asset added: %s\n", revisionAssetDescriptorJSON(
			newAsset.ContentType, newAsset.Size, newAsset.digest,
		))
	case !newOK:
		fmt.Fprintf(out, "asset removed: %s\n", revisionAssetDescriptorJSON(
			oldAsset.ContentType, oldAsset.Bytes, oldAsset.SHA256,
		))
	default:
		encoded, _ := json.Marshal(revisionAssetChange{
			Before: revisionAssetDescriptor{
				ContentType: oldAsset.ContentType,
				Bytes:       oldAsset.Bytes, SHA256: oldAsset.SHA256,
			},
			After: revisionAssetDescriptor{
				ContentType: newAsset.ContentType,
				Bytes:       newAsset.Size, SHA256: newAsset.digest,
			},
		})
		fmt.Fprintf(out, "asset changed: %s\n", encoded)
	}
	if maxSize > 0 && out.Len() > maxSize {
		return bundleRevisionDiffSizeError(maxSize)
	}
	return nil
}

func bundleRevisionDiffSizeError(maxSize int) error {
	return fmt.Errorf(
		"airplan: revision diff exceeds maximum size of %s",
		formatSize(int64(maxSize)),
	)
}

func writeRevisionDiffSectionHeader(out *bytes.Buffer, kind, logical string) {
	encoded, _ := json.Marshal(logical)
	fmt.Fprintf(out, "# airplan %s: %s\n", kind, encoded)
}

func revisionPathOrderEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func pageDescriptorJSON(format, title, lang string) string {
	encoded, _ := json.Marshal(struct {
		Format string `json:"format"`
		Title  string `json:"title,omitempty"`
		Lang   string `json:"lang"`
	}{format, title, lang})
	return string(encoded)
}

func revisionAssetDescriptorJSON(contentType string, size int64, digest string) string {
	encoded, _ := json.Marshal(revisionAssetDescriptor{
		ContentType: contentType,
		Bytes:       size,
		SHA256:      digest,
	})
	return string(encoded)
}

func markerObjectNamed(marker *UploadMarker, name string) (MarkerObject, bool) {
	for _, object := range marker.Objects {
		if object.Name == name {
			return object, true
		}
	}
	return MarkerObject{}, false
}

func inlinePageDiffs(
	report *revisionDiffReport, pages []RenderedBundlePage,
) (map[string]string, map[string]bool) {
	inline := make(map[string]string)
	changed := make(map[string]bool)
	if report == nil {
		return inline, changed
	}
	for _, page := range pages {
		section, ok := report.PageSections[page.Path]
		if !ok {
			continue
		}
		changed[page.Path] = true
		if len(section) <= MaxInlineDiffSize {
			inline[page.Path] = string(section)
		}
	}
	return inline, changed
}
