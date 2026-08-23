package airplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"
	"unicode/utf8"
)

// UpdateDocumentInput links a complete new Markdown upload to an existing
// source-backed Markdown document or chain member.
//
// Deprecated: use CreateDocumentRevisionInput.
type UpdateDocumentInput struct {
	Target string `json:"target"`
	Input  Input  `json:"-"`
}

// UpdateDocumentResult describes a new revision or an identical-source no-op.
//
// Deprecated: use DocumentRevisionResult.
type UpdateDocumentResult struct {
	Result
	PreviousURL string `json:"previous_url,omitempty"`
	DiffURL     string `json:"diff_url,omitempty"`
	Unchanged   bool   `json:"unchanged"`
}

// CreateDocumentRevisionInput supplies a complete replacement document bundle.
type CreateDocumentRevisionInput struct {
	Target   string        `json:"target"`
	Document DocumentInput `json:"-"`
}

// DocumentRevisionResult describes a new complete revision or an unchanged
// logical-bundle no-op.
type DocumentRevisionResult struct {
	DocumentResult
	PreviousURL string `json:"previous_url,omitempty"`
	DiffURL     string `json:"diff_url,omitempty"`
	Unchanged   bool   `json:"unchanged"`
}

type revisionAppendConflictError struct{ err error }

type updateRefusalKind string

const (
	updateRefusalMissing       updateRefusalKind = "missing"
	updateRefusalInvalidTarget updateRefusalKind = "invalid_target"
	updateRefusalInvalidUpload updateRefusalKind = "invalid_upload"
)

type updateRefusalError struct {
	kind updateRefusalKind
	err  error
}

var errRevisionTransitionReserved = errors.New(
	"airplan: standalone revision transition is reserved or invalid",
)

func (e *revisionAppendConflictError) Error() string { return e.err.Error() }
func (e *revisionAppendConflictError) Unwrap() error { return e.err }
func (e *updateRefusalError) Error() string          { return e.err.Error() }
func (e *updateRefusalError) Unwrap() error          { return e.err }

type revisionDocument struct {
	marker          *UploadMarker
	markerBody      []byte
	markerETag      string
	pageBody        []byte
	pageETag        string
	sourceBody      []byte
	sourceETag      string
	dirPrefix       string
	pageKey         string
	sourceKey       string
	pageURL         string
	versions        *VersionsMetadata
	versionsBody    []byte
	versionsETag    string
	needsPromotion  bool
	needsMetadata   bool
	needsPageRepair bool
}

// UpdateDocument resolves target to the latest live member, then performs a
// conditional linear append. The existing page is never repointed.
//
// Deprecated: use CreateDocumentRevision.
func (c *Client) UpdateDocument(
	ctx context.Context, input UpdateDocumentInput,
) (*UpdateDocumentResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if err := validateUpdateDocumentInput(input); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.UpdateDocument(ctx, input)
	}
	return c.updateDocument(ctx, input, MaxDiffSize)
}

// CreateDocumentRevision is the canonical complete-document revision API.
func (c *Client) CreateDocumentRevision(
	ctx context.Context, input CreateDocumentRevisionInput,
) (*DocumentRevisionResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if input.Document.Entry.Reader == nil {
		return nil, errors.New("airplan: document revision entry reader is nil")
	}
	if err := validateDocumentItemCount(input.Document); err != nil {
		return nil, err
	}
	if c != nil && c.remote != nil {
		return c.remote.CreateDocumentRevision(ctx, input)
	}
	return c.createBundleRevision(ctx, input, MaxDiffSize)
}

