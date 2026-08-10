package cli

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jimeh/airplan/airplan"
	"github.com/spf13/cobra"
)

type listFilters struct {
	newerThan time.Time
	olderThan time.Time
	kind      airplan.UploadKind
	slug      string
	limit     int
	newerSet  bool
	olderSet  bool
	kindSet   bool
	slugSet   bool
	limitSet  bool
}

func parseListFilters(
	cmd *cobra.Command, opts *listOptions, now time.Time,
) (listFilters, error) {
	filters := listFilters{
		kind: airplan.UploadKind(opts.kind), slug: opts.slug, limit: opts.limit,
		newerSet: cmd.Flags().Changed("newer-than"),
		olderSet: cmd.Flags().Changed("older-than"),
		kindSet:  cmd.Flags().Changed("kind"),
		slugSet:  cmd.Flags().Changed("slug"),
		limitSet: cmd.Flags().Changed("limit"),
	}
	if filters.limitSet && filters.limit < 0 {
		return listFilters{}, errors.New("--limit must not be negative")
	}
	if filters.kindSet && filters.kind != airplan.UploadKindDocument &&
		filters.kind != airplan.UploadKindCollection {
		return listFilters{}, errors.New("--kind must be document or collection")
	}
	if filters.slugSet {
		if _, err := path.Match(filters.slug, ""); err != nil {
			return listFilters{}, fmt.Errorf("--slug: invalid pattern: %w", err)
		}
	}
	if filters.newerSet {
		parsed, err := airplan.ParseTimeFilter(opts.newerThan, now)
		if err != nil {
			return listFilters{}, fmt.Errorf("--newer-than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		filters.newerThan = parsed
	}
	if filters.olderSet {
		parsed, err := airplan.ParseTimeFilter(opts.olderThan, now)
		if err != nil {
			return listFilters{}, fmt.Errorf("--older-than: %s",
				strings.TrimPrefix(err.Error(), "airplan: "))
		}
		filters.olderThan = parsed
	}
	return filters, nil
}

func selectManifestList(
	uploads []airplan.ManifestRecord, filters listFilters,
) []airplan.ManifestRecord {
	selected := make([]airplan.ManifestRecord, 0, len(uploads))
	for _, upload := range uploads {
		kind := airplan.UploadKind(upload.Kind)
		if kind == "" {
			kind = airplan.UploadKindDocument
		}
		if !matchesListFilters(
			upload.Time, kind, upload.Slug, false, filters,
		) {
			continue
		}
		selected = append(selected, upload)
	}
	return limitMostRecent(selected, filters)
}

func selectRemoteList(
	uploads []airplan.RemoteUpload, filters listFilters,
) []airplan.RemoteUpload {
	selected := make([]airplan.RemoteUpload, 0, len(uploads))
	for _, upload := range uploads {
		if !matchesListFilters(
			upload.LastModified, upload.Kind, upload.Slug, upload.Conflict, filters,
		) {
			continue
		}
		selected = append(selected, upload)
	}
	return limitMostRecent(selected, filters)
}

func matchesListFilters(
	when time.Time, kind airplan.UploadKind, slug string, conflict bool,
	filters listFilters,
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
		if conflict || kind != airplan.UploadKindDocument {
			return false
		}
		matched, _ := path.Match(filters.slug, slug)
		return matched
	}
	return true
}

func limitMostRecent[T any](items []T, filters listFilters) []T {
	if !filters.limitSet || filters.limit >= len(items) {
		return items
	}
	return items[len(items)-filters.limit:]
}
