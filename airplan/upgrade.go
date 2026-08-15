package airplan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"path"
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

const maxUpgradePageSize = DefaultMaxInputSize * 4

// UpgradeDocumentOptions controls one document upgrade plan.
type UpgradeDocumentOptions struct {
	Force bool `json:"force,omitempty"`
}

// UpgradeDocumentPlan binds a proposed rewrite to the exact remote objects
// that were inspected. It is safe to serialize through REST and MCP.
type UpgradeDocumentPlan struct {
	Target                    string       `json:"target"`
	Profile                   string       `json:"profile,omitempty"`
	Bucket                    string       `json:"bucket,omitempty"`
	State                     UpgradeState `json:"state"`
	Reason                    string       `json:"reason,omitempty"`
	URL                       string       `json:"url,omitempty"`
	MarkerKey                 string       `json:"marker_key,omitempty"`
	PageKey                   string       `json:"page_key,omitempty"`
	SourceKey                 string       `json:"source_key,omitempty"`
	CurrentMarkerVersion      int          `json:"current_marker_version,omitempty"`
	CurrentProducerVersion    string       `json:"current_producer_version,omitempty"`
	CurrentRendererGeneration int          `json:"current_renderer_generation,omitempty"`
	TargetMarkerVersion       int          `json:"target_marker_version"`
	TargetProducerVersion     string       `json:"target_producer_version"`
	TargetRendererGeneration  int          `json:"target_renderer_generation"`
	MarkerETag                string       `json:"marker_etag,omitempty"`
	PageETag                  string       `json:"page_etag,omitempty"`
	SourceETag                string       `json:"source_etag,omitempty"`
	Force                     bool         `json:"force,omitempty"`

	markerBody []byte
	pageBody   []byte
	sourceBody []byte
	marker     *UploadMarker
	newMarker  []byte
	newPage    []byte
	protection []byte
	versions   []byte
	protected  bool
	versioned  bool
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
	if marker.Version == MarkerVersion && marker.Render != nil {
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
	pageKey := dirPrefix + marker.Page
	sourceKey := dirPrefix + marker.Source
	pageBody, pageETag, _, err := c.st.getBytesWithETag(ctx, pageKey, maxUpgradePageSize)
	if err != nil {
		return nil, err
	}
	if int64(len(pageBody)) > maxUpgradePageSize {
		return nil, fmt.Errorf("airplan: rendered page exceeds the maximum upgrade size")
	}
	if pageETag == "" {
		return nil, errors.New("airplan: rendered page has no ETag; conditional upgrade is unavailable")
	}
	sourceBody, sourceETag, _, err := c.st.getBytesWithETag(ctx, sourceKey, DefaultMaxInputSize)
	if err != nil {
		return nil, err
	}
	if sourceETag == "" {
		return nil, errors.New("airplan: Markdown source has no ETag; conditional upgrade is unavailable")
	}
	if len(sourceBody) > DefaultMaxInputSize {
		return nil, fmt.Errorf("airplan: source exceeds the maximum input size")
	}
	protection, protected, err := c.optionalUpgradeControlObject(
		ctx, dirPrefix+ProtectedFilename, MaxMarkerSize,
	)
	if err != nil {
		return nil, err
	}
	versions, versioned, err := c.optionalUpgradeControlObject(
		ctx, dirPrefix+".airplan-versions.json", DefaultMaxInputSize,
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
	plan.markerBody, plan.pageBody, plan.sourceBody = markerBody, pageBody, sourceBody
	plan.protection, plan.versions = protection, versions
	plan.protected, plan.versioned = protected, versioned
	plan.marker = marker
	pageContentCurrent := marker.PageBytes == int64(len(pageBody)) &&
		(marker.Version < MarkerVersion ||
			marker.PageSHA256 == contentSHA256(pageBody))
	switch {
	case marker.Version < MarkerVersion:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "ownership marker schema is older"
	case marker.Render == nil || marker.Render.Generation < RendererGeneration:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "renderer generation is older"
	case !pageContentCurrent:
		plan.State, plan.Reason = UpgradeStateUpgradeable, "rendered page requires repair"
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

// UpgradeDocument applies an exact plan. Locally created plans retain their
// rendered bytes; deserialized plans are safely re-planned and identity-bound.
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
		return nil, fmt.Errorf("airplan: document is %s: %s", plan.State, plan.Reason)
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
		return nil, fmt.Errorf("airplan: document is %s: %s", fresh.State, fresh.Reason)
	}
	if plan.MarkerETag == "" || plan.PageETag == "" || plan.SourceETag == "" {
		return nil, errors.New("airplan: upgrade plan is missing required object ETags")
	}
	if fresh.MarkerKey != plan.MarkerKey || fresh.PageKey != plan.PageKey ||
		fresh.MarkerETag != plan.MarkerETag || fresh.PageETag != plan.PageETag ||
		fresh.SourceETag != plan.SourceETag {
		return nil, ErrConflict
	}
	plan = *fresh
	currentSource, currentSourceETag, _, err := c.st.getBytesWithETag(
		ctx, plan.SourceKey, DefaultMaxInputSize,
	)
	if err != nil {
		return nil, err
	}
	if currentSourceETag != plan.SourceETag || !bytes.Equal(currentSource, plan.sourceBody) {
		return nil, ErrConflict
	}
	if err := c.materializeUpgrade(&plan); err != nil {
		return nil, err
	}
	if err := c.st.putConditional(ctx, object{
		Key: plan.MarkerKey, Body: plan.newMarker,
		ContentType: markerContentType,
	}, plan.MarkerETag); err != nil {
		return nil, err
	}
	if err := c.st.putConditional(ctx, object{
		Key: plan.PageKey, Body: plan.newPage, ContentType: pageContentType,
		Metadata: titleMetadata(plan.marker.Title),
	}, plan.PageETag); err != nil {
		return nil, err
	}
	verifiedMarker, _, _, err := c.st.getBytesWithETag(ctx, plan.MarkerKey, MaxMarkerSize)
	if err != nil {
		return nil, fmt.Errorf("airplan: upgraded marker verification failed: %w", err)
	}
	if !bytes.Equal(verifiedMarker, plan.newMarker) {
		return nil, errors.New("airplan: upgraded marker verification failed: content changed")
	}
	verifiedPage, _, _, err := c.st.getBytesWithETag(ctx, plan.PageKey, int64(len(plan.newPage)))
	if err != nil {
		return nil, fmt.Errorf("airplan: upgraded page verification failed: %w", err)
	}
	if !bytes.Equal(verifiedPage, plan.newPage) {
		return nil, errors.New("airplan: upgraded page verification failed: content changed")
	}
	verifiedSource, _, _, err := c.st.getBytesWithETag(
		ctx, plan.SourceKey, DefaultMaxInputSize,
	)
	if err != nil {
		return nil, fmt.Errorf("airplan: upgraded source verification failed: %w", err)
	}
	if !bytes.Equal(verifiedSource, plan.sourceBody) {
		return nil, errors.New("airplan: upgraded source verification failed: content changed")
	}
	dirPrefix := strings.TrimSuffix(plan.MarkerKey, MarkerFilename)
	if err := c.verifyUpgradeControlObject(
		ctx, dirPrefix+ProtectedFilename, plan.protection, plan.protected, MaxMarkerSize,
	); err != nil {
		return nil, err
	}
	if err := c.verifyUpgradeControlObject(
		ctx, dirPrefix+".airplan-versions.json", plan.versions, plan.versioned, DefaultMaxInputSize,
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

func resultFromUpgradePlan(cfg *Config, plan UpgradeDocumentPlan) Result {
	marker := plan.marker
	if marker == nil {
		return Result{
			URL: plan.URL, Key: plan.PageKey, SourceKey: plan.SourceKey,
			Bucket: cfg.Bucket, MarkerVersion: plan.CurrentMarkerVersion,
			MarkerKey: plan.MarkerKey,
		}
	}
	pageBytes := int64(len(plan.pageBody))
	if plan.newPage != nil {
		pageBytes = int64(len(plan.newPage))
	}
	sourceURL, _, _ := PublicURL(cfg, plan.SourceKey)
	return Result{
		ID: marker.Directory, URL: plan.URL, Key: plan.PageKey,
		SourceURL: sourceURL, SourceKey: plan.SourceKey, Bucket: cfg.Bucket,
		Bytes: pageBytes, ContentType: pageContentType,
		Title: marker.Title, CreatedAt: marker.CreatedAt,
		MarkerVersion: MarkerVersion, MarkerKey: plan.MarkerKey,
		Format: marker.Format, Kind: string(marker.Kind), Slug: marker.Slug,
		RepositoryURL: marker.Repo,
	}
}

func (c *Client) materializeUpgrade(plan *UpgradeDocumentPlan) error {
	if c.templateErr != nil {
		return c.templateErr
	}
	marker := plan.marker
	recipe := documentRenderRecipe(c.cfg, c.templateDigest)
	if marker.Version == MarkerVersion && marker.Render != nil &&
		c.upgradeTemplateMatches(marker.Render) {
		copyRecipe := *marker.Render
		recipe = &copyRecipe
		recipe.Generation = RendererGeneration
	}
	newPage, err := RenderMarkdown(plan.sourceBody, RenderOptions{
		Title: marker.Title, Slug: marker.Slug, SourceName: marker.Source,
		SourcePath: "./" + marker.Source, Indexable: recipe.Indexable,
		NoExternalAssets: recipe.NoExternalAssets,
		MermaidURL:       recipe.MermaidURL, RepositoryURL: marker.Repo,
		Template: c.template,
	})
	if err != nil {
		return err
	}
	newMarker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion,
		Directory: marker.Directory, CreatedAt: marker.CreatedAt,
		Kind: UploadKindDocument, Slug: marker.Slug, Format: marker.Format,
		Title: marker.Title, Repo: marker.Repo,
		Producer: Producer{Name: "airplan", Version: plan.TargetProducerVersion},
		Render:   recipe,
		Objects: []MarkerObject{
			{Name: marker.Page, Role: MarkerRolePage, Bytes: int64(len(newPage)), ContentType: pageContentType, SHA256: contentSHA256(newPage)},
			{Name: marker.Source, Role: MarkerRoleSource, Bytes: int64(len(plan.sourceBody)), ContentType: sourceContentType},
		},
	}
	newMarkerBody, err := EncodeUploadMarker(newMarker)
	if err != nil {
		return err
	}
	plan.newMarker, plan.newPage = newMarkerBody, newPage
	return nil
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
	rec := ManifestRecord{
		Type: "upgrade", Time: time.Now().UTC().Truncate(time.Second),
		CreatedAt: res.CreatedAt, Key: res.Key, SourceKey: res.SourceKey,
		MarkerKey: res.MarkerKey, URL: res.URL, Bucket: res.Bucket,
		Profile: c.cfg.Profile, Format: res.Format, Kind: res.Kind,
		Slug: res.Slug, Title: res.Title, Repo: res.RepositoryURL,
		Bytes: res.Bytes, MarkerVersion: MarkerVersion,
		ProducerVersion: producerVersion(c.cfg.ProducerVersion),
		RendererVersion: RendererGeneration,
	}
	if marker, err := DecodeUploadMarker(markerBody, res.ID); err == nil {
		declared := markerDeclaredTotals(*marker, markerBody)
		rec.Objects = declared.objects
		rec.TotalBytes = declared.bytes
	}
	if err := appendManifestRecord(ctx, manifestPath, rec); err != nil {
		res.Warnings = append(res.Warnings, "manifest not recorded: "+err.Error())
	}
}