func (c *Client) updateDocument(
	ctx context.Context, input UpdateDocumentInput, maxDiffSize int,
) (*UpdateDocumentResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if err := validateUpdateDocumentInput(input); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.UpdateDocument(ctx, input)
	}
	if err := c.ensureStorage(ctx); err != nil {
		return nil, err
	}

	limit := input.Input.MaxSize
	if limit == 0 {
		limit = DefaultMaxInputSize
	} else if limit < 0 {
		limit = 0
	}
	proposed, err := readInput(ctx, input.Input.Reader, limit)
	if err != nil {
		return nil, err
	}
	if len(proposed) == 0 {
		return nil, ErrEmptyInput
	}
	if !utf8.Valid(proposed) {
		return nil, ErrInvalidUTF8
	}
	if IsBinary(proposed) {
		return nil, ErrBinaryInput
	}
	format, err := documentPageFormat(input.Input.Format, input.Input.Name, proposed)
	if err != nil {
		return nil, err
	}
	if format != FormatMarkdown {
		return nil, errors.New("airplan: update supports Markdown input only")
	}

	_, latest, err := c.prepareRevisionTarget(ctx, input.Target)
	if err != nil {
		return nil, err
	}
	if input.Input.Name != "" &&
		SanitizeSlug(filenameStem(input.Input.Name)) != latest.marker.Slug {
		return nil, &updateRefusalError{
			kind: updateRefusalInvalidTarget,
			err:  errors.New("airplan: update input name must match the existing document slug"),
		}
	}

	if bytes.Equal(proposed, latest.sourceBody) {
		if err := c.reconcileRevisionMetadata(ctx, latest); err != nil {
			return nil, err
		}
		result := c.updateResultFromDocument(latest)
		result.Unchanged = true
		return result, nil
	}

	previousRevision := 1
	newRevision := 2
	chainID := ""
	if latest.versions != nil {
		chainID = latest.versions.ChainID
		previousRevision = latest.versions.CurrentRevision
		newRevision = latest.versions.LastAssignedRevision + 1
	} else {
		chainID, err = RandomDirName()
		if err != nil {
			return nil, fmt.Errorf("airplan: generate revision chain ID: %w", err)
		}
	}
	diffBody, err := generateRevisionDiff(
		latest.sourceBody, proposed, previousRevision, newRevision, maxDiffSize,
	)
	if err != nil {
		return nil, err
	}
	dir, err := RandomDirName()
	if err != nil {
		return nil, fmt.Errorf("airplan: generate revision upload key: %w", err)
	}
	createdAt := time.Now().UTC().Truncate(time.Second)
	title := latest.marker.Title
	if input.Input.Title != "" {
		title = input.Input.Title
	}
	recipe := cloneRenderRecipe(latest.marker.Render)
	if recipe == nil {
		recipe = documentRenderRecipe(c.cfg, c.templateDigest)
	}
	if !c.upgradeTemplateMatches(recipe) {
		return nil, &updateRefusalError{
			kind: updateRefusalInvalidTarget,
			err:  errors.New("airplan: latest revision's rendering template cannot be reproduced"),
		}
	}
	if !themeRecipeMatches(recipe.Themes, c.cfg.ThemeBundle) {
		return nil, &updateRefusalError{
			kind: updateRefusalInvalidTarget,
			err: errors.New(
				"airplan: latest revision's themes do not match the configured catalog; " +
					"run airplan upgrade --force on the target before updating",
			),
		}
	}
	pageName := latest.marker.Slug + ".html"
	sourceName := latest.marker.Slug + ".md"
	pageKey := BuildKey(c.cfg.KeyPrefix, dir, pageName)
	sourceKey := BuildKey(c.cfg.KeyPrefix, dir, sourceName)
	diffKey := BuildKey(c.cfg.KeyPrefix, dir, DiffFilename)
	pageURL, fallback, err := PublicURL(c.cfg, pageKey)
	if err != nil {
		return nil, err
	}
	sourceURL, _, err := PublicURL(c.cfg, sourceKey)
	if err != nil {
		return nil, err
	}
	diffURL, _, err := PublicURL(c.cfg, diffKey)
	if err != nil {
		return nil, err
	}
	pageBody, err := RenderMarkdown(proposed, RenderOptions{
		Title: title, Slug: latest.marker.Slug, SourceName: sourceName,
		SourcePath: "./" + sourceName, Indexable: recipe.Indexable,
		NoExternalAssets: recipe.NoExternalAssets, MermaidURL: recipe.MermaidURL,
		RepositoryURL: latest.marker.Repo, Template: c.template,
		Themes:   c.cfg.ThemeBundle,
		Revision: newRevision, RevisionCount: newRevision,
		PreviousRevision: previousRevision, RevisionChainID: chainID,
		VersionsPath: VersionsFilename,
		DiffPath:     "./" + DiffFilename, DiffText: inlineRevisionDiff(diffBody),
		HasCompleteDiff:    true,
		CurrentLogicalPath: sourceName, CurrentRenderedPath: pageName,
		Entrypoint: pageName, AllChangesPath: "./" + pageName + "#airplan-all-changes",
	})
	if err != nil {
		return nil, err
	}
	spool, err := newPayloadSpool()
	if err != nil {
		return nil, err
	}
	defer spool.cleanup()
	sourcePayload, err := spool.put("source", proposed)
	if err != nil {
		return nil, err
	}
	diffPayload, err := spool.put("diff", diffBody)
	if err != nil {
		return nil, err
	}
	pagePayload, err := spool.put("page", pageBody)
	if err != nil {
		return nil, err
	}
	logicalPath := docInputLogicalPath(input.Input.Name, FormatMarkdown)
	if input.Input.Name == "" && len(latest.marker.Pages) > 0 {
		logicalPath = latest.marker.Pages[0].Path
	}
	marker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: createdAt, Kind: UploadKindDocument,
		Slug: latest.marker.Slug, Format: "md", Title: title,
		Repo:       latest.marker.Repo,
		Producer:   Producer{Name: "airplan", Version: producerVersion(c.cfg.ProducerVersion)},
		Render:     recipe,
		Entrypoint: pageName,
		Pages: []MarkerPage{{
			Path: logicalPath,
			Page: pageName, Source: sourceName, Format: "md", Title: title,
			Lang: "",
		}},
		Revision: &RevisionDescriptor{
			ChainID: chainID, Number: newRevision,
			PreviousURL: latest.pageURL,
		},
		Objects: []MarkerObject{
			{
				Name: pageName, Role: MarkerRolePage, Bytes: pagePayload.size,
				ContentType: pageContentType, SHA256: pagePayload.digest,
			},
			{
				Name: sourceName, Role: MarkerRoleSource, Bytes: sourcePayload.size,
				ContentType: sourceContentType, SHA256: sourcePayload.digest,
			},
			{
				Name: DiffFilename, Role: MarkerRoleDiff, Bytes: diffPayload.size,
				ContentType: diffContentType, SHA256: diffPayload.digest,
			},
		},
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		return nil, err
	}

	metadata := c.nextVersionsMetadata(latest, chainID, newRevision,
		pageURL, diffURL, createdAt)
	memberBodies, err := c.encodeMemberMetadata(metadata)
	if err != nil {
		return nil, err
	}

	markerKey := BuildKey(c.cfg.KeyPrefix, dir, MarkerFilename)
	candidateKeys := []string{sourceKey, diffKey, pageKey}
	cleanupContext := func() (context.Context, context.CancelFunc) {
		rollbackCtx := context.WithoutCancel(ctx)
		rollbackTimeout := c.cfg.Timeout
		if rollbackTimeout <= 0 {
			rollbackTimeout = DefaultTimeout
		}
		return context.WithTimeout(rollbackCtx, rollbackTimeout)
	}
	rollback := func() error {
		rollbackCtx, cancel := cleanupContext()
		defer cancel()
		if deleteErr := c.st.deleteKeys(rollbackCtx, candidateKeys); deleteErr != nil {
			return deleteErr
		}
		return c.st.deleteMarker(rollbackCtx, markerKey)
	}
	if err := c.st.put(ctx, object{
		Key: markerKey, Body: markerBody, ContentType: markerContentType,
	}); err != nil {
		return nil, err
	}
	for _, payload := range []struct {
		key, contentType string
		body             spooledPayload
		metadata         map[string]string
	}{
		{sourceKey, sourceContentType, sourcePayload, titleMetadata(title)},
		{diffKey, diffContentType, diffPayload, nil},
		{pageKey, pageContentType, pagePayload, titleMetadata(title)},
	} {
		if err := c.putDocumentPayload(
			ctx, payload.key, payload.body, payload.contentType, payload.metadata,
		); err != nil {
			if cleanupErr := rollback(); cleanupErr != nil {
				return nil, fmt.Errorf("%w; candidate rollback failed: %v", err, cleanupErr)
			}
			return nil, err
		}
	}

	latestMetadataKey := latest.dirPrefix + VersionsFilename
	newMetadataKey := BuildKey(c.cfg.KeyPrefix, dir, VersionsFilename)
	latestBody := memberBodies[previousRevision]
	if latest.versions == nil {
		err = c.st.putIfAbsent(ctx, object{
			Key:  latestMetadataKey,
			Body: latestBody, ContentType: markerContentType,
		})
	} else {
		err = c.st.putConditional(ctx, object{
			Key:  latestMetadataKey,
			Body: latestBody, ContentType: markerContentType,
		}, latest.versionsETag)
	}
	if err != nil {
		originalErr := err
		reconcileCtx, cancel := cleanupContext()
		observed, committed, reconcileErr := c.reconcileRevisionAppendCommit(
			reconcileCtx, latestMetadataKey, latest.pageKey, latestBody,
			newMetadataKey, pageKey, metadata, newRevision, pageURL,
			latest.versions == nil, originalErr,
		)
		cancel()
		if reconcileErr != nil {
			return nil, fmt.Errorf(
				"%w; revision candidate state is uncertain and was not rolled back: %v",
				originalErr, reconcileErr,
			)
		}
		if committed {
			metadata = *observed
			memberBodies, err = c.encodeMemberMetadata(metadata)
			if err != nil {
				return nil, err
			}
		} else {
			if errors.Is(originalErr, ErrConflict) {
				originalErr = &revisionAppendConflictError{err: originalErr}
			}
			cleanupErr := rollback()
			if cleanupErr != nil {
				return nil, fmt.Errorf("%w; candidate rollback failed: %v", originalErr, cleanupErr)
			}
			return nil, originalErr
		}
	}

	// Once the latest metadata transition succeeds, this append has won. Any
	// subsequent error is repairable and deliberately does not roll it back.
	postCommitCtx, postCommitCancel := cleanupContext()
	defer postCommitCancel()
	if latest.versions == nil {
		if err := c.promoteStandaloneRevision(postCommitCtx, latest, chainID, metadata); err != nil {
			return nil, err
		}
		promoted, loadErr := c.loadRevisionDocumentForUpdate(postCommitCtx, latest.pageURL)
		if loadErr != nil || promoted.needsPromotion || promoted.needsPageRepair ||
			promoted.marker.Revision == nil || promoted.marker.Revision.ChainID != chainID ||
			promoted.marker.Revision.Number != 1 {
			if loadErr != nil {
				return nil, fmt.Errorf("airplan: revision committed but predecessor verification failed: %w", loadErr)
			}
			return nil, errors.New("airplan: revision committed but predecessor verification failed")
		}
		latest = promoted
	}
	if err := c.st.putIfAbsent(postCommitCtx, object{
		Key:  newMetadataKey,
		Body: memberBodies[newRevision], ContentType: markerContentType,
	}); err != nil {
		actual, readErr := c.st.getBytes(postCommitCtx, newMetadataKey, MaxVersionsMetadataSize)
		if !errors.Is(err, ErrConflict) || readErr != nil ||
			!c.revisionMetadataAtLeast(actual, memberBodies[newRevision], pageKey) {
			return nil, fmt.Errorf("airplan: revision committed but metadata propagation failed: %w", err)
		}
	}
	if err := c.propagateVersionsMetadata(postCommitCtx, metadata, memberBodies,
		latestMetadataKey, newMetadataKey); err != nil {
		return nil, fmt.Errorf("airplan: revision committed but metadata propagation failed: %w", err)
	}
	if err := c.verifyRevisionChain(postCommitCtx, metadata, memberBodies); err != nil {
		return nil, err
	}

	result := &UpdateDocumentResult{
		Result: Result{
			ID: dir, URL: pageURL, Key: pageKey,
			SourceURL: sourceURL, SourceKey: sourceKey, Bucket: c.cfg.Bucket,
			Bytes: pagePayload.size, ContentType: pageContentType,
			Title: title, CreatedAt: createdAt, MarkerVersion: MarkerVersion,
			MarkerKey: markerKey, Format: "md", Kind: string(UploadKindDocument),
			Slug: marker.Slug, RepositoryURL: marker.Repo,
			RevisionChainID: chainID, Revision: newRevision,
			LatestRevision: metadata.LatestRevision,
		},
		PreviousURL: latest.pageURL, DiffURL: diffURL,
	}
	if fallback {
		result.Warnings = append(result.Warnings, PublicURLFallbackWarning)
	}
	c.recordRevisionAppend(postCommitCtx, result, markerBody, chainID, metadata)
	return result, nil
}

