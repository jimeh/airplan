package airplan

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type updateCountingTransport struct {
	operationTransport
	calls int
}

func (t *updateCountingTransport) UpdateDocument(
	context.Context, UpdateDocumentInput,
) (*UpdateDocumentResult, error) {
	t.calls++
	return nil, errors.New("unexpected remote update")
}

func TestUpdateDocumentValidatesBeforeRemoteDispatch(t *testing.T) {
	remote := &updateCountingTransport{}
	client := &Client{cfg: &Config{Backend: BackendAirplan}, remote: remote}
	tests := []UpdateDocumentInput{
		{},
		{Target: "target"},
		{Target: "target", Input: Input{Reader: strings.NewReader("x"), Slug: "changed"}},
		{Target: "target", Input: Input{Reader: strings.NewReader("x"), Format: "txt"}},
		{Target: "target", Input: Input{Reader: strings.NewReader("x"), Lang: "go"}},
		{Target: "target", Input: Input{Reader: strings.NewReader("x"), RepositoryURL: "https://example.com"}},
	}
	for index, input := range tests {
		if _, err := client.UpdateDocument(context.Background(), input); err == nil {
			t.Fatalf("case %d unexpectedly succeeded", index)
		}
	}
	if remote.calls != 0 {
		t.Fatalf("remote update calls = %d, want 0", remote.calls)
	}
}

