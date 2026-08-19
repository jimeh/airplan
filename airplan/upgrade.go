package airplan

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"strconv"
	"strings"
	"time"
)

// UpgradeState classifies a source-backed document upgrade plan.
type UpgradeState string

const (
	UpgradeStateUpgradeable UpgradeState = "upgradeable"
	UpgradeStateCurrent     UpgradeState = "current"
	UpgradeStateIneligible  UpgradeState = "ineligible"
	UpgradeStateInvalid     UpgradeState = "invalid"
	UpgradeStateMissing     UpgradeState = "missing"
)

const maxUpgradePageSize int64 = DefaultMaxInputSize * 4

type upgradeRefusalError struct {
	state  UpgradeState
	reason string
}

func (e *upgradeRefusalError) Error() string {
	return fmt.Sprintf("airplan: document is %s: %s", e.state, e.reason)
}

func refuseUpgrade(plan UpgradeDocumentPlan) error {
	return &upgradeRefusalError{state: plan.State, reason: plan.Reason}
}

// UpgradeDocumentOptions controls one document upgrade plan.
type UpgradeDocumentOptions struct {
	Force bool `json:"force,omitempty"`
}

// UpgradeDocumentPlan binds a proposed rewrite to the exact remote objects
// that were inspected. It is safe to serialize through REST and MCP.
type UpgradeDocumentPlan struct {
	Target                    string            `json:"target"`
	Profile                   string            `json:"profile,omitempty"`
	Bucket                    string            `json:"bucket,omitempty"`
	State                     UpgradeState      `json:"state"`
	Reason                    string            `json:"reason,omitempty"`
	URL                       string            `json:"url,omitempty"`
	MarkerKey                 string            `json:"marker_key,omitempty"`
	PageKey                   string            `json:"page_key,omitempty"`
	SourceKey                 string            `json:"source_key,omitempty"`
	CurrentMarkerVersion      int               `json:"current_marker_version,omitempty"`
	CurrentProducerVersion    string            `json:"current_producer_version,omitempty"`
	CurrentRendererGeneration int               `json:"current_renderer_generation,omitempty"`
	TargetMarkerVersion       int               `json:"target_marker_version"`
	TargetProducerVersion     string            `json:"target_producer_version"`
	TargetRendererGeneration  int               `json:"target_renderer_generation"`
	MarkerETag                string            `json:"marker_etag,omitempty"`
	PageETag                  string            `json:"page_etag,omitempty"`
	SourceETag                string            `json:"source_etag,omitempty"`
	PageETags                 map[string]string `json:"page_etags,omitempty"`
	SourceETags               map[string]string `json:"source_etags,omitempty"`
	Force                     bool              `json:"force,omitempty"`

	markerBody    []byte
	marker        *UploadMarker
	newMarker     []byte
	protection    []byte
	versions      []byte
	diff          []byte
	pageSizes     map[string]int64
	pageDigests   map[string]string
	sourceSizes   map[string]int64
	sourceDigests map[string]string
	newBundle     *RenderedDocumentBundle
	protected     bool
	versioned     bool
}

// UpgradeDocumentResult reports a conditionally applied or no-op upgrade.
type UpgradeDocumentResult struct {
	Result   Result       `json:"result"`
	State    UpgradeState `json:"state"`
	Upgraded bool         `json:"upgraded"`
	Reason   string       `json:"reason,omitempty"`
}

