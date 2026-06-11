package app

import (
	"context"
)

type Intercept struct {
	cdp    *CDP
	config *Config
}

func NewIntercept(cdp *CDP, config *Config) *Intercept {
	return &Intercept{
		cdp:    cdp,
		config: config,
	}
}

func (i *Intercept) Start(ctx context.Context, targetID string) error {
	if err := i.cdp.EnableNetwork(ctx, targetID); err != nil {
		return err
	}

	if err := i.cdp.SetRequestInterceptor(ctx, targetID); err != nil {
		return err
	}

	if err := i.cdp.SetResponseInterceptor(ctx, targetID); err != nil {
		return err
	}

	return nil
}

func (i *Intercept) Wait(ctx context.Context) error {
	<-ctx.Done()
	return nil
}
