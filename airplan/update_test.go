package airplan

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
)

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
		BuildKey("", second.ID, "plan.md"),
		BuildKey("", second.ID, DiffFilename),
		BuildKey("", second.ID, "plan.html"),
		second.MarkerKey,
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
			t.Fatalf("candidate PUT order = %v; %q was not marker-last", putKeys, want)
		}
	}
}

func TestUpdateDocumentCandidateFailureNeverPublishesMarker(t *testing.T) {
	store := newUpgradeStore(t)
	client := store.client(t, "")
	first, err := client.Upload(context.Background(), Input{
		Reader: strings.NewReader("one\n"), Name: "plan.md",
	})
	if err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	store.failPutSuffix = "/plan.html"
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