func TestUpdateDocumentCreatesLinkedRevisionsOnlyOnFirstUpdate(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("# Plan\n\nOriginal.\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	firstVersions := BuildKey("", first.ID, VersionsFilename)
	if _, ok := store.get(firstVersions); ok {
		t.Fatal("standalone upload created versions metadata")
	}

	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL,
		Input:  Input{Reader: strings.NewReader("# Plan\n\nRevised.\n"), Name: "plan.md"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.Revision != 2 || second.LatestRevision != 2 || second.Unchanged ||
		second.PreviousURL != first.URL || second.DiffURL == "" {
		t.Fatalf("unexpected result: %#v", second)
	}
	for _, key := range []string{
		firstVersions,
		BuildKey("", second.ID, VersionsFilename),
		BuildKey("", second.ID, DiffFilename),
	} {
		if _, ok := store.get(key); !ok {
			t.Fatalf("missing %s", key)
		}
	}
	firstMarkerBody, _ := store.get(first.MarkerKey)
	firstMarker, err := DecodeUploadMarker(firstMarkerBody, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	if firstMarker.Revision == nil || firstMarker.Revision.Number != 1 {
		t.Fatalf("first marker revision = %#v", firstMarker.Revision)
	}
	secondMarkerBody, _ := store.get(second.MarkerKey)
	secondMarker, err := DecodeUploadMarker(secondMarkerBody, second.ID)
	if err != nil {
		t.Fatal(err)
	}
	if secondMarker.Revision == nil || secondMarker.Revision.Number != 2 ||
		secondMarker.Revision.ChainID != firstMarker.Revision.ChainID {
		t.Fatalf("second marker revision = %#v", secondMarker.Revision)
	}
	store.mu.Lock()
	putKeys := append([]string(nil), store.putKeys...)
	store.mu.Unlock()
	wantOrder := []string{
		second.MarkerKey,
		BuildKey("", second.ID, "plan.md"),
		BuildKey("", second.ID, DiffFilename),
		BuildKey("", second.ID, "plan.html"),
	}
	position := -1
	for _, want := range wantOrder {
		for index := position + 1; index < len(putKeys); index++ {
			if putKeys[index] == want {
				position = index
				break
			}
		}
		if position < 0 || putKeys[position] != want {
			t.Fatalf("candidate PUT order = %v; missing ordered write %q", putKeys, want)
		}
	}
}

func TestUpdateDocumentCandidateFailureRollsBackPublishedMarker(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failPutSuffix = ".html"
	store.mu.Unlock()
	_, err = client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err == nil {
		t.Fatal("update unexpectedly succeeded")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	markers := 0
	for key := range store.objects {
		if strings.HasSuffix(key, "/"+MarkerFilename) {
			markers++
		}
	}
	if markers != 1 {
		t.Fatalf("candidate failure left %d markers, want only predecessor", markers)
	}
}

func TestUpdateDocumentRollbackFailureLeavesDiscoverableManagedCandidate(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failPutSuffix = ".html"
	store.failDeleteKeys = true
	store.mu.Unlock()
	_, err = client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "candidate rollback failed") {
		t.Fatalf("update error = %v", err)
	}
	store.mu.Lock()
	candidateMarkerKey := ""
	for key := range store.objects {
		if strings.HasSuffix(key, "/"+MarkerFilename) && key != first.MarkerKey {
			candidateMarkerKey = key
		}
	}
	store.mu.Unlock()
	if candidateMarkerKey == "" {
		t.Fatal("rollback failure did not leave a discoverable ownership marker")
	}
	candidateDir := strings.TrimSuffix(candidateMarkerKey, "/"+MarkerFilename)
	deleted, err := client.DeleteUpload(context.Background(), candidateDir+"/plan.md")
	if err != nil {
		t.Fatalf("recover managed candidate: %v", err)
	}
	if deleted.MarkerKey != candidateMarkerKey ||
		!strings.Contains(strings.Join(deleted.Warnings, "\n"), "unannounced revision candidate") {
		t.Fatalf("recovery result = %+v", deleted)
	}
}

func TestUpdateDocumentFromOldURLResolvesLatestAndIdenticalIsNoop(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("three\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if third.Revision != 3 || third.PreviousURL != second.URL {
		t.Fatalf("third result = %#v", third)
	}
	store.mu.Lock()
	puts := store.puts
	store.mu.Unlock()
	noop, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("three\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !noop.Unchanged || noop.URL != third.URL || noop.Revision != 3 {
		t.Fatalf("no-op result = %#v", noop)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.puts != puts {
		t.Fatalf("no-op performed %d writes", store.puts-puts)
	}
}

func TestRevisionLinkManifestEventsPreserveCompleteProjection(t *testing.T) {
	store := newUpgradeStore(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := store.client(t, manifest)
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md", Title: "Plan",
		RepositoryURL: "https://github.com/jimeh/airplan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	}); err != nil {
		t.Fatal(err)
	}
	records, warnings, err := ReadManifest(manifest)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("records = %+v, warnings = %v, err = %v", records, warnings, err)
	}
	var link *ManifestRecord
	for index := range records {
		if records[index].Type == "link" && records[index].MarkerKey == first.MarkerKey {
			link = &records[index]
		}
	}
	if link == nil || link.RevisionChainID == "" || link.Revision != 1 ||
		link.LatestRevision != 2 || link.Objects != 3 || link.TotalBytes <= link.Bytes ||
		link.Title != "Plan" || link.Repo != "https://github.com/jimeh/airplan" ||
		link.SourceKey != first.SourceKey {
		t.Fatalf("first revision link projection = %+v", link)
	}
}

func TestUpdateDocumentRetryRecreatesMissingSiblingMetadata(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: second.URL, Input: Input{Reader: strings.NewReader("three\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	firstMetadataKey := first.ID + "/" + VersionsFilename
	store.mu.Lock()
	delete(store.objects, firstMetadataKey)
	delete(store.etags, firstMetadataKey)
	store.mu.Unlock()

	retried, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: third.URL, Input: Input{Reader: strings.NewReader("three\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !retried.Unchanged {
		t.Fatalf("retry result = %+v", retried)
	}
	body, ok := store.get(firstMetadataKey)
	if !ok {
		t.Fatal("retry did not recreate missing sibling metadata")
	}
	metadata, err := DecodeVersionsMetadata(body, client.cfg, first.Key)
	if err != nil || metadata.CurrentRevision != 1 || metadata.LatestRevision != 3 {
		t.Fatalf("recreated metadata = %+v, err = %v", metadata, err)
	}
}

func TestStaleAppendRepairCannotResurrectConcurrentRevisionDeletion(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	third, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: second.URL, Input: Input{Reader: strings.NewReader("three\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	stale, err := client.loadRevisionDocument(context.Background(), third.URL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.DeleteUpload(context.Background(), second.URL); err != nil {
		t.Fatal(err)
	}
	if err := client.reconcileRevisionMetadata(context.Background(), stale); err == nil ||
		!strings.Contains(err.Error(), "revision chain changed") {
		t.Fatalf("stale reconciliation error = %v", err)
	}
	body, ok := store.get(first.ID + "/" + VersionsFilename)
	if !ok {
		t.Fatal("first revision metadata missing")
	}
	metadata, err := DecodeVersionsMetadata(body, client.cfg, first.Key)
	if err != nil {
		t.Fatal(err)
	}
	if !metadata.Revisions[1].Deleted {
		t.Fatalf("concurrent deletion was resurrected: %+v", metadata)
	}
}

func TestGenerateRevisionDiffDeterministicAndPreservesNewlineSignal(t *testing.T) {
	diff, err := GenerateRevisionDiff([]byte("one\ntwo\n"), []byte("one\nthree"), 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--- revision-1/plan.md\n", "+++ revision-2/plan.md\n", "-two\n", "+three\n", "\\ No newline at end of file"} {
		if !bytes.Contains(diff, []byte(want)) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
}

func TestGenerateRevisionDiffFinalNewlineOnlyChanges(t *testing.T) {
	tests := []struct {
		name     string
		previous string
		current  string
		want     []string
	}{
		{
			name: "adds final newline", previous: "a", current: "a\n",
			want: []string{"-a\n\\ No newline at end of file\n", "+a\n"},
		},
		{
			name: "removes final newline", previous: "a\n", current: "a",
			want: []string{"-a\n", "+a\n\\ No newline at end of file\n"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			diff, err := GenerateRevisionDiff(
				[]byte(tt.previous), []byte(tt.current), 1, 2,
			)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range tt.want {
				if !bytes.Contains(diff, []byte(want)) {
					t.Fatalf("diff missing %q:\n%s", want, diff)
				}
			}
			if got := bytes.Count(diff, []byte("\\ No newline at end of file\n")); got != 1 {
				t.Fatalf("newline annotations = %d, want 1:\n%s", got, diff)
			}
		})
	}
}

func TestGenerateRevisionDiffAnnotatesBothChangedMissingNewlines(t *testing.T) {
	diff, err := GenerateRevisionDiff(
		[]byte("head\nold"), []byte("head\nnew"), 1, 2,
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"-old\n\\ No newline at end of file\n",
		"+new\n\\ No newline at end of file\n",
	} {
		if !bytes.Contains(diff, []byte(want)) {
			t.Fatalf("diff missing %q:\n%s", want, diff)
		}
	}
}

func TestGenerateRevisionDiffAllowsTombstoneGap(t *testing.T) {
	diff, err := GenerateRevisionDiff([]byte("one\n"), []byte("four\n"), 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(diff, []byte("--- revision-1/plan.md\n")) ||
		!bytes.Contains(diff, []byte("+++ revision-4/plan.md\n")) {
		t.Fatalf("diff headers = %q", diff)
	}
}

func TestUpdateDocumentDiffLimitFailsBeforeMutation(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	putsBefore := store.puts
	store.mu.Unlock()
	_, err = client.updateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL,
		Input:  Input{Reader: strings.NewReader(strings.Repeat("changed line\n", 20))},
	}, 64)
	if err == nil || !strings.Contains(err.Error(), "maximum is 64") {
		t.Fatalf("diff limit error = %v", err)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.puts != putsBefore {
		t.Fatalf("diff limit failure performed %d writes", store.puts-putsBefore)
	}
}

func TestLoadRevisionDocumentUsesDeclaredSourceSizeAndRejectsTruncation(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	if err := client.ensureStorage(context.Background()); err != nil {
		t.Fatal(err)
	}
	dir := strings.Repeat("l", 26)
	page := []byte("<html>page</html>")
	source := bytes.Repeat([]byte("a"), DefaultMaxInputSize+1)
	marker := UploadMarker{
		Schema: MarkerSchema, Version: MarkerVersion, Directory: dir,
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Kind:      UploadKindDocument, Slug: "plan", Format: "md",
		Producer: Producer{Name: "airplan", Version: "test"},
		Render: &RenderRecipe{
			Generation: RendererGeneration,
			Template:   RenderTemplate{Kind: "builtin"},
		},
		Objects: []MarkerObject{
			{Name: "plan.html", Role: MarkerRolePage, Bytes: int64(len(page)), ContentType: pageContentType, SHA256: contentSHA256(page)},
			{Name: "plan.md", Role: MarkerRoleSource, Bytes: int64(len(source)), ContentType: sourceContentType},
		},
	}
	markerBody, err := EncodeUploadMarker(marker)
	if err != nil {
		t.Fatal(err)
	}
	store.set(dir+"/"+MarkerFilename, markerBody)
	store.set(dir+"/plan.html", page)
	store.set(dir+"/plan.md", source)
	doc, err := client.loadRevisionDocument(context.Background(), dir+"/plan.html")
	if err != nil || len(doc.sourceBody) != len(source) {
		t.Fatalf("declared-size load = %d bytes, %v", len(doc.sourceBody), err)
	}
	store.set(dir+"/plan.md", source[:len(source)-1])
	if _, err := client.loadRevisionDocument(
		context.Background(), dir+"/plan.html",
	); err == nil || !strings.Contains(err.Error(), "source size") {
		t.Fatalf("truncated source error = %v", err)
	}
}

func TestLoadRevisionDocumentRejectsIncompleteRequiredDiff(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	diffKey := BuildKey("", second.ID, DiffFilename)
	diff, _ := store.get(diffKey)
	store.set(diffKey, diff[:len(diff)-1])
	if _, err := client.loadRevisionDocument(
		context.Background(), second.URL,
	); err == nil || !strings.Contains(err.Error(), "diff is missing or incomplete") {
		t.Fatalf("incomplete diff error = %v", err)
	}
}

func TestUpdateDocumentRepairsCommittedMetadataPropagation(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failPutAttempt = store.putAttempts + 8
	store.mu.Unlock()
	_, err = client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "revision committed") {
		t.Fatalf("first update error = %v", err)
	}
	store.mu.Lock()
	store.failPutAttempt = 0
	store.mu.Unlock()
	repaired, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if repaired.Revision != 2 || !repaired.Unchanged {
		t.Fatalf("repair result = %#v", repaired)
	}
}

func TestUpdateDocumentRetryFromCandidateRepairsFirstPredecessorPromotion(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failPutKey = first.MarkerKey
	store.mu.Unlock()
	_, err = client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err == nil || !strings.Contains(err.Error(), "predecessor promotion failed") {
		t.Fatalf("first update error = %v", err)
	}
	store.mu.Lock()
	candidateMarkerKey := ""
	for key := range store.objects {
		if strings.HasSuffix(key, "/"+MarkerFilename) && key != first.MarkerKey {
			candidateMarkerKey = key
		}
	}
	store.mu.Unlock()
	if candidateMarkerKey == "" {
		t.Fatal("committed candidate marker is missing")
	}
	candidatePageKey := strings.TrimSuffix(candidateMarkerKey, MarkerFilename) + "plan.html"
	candidateURL, _, err := PublicURL(client.cfg, candidatePageKey)
	if err != nil {
		t.Fatal(err)
	}
	repaired, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: candidateURL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if err != nil || !repaired.Unchanged || repaired.Revision != 2 {
		t.Fatalf("candidate retry = %+v, %v", repaired, err)
	}
	firstMarkerBody, _ := store.get(first.MarkerKey)
	firstMarker, err := DecodeUploadMarker(firstMarkerBody, first.ID)
	if err != nil || firstMarker.Revision == nil || firstMarker.Revision.Number != 1 {
		t.Fatalf("repaired predecessor marker = %+v, %v", firstMarker, err)
	}
}

func TestUpdateDocumentConcurrentFirstAppendHasOneWinnerAndRollsBackLoser(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.conditionalBarrier = make(chan struct{})
	store.mu.Unlock()

	type outcome struct {
		result *UpdateDocumentResult
		err    error
	}
	results := make(chan outcome, 2)
	var start sync.WaitGroup
	start.Add(1)
	for _, body := range []string{"two-a\n", "two-b\n"} {
		body := body
		go func() {
			start.Wait()
			result, updateErr := client.UpdateDocument(context.Background(), UpdateDocumentInput{
				Target: first.URL, Input: Input{Reader: strings.NewReader(body)},
			})
			results <- outcome{result: result, err: updateErr}
		}()
	}
	start.Done()

	winners, conflicts := 0, 0
	for range 2 {
		result := <-results
		switch {
		case result.err == nil:
			winners++
			if result.result.Revision != 2 {
				t.Fatalf("winning result = %#v", result.result)
			}
		case errors.Is(result.err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected update error: %v", result.err)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners = %d, conflicts = %d", winners, conflicts)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	markers := 0
	for key := range store.objects {
		if strings.HasSuffix(key, "/"+MarkerFilename) {
			markers++
		}
	}
	if markers != 2 {
		t.Fatalf("marker objects after rollback = %d, want original and winner", markers)
	}
}

func TestFirstLinkAndStandaloneDeleteSerializeAfterBothReadStandalone(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.conditionalBarrier = make(chan struct{})
	store.mu.Unlock()
	type outcome struct {
		operation string
		update    *UpdateDocumentResult
		err       error
	}
	results := make(chan outcome, 2)
	go func() {
		result, updateErr := client.UpdateDocument(context.Background(), UpdateDocumentInput{
			Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
		})
		results <- outcome{operation: "update", update: result, err: updateErr}
	}()
	go func() {
		_, deleteErr := client.DeleteUpload(context.Background(), first.URL)
		results <- outcome{operation: "delete", err: deleteErr}
	}()
	winner := outcome{}
	conflicts := 0
	for range 2 {
		result := <-results
		if result.err == nil {
			if winner.operation != "" {
				t.Fatal("both first link and standalone delete succeeded")
			}
			winner = result
		} else if errors.Is(result.err, ErrConflict) {
			conflicts++
		} else {
			t.Fatalf("%s error = %v", result.operation, result.err)
		}
	}
	if winner.operation == "" || conflicts != 1 {
		t.Fatalf("winner = %q, conflicts = %d", winner.operation, conflicts)
	}
	if winner.operation == "update" {
		inspection, inspectErr := client.InspectUpload(context.Background(), first.URL)
		if inspectErr != nil || inspection.Revision != 1 ||
			inspection.LatestRevision != 2 || winner.update.Revision != 2 {
			t.Fatalf("winning chain = %+v, result = %+v, error = %v",
				inspection, winner.update, inspectErr)
		}
	} else if _, ok := store.get(first.MarkerKey); ok {
		t.Fatal("standalone delete won but ownership marker remains")
	}
}

func TestStandaloneDeleteTombstoneDefeatsPreflightedFirstLinkAfterDeleteCompletes(t *testing.T) {
	store := newUpgradeStore(t)
	manifest := filepath.Join(t.TempDir(), "manifest.jsonl")
	client := store.client(t, manifest)
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	reservationKey := first.ID + "/" + VersionsFilename
	reached := make(chan struct{})
	release := make(chan struct{})
	store.mu.Lock()
	store.pauseIfNoneKey = reservationKey
	store.pauseIfNoneReached = reached
	store.pauseIfNoneRelease = release
	store.mu.Unlock()

	updateDone := make(chan error, 1)
	go func() {
		_, updateErr := client.UpdateDocument(context.Background(), UpdateDocumentInput{
			Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
		})
		updateDone <- updateErr
	}()
	select {
	case <-reached:
	case <-time.After(5 * time.Second):
		t.Fatal("update did not reach predecessor metadata commit")
	}

	store.mu.Lock()
	candidateMarkerKey := ""
	for key, body := range store.objects {
		if !strings.HasSuffix(key, "/"+MarkerFilename) || key == first.MarkerKey {
			continue
		}
		candidateMarkerKey = key
		marker, decodeErr := DecodeUploadMarker(body,
			strings.TrimSuffix(key, "/"+MarkerFilename))
		if decodeErr != nil {
			store.mu.Unlock()
			t.Fatal(decodeErr)
		}
		for _, object := range marker.Objects {
			candidatePrefix := strings.TrimSuffix(key, MarkerFilename)
			if _, ok := store.objects[candidatePrefix+object.Name]; !ok {
				store.mu.Unlock()
				t.Fatalf("candidate object %q is not published before commit", object.Name)
			}
		}
	}
	store.mu.Unlock()
	if candidateMarkerKey == "" {
		t.Fatal("update did not publish a complete marker-first candidate")
	}

	deleted, err := client.DeleteUpload(context.Background(), first.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range deleted.Keys {
		if key == reservationKey {
			t.Fatalf("delete result incorrectly reports durable tombstone as deleted: %+v",
				deleted.Keys)
		}
	}
	if _, ok := store.get(first.MarkerKey); ok {
		t.Fatal("standalone marker remains after delete")
	}
	if body, ok := store.get(reservationKey); !ok ||
		!bytes.Equal(body, standaloneDeleteReservationBody) {
		t.Fatalf("durable delete tombstone = %q, exists %t", body, ok)
	}

	close(release)
	select {
	case updateErr := <-updateDone:
		if !errors.Is(updateErr, ErrConflict) {
			t.Fatalf("stale preflighted update error = %v, want conflict", updateErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("stale update did not finish")
	}
	if _, ok := store.get(candidateMarkerKey); ok {
		t.Fatal("losing update candidate marker survived rollback")
	}
	if body, ok := store.get(reservationKey); !ok ||
		!bytes.Equal(body, standaloneDeleteReservationBody) {
		t.Fatal("losing update disturbed durable delete tombstone")
	}
	remote, err := client.ListRemote(context.Background())
	if err != nil || len(remote) != 0 {
		t.Fatalf("tombstone-only prefix listed as upload: %+v, %v", remote, err)
	}
	synced, err := client.SyncManifest(context.Background(), SyncManifestOptions{Prune: true})
	if err != nil || synced.Invalid != 0 || synced.Incomplete != 0 ||
		len(synced.Added) != 0 || len(synced.Tombstoned) != 0 {
		t.Fatalf("tombstone-only sync result = %+v, %v", synced, err)
	}
}

func TestStandaloneDeleteReservationMakesLaterFirstLinkFailClosed(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	dirPrefix := strings.TrimSuffix(first.MarkerKey, MarkerFilename)
	if err := client.reserveStandaloneDelete(context.Background(), dirPrefix); err != nil {
		t.Fatal(err)
	}
	_, err = client.UpdateDocument(context.Background(), UpdateDocumentInput{
		Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
	})
	if !errors.Is(err, errRevisionTransitionReserved) {
		t.Fatalf("update error = %v, want reserved transition", err)
	}
	if _, err := client.DeleteUpload(context.Background(), first.URL); err != nil {
		t.Fatalf("resume reserved standalone delete: %v", err)
	}
	if _, ok := store.get(first.MarkerKey); ok {
		t.Fatal("reserved standalone delete did not remove marker")
	}
	if body, ok := store.get(dirPrefix + VersionsFilename); !ok ||
		!bytes.Equal(body, standaloneDeleteReservationBody) {
		t.Fatal("reserved standalone delete did not retain its tombstone")
	}
}

func TestConcurrentAppendAndTargetedDeleteHaveOneSerializationWinner(t *testing.T) {
	for _, test := range []struct {
		name             string
		deleteLatest     bool
		deleteWinnerLast int
	}{
		{name: "latest", deleteLatest: true, deleteWinnerLast: 1},
		{name: "historical", deleteWinnerLast: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			store := newUpgradeStore(t)
			client := store.client(t, "")
			first, err := client.Upload(context.Background(), Input{
				Reader: strings.NewReader("one\n"), Name: "plan.md",
			})
			if err != nil {
				t.Fatal(err)
			}
			second, err := client.UpdateDocument(context.Background(), UpdateDocumentInput{
				Target: first.URL, Input: Input{Reader: strings.NewReader("two\n")},
			})
			if err != nil {
				t.Fatal(err)
			}
			deleteTarget, survivor := first.URL, second.URL
			if test.deleteLatest {
				deleteTarget, survivor = second.URL, first.URL
			}
			store.mu.Lock()
			store.ifMatchBarrier = make(chan struct{})
			store.mu.Unlock()

			type outcome struct {
				operation string
				err       error
			}
			results := make(chan outcome, 2)
			go func() {
				_, updateErr := client.UpdateDocument(context.Background(), UpdateDocumentInput{
					Target: second.URL, Input: Input{Reader: strings.NewReader("three\n")},
				})
				results <- outcome{operation: "append", err: updateErr}
			}()
			go func() {
				_, deleteErr := client.DeleteUpload(context.Background(), deleteTarget)
				results <- outcome{operation: "delete", err: deleteErr}
			}()

			winner := ""
			for range 2 {
				result := <-results
				if result.err == nil {
					if winner != "" {
						t.Fatalf("both append and delete succeeded")
					}
					winner = result.operation
				} else if !errors.Is(result.err, ErrConflict) {
					t.Fatalf("%s error = %v, want conflict", result.operation, result.err)
				}
			}
			if winner == "" {
				t.Fatal("neither append nor delete won")
			}
			inspection, err := client.InspectUpload(context.Background(), survivor)
			if err != nil || inspection.Versions == nil {
				t.Fatalf("survivor inspection = %+v, %v", inspection, err)
			}
			if winner == "append" && inspection.LatestRevision != 3 {
				t.Fatalf("append winner latest = %d, want 3", inspection.LatestRevision)
			}
			if winner == "delete" && inspection.LatestRevision != test.deleteWinnerLast {
				t.Fatalf("delete winner latest = %d, want %d",
					inspection.LatestRevision, test.deleteWinnerLast)
			}
		})
	}
}