// PlanUpgradeDocument performs the remote reads required to classify one
// target without rendering or writing any object.
func (c *Client) PlanUpgradeDocument(
	ctx context.Context, target string, opts UpgradeDocumentOptions,
) (*UpgradeDocumentPlan, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.PlanUpgradeDocument(ctx, target, opts)
	}
	if err := c.ensureStorage(ctx); err != nil {
		return nil, err
	}
	plan := &UpgradeDocumentPlan{
		Target: target, Profile: c.cfg.Profile, Bucket: c.cfg.Bucket,
		Force:                    opts.Force,
		TargetMarkerVersion:      MarkerVersion,
		TargetProducerVersion:    producerVersion(c.cfg.ProducerVersion),
		TargetRendererGeneration: RendererGeneration,
	}
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
		if errors.Is(err, errOwnershipMarkerMissing) {
			plan.State, plan.Reason = UpgradeStateMissing, "ownership marker is missing"
			return plan, nil
		}
		if _, ok := MarkerCode(err); ok {
			plan.State, plan.Reason = UpgradeStateInvalid, "ownership marker is invalid"
			return plan, nil
		}
		return nil, err
	}
	markerBody, markerETag, _, err := c.st.getBytesWithETag(
		ctx, resolved.Key, MaxMarkerSize,
	)
	if err != nil {
		return nil, err
	}
	if markerETag == "" {
		return nil, errors.New("airplan: ownership marker has no ETag; conditional upgrade is unavailable")
	}
	marker, err := DecodeUploadMarkerForName(
		markerBody, strings.TrimSuffix(path.Base(dirPrefix), "/"), resolved.Basename,
	)
	if err != nil {
		if _, ok := MarkerCode(err); ok {
			plan.State, plan.Reason = UpgradeStateInvalid, "ownership marker is invalid"
			return plan, nil
		}
		return nil, err
	}
	plan.MarkerKey = resolved.Key
	plan.MarkerETag = markerETag
	plan.CurrentMarkerVersion = marker.Version
	plan.CurrentProducerVersion = marker.Producer.Version
	if marker.Render != nil {
		plan.CurrentRendererGeneration = marker.Render.Generation
	}
	if marker.Kind != UploadKindDocument || marker.Format != "md" || marker.Source == "" {
		plan.State, plan.Reason = UpgradeStateIneligible,
			"only source-backed Markdown documents can be upgraded"
		return plan, nil
	}
	if marker.Render != nil && marker.Render.Generation > RendererGeneration {
		plan.State, plan.Reason = UpgradeStateIneligible,
			"document was produced by a newer renderer"
		return plan, nil
	}
	if marker.Version >= 4 && marker.Render != nil {
		switch marker.Render.Template.Kind {
		case "custom":
			if (c.templateDigest == "" ||
				c.templateDigest != marker.Render.Template.SHA256) && !opts.Force {
				plan.State, plan.Reason = UpgradeStateIneligible,
					"matching custom template is not configured"
				return plan, nil
			}
		case "builtin":
			if c.cfg.Template != "" && !opts.Force {
				plan.State, plan.Reason = UpgradeStateIneligible,
					"configured custom template does not match stored built-in recipe"
				return plan, nil
			}
		}
	}
	if marker.Version >= 5 && marker.Render != nil &&
		!themeRecipeMatches(marker.Render.Themes, c.cfg.ThemeBundle) && !opts.Force {
		plan.State, plan.Reason = UpgradeStateIneligible,
			"configured themes do not match the stored render recipe; rerun with upgrade --force to replace it"
		return plan, nil
	}
	pageKey := dirPrefix + marker.Page
	sourceKey := dirPrefix + marker.Source
	if err := validateUpgradeDeclaredPayloadSizes(marker); err != nil {
		return nil, err
	}
	descriptors := upgradeMarkerPages(marker)
	plan.PageETags = make(map[string]string, len(descriptors))
	plan.SourceETags = make(map[string]string, len(descriptors))
	plan.pageSizes = make(map[string]int64, len(descriptors))
	plan.pageDigests = make(map[string]string, len(descriptors))
	plan.sourceSizes = make(map[string]int64, len(descriptors))
	plan.sourceDigests = make(map[string]string, len(descriptors))
	bundleContentCurrent := true
	var generatedTotal, sourceTotal int64
	for _, descriptor := range descriptors {
		pageObject, pageDeclared := markerObjectNamed(marker, descriptor.Page)
		if marker.Version >= 6 && (!pageDeclared || pageObject.Role != MarkerRolePage) {
			plan.State, plan.Reason = UpgradeStateInvalid, "managed page graph is incomplete"
			return plan, nil
		}
		pageLimit := maxUpgradePageSize
		if marker.Version >= 6 {
			pageLimit = DefaultMaxGeneratedPageSize - generatedTotal
		}
		if pageLimit <= 0 {
			return nil, errors.New("airplan: rendered page exceeds the maximum upgrade size")
		}
		pageState, readErr := c.inspectUpgradePayload(
			ctx, dirPrefix+descriptor.Page, pageLimit,
		)
		if readErr != nil {
			return nil, readErr
		}
		if pageState.size > pageLimit {
			return nil, errors.New("airplan: rendered page exceeds the maximum upgrade size")
		}
		if pageState.etag == "" {
			return nil, errors.New("airplan: rendered page has no ETag; conditional upgrade is unavailable")
		}
		generatedTotal += pageState.size
		plan.PageETags[descriptor.Page] = pageState.etag
		plan.pageSizes[descriptor.Page] = pageState.size
		plan.pageDigests[descriptor.Page] = pageState.digest
		if marker.Version >= 6 &&
			(pageState.size != pageObject.Bytes || pageState.digest != pageObject.SHA256) {
			bundleContentCurrent = false
		}

		if descriptor.Source == "" {
			plan.State, plan.Reason = UpgradeStateIneligible,
				"every managed page must retain its source to be upgraded"
			return plan, nil
		}
		if _, loaded := plan.SourceETags[descriptor.Source]; loaded {
			continue
		}
		sourceObject, sourceDeclared := markerObjectNamed(marker, descriptor.Source)
		if marker.Version >= 6 && (!sourceDeclared || sourceObject.Role != MarkerRoleSource) {
			plan.State, plan.Reason = UpgradeStateInvalid, "managed page graph is incomplete"
			return plan, nil
		}
		sourceLimit := upgradeSourceReadLimit(marker)
		if marker.Version >= 6 {
			sourceLimit = min(DefaultMaxInputSize, DefaultMaxTotalPageSize-sourceTotal)
		}
		if sourceLimit <= 0 {
			return nil, errors.New("airplan: source exceeds the maximum input size")
		}
		sourceState, readErr := c.inspectUpgradePayload(
			ctx, dirPrefix+descriptor.Source, sourceLimit,
		)
		if readErr != nil {
			return nil, readErr
		}
		if sourceState.size > sourceLimit {
			return nil, errors.New("airplan: source exceeds the maximum input size")
		}
		if sourceState.etag == "" {
			return nil, errors.New("airplan: managed source has no ETag; conditional upgrade is unavailable")
		}
		sourceTotal += sourceState.size
		plan.SourceETags[descriptor.Source] = sourceState.etag
		plan.sourceSizes[descriptor.Source] = sourceState.size
		plan.sourceDigests[descriptor.Source] = sourceState.digest
		if sourceDeclared && sourceObject.Bytes > 0 && sourceState.size != sourceObject.Bytes {
			if marker.Version >= 6 {
				plan.State, plan.Reason = UpgradeStateInvalid,
					"managed source content does not match its marker"
				return plan, nil
			}
			return nil, errors.New("airplan: Markdown source size does not match its marker")
		}
		if marker.Version >= 6 && sourceState.digest != sourceObject.SHA256 {
			plan.State, plan.Reason = UpgradeStateInvalid,
				"managed source content does not match its marker"
			return plan, nil
		}
	}
	pageETag := plan.PageETags[marker.Page]
	sourceETag := plan.SourceETags[marker.Source]
	protection, protected, err := c.optionalUpgradeControlObject(
		ctx, dirPrefix+ProtectedFilename, MaxMarkerSize,
	)
	if err != nil {
		return nil, err
	}
	versions, versioned, err := c.optionalUpgradeControlObject(
		ctx, dirPrefix+VersionsFilename, MaxVersionsMetadataSize,
	)
	if err != nil {
		return nil, err
	}
	plan.PageKey, plan.SourceKey = pageKey, sourceKey
	plan.PageETag = pageETag
	plan.SourceETag = sourceETag
	plan.URL, _, err = PublicURL(c.cfg, pageKey)
	if err != nil {
		return nil, err
	}
	plan.markerBody = markerBody
	plan.protection, plan.versions = protection, versions
	plan.protected, plan.versioned = protected, versioned
	plan.marker = marker
	if marker.Revision != nil {
		if !versioned {
			plan.State, plan.Reason = UpgradeStateInvalid, "linked document is missing versions metadata"
			return plan, nil
		}
		metadata, decodeErr := DecodeVersionsMetadata(versions, c.cfg, pageKey)
		if decodeErr != nil || metadata.ChainID != marker.Revision.ChainID ||
			metadata.CurrentRevision != marker.Revision.Number {
			plan.State, plan.Reason = UpgradeStateInvalid, "revision metadata is invalid"
			return plan, nil
		}
		if marker.Revision.Number > 1 {
			diffObject, declared := markerObjectForRole(marker, MarkerRoleDiff)
			if !declared {
				plan.State, plan.Reason = UpgradeStateInvalid, "revision diff is missing or invalid"
				return plan, nil
			}
			diffKey := dirPrefix + diffObject.Name
			plan.diff, err = c.st.getBytes(ctx, diffKey, diffObject.Bytes)
			if err != nil {
				if !errors.Is(err, errObjectNotFound) {
					return nil, err
				}
				plan.State, plan.Reason = UpgradeStateInvalid, "revision diff is missing or invalid"
				return plan, nil
			}
			if int64(len(plan.diff)) != diffObject.Bytes {
				plan.State, plan.Reason = UpgradeStateInvalid, "revision diff is missing or invalid"
				return plan, nil
			}
		}
	}
	pageContentCurrent := marker.PageBytes == plan.pageSizes[marker.Page] &&
		(marker.Version < 4 ||
			marker.PageSHA256 == plan.pageDigests[marker.Page])
	switch {
	case marker.Version < MarkerVersion:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "ownership marker schema is older"
	case marker.Render == nil || marker.Render.Generation < RendererGeneration:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "renderer generation is older"
	case !pageContentCurrent:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "rendered page requires repair"
	case !bundleContentCurrent:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "rendered bundle requires repair"
	case opts.Force:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "forced re-render"
	default:
		comparison, comparable := compareProducerVersions(
			marker.Producer.Version, plan.TargetProducerVersion,
		)
		switch {
		case comparable && comparison < 0:
			plan.State, plan.Reason = UpgradeStateUpgradeable, "producer release is older"
		case comparable && comparison > 0:
			plan.State, plan.Reason = UpgradeStateIneligible,
				"document was produced by a newer airplan release"
		default:
			plan.State, plan.Reason = UpgradeStateCurrent, "already current"
		}
	}
	return plan, nil
}