func (c *Client) reconcileRevisionAppendCommit(
	ctx context.Context, metadataKey, containingPageKey string,
	expectedSerializationBody []byte, candidateMetadataKey, candidatePageKey string,
	expected VersionsMetadata, newRevision int, newURL string,
	standalone bool, originalErr error,
) (*VersionsMetadata, bool, error) {
	committedMetadata := func(body []byte, pageKey string) *VersionsMetadata {
		observed, err := DecodeVersionsMetadata(body, c.cfg, pageKey)
		if err != nil {
			return nil
		}
		entry := liveVersionsRevision(observed, newRevision)
		if observed.ChainID != expected.ChainID || entry == nil || entry.URL != newURL ||
			!versionsMetadataMonotonicallyPrecedes(&expected, observed) {
			return nil
		}
		return observed
	}
	inspectCandidate := func() (*VersionsMetadata, bool, error) {
		body, err := c.st.getBytes(ctx, candidateMetadataKey, MaxVersionsMetadataSize)
		if errors.Is(err, errObjectNotFound) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, fmt.Errorf(
				"airplan: inspect ambiguous revision candidate: %w", err,
			)
		}
		observed := committedMetadata(body, candidatePageKey)
		return observed, observed != nil, nil
	}

	body, etag, _, err := c.st.getBytesWithETag(
		ctx, metadataKey, MaxVersionsMetadataSize,
	)
	if errors.Is(err, errObjectNotFound) {
		if observed, committed, candidateErr := inspectCandidate(); candidateErr != nil {
			return nil, false, candidateErr
		} else if committed {
			return observed, true, nil
		}
		if errors.Is(originalErr, ErrConflict) {
			return nil, false, nil
		}
		if standalone {
			putErr := c.st.putIfAbsent(ctx, object{
				Key: metadataKey, Body: expectedSerializationBody,
				ContentType: markerContentType,
			})
			if putErr == nil {
				return &expected, true, nil
			}
			if errors.Is(putErr, ErrConflict) {
				actual, readErr := c.st.getBytes(
					ctx, metadataKey, MaxVersionsMetadataSize,
				)
				if readErr == nil {
					if observed := committedMetadata(actual, containingPageKey); observed != nil {
						return observed, true, nil
					}
					return nil, false, nil
				}
			}
			return nil, false, fmt.Errorf(
				"airplan: retry ambiguous first revision append: %w", putErr,
			)
		}
		return nil, false, errors.New(
			"airplan: revision serialization object disappeared during append reconciliation",
		)
	}
	if err != nil {
		return nil, false, fmt.Errorf("airplan: inspect ambiguous revision append: %w", err)
	}
	if observed := committedMetadata(body, containingPageKey); observed != nil {
		return observed, true, nil
	}
	if observed, committed, candidateErr := inspectCandidate(); candidateErr != nil {
		return nil, false, candidateErr
	} else if committed {
		return observed, true, nil
	}
	if errors.Is(originalErr, ErrConflict) {
		return nil, false, nil
	}
	if _, decodeErr := DecodeVersionsMetadata(body, c.cfg, containingPageKey); decodeErr != nil {
		return nil, false, errors.New(
			"airplan: revision serialization state is invalid during append reconciliation",
		)
	}
	claimBody, claimErr := revisionCandidateCleanupClaimBody(body)
	if claimErr != nil {
		return nil, false, claimErr
	}
	if claimErr = c.st.putConditional(ctx, object{
		Key: metadataKey, Body: claimBody, ContentType: markerContentType,
	}, etag); claimErr == nil {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf(
		"airplan: claim ambiguous revision candidate cleanup: %w", claimErr,
	)
}

func validateUpdateDocumentInput(input UpdateDocumentInput) error {
	if input.Target == "" {
		return errors.New("airplan: update target is required")
	}
	if input.Input.Reader == nil {
		return errors.New("airplan: update input reader is nil")
	}
	if input.Input.Slug != "" {
		return errors.New("airplan: update cannot change the document slug")
	}
	if input.Input.Format != "" && input.Input.Format != "md" {
		return errors.New("airplan: update supports Markdown input only")
	}
	if input.Input.Lang != "" {
		return errors.New("airplan: update does not accept a text highlight language")
	}
	if input.Input.RepositoryURL != "" {
		return errors.New("airplan: update cannot change the repository URL")
	}
	return nil
}

