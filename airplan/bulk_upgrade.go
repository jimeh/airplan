package airplan

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// BulkUpgradeOptions controls manifest-backed upgrade planning.
type BulkUpgradeOptions struct {
	Concurrency int `json:"concurrency,omitempty"`
}

// BulkUpgradePlan is an ordered read-only classification of active records.
type BulkUpgradePlan struct {
	Items    []UpgradeDocumentPlan `json:"items"`
	Warnings []string              `json:"warnings,omitempty"`
	Counts   map[UpgradeState]int  `json:"counts"`
}

// BulkUpgradeRequest executes only exact upgradeable items from a preview.
type BulkUpgradeRequest struct {
	Items       []UpgradeDocumentPlan `json:"items"`
	Concurrency int                   `json:"concurrency,omitempty"`
}

// BulkUpgradeItemResult reports one independent planned item.
type BulkUpgradeItemResult struct {
	Plan   UpgradeDocumentPlan    `json:"plan"`
	Result *UpgradeDocumentResult `json:"result,omitempty"`
	Error  string                 `json:"error,omitempty"`
}

// BulkUpgradeResult preserves preview order while independent work proceeds.
type BulkUpgradeResult struct {
	Items    []BulkUpgradeItemResult `json:"items"`
	Upgraded int                     `json:"upgraded"`
	Failed   int                     `json:"failed"`
}

// PlanBulkUpgrade classifies active records in the selected service manifest.
func (c *Client) PlanBulkUpgrade(
	ctx context.Context, opts BulkUpgradeOptions,
) (*BulkUpgradePlan, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	concurrency, err := normalizeBulkUpgradeConcurrency(opts.Concurrency)
	if err != nil {
		return nil, err
	}
	opts.Concurrency = concurrency
	if c.remote != nil {
		return c.remote.PlanBulkUpgrade(ctx, opts)
	}
	listed, err := c.ListManifest(ctx, ListManifestOptions{Scope: ManifestScopeService})
	if err != nil {
		return nil, err
	}
	plan := &BulkUpgradePlan{
		Items:    []UpgradeDocumentPlan{},
		Warnings: append([]string(nil), listed.Warnings...),
		Counts:   map[UpgradeState]int{},
	}
	seen := map[string]struct{}{}
	candidates := make([]ManifestRecord, 0, len(listed.Records))
	for _, record := range listed.Records {
		if record.MarkerKey == "" || record.MarkerVersion == 0 {
			continue
		}
		identity := record.Bucket + "\x00" + record.MarkerKey
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		candidates = append(candidates, record)
	}
	plan.Items = make([]UpgradeDocumentPlan, len(candidates))
	warnings := make([]string, len(candidates))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				record := candidates[index]
				item, planErr := c.PlanUpgradeDocument(ctx, record.MarkerKey,
					UpgradeDocumentOptions{})
				if planErr != nil {
					item = &UpgradeDocumentPlan{
						Target: record.MarkerKey, Profile: record.Profile,
						Bucket: record.Bucket, State: UpgradeStateInvalid,
						Reason:                   "remote inspection failed",
						TargetMarkerVersion:      MarkerVersion,
						TargetProducerVersion:    producerVersion(c.cfg.ProducerVersion),
						TargetRendererGeneration: RendererGeneration,
					}
					warnings[index] = fmt.Sprintf(
						"could not inspect %s: %v", record.MarkerKey, planErr,
					)
				}
				plan.Items[index] = *item
			}
		}()
	}
	for index := range candidates {
		select {
		case jobs <- index:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return plan, ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	for index, item := range plan.Items {
		plan.Counts[item.State]++
		if warnings[index] != "" {
			plan.Warnings = append(plan.Warnings, warnings[index])
		}
	}
	return plan, nil
}

// ExecuteBulkUpgrade applies exact preview items with bounded concurrency.
func (c *Client) ExecuteBulkUpgrade(
	ctx context.Context, req BulkUpgradeRequest,
) (*BulkUpgradeResult, error) {
	if err := c.validate(ctx); err != nil {
		return nil, err
	}
	concurrency, err := normalizeBulkUpgradeConcurrency(req.Concurrency)
	if err != nil {
		return nil, err
	}
	req.Concurrency = concurrency
	if c.remote != nil {
		return c.remote.ExecuteBulkUpgrade(ctx, req)
	}
	result := &BulkUpgradeResult{Items: make([]BulkUpgradeItemResult, len(req.Items))}
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range concurrency {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				plan := req.Items[index]
				item := BulkUpgradeItemResult{Plan: plan}
				if plan.State != UpgradeStateUpgradeable {
					item.Error = "candidate is not upgradeable"
					result.Items[index] = item
					continue
				}
				upgraded, err := c.UpgradeDocument(ctx, plan)
				if err != nil {
					item.Error = err.Error()
				} else {
					item.Result = upgraded
				}
				result.Items[index] = item
			}
		}()
	}
	for index := range req.Items {
		select {
		case jobs <- index:
		case <-ctx.Done():
			for remaining := index; remaining < len(req.Items); remaining++ {
				result.Items[remaining] = BulkUpgradeItemResult{
					Plan:  req.Items[remaining],
					Error: "upgrade context expired before start",
				}
			}
			close(jobs)
			wg.Wait()
			return countBulkUpgradeResult(result), ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return countBulkUpgradeResult(result), nil
}

func normalizeBulkUpgradeConcurrency(value int) (int, error) {
	if value == 0 {
		return 4, nil
	}
	if value < 0 {
		return 0, errors.New("airplan: bulk upgrade concurrency must not be negative")
	}
	if value > 32 {
		return 0, errors.New("airplan: bulk upgrade concurrency exceeds 32")
	}
	return value, nil
}

func countBulkUpgradeResult(result *BulkUpgradeResult) *BulkUpgradeResult {
	for _, item := range result.Items {
		if item.Result != nil && item.Result.Upgraded {
			result.Upgraded++
		}
		if item.Error != "" {
			result.Failed++
		}
	}
	return result
}