// UpgradeDocument applies an exact body-free plan. Execution safely replans,
// spools live payloads, and binds them to the submitted object identities.
func (c *Client) UpgradeDocument(
	ctx context.Context, plan UpgradeDocumentPlan,
) (*UpgradeDocumentResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	if c.remote != nil {
		return c.remote.UpgradeDocument(ctx, plan)
	}
	if plan.State != UpgradeStateCurrent && plan.State != UpgradeStateUpgradeable {
		return nil, refuseUpgrade(plan)
	}
	fresh, err := c.PlanUpgradeDocument(ctx, plan.Target,
		UpgradeDocumentOptions{Force: plan.Force})
	if err != nil {
		return nil, err
	}
	if fresh.State == UpgradeStateCurrent {
		return &UpgradeDocumentResult{
			Result: resultFromUpgradePlan(c.cfg, *fresh),
			State:  fresh.State, Reason: fresh.Reason,
		}, nil
	}
	if fresh.State != UpgradeStateUpgradeable {
		return nil, refuseUpgrade(*fresh)
	}
	if plan.MarkerETag == "" || plan.PageETag == "" || plan.SourceETag == "" {
		return nil, errors.New("airplan: upgrade plan is missing required object ETags")
	}
	if fresh.MarkerKey != plan.MarkerKey || fresh.PageKey != plan.PageKey ||
		fresh.MarkerETag != plan.MarkerETag || fresh.PageETag != plan.PageETag ||
		fresh.SourceETag != plan.SourceETag ||
		!equalStringMap(fresh.PageETags, plan.PageETags) ||
		!equalStringMap(fresh.SourceETags, plan.SourceETags) {
		return nil, ErrConflict
	}
	plan = *fresh
	payloads, err := c.spoolUpgradePayloads(ctx, &plan)
	if err != nil {
		return nil, err
	}
	defer payloads.cleanup()
	if err := c.materializeUpgradeSpooled(ctx, &plan, payloads); err != nil {
		return nil, err
	}
	defer plan.newBundle.cleanup()
	if err := c.verifyUpgradeSourceIdentity(ctx, &plan, payloads); err != nil {
		return nil, err
	}
	if !bytes.Equal(plan.markerBody, plan.newMarker) {
		if err := c.st.putConditional(ctx, object{
			Key: plan.MarkerKey, Body: plan.newMarker,
			ContentType: markerContentType,
		}, plan.MarkerETag); err != nil {
			return nil, err
		}
	}
	dirPrefix := strings.TrimSuffix(plan.MarkerKey, MarkerFilename)
	for _, descriptor := range plan.marker.Pages {
		if descriptor.Page == plan.marker.Entrypoint {
			continue
		}
		desired, ok := renderedPageNamed(plan.newBundle, descriptor.Page)
		if !ok {
			return nil, fmt.Errorf("airplan: upgraded page %q was not rendered", descriptor.Path)
		}
		if payloads.pages[descriptor.Page].digest != desired.pageDigest() {
			if err := c.putDocumentPayloadConditional(
				ctx, dirPrefix+descriptor.Page, desired.htmlFile,
				pageContentType, titleMetadata(descriptor.Title),
				plan.PageETags[descriptor.Page],
			); err != nil {
				return nil, err
			}
		}
	}
	for _, descriptor := range plan.marker.Pages {
		if descriptor.Page == plan.marker.Entrypoint {
			continue
		}
		desired, _ := renderedPageNamed(plan.newBundle, descriptor.Page)
		verified, readErr := c.inspectUpgradePayload(
			ctx, dirPrefix+descriptor.Page, desired.pageSize(),
		)
		if readErr != nil || verified.size != desired.pageSize() ||
			verified.digest != desired.pageDigest() {
			if readErr != nil {
				return nil, fmt.Errorf("airplan: upgraded page %q verification failed: %w", descriptor.Path, readErr)
			}
			return nil, fmt.Errorf("airplan: upgraded page %q verification failed: content changed", descriptor.Path)
		}
	}
	entry, ok := renderedPageNamed(plan.newBundle, plan.marker.Page)
	if !ok {
		return nil, errors.New("airplan: upgraded entry page was not rendered")
	}
	if payloads.pages[plan.marker.Page].digest != entry.pageDigest() {
		if err := c.putDocumentPayloadConditional(
			ctx, plan.PageKey, entry.htmlFile, pageContentType,
			titleMetadata(plan.marker.Title), plan.PageETags[plan.marker.Page],
		); err != nil {
			return nil, err
		}
	}
	verifiedMarker, _, _, err := c.st.getBytesWithETag(ctx, plan.MarkerKey, MaxMarkerSize)
	if err != nil {
		return nil, fmt.Errorf("airplan: upgraded marker verification failed: %w", err)
	}
	if !bytes.Equal(verifiedMarker, plan.newMarker) {
		return nil, errors.New("airplan: upgraded marker verification failed: content changed")
	}
	verifiedEntry, err := c.inspectUpgradePayload(
		ctx, plan.PageKey, entry.pageSize(),
	)
	if err != nil {
		return nil, fmt.Errorf("airplan: upgraded entry page verification failed: %w", err)
	}
	if verifiedEntry.size != entry.pageSize() || verifiedEntry.digest != entry.pageDigest() {
		return nil, errors.New("airplan: upgraded entry page verification failed: content changed")
	}
	for index := range plan.newBundle.Pages {
		page := &plan.newBundle.Pages[index]
		sourceName := page.SourcePath
		if sourceName == "" {
			continue
		}
		verifiedSource, readErr := c.inspectUpgradePayload(
			ctx, dirPrefix+sourceName, page.sourceSize(),
		)
		if readErr != nil {
			return nil, fmt.Errorf("airplan: upgraded source %q verification failed: %w", sourceName, readErr)
		}
		if verifiedSource.size != page.sourceSize() ||
			verifiedSource.digest != page.sourceDigest() {
			return nil, fmt.Errorf("airplan: upgraded source %q verification failed: content changed", sourceName)
		}
	}
	if err := c.verifyUpgradeControlObject(
		ctx, dirPrefix+ProtectedFilename, plan.protection, plan.protected, MaxMarkerSize,
	); err != nil {
		return nil, err
	}
	if err := c.verifyUpgradeControlObject(
		ctx, dirPrefix+VersionsFilename, plan.versions, plan.versioned, MaxVersionsMetadataSize,
	); err != nil {
		return nil, err
	}
	result := resultFromUpgradePlan(c.cfg, plan)
	c.recordUpgrade(ctx, &result, plan.newMarker)
	return &UpgradeDocumentResult{
		Result: result, State: UpgradeStateCurrent, Upgraded: true,
		Reason: plan.Reason,
	}, nil
}

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