// prepareRevisionTarget applies the shared prerequisite state machine used by
// both the canonical complete-replacement API and the deprecated entry-only
// compatibility API. It returns the originally addressed member and the
// latest live member after upgrade, repair, and chain validation.
func (c *Client) prepareRevisionTarget(
	ctx context.Context, targetValue string,
) (*revisionDocument, *revisionDocument, error) {
	target, err := c.loadRevisionDocumentForUpdate(ctx, targetValue)
	if err != nil {
		var refusal *updateRefusalError
		if errors.As(err, &refusal) {
			return nil, nil, err
		}
		if errors.Is(err, errRevisionTransitionReserved) {
			return nil, nil, &updateRefusalError{
				kind: updateRefusalInvalidUpload, err: err,
			}
		}
		plan, planErr := c.PlanUpgradeDocument(
			ctx, targetValue, UpgradeDocumentOptions{},
		)
		if planErr != nil || plan.State == UpgradeStateCurrent {
			return nil, nil, err
		}
		if plan.State != UpgradeStateUpgradeable {
			kind := updateRefusalInvalidTarget
			switch plan.State {
			case UpgradeStateMissing:
				kind = updateRefusalMissing
			case UpgradeStateInvalid:
				kind = updateRefusalInvalidUpload
			}
			return nil, nil, &updateRefusalError{kind: kind, err: err}
		}
		if _, upgradeErr := c.UpgradeDocument(ctx, *plan); upgradeErr != nil {
			return nil, nil, fmt.Errorf(
				"airplan: update prerequisite upgrade failed: %w", upgradeErr,
			)
		}
		target, err = c.loadRevisionDocumentForUpdate(ctx, targetValue)
		if err != nil {
			return nil, nil, err
		}
	}
	if target.needsMetadata {
		target, err = c.repairMissingRevisionMetadata(ctx, target, nil)
		if err != nil {
			return nil, nil, err
		}
	}
	if target.needsPageRepair {
		target, err = c.repairRevisionPage(ctx, target)
		if err != nil {
			return nil, nil, err
		}
	}
	if err := c.repairFirstRevisionPromotionFromMember(ctx, target); err != nil {
		return nil, nil, err
	}

	latest := target
	seen := map[string]bool{latest.pageKey: true}
	for latest.versions != nil &&
		latest.versions.LatestRevision != latest.versions.CurrentRevision {
		entry := liveVersionsRevision(latest.versions, latest.versions.LatestRevision)
		if entry == nil {
			return nil, nil, &updateRefusalError{
				kind: updateRefusalInvalidUpload,
				err:  errors.New("airplan: revision metadata has no live latest entry"),
			}
		}
		previous := latest
		latest, err = c.loadRevisionDocumentForUpdate(ctx, entry.URL)
		if err != nil {
			return nil, nil, fmt.Errorf("airplan: resolve latest revision: %w", err)
		}
		if seen[latest.pageKey] {
			return nil, nil, &updateRefusalError{
				kind: updateRefusalInvalidUpload,
				err:  errors.New("airplan: revision metadata latest link forms a cycle"),
			}
		}
		seen[latest.pageKey] = true
		if latest.needsMetadata {
			latest, err = c.repairMissingRevisionMetadata(ctx, latest, previous.versions)
			if err != nil {
				return nil, nil, err
			}
		}
		if latest.versions == nil ||
			latest.versions.ChainID != previous.versions.ChainID ||
			latest.versions.CurrentRevision != previous.versions.LatestRevision {
			return nil, nil, &updateRefusalError{
				kind: updateRefusalInvalidUpload,
				err: errors.New(
					"airplan: revision metadata cannot be reconciled with latest member",
				),
			}
		}
		if latest.needsPageRepair {
			latest, err = c.repairRevisionPage(ctx, latest)
			if err != nil {
				return nil, nil, err
			}
		}
	}
	if target.needsPromotion {
		if latest.marker.Revision == nil || target.versions == nil ||
			latest.marker.Revision.ChainID != target.versions.ChainID ||
			latest.marker.Revision.PreviousURL != target.pageURL {
			return nil, nil, errors.New(
				"airplan: interrupted predecessor promotion cannot be reconciled",
			)
		}
		if err := c.promoteStandaloneRevision(
			ctx, target, target.versions.ChainID, *target.versions,
		); err != nil {
			return nil, nil, err
		}
		promoted, loadErr := c.loadRevisionDocumentForUpdate(ctx, target.pageURL)
		if loadErr != nil {
			return nil, nil, loadErr
		}
		if promoted.needsPromotion || promoted.needsPageRepair ||
			promoted.marker.Revision == nil ||
			promoted.marker.Revision.ChainID != target.versions.ChainID ||
			promoted.marker.Revision.Number != 1 {
			return nil, nil, errors.New(
				"airplan: interrupted predecessor promotion verification failed",
			)
		}
		target = promoted
	}
	if latest.marker.Render != nil &&
		latest.marker.Render.Generation < RendererGeneration {
		plan, planErr := c.PlanUpgradeDocument(
			ctx, latest.pageURL, UpgradeDocumentOptions{},
		)
		if planErr != nil {
			return nil, nil, planErr
		}
		if plan.State != UpgradeStateUpgradeable {
			return nil, nil, fmt.Errorf(
				"airplan: latest revision requires an upgrade: %s", plan.Reason,
			)
		}
		if _, upgradeErr := c.UpgradeDocument(ctx, *plan); upgradeErr != nil {
			return nil, nil, fmt.Errorf(
				"airplan: update prerequisite upgrade failed: %w", upgradeErr,
			)
		}
		latest, err = c.loadRevisionDocumentForUpdate(ctx, latest.pageURL)
		if err != nil {
			return nil, nil, err
		}
	}
	return target, latest, nil
}

func (c *Client) repairFirstRevisionPromotionFromMember(
	ctx context.Context, member *revisionDocument,
) error {
	if member == nil || member.marker.Revision == nil ||
		member.marker.Revision.Number != 2 ||
		member.marker.Revision.PreviousURL == "" || member.versions == nil {
		return nil
	}
	var first *VersionsRevision
	for index := range member.versions.Revisions {
		if member.versions.Revisions[index].Number == 1 {
			first = &member.versions.Revisions[index]
			break
		}
	}
	if first == nil {
		return errors.New("airplan: first revision predecessor cannot be reconciled")
	}
	if first.Deleted {
		return nil
	}
	if first.URL != member.marker.Revision.PreviousURL {
		return errors.New("airplan: first revision predecessor cannot be reconciled")
	}
	predecessor, err := c.loadRevisionDocumentForUpdate(ctx, first.URL)
	if err != nil {
		return fmt.Errorf("airplan: load first revision predecessor: %w", err)
	}
	if predecessor.marker.Revision != nil {
		if predecessor.marker.Revision.ChainID != member.versions.ChainID ||
			predecessor.marker.Revision.Number != 1 {
			return errors.New("airplan: first revision predecessor marker conflicts with chain")
		}
		if predecessor.needsPageRepair {
			predecessor, err = c.repairRevisionPage(ctx, predecessor)
			if err != nil {
				return fmt.Errorf("airplan: repair first revision predecessor page: %w", err)
			}
			if predecessor.needsPageRepair || predecessor.marker.Revision == nil ||
				predecessor.marker.Revision.ChainID != member.versions.ChainID ||
				predecessor.marker.Revision.Number != 1 {
				return errors.New("airplan: verify first revision predecessor page repair failed")
			}
		}
		return nil
	}
	if !predecessor.needsPromotion || predecessor.versions == nil ||
		predecessor.versions.ChainID != member.versions.ChainID {
		return errors.New("airplan: first revision predecessor promotion cannot be reconciled")
	}
	if err := c.promoteStandaloneRevision(
		ctx, predecessor, member.versions.ChainID, *member.versions,
	); err != nil {
		return err
	}
	verified, err := c.loadRevisionDocumentForUpdate(ctx, predecessor.pageURL)
	if err != nil || verified.marker.Revision == nil ||
		verified.marker.Revision.ChainID != member.versions.ChainID ||
		verified.marker.Revision.Number != 1 || verified.needsPageRepair {
		if err != nil {
			return fmt.Errorf("airplan: verify first revision predecessor repair: %w", err)
		}
		return errors.New("airplan: verify first revision predecessor repair failed")
	}
	return nil
}

