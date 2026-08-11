package airplan

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"
)

type mcpListFilters struct {
	newerThan time.Time
	olderThan time.Time
	kind      UploadKind
	slug      string
	limit     int
	newerSet  bool
	olderSet  bool
	kindSet   bool
	slugSet   bool
	limitSet  bool
}

func parseMCPListFilters(input mcpListInput, now time.Time) (mcpListFilters, error) {
	filters := mcpListFilters{
		newerSet: input.NewerThan != nil, olderSet: input.OlderThan != nil,
		kindSet: input.Kind != nil, slugSet: input.Slug != nil,
		limitSet: input.Limit != nil,
	}
	if input.Kind != nil {
		filters.kind = UploadKind(*input.Kind)
	}
	if input.Slug != nil {
		filters.slug = *input.Slug
	}
	if input.Limit != nil {
		filters.limit = *input.Limit
		if filters.limit < 0 {
			return mcpListFilters{}, errors.New("airplan: limit must not be negative")
		}
	}
	if filters.kindSet && filters.kind != UploadKindDocument &&
		filters.kind != UploadKindCollection {
		return mcpListFilters{}, errors.New(
			"airplan: kind must be document or collection",
		)
	}
	if filters.slugSet {
		if _, err := path.Match(filters.slug, ""); err != nil {
			return mcpListFilters{}, fmt.Errorf(
				"airplan: slug: invalid pattern: %w", err,
			)
		}
	}
	if filters.newerSet {
		parsed, err := ParseTimeFilter(*input.NewerThan, now)
		if err != nil {
			return mcpListFilters{}, fmt.Errorf("airplan: newer_than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		filters.newerThan = parsed
	}
	if filters.olderSet {
		parsed, err := ParseTimeFilter(*input.OlderThan, now)
		if err != nil {
			return mcpListFilters{}, fmt.Errorf("airplan: older_than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		filters.olderThan = parsed
	}
	return filters, nil
}

func selectMCPManifestRecords(
	records []ManifestRecord, filters mcpListFilters,
) []ManifestRecord {
	selected := make([]ManifestRecord, 0, len(records))
	for _, record := range records {
		kind := UploadKind(record.Kind)
		if kind == "" {
			kind = UploadKindDocument
		}
		if matchesMCPListFilters(record.Time, kind, record.Slug, false, filters) {
			selected = append(selected, record)
		}
	}
	return limitMCPList(selected, filters)
}

func selectMCPRemoteUploads(
	uploads []RemoteUpload, filters mcpListFilters,
) []RemoteUpload {
	selected := make([]RemoteUpload, 0, len(uploads))
	for _, upload := range uploads {
		if matchesMCPListFilters(upload.LastModified, upload.Kind, upload.Slug,
			upload.Conflict, filters) {
			selected = append(selected, upload)
		}
	}
	return limitMCPList(selected, filters)
}

func matchesMCPListFilters(
	when time.Time, kind UploadKind, slug string, conflict bool,
	filters mcpListFilters,
) bool {
	if filters.newerSet && when.Before(filters.newerThan) {
		return false
	}
	if filters.olderSet && !when.Before(filters.olderThan) {
		return false
	}
	if filters.kindSet && (conflict || kind != filters.kind) {
		return false
	}
	if filters.slugSet {
		if conflict || kind != UploadKindDocument {
			return false
		}
		matched, _ := path.Match(filters.slug, slug)
		return matched
	}
	return true
}

func limitMCPList[T any](items []T, filters mcpListFilters) []T {
	if !filters.limitSet || filters.limit >= len(items) {
		return items
	}
	return items[len(items)-filters.limit:]
}