type upgradePayloadState struct {
	size   int64
	digest string
	etag   string
}

type upgradePayloads struct {
	spool   *payloadSpool
	pages   map[string]spooledPayload
	sources map[string]spooledPayload
}

func (p *upgradePayloads) cleanup() {
	if p != nil && p.spool != nil {
		p.spool.cleanup()
	}
}

func (c *Client) spoolUpgradePayloads(
	ctx context.Context, plan *UpgradeDocumentPlan,
) (*upgradePayloads, error) {
	spool, err := newPayloadSpool()
	if err != nil {
		return nil, err
	}
	payloads := &upgradePayloads{
		spool: spool, pages: make(map[string]spooledPayload),
		sources: make(map[string]spooledPayload),
	}
	fail := func(err error) (*upgradePayloads, error) {
		payloads.cleanup()
		return nil, err
	}
	dirPrefix := strings.TrimSuffix(plan.MarkerKey, MarkerFilename)
	for _, descriptor := range upgradeMarkerPages(plan.marker) {
		if _, loaded := payloads.pages[descriptor.Page]; !loaded {
			payload, etag, readErr := c.spoolUpgradePayload(
				ctx, payloads.spool, "page", dirPrefix+descriptor.Page,
				plan.pageSizes[descriptor.Page], plan.PageETags[descriptor.Page],
			)
			if readErr != nil {
				return fail(readErr)
			}
			if etag != plan.PageETags[descriptor.Page] ||
				payload.size != plan.pageSizes[descriptor.Page] ||
				payload.digest != plan.pageDigests[descriptor.Page] {
				return fail(ErrConflict)
			}
			payloads.pages[descriptor.Page] = payload
		}
		if _, loaded := payloads.sources[descriptor.Source]; loaded {
			continue
		}
		payload, etag, readErr := c.spoolUpgradePayload(
			ctx, payloads.spool, "source", dirPrefix+descriptor.Source,
			plan.sourceSizes[descriptor.Source], plan.SourceETags[descriptor.Source],
		)
		if readErr != nil {
			return fail(readErr)
		}
		if etag != plan.SourceETags[descriptor.Source] ||
			payload.size != plan.sourceSizes[descriptor.Source] ||
			payload.digest != plan.sourceDigests[descriptor.Source] {
			return fail(ErrConflict)
		}
		payloads.sources[descriptor.Source] = payload
	}
	return payloads, nil
}

