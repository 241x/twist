package app

import (
	"context"
	"fmt"
)

type Target struct {
	cdp *CDP
}

func NewTarget(cdp *CDP) *Target {
	return &Target{cdp: cdp}
}

func (t *Target) Select(ctx context.Context, targetID string, url string) (*CDPTarget, error) {
	if targetID != "" {
		return t.selectByID(ctx, targetID)
	}

	if url != "" {
		return t.createAndNavigate(ctx, url)
	}

	return t.selectFirstPage(ctx)
}

func (t *Target) selectByID(ctx context.Context, targetID string) (*CDPTarget, error) {
	targets, err := t.cdp.ListTargets(ctx)
	if err != nil {
		return nil, err
	}

	for _, target := range targets {
		if target.ID == targetID {
			return &target, nil
		}
	}

	return nil, fmt.Errorf("target %q not found", targetID)
}

func (t *Target) createAndNavigate(ctx context.Context, url string) (*CDPTarget, error) {
	target, err := t.cdp.NewTab(ctx)
	if err != nil {
		return nil, err
	}

	if err := t.cdp.NavigateTo(ctx, target.ID, url); err != nil {
		return nil, err
	}

	return target, nil
}

func (t *Target) selectFirstPage(ctx context.Context) (*CDPTarget, error) {
	targets, err := t.cdp.ListTargets(ctx)
	if err != nil {
		return nil, err
	}

	for _, target := range targets {
		if target.Type == "page" {
			return &target, nil
		}
	}

	return nil, fmt.Errorf("no page target found")
}
