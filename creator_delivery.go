package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type deliveryInput struct {
	AssetID      string `json:"asset_id"`
	JobAssetID   string `json:"job_asset_id"`
	JobID        string `json:"job_id"`
	JobStatus    string `json:"job_status"`
	CreatorEmail string `json:"creator_email"`
	Domain       string `json:"domain"`
	SPF          string `json:"spf"`
	DKIMName     string `json:"dkim_name"`
	DKIMContent  string `json:"dkim_content"`
}

type deliveryResult struct {
	AssetID      string `json:"asset_id"`
	JobID        string `json:"job_id"`
	CreatorEmail string `json:"creator_email"`
	ZoneID       string `json:"zone_id"`
	MailStatus   string `json:"mail_status"`
	State        string `json:"state"`
}

type mailDomain interface {
	zone(context.Context, string) (zone, error)
	record(context.Context, string, string, string, string) error
	verify(context.Context, string) (verification, error)
}

func prepareDelivery(ctx context.Context, api mailDomain, in deliveryInput) (deliveryResult, error) {
	if in.AssetID == "" || in.JobID == "" || in.JobAssetID != in.AssetID || in.JobStatus != "completed" {
		return deliveryResult{}, errors.New("asset must have a completed processing job for the same asset")
	}
	if in.Domain == "" || in.CreatorEmail == "" || in.SPF == "" || in.DKIMName == "" || in.DKIMContent == "" {
		return deliveryResult{}, errors.New("creator, domain, SPF and DKIM records are required")
	}
	if !strings.HasSuffix(strings.ToLower(in.CreatorEmail), "@"+strings.ToLower(in.Domain)) {
		return deliveryResult{}, errors.New("creator address must belong to the sending domain")
	}
	z, err := api.zone(ctx, in.Domain)
	if err != nil {
		return deliveryResult{}, err
	}
	if z.ZoneID == "" {
		return deliveryResult{}, errors.New("domain response has no zone_id")
	}
	for _, rec := range []struct{ kind, name, value string }{
		{"TXT", in.Domain, in.SPF},
		{"CNAME", in.DKIMName, in.DKIMContent},
	} {
		if err := api.record(ctx, z.ZoneID, rec.kind, rec.name, rec.value); err != nil {
			return deliveryResult{}, fmt.Errorf("write %s %s: %w", rec.kind, rec.name, err)
		}
	}
	v, err := api.verify(ctx, in.Domain)
	if err != nil {
		return deliveryResult{}, err
	}
	state := "awaiting_domain_verification"
	if v.Verification.Status == "verified" {
		state = "ready_for_creator_delivery"
	}
	return deliveryResult{in.AssetID, in.JobID, in.CreatorEmail, z.ZoneID, v.Verification.Status, state}, nil
}