func (c *Client) spoolUpgradePayload(
	ctx context.Context, spool *payloadSpool, kind, key string, limit int64,
	expectedETag string,
) (spooledPayload, string, error) {
	body, etag, _, err := c.st.openWithETag(ctx, key)
	if err != nil {
		return spooledPayload{}, "", err
	}
	if etag != expectedETag {
		_ = body.Close()
		return spooledPayload{}, "", ErrConflict
	}
	payload, spoolErr := spool.putReader(kind, body, limit)
	closeErr := body.Close()
	if spoolErr != nil {
		return spooledPayload{}, "", spoolErr
	}
	if closeErr != nil {
		return spooledPayload{}, "", fmt.Errorf("airplan: close object %q: %w", key, closeErr)
	}
	return payload, etag, nil
}

func (c *Client) verifyUpgradeSourceIdentity(
	ctx context.Context, plan *UpgradeDocumentPlan, payloads *upgradePayloads,
) error {
	dirPrefix := strings.TrimSuffix(plan.MarkerKey, MarkerFilename)
	for name, payload := range payloads.sources {
		state, err := c.inspectUpgradePayload(ctx, dirPrefix+name, payload.size)
		if err != nil {
			return err
		}
		if state.etag != plan.SourceETags[name] || state.size != payload.size ||
			state.digest != payload.digest {
			return ErrConflict
		}
	}
	return nil
}