func (c *Client) loadRevisionDocument(ctx context.Context, target string) (*revisionDocument, error) {
	key, err := KeyFromURLOrKey(c.cfg, target)
	if err != nil {
		return nil, err
	}
	dirPrefix, err := uploadDirPrefixForKeyPrefix(key, c.cfg.KeyPrefix)
	if err != nil {
		return nil, err
	}
	resolved, err := c.resolveMarker(ctx, dirPrefix)
	if err != nil {
		return nil, err
	}
	dir := path.Base(strings.TrimSuffix(dirPrefix, "/"))
	markerBody, markerETag, _, err := c.st.getBytesWithETag(ctx, resolved.Key, MaxMarkerSize)
	if err != nil {
		return nil, err
	}
	if markerETag == "" {
		return nil, errors.New("airplan: revision marker has no ETag; conditional update is unavailable")
	}
	marker, err := DecodeUploadMarkerForName(markerBody, dir, resolved.Basename)
	if err != nil {
		return nil, err
	}
	if marker.Version != MarkerVersion || marker.Kind != UploadKindDocument ||
		marker.Format != "md" || marker.Source == "" || marker.Render == nil {
		return nil, errors.New("airplan: update requires a complete v5 source-backed Markdown document; run upgrade first")
	}
	if err := validateManagedTarget("update", key, dirPrefix, marker); err != nil {
		return nil, err
	}
	pageKey := dirPrefix + marker.Page
	sourceKey := dirPrefix + marker.Source
	pageLimit := maxUpgradePageSize
	if marker.Version >= 6 {
		pageObject, declared := markerObjectNamed(marker, marker.Page)
		if !declared || pageObject.Role != MarkerRolePage ||
			pageObject.Bytes > DefaultMaxGeneratedPageSize {
			return nil, errors.New(
				"airplan: revision marker entry page size is invalid",
			)
		}
		pageLimit = pageObject.Bytes
	}
	pageBody, pageETag, _, err := c.st.getBytesWithETag(ctx, pageKey, pageLimit)
	if err != nil {
		return nil, err
	}
	sourceObject, ok := markerObjectForRole(marker, MarkerRoleSource)
	if !ok {
		return nil, errors.New("airplan: revision marker has no declared source object")
	}
	sourceBody, sourceETag, _, err := c.st.getBytesWithETag(
		ctx, sourceKey, sourceObject.Bytes,
	)
	if err != nil {
		return nil, err
	}
	if int64(len(sourceBody)) != sourceObject.Bytes {
		return nil, errors.New("airplan: revision source size does not match its marker")
	}
	if marker.Revision != nil && marker.Revision.Number > 1 {
		diffObject, declared := markerObjectForRole(marker, MarkerRoleDiff)
		if !declared {
			return nil, errors.New("airplan: revision marker has no declared diff object")
		}
		diffETag, diffBytes, diffErr := c.st.objectMetadata(
			ctx, dirPrefix+diffObject.Name,
		)
		if diffErr != nil {
			if !errors.Is(diffErr, errObjectNotFound) {
				return nil, diffErr
			}
			return nil, errors.New("airplan: revision diff is missing or incomplete")
		}
		if diffETag == "" || diffBytes != diffObject.Bytes {
			return nil, errors.New("airplan: revision diff is missing or incomplete")
		}
	}
	if pageETag == "" || sourceETag == "" {
		return nil, errors.New("airplan: revision payload has no ETag; conditional update is unavailable")
	}
	pageURL, _, err := PublicURL(c.cfg, pageKey)
	if err != nil {
		return nil, err
	}
	doc := &revisionDocument{
		marker: marker, markerBody: markerBody,
		markerETag: markerETag, pageBody: pageBody, pageETag: pageETag,
		sourceBody: sourceBody, sourceETag: sourceETag, dirPrefix: dirPrefix,
		pageKey: pageKey, sourceKey: sourceKey, pageURL: pageURL,
	}
	doc.needsPageRepair = marker.PageBytes != int64(len(pageBody)) ||
		marker.PageSHA256 != contentSHA256(pageBody)
	for _, descriptor := range marker.Pages {
		if descriptor.Page == marker.Page {
			continue
		}
		object, declared := markerObjectNamed(marker, descriptor.Page)
		if !declared || object.Role != MarkerRolePage {
			return nil, errors.New("airplan: revision marker page graph is incomplete")
		}
		state, inspectErr := c.inspectUpgradePayload(
			ctx, dirPrefix+descriptor.Page, object.Bytes,
		)
		if errors.Is(inspectErr, errObjectNotFound) {
			doc.needsPageRepair = true
			continue
		}
		if inspectErr != nil {
			return nil, inspectErr
		}
		if state.size != object.Bytes || state.digest != object.SHA256 {
			doc.needsPageRepair = true
		}
	}
	versionsBody, versionsETag, _, versionsErr := c.st.getBytesWithETag(
		ctx, dirPrefix+VersionsFilename, MaxVersionsMetadataSize,
	)
	if errors.Is(versionsErr, errObjectNotFound) {
		if marker.Revision != nil {
			doc.needsMetadata = true
			return doc, nil
		}
		return doc, nil
	}
	if versionsErr != nil {
		return nil, versionsErr
	}
	if versionsETag == "" {
		return nil, errors.New("airplan: revision metadata has no ETag; conditional update is unavailable")
	}
	versions, err := DecodeVersionsMetadata(versionsBody, c.cfg, pageKey)
	if err != nil {
		if marker.Revision == nil && bytes.Equal(
			versionsBody, standaloneDeleteReservationBody,
		) {
			return nil, fmt.Errorf("%w: %v", errRevisionTransitionReserved, err)
		}
		return nil, err
	}
	if marker.Revision == nil {
		if versions.CurrentRevision != 1 {
			return nil, errors.New("airplan: standalone marker conflicts with versions metadata")
		}
		doc.needsPromotion = true
	} else if marker.Revision.ChainID != versions.ChainID ||
		marker.Revision.Number != versions.CurrentRevision {
		return nil, errors.New("airplan: marker revision descriptor conflicts with versions metadata")
	}
	doc.versions, doc.versionsBody, doc.versionsETag = versions, versionsBody, versionsETag
	return doc, nil
}

func (c *Client) loadRevisionDocumentForUpdate(
	ctx context.Context, target string,
) (*revisionDocument, error) {
	doc, err := c.loadRevisionDocument(ctx, target)
	if err != nil {
		return nil, err
	}
	if doc.marker.Render == nil ||
		!themeRecipeMatches(doc.marker.Render.Themes, c.cfg.ThemeBundle) {
		return nil, &updateRefusalError{
			kind: updateRefusalInvalidTarget,
			err: errors.New(
				"airplan: revision themes do not match the configured catalog; " +
					"run airplan upgrade --force on the target before updating",
			),
		}
	}
	return doc, nil
}

