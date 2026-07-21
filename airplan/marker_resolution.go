package airplan

import (
	"context"
	"errors"
	"fmt"
)

var errOwnershipMarkerMissing = errors.New("ownership marker is missing")

type resolvedMarker struct {
	Key      string
	Basename string
	Body     []byte
}

// resolveMarker probes both exact ownership marker names without requiring
// bucket LIST permission. Exactly one marker must exist.
func (c *Client) resolveMarker(
	ctx context.Context, dirPrefix string,
) (*resolvedMarker, error) {
	type probe struct {
		name string
		body []byte
		err  error
	}
	ch := make(chan probe, 2)
	for _, name := range []string{MarkerFilename, CollectionMarkerFilename} {
		go func() {
			body, err := c.st.getBytes(ctx, dirPrefix+name, MaxMarkerSize)
			ch <- probe{name: name, body: body, err: err}
		}()
	}
	results := []probe{<-ch, <-ch}
	var found *probe
	for i := range results {
		p := &results[i]
		if p.err == nil {
			if found != nil {
				return nil, markerInvalid(MarkerErrorConflictingMarkers,
					errors.New("conflicting ownership markers"))
			}
			found = p
			continue
		}
		if !errors.Is(p.err, errObjectNotFound) {
			return nil, p.err
		}
	}
	if found == nil {
		return nil, fmt.Errorf("airplan: %w under %q",
			errOwnershipMarkerMissing, dirPrefix)
	}
	return &resolvedMarker{
		Key: dirPrefix + found.name, Basename: found.name, Body: found.body,
	}, nil
}