func (c *Client) inspectUpgradePayload(
	ctx context.Context, key string, limit int64,
) (upgradePayloadState, error) {
	body, etag, _, err := c.st.openWithETag(ctx, key)
	if err != nil {
		return upgradePayloadState{}, err
	}
	defer func() { _ = body.Close() }()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(body, limit+1))
	if err != nil {
		return upgradePayloadState{}, fmt.Errorf("airplan: read object %q: %w", key, err)
	}
	return upgradePayloadState{
		size: size, digest: hex.EncodeToString(hash.Sum(nil)), etag: etag,
	}, nil
}

func upgradeMarkerPages(marker *UploadMarker) []MarkerPage {
	if marker == nil {
		return nil
	}
	if marker.Version >= 6 {
		return append([]MarkerPage(nil), marker.Pages...)
	}
	return []MarkerPage{{
		Path: marker.Source, Page: marker.Page, Source: marker.Source,
		Format: marker.Format, Title: marker.Title,
	}}
}

func validateUpgradeDeclaredPayloadSizes(marker *UploadMarker) error {
	if marker == nil || marker.Version < 6 {
		return nil
	}
	var generatedTotal, sourceTotal int64
	for _, object := range marker.Objects {
		switch object.Role {
		case MarkerRolePage:
			if object.Bytes > DefaultMaxGeneratedPageSize-generatedTotal {
				return errors.New("airplan: rendered pages exceed the maximum generated HTML size")
			}
			generatedTotal += object.Bytes
		case MarkerRoleSource:
			if object.Bytes > DefaultMaxInputSize {
				return errors.New("airplan: managed source exceeds the maximum input size")
			}
			if object.Bytes > DefaultMaxTotalPageSize-sourceTotal {
				return errors.New("airplan: managed sources exceed the maximum total source size")
			}
			sourceTotal += object.Bytes
		}
	}
	return nil
}

func upgradeSourceReadLimit(marker *UploadMarker) int64 {
	if sourceObject, ok := markerObjectForRole(marker, MarkerRoleSource); ok && sourceObject.Bytes > 0 {
		return sourceObject.Bytes
	}
	return DefaultMaxInputSize
}

func resultFromUpgradePlan(cfg *Config, plan UpgradeDocumentPlan) Result {
	marker := plan.marker
	if marker == nil {
		return Result{
			URL: plan.URL, Key: plan.PageKey, SourceKey: plan.SourceKey,
			Bucket: cfg.Bucket, MarkerVersion: plan.CurrentMarkerVersion,
			MarkerKey: plan.MarkerKey,
		}
	}
	pageBytes := plan.pageSizes[marker.Page]
	if entry, ok := renderedPageNamed(plan.newBundle, marker.Page); ok {
		pageBytes = entry.pageSize()
	}
	sourceURL, _, _ := PublicURL(cfg, plan.SourceKey)
	result := Result{
		ID: marker.Directory, URL: plan.URL, Key: plan.PageKey,
		SourceURL: sourceURL, SourceKey: plan.SourceKey, Bucket: cfg.Bucket,
		Bytes: pageBytes, ContentType: pageContentType,
		Title: marker.Title, CreatedAt: marker.CreatedAt,
		MarkerVersion: MarkerVersion, MarkerKey: plan.MarkerKey,
		Format: marker.Format, Kind: string(marker.Kind), Slug: marker.Slug,
		RepositoryURL: marker.Repo,
	}
	if marker.Revision != nil {
		result.RevisionChainID = marker.Revision.ChainID
		result.Revision = marker.Revision.Number
		result.LatestRevision = revisionLatest(plan.versions, cfg, plan.PageKey)
	}
	return result
}