func (c *Client) repairMissingRevisionMetadata(
	ctx context.Context, doc *revisionDocument, hint *VersionsMetadata,
) (*revisionDocument, error) {
	if !doc.needsMetadata || doc.marker.Revision == nil {
		return doc, nil
	}
	metadata := hint
	if metadata == nil && doc.marker.Revision.Number > 1 &&
		doc.marker.Revision.PreviousURL != "" {
		previous, err := c.loadRevisionDocument(ctx, doc.marker.Revision.PreviousURL)
		if err != nil {
			if !errors.Is(err, errOwnershipMarkerMissing) {
				return nil, fmt.Errorf("airplan: find revision metadata repair source: %w", err)
			}
			metadata, err = c.loadDeletedRevisionMetadataReceipt(
				ctx, doc.marker.Revision.PreviousURL,
				doc.marker.Revision.Number,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"airplan: find deleted predecessor metadata repair source: %w", err,
				)
			}
		} else {
			metadata = previous.versions
		}
	}
	if metadata == nil || metadata.ChainID != doc.marker.Revision.ChainID ||
		liveVersionsRevision(metadata, doc.marker.Revision.Number) == nil {
		return nil, errors.New("airplan: missing revision metadata cannot be reconciled")
	}
	copyMetadata := *metadata
	copyMetadata.CurrentRevision = doc.marker.Revision.Number
	body, err := EncodeVersionsMetadata(copyMetadata, c.cfg, doc.pageKey)
	if err != nil {
		return nil, err
	}
	if err := c.st.putIfAbsent(ctx, object{
		Key:  doc.dirPrefix + VersionsFilename,
		Body: body, ContentType: markerContentType,
	}); err != nil {
		return nil, fmt.Errorf("airplan: repair revision metadata: %w", err)
	}
	return c.loadRevisionDocumentForUpdate(ctx, doc.pageURL)
}

func (c *Client) loadDeletedRevisionMetadataReceipt(
	ctx context.Context, pageURL string, targetRevision int,
) (*VersionsMetadata, error) {
	pageKey, err := KeyFromURLOrKey(c.cfg, pageURL)
	if err != nil {
		return nil, err
	}
	dirPrefix, err := uploadDirPrefixForKeyPrefix(pageKey, c.cfg.KeyPrefix)
	if err != nil {
		return nil, err
	}
	body, err := c.st.getBytes(
		ctx, dirPrefix+VersionsFilename, MaxVersionsMetadataSize,
	)
	if err != nil {
		return nil, err
	}
	metadata, err := decodeVersionsMetadata(body, c.cfg, pageKey, true)
	if err != nil {
		return nil, err
	}
	current := &metadata.Revisions[metadata.CurrentRevision-1]
	if !current.Deleted {
		return nil, errors.New(
			"airplan: predecessor marker is missing but its metadata is not a delete receipt",
		)
	}
	for index := len(metadata.Revisions) - 1; index >= 0; index-- {
		entry := metadata.Revisions[index]
		if entry.Deleted || entry.Number == targetRevision {
			continue
		}
		witness, loadErr := c.loadRevisionDocument(ctx, entry.URL)
		if loadErr != nil || witness.versions == nil ||
			witness.versions.ChainID != metadata.ChainID ||
			liveVersionsRevision(witness.versions, targetRevision) == nil {
			continue
		}
		return witness.versions, nil
	}
	return nil, errors.New(
		"airplan: deleted predecessor receipt has no complete live metadata witness",
	)
}

func (c *Client) repairRevisionPage(
	ctx context.Context, doc *revisionDocument,
) (*revisionDocument, error) {
	plan, err := c.PlanUpgradeDocument(ctx, doc.pageURL, UpgradeDocumentOptions{})
	if err != nil {
		return nil, err
	}
	if plan.State != UpgradeStateUpgradeable {
		return nil, fmt.Errorf("airplan: revision page requires repair: %s", plan.Reason)
	}
	if _, err := c.UpgradeDocument(ctx, *plan); err != nil {
		return nil, fmt.Errorf("airplan: repair revision page: %w", err)
	}
	return c.loadRevisionDocumentForUpdate(ctx, doc.pageURL)
}

func cloneRenderRecipe(recipe *RenderRecipe) *RenderRecipe {
	if recipe == nil {
		return nil
	}
	copyRecipe := *recipe
	return &copyRecipe
}

func liveVersionsRevision(metadata *VersionsMetadata, number int) *VersionsRevision {
	if metadata == nil {
		return nil
	}
	for index := range metadata.Revisions {
		if metadata.Revisions[index].Number == number && !metadata.Revisions[index].Deleted {
			return &metadata.Revisions[index]
		}
	}
	return nil
}

func (c *Client) nextVersionsMetadata(
	latest *revisionDocument, chainID string, newRevision int,
	newURL, diffURL string, createdAt time.Time,
) VersionsMetadata {
	metadata := VersionsMetadata{
		Schema: "airplan-versions", Version: 1,
		ChainID: chainID, LatestRevision: newRevision,
		LastAssignedRevision: newRevision,
	}
	if latest.versions == nil {
		metadata.Revisions = []VersionsRevision{{
			Number: 1, URL: latest.pageURL,
			CreatedAt: latest.marker.CreatedAt.Truncate(time.Second),
		}}
	} else {
		metadata.Revisions = append([]VersionsRevision(nil), latest.versions.Revisions...)
	}
	metadata.Revisions = append(metadata.Revisions, VersionsRevision{
		Number: newRevision, URL: newURL, DiffURL: diffURL, CreatedAt: createdAt,
	})
	return metadata
}

func (c *Client) encodeMemberMetadata(metadata VersionsMetadata) (map[int][]byte, error) {
	bodies := make(map[int][]byte)
	for _, revision := range metadata.Revisions {
		if revision.Deleted {
			continue
		}
		copyMetadata := metadata
		copyMetadata.CurrentRevision = revision.Number
		key, err := KeyFromURLOrKey(c.cfg, revision.URL)
		if err != nil {
			return nil, err
		}
		body, err := EncodeVersionsMetadata(copyMetadata, c.cfg, key)
		if err != nil {
			return nil, err
		}
		bodies[revision.Number] = body
	}
	return bodies, nil
}

