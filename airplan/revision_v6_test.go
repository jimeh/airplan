package airplan

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
)

// downgradeDocumentMarkers rewrites every document marker in store to marker
// version 6, as an airplan release before marker v7 would have written it.
// v6 and v7 share one wire format, so only the version number changes.
func downgradeDocumentMarkers(t *testing.T, store *upgradeStore) {
	t.Helper()
	store.mu.Lock()
	keys := make([]string, 0, len(store.objects))
	for key := range store.objects {
		if strings.HasSuffix(key, "/"+MarkerFilename) {
			keys = append(keys, key)
		}
	}
	store.mu.Unlock()
	current := []byte(fmt.Sprintf(`"version":%d,`, MarkerVersion))
	for _, key := range keys {
		body, _ := store.get(key)
		if !bytes.Contains(body, current) {
			t.Fatalf("marker %q does not declare version %d", key, MarkerVersion)
		}
		store.set(key, bytes.Replace(body, current, []byte(`"version":6,`), 1))
	}
}

// A revision chain written entirely as v6 by an earlier release stays fully
// manageable: new revisions through any member and non-latest deletion.
func TestV6RevisionChainRemainsManageable(t *testing.T) {
	document := func(body string) DocumentInput {
		return DocumentInput{
			Entry:         PageInput{Reader: strings.NewReader(body), Path: "plan.md"},
			RepositoryURL: "none",
		}
	}
	newChain := func(t *testing.T) (*Client, []string) {
		t.Helper()
		store := newUpgradeStore(t)
		client := store.client(t, "")
		ctx := context.Background()
		first, err := client.UploadDocument(ctx, document("# Plan\n\nOne\n"))
		if err != nil {
			t.Fatal(err)
		}
		urls := []string{first.URL}
		for _, body := range []string{"# Plan\n\nTwo\n", "# Plan\n\nThree\n"} {
			revision, err := client.CreateDocumentRevision(ctx, CreateDocumentRevisionInput{
				Target: first.URL, Document: document(body),
			})
			if err != nil {
				t.Fatal(err)
			}
			urls = append(urls, revision.URL)
		}
		downgradeDocumentMarkers(t, store)
		return client, urls
	}

	for _, tt := range []struct {
		name   string
		target int
	}{
		{"new revision via older member", 0},
		{"new revision via latest member", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, urls := newChain(t)
			result, err := client.CreateDocumentRevision(context.Background(),
				CreateDocumentRevisionInput{
					Target: urls[tt.target], Document: document("# Plan\n\nFour\n"),
				})
			if err != nil {
				t.Fatalf("new revision via v6 member %d: %v", tt.target+1, err)
			}
			if result.Revision != 4 || result.MarkerVersion != MarkerVersion {
				t.Fatalf("revision = %d, marker version = %d", result.Revision, result.MarkerVersion)
			}
		})
	}

	t.Run("delete non-latest member", func(t *testing.T) {
		client, urls := newChain(t)
		if _, err := client.DeleteUpload(context.Background(), urls[1]); err != nil {
			t.Fatalf("delete v6 revision 2: %v", err)
		}
		latest, err := client.InspectUpload(context.Background(), urls[2])
		if err != nil || latest.State != UploadComplete || latest.Revision != 3 {
			t.Fatalf("latest after delete = %+v, %v", latest, err)
		}
	})
}