func (c *Client) materializeUpgradeSpooled(
	ctx context.Context, plan *UpgradeDocumentPlan, payloads *upgradePayloads,
) error {
	if c.templateErr != nil {
		return c.templateErr
	}
	marker := plan.marker
	recipe := documentRenderRecipe(c.cfg, c.templateDigest)
	if marker.Version >= 4 && marker.Render != nil &&
		c.upgradeTemplateMatches(marker.Render) {
		copyRecipe := *marker.Render
		recipe = &copyRecipe
		recipe.Generation = RendererGeneration
		themes := themeRecipe(c.cfg.ThemeBundle)
		recipe.Themes = &themes
	}
	previousRevision, err := revisionPrevious(
		plan.versions, c.cfg, plan.PageKey, marker, plan.diff,
	)
	if err != nil {
		return err
	}
	descriptors := upgradeMarkerPages(marker)
	inputs := make([]PageInput, len(descriptors))
	opened := make([]io.Closer, 0, len(descriptors))
	closeOpened := func() {
		for _, closer := range opened {
			_ = closer.Close()
		}
	}
	for index, descriptor := range descriptors {
		source, ok := payloads.sources[descriptor.Source]
		if !ok {
			closeOpened()
			return fmt.Errorf("airplan: managed source %q was not loaded", descriptor.Source)
		}
		file, err := source.open()
		if err != nil {
			closeOpened()
			return err
		}
		opened = append(opened, file)
		inputs[index] = PageInput{
			Reader: file, Path: descriptor.Path,
			Format: descriptor.Format, Title: descriptor.Title, Lang: descriptor.Lang,
		}
	}
	assets := make([]AssetInput, 0)
	for _, object := range marker.Objects {
		if object.Role == MarkerRoleAsset {
			assets = append(assets, AssetInput{
				Reader: bytes.NewReader(nil), Path: object.Name,
				Size: object.Bytes, ContentType: object.ContentType,
			})
		}
	}
	document := DocumentInput{
		Entry: inputs[0], Pages: inputs[1:], Assets: assets,
		Slug: marker.Slug, Title: marker.Title,
		MaxPageSize: -1, MaxTotalPageSize: -1,
		RepositoryURL: marker.Repo,
	}
	bundle, err := renderDocumentWithSpool(ctx, document, DocumentRenderOptions{
		RenderInputOptions: RenderInputOptions{
			Indexable: recipe.Indexable, IncludeSource: true,
			NoExternalAssets: recipe.NoExternalAssets,
			MermaidURL:       recipe.MermaidURL, Repository: marker.Repo,
			Themes: c.cfg.ThemeBundle,
		},
		MaxGeneratedPageSize: -1,
		Revision:             revisionNumber(marker),
		RevisionCount:        revisionLatest(plan.versions, c.cfg, plan.PageKey),
		PreviousRevision:     previousRevision,
		VersionsPath:         VersionsFilename,
		DiffPath:             revisionDiffPath(marker),
		DiffText:             inlineRevisionDiff(plan.diff),
	}, c.template, c.templateErr, payloads.spool)
	closeOpened()
	if err != nil {
		return err
	}
	keepBundle := false
	defer func() {
		if !keepBundle {
			bundle.cleanup()
		}
	}()
	if len(bundle.Pages) != len(descriptors) {
		return errors.New("airplan: upgraded page graph changed unexpectedly")
	}
	for index, page := range bundle.Pages {
		if page.Path != descriptors[index].Path || page.PagePath != descriptors[index].Page ||
			page.SourcePath != descriptors[index].Source {
			return fmt.Errorf("airplan: upgraded page graph changed for %q", descriptors[index].Path)
		}
	}
	newMarker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: marker.Directory, CreatedAt: marker.CreatedAt,
		Kind: UploadKindDocument, Slug: marker.Slug, Format: marker.Format,
		Title: marker.Title, Repo: marker.Repo,
		Producer:   Producer{Name: "airplan", Version: plan.TargetProducerVersion},
		Render:     recipe,
		Revision:   marker.Revision,
		Entrypoint: bundle.Entrypoint,
	}
	for _, page := range bundle.Pages {
		newMarker.Pages = append(newMarker.Pages, MarkerPage{
			Path: page.Path, Page: page.PagePath, Source: page.SourcePath,
			Format: page.Format, Title: page.Title, Lang: page.Lang,
		})
		newMarker.Objects = append(newMarker.Objects, MarkerObject{
			Name: page.PagePath, Role: MarkerRolePage,
			Bytes: page.pageSize(), ContentType: pageContentType,
			SHA256: page.pageDigest(),
		})
		contentType := textContentType
		if page.Format == FormatMarkdown.String() {
			contentType = sourceContentType
		}
		newMarker.Objects = append(newMarker.Objects, MarkerObject{
			Name: page.SourcePath, Role: MarkerRoleSource,
			Bytes: page.sourceSize(), ContentType: contentType,
			SHA256: page.sourceDigest(),
		})
	}
	for _, object := range marker.Objects {
		if object.Role == MarkerRoleAsset {
			newMarker.Objects = append(newMarker.Objects, object)
		}
	}
	if marker.Revision != nil && marker.Revision.Number > 1 {
		newMarker.Objects = append(newMarker.Objects, MarkerObject{
			Name: DiffFilename, Role: MarkerRoleDiff,
			Bytes: int64(len(plan.diff)), ContentType: diffContentType,
			SHA256: contentSHA256(plan.diff),
		})
	}
	newMarkerBody, err := EncodeUploadMarker(newMarker)
	if err != nil {
		return err
	}
	plan.newMarker = newMarkerBody
	plan.newBundle = bundle
	keepBundle = true
	return nil
}

func renderedPageNamed(
	bundle *RenderedDocumentBundle, name string,
) (*RenderedBundlePage, bool) {
	if bundle == nil {
		return nil, false
	}
	for index := range bundle.Pages {
		if bundle.Pages[index].PagePath == name {
			return &bundle.Pages[index], true
		}
	}
	return nil, false
}