func (c *Client) promoteStandaloneRevision(
	ctx context.Context, doc *revisionDocument, chainID string,
	metadata VersionsMetadata,
) error {
	recipe := cloneRenderRecipe(doc.marker.Render)
	if recipe == nil {
		return errors.New("airplan: standalone revision has no render recipe")
	}
	spool, err := newPayloadSpool()
	if err != nil {
		return err
	}
	defer spool.cleanup()
	descriptors := upgradeMarkerPages(doc.marker)
	if len(descriptors) == 0 {
		return errors.New("airplan: standalone revision has no managed entry page")
	}
	inputs := make([]PageInput, len(descriptors))
	opened := make([]io.Closer, 0, len(inputs))
	defer func() {
		for _, closer := range opened {
			_ = closer.Close()
		}
	}()
	for index, descriptor := range descriptors {
		object, ok := markerObjectNamed(doc.marker, descriptor.Source)
		if !ok || object.Role != MarkerRoleSource {
			return fmt.Errorf(
				"airplan: standalone page %q has no declared source", descriptor.Path,
			)
		}
		payload, spoolErr := c.spoolRevisionObject(
			ctx, spool, doc.dirPrefix+descriptor.Source, "source", object.Bytes,
		)
		if spoolErr != nil {
			return spoolErr
		}
		if doc.marker.Version >= 6 && payload.digest != object.SHA256 {
			return fmt.Errorf(
				"airplan: standalone source %q checksum does not match marker",
				descriptor.Source,
			)
		}
		reader, openErr := payload.open()
		if openErr != nil {
			return openErr
		}
		opened = append(opened, reader)
		inputs[index] = PageInput{
			Reader: reader, Path: descriptor.Path, Format: descriptor.Format,
			Title: descriptor.Title, Lang: descriptor.Lang,
		}
	}
	assets := make([]AssetInput, 0)
	for _, object := range doc.marker.Objects {
		if object.Role == MarkerRoleAsset {
			assets = append(assets, AssetInput{
				Path: object.Name, Size: object.Bytes,
				ContentType: object.ContentType,
			})
		}
	}
	input := DocumentInput{
		Entry: inputs[0], Pages: inputs[1:], Assets: assets,
		Slug: doc.marker.Slug, Title: doc.marker.Title,
		MaxPageSize: -1, MaxTotalPageSize: -1,
		RepositoryURL: doc.marker.Repo,
	}
	bundle, err := renderDocumentSpooled(ctx, input, DocumentRenderOptions{
		RenderInputOptions: RenderInputOptions{
			Indexable: recipe.Indexable, IncludeSource: true,
			NoExternalAssets: recipe.NoExternalAssets,
			MermaidURL:       recipe.MermaidURL,
			Repository:       doc.marker.Repo,
			Themes:           c.cfg.ThemeBundle,
		},
		MaxGeneratedPageSize: -1,
		Revision:             1,
		RevisionCount:        metadata.LatestRevision,
		RevisionChainID:      chainID, VersionsPath: VersionsFilename,
	}, c.template, c.templateErr)
	if err != nil {
		return err
	}
	defer bundle.cleanup()
	marker := *doc.marker
	marker.Objects = append([]MarkerObject(nil), doc.marker.Objects...)
	marker.Revision = &RevisionDescriptor{ChainID: chainID, Number: 1}
	marker.Producer = Producer{Name: "airplan", Version: producerVersion(c.cfg.ProducerVersion)}
	pages := make(map[string]RenderedBundlePage, len(bundle.Pages))
	for _, page := range bundle.Pages {
		pages[page.PagePath] = page
	}
	for index := range marker.Objects {
		if marker.Objects[index].Role == MarkerRolePage {
			page, ok := pages[marker.Objects[index].Name]
			if !ok {
				return fmt.Errorf(
					"airplan: promoted bundle no longer renders page %q",
					marker.Objects[index].Name,
				)
			}
			marker.Objects[index].Bytes = page.pageSize()
			marker.Objects[index].SHA256 = page.pageDigest()
		}
	}
	body, err := EncodeUploadMarker(marker)
	if err != nil {
		return err
	}
	if err := c.st.putConditional(ctx, object{
		Key:  doc.dirPrefix + MarkerFilename,
		Body: body, ContentType: markerContentType,
	}, doc.markerETag); err != nil {
		return fmt.Errorf("airplan: revision committed but predecessor promotion failed: %w", err)
	}
	for index := 1; index < len(bundle.Pages); index++ {
		page := bundle.Pages[index]
		key := doc.dirPrefix + page.PagePath
		etag, _, metadataErr := c.st.objectMetadata(ctx, key)
		if metadataErr != nil {
			return fmt.Errorf(
				"airplan: inspect predecessor page %q: %w", page.PagePath, metadataErr,
			)
		}
		if err := c.putDocumentPayloadConditional(
			ctx, key, page.htmlFile, pageContentType,
			titleMetadata(page.Title), etag,
		); err != nil {
			return fmt.Errorf(
				"airplan: revision committed but predecessor page promotion failed: %w", err,
			)
		}
	}
	if err := c.putDocumentPayloadConditional(
		ctx, doc.pageKey, bundle.Pages[0].htmlFile, pageContentType,
		titleMetadata(marker.Title), doc.pageETag,
	); err != nil {
		return fmt.Errorf(
			"airplan: revision committed but predecessor page promotion failed: %w", err,
		)
	}
	return nil
}

func (c *Client) spoolRevisionObject(
	ctx context.Context, spool *payloadSpool, key, kind string, limit int64,
) (spooledPayload, error) {
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() {
		err := c.st.getTo(ctx, key, writer)
		_ = writer.CloseWithError(err)
		done <- err
	}()
	payload, err := spool.putReader(kind, reader, limit)
	_ = reader.Close()
	downloadErr := <-done
	if err != nil {
		return spooledPayload{}, err
	}
	if downloadErr != nil {
		return spooledPayload{}, downloadErr
	}
	if payload.size != limit {
		return spooledPayload{}, fmt.Errorf(
			"airplan: revision payload %q size does not match its marker", key,
		)
	}
	return payload, nil
}

func (c *Client) propagateVersionsMetadata(
	ctx context.Context, metadata VersionsMetadata, bodies map[int][]byte,
	serializedKey, newKey string,
) error {
	for _, revision := range metadata.Revisions {
		if revision.Deleted {
			continue
		}
		key, err := KeyFromURLOrKey(c.cfg, revision.URL)
		if err != nil {
			return err
		}
		dirPrefix, err := uploadDirPrefixForKeyPrefix(key, c.cfg.KeyPrefix)
		if err != nil {
			return err
		}
		metadataKey := dirPrefix + VersionsFilename
		if metadataKey == serializedKey || metadataKey == newKey {
			continue
		}
		current, etag, _, err := c.st.getBytesWithETag(ctx, metadataKey, MaxVersionsMetadataSize)
		if err != nil {
			if errors.Is(err, errObjectNotFound) {
				authoritativeKey := serializedKey
				if newKey != "" {
					authoritativeKey = newKey
				}
				if err := c.verifySerializationBody(
					ctx, metadata, bodies, authoritativeKey,
				); err != nil {
					return err
				}
				resolved, markerErr := c.resolveMarker(ctx, dirPrefix)
				if markerErr != nil {
					return errors.New("airplan: revision metadata recreation refused because the member marker is unavailable")
				}
				marker, markerErr := DecodeUploadMarkerForName(resolved.Body,
					path.Base(strings.TrimSuffix(dirPrefix, "/")), resolved.Basename)
				if markerErr != nil || marker.Revision == nil ||
					marker.Revision.ChainID != metadata.ChainID ||
					marker.Revision.Number != revision.Number {
					return errors.New("airplan: revision metadata recreation refused because the member marker changed")
				}
				if err := c.st.putIfAbsent(ctx, object{
					Key: metadataKey, Body: bodies[revision.Number],
					ContentType: markerContentType,
				}); err != nil {
					return fmt.Errorf("airplan: revision metadata recreation failed: %w", err)
				}
				continue
			}
			return err
		}
		if bytes.Equal(current, bodies[revision.Number]) {
			continue
		}
		observed, decodeErr := DecodeVersionsMetadata(current, c.cfg, key)
		if decodeErr != nil || !versionsMetadataMonotonicallyPrecedes(observed, &metadata) {
			return errors.New("airplan: revision chain changed during metadata propagation")
		}
		if err := c.st.putConditional(ctx, object{
			Key:  metadataKey,
			Body: bodies[revision.Number], ContentType: markerContentType,
		}, etag); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) verifySerializationBody(
	ctx context.Context, metadata VersionsMetadata, bodies map[int][]byte,
	serializedKey string,
) error {
	for _, revision := range metadata.Revisions {
		if revision.Deleted {
			continue
		}
		key, err := KeyFromURLOrKey(c.cfg, revision.URL)
		if err != nil {
			return err
		}
		dirPrefix, err := uploadDirPrefixForKeyPrefix(key, c.cfg.KeyPrefix)
		if err != nil {
			return err
		}
		if dirPrefix+VersionsFilename != serializedKey {
			continue
		}
		expected, ok := bodies[revision.Number]
		if !ok {
			break
		}
		return c.verifyObjectBody(ctx, serializedKey, expected)
	}
	return errors.New("airplan: revision serialization point is not a live chain member")
}

func (c *Client) verifyObjectBody(
	ctx context.Context, key string, expected []byte,
) error {
	actual, err := c.st.getBytes(ctx, key, MaxVersionsMetadataSize)
	if err != nil {
		return errors.New("airplan: revision serialization point changed during metadata repair")
	}
	if bytes.Equal(actual, expected) {
		return nil
	}
	actualMetadata, actualErr := decodeVersionsMetadata(actual, c.cfg, "", true)
	expectedMetadata, expectedErr := decodeVersionsMetadata(expected, c.cfg, "", true)
	if actualErr != nil || expectedErr != nil ||
		!sameVersionsMetadata(actualMetadata, expectedMetadata) {
		return errors.New("airplan: revision serialization point changed during metadata repair")
	}
	return nil
}

func (c *Client) verifyRevisionChain(
	ctx context.Context, metadata VersionsMetadata, bodies map[int][]byte,
) error {
	for _, revision := range metadata.Revisions {
		if revision.Deleted {
			continue
		}
		key, err := KeyFromURLOrKey(c.cfg, revision.URL)
		if err != nil {
			return err
		}
		dirPrefix, err := uploadDirPrefixForKeyPrefix(key, c.cfg.KeyPrefix)
		if err != nil {
			return err
		}
		actual, err := c.st.getBytes(ctx, dirPrefix+VersionsFilename, MaxVersionsMetadataSize)
		if err != nil {
			return errors.New("airplan: revision metadata verification failed")
		}
		expectedBody := bodies[revision.Number]
		if !c.revisionMetadataAtLeast(actual, expectedBody, key) {
			return errors.New("airplan: revision metadata verification failed")
		}
		resolved, err := c.resolveMarker(ctx, dirPrefix)
		if err != nil {
			return err
		}
		marker, err := DecodeUploadMarkerForName(resolved.Body,
			path.Base(strings.TrimSuffix(dirPrefix, "/")), resolved.Basename)
		if err != nil {
			return err
		}
		if marker.Revision == nil || marker.Revision.ChainID != metadata.ChainID ||
			marker.Revision.Number != revision.Number {
			return errors.New("airplan: revision marker verification failed")
		}
	}
	return nil
}

func (c *Client) revisionMetadataAtLeast(actual, expected []byte, containingKey string) bool {
	if bytes.Equal(actual, expected) {
		return true
	}
	observed, observedErr := DecodeVersionsMetadata(actual, c.cfg, containingKey)
	want, expectedErr := DecodeVersionsMetadata(expected, c.cfg, containingKey)
	return observedErr == nil && expectedErr == nil &&
		(sameVersionsMetadata(observed, want) ||
			versionsMetadataMonotonicallyPrecedes(want, observed))
}

func (c *Client) reconcileRevisionMetadata(ctx context.Context, latest *revisionDocument) error {
	if latest.versions == nil {
		return nil
	}
	bodies, err := c.encodeMemberMetadata(*latest.versions)
	if err != nil {
		return err
	}
	if err := c.propagateVersionsMetadata(ctx, *latest.versions, bodies,
		latest.dirPrefix+VersionsFilename, ""); err != nil {
		return err
	}
	return c.verifyRevisionChain(ctx, *latest.versions, bodies)
}

func (c *Client) updateResultFromDocument(doc *revisionDocument) *UpdateDocumentResult {
	result := &UpdateDocumentResult{Result: Result{
		ID:  doc.marker.Directory,
		URL: doc.pageURL, Key: doc.pageKey, SourceKey: doc.sourceKey,
		Bucket: c.cfg.Bucket, Bytes: int64(len(doc.pageBody)),
		ContentType: pageContentType, Title: doc.marker.Title,
		CreatedAt: doc.marker.CreatedAt, MarkerVersion: doc.marker.Version,
		MarkerKey: doc.dirPrefix + MarkerFilename, Format: doc.marker.Format,
		Kind: string(doc.marker.Kind), Slug: doc.marker.Slug,
		RepositoryURL: doc.marker.Repo,
	}}
	result.SourceURL, _, _ = PublicURL(c.cfg, doc.sourceKey)
	if doc.versions != nil {
		result.RevisionChainID = doc.versions.ChainID
		result.Revision = doc.versions.CurrentRevision
		result.LatestRevision = doc.versions.LatestRevision
		entry := liveVersionsRevision(doc.versions, result.Revision)
		if entry != nil {
			result.DiffURL = entry.DiffURL
		}
		if doc.marker.Revision != nil {
			result.PreviousURL = doc.marker.Revision.PreviousURL
		}
	}
	return result
}

func (c *Client) recordRevisionAppend(
	ctx context.Context, result *UpdateDocumentResult, markerBody []byte, chainID string,
	metadata VersionsMetadata,
) {
	declared := declaredTotals{}
	if marker, err := DecodeUploadMarker(markerBody, result.ID); err == nil {
		declared = markerDeclaredTotals(*marker, markerBody)
	}
	c.recordUpload(ctx, &result.Result, declared)
	if c.cfg.DisableManifest {
		return
	}
	manifestPath := c.cfg.ManifestPath
	if manifestPath == "" {
		var err error
		manifestPath, err = DefaultManifestPath()
		if err != nil {
			return
		}
	}
	for _, revision := range metadata.Revisions {
		if revision.Deleted || revision.Number == result.Revision {
			continue
		}
		doc, err := c.loadRevisionDocument(ctx, revision.URL)
		if err != nil {
			result.Warnings = append(result.Warnings, "manifest chain links not fully recorded")
			return
		}
		projection := c.updateResultFromDocument(doc).Result
		projection.RevisionChainID = chainID
		projection.Revision = revision.Number
		projection.LatestRevision = result.LatestRevision
		declared := markerDeclaredTotals(*doc.marker, doc.markerBody)
		rec := completeManifestProjection(c.cfg, "link",
			time.Now().UTC().Truncate(time.Second), &projection, declared,
			doc.marker.Producer.Version, doc.marker.Render.Generation)
		if err := appendManifestRecord(ctx, manifestPath, rec); err != nil {
			result.Warnings = append(result.Warnings, "manifest link not recorded: "+err.Error())
			return
		}
	}
}