func revisionNumber(marker *UploadMarker) int {
	if marker == nil || marker.Revision == nil {
		return 0
	}
	return marker.Revision.Number
}

func revisionDiffPath(marker *UploadMarker) string {
	if revisionNumber(marker) > 1 {
		return "./" + DiffFilename
	}
	return ""
}

func revisionLatest(body []byte, cfg *Config, pageKey string) int {
	metadata, err := DecodeVersionsMetadata(body, cfg, pageKey)
	if err != nil {
		return 0
	}
	return metadata.LatestRevision
}

func revisionPrevious(
	body []byte, cfg *Config, pageKey string, marker *UploadMarker, diff []byte,
) (int, error) {
	if marker == nil || marker.Revision == nil || marker.Revision.PreviousURL == "" {
		return 0, nil
	}
	metadata, err := DecodeVersionsMetadata(body, cfg, pageKey)
	if err == nil {
		for _, revision := range metadata.Revisions {
			if !revision.Deleted && revision.URL == marker.Revision.PreviousURL {
				return revision.Number, nil
			}
		}
	}
	previous, current, err := revisionDiffRange(diff)
	if err != nil {
		return 0, fmt.Errorf("airplan: identify adjacent revision diff: %w", err)
	}
	if current != marker.Revision.Number {
		return 0, errors.New("airplan: adjacent revision diff does not match its marker")
	}
	return previous, nil
}

func revisionDiffRange(diff []byte) (int, int, error) {
	lines := strings.SplitN(string(diff), "\n", 3)
	if len(lines) < 2 {
		return 0, 0, errors.New("diff headers are missing")
	}
	parse := func(line, prefix string) (int, error) {
		line = strings.TrimSuffix(line, "\r")
		if !strings.HasPrefix(line, prefix) || !strings.HasSuffix(line, "/plan.md") {
			return 0, errors.New("diff header is invalid")
		}
		number, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(
			line, prefix,
		), "/plan.md"))
		if err != nil || number <= 0 {
			return 0, errors.New("diff revision number is invalid")
		}
		return number, nil
	}
	previous, err := parse(lines[0], "--- revision-")
	if err != nil {
		return 0, 0, err
	}
	current, err := parse(lines[1], "+++ revision-")
	if err != nil {
		return 0, 0, err
	}
	if current <= previous {
		return 0, 0, errors.New("diff revision order is invalid")
	}
	return previous, current, nil
}

func (c *Client) upgradeTemplateMatches(recipe *RenderRecipe) bool {
	if recipe == nil {
		return false
	}
	switch recipe.Template.Kind {
	case "builtin":
		return c.cfg.Template == ""
	case "custom":
		return c.templateDigest != "" &&
			c.templateDigest == recipe.Template.SHA256
	default:
		return false
	}
}

func themeRecipeMatches(recipe *ThemeRecipe, bundle *ThemeBundle) bool {
	if recipe == nil || bundle == nil {
		return false
	}
	want := themeRecipe(bundle)
	return *recipe == want
}

func (c *Client) optionalUpgradeControlObject(
	ctx context.Context, key string, limit int64,
) ([]byte, bool, error) {
	body, _, _, err := c.st.getBytesWithETag(ctx, key, limit)
	if errors.Is(err, errObjectNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if limit > 0 && int64(len(body)) > limit {
		return nil, false, fmt.Errorf(
			"airplan: control object %q exceeds %d bytes", key, limit,
		)
	}
	return body, true, nil
}

func (c *Client) verifyUpgradeControlObject(
	ctx context.Context, key string, before []byte, existed bool, limit int64,
) error {
	after, exists, err := c.optionalUpgradeControlObject(ctx, key, limit)
	if err != nil {
		return fmt.Errorf("airplan: upgrade control-object verification failed: %w", err)
	}
	if exists != existed || !bytes.Equal(after, before) {
		return errors.New("airplan: upgrade control-object verification failed: content changed")
	}
	return nil
}

func (c *Client) recordUpgrade(ctx context.Context, res *Result, markerBody []byte) {
	if c.cfg.DisableManifest {
		return
	}
	manifestPath := c.cfg.ManifestPath
	if manifestPath == "" {
		var err error
		manifestPath, err = DefaultManifestPath()
		if err != nil {
			res.Warnings = append(res.Warnings, "manifest not recorded: "+err.Error())
			return
		}
	}
	declared := declaredTotals{}
	producer := producerVersion(c.cfg.ProducerVersion)
	renderer := RendererGeneration
	if marker, err := DecodeUploadMarker(markerBody, res.ID); err == nil {
		declared = markerDeclaredTotals(*marker, markerBody)
		producer = marker.Producer.Version
		if marker.Render != nil {
			renderer = marker.Render.Generation
		}
	}
	rec := completeManifestProjection(c.cfg, "upgrade",
		time.Now().UTC().Truncate(time.Second), res, declared, producer, renderer)
	if err := appendManifestRecord(ctx, manifestPath, rec); err != nil {
		res.Warnings = append(res.Warnings, "manifest not recorded: "+err.Error())
	}
}
