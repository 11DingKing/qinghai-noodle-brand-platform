package brand

import (
	"context"
	"fmt"
	"slices"
	"time"
)

type Service struct {
	registry *Registry
	now      func() time.Time
}

func NewService(registry *Registry, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{registry: registry, now: now}
}

func (s *Service) ActivateLicense(ctx context.Context, license BrandLicense, store StoreProfile) (BrandLicense, error) {
	if err := ValidateThreeStoreModel(store); err != nil {
		return BrandLicense{}, err
	}
	if err := ValidateLogoVersion(license, store); err != nil {
		return BrandLicense{}, err
	}
	if err := ValidateMenuBrandName(store); err != nil {
		return BrandLicense{}, err
	}
	license.Status = LicenseActive
	if err := ValidateLicenseAt(license, store, s.now()); err != nil {
		return BrandLicense{}, err
	}
	if err := s.registry.SaveLicense(ctx, license, license.Version); err != nil {
		return BrandLicense{}, fmt.Errorf("activate brand license: %w", err)
	}
	license.Version++
	return license, nil
}

func (s *Service) RenewLicense(ctx context.Context, licenseID string, store StoreProfile, extension time.Duration) (BrandLicense, error) {
	license, ok, err := s.registry.License(ctx, licenseID)
	if err != nil {
		return BrandLicense{}, err
	}
	if !ok {
		return BrandLicense{}, fmt.Errorf("%w: license %s not found", ErrInvalidLicense, licenseID)
	}
	if extension <= 0 {
		return BrandLicense{}, fmt.Errorf("%w: extension must be positive", ErrLicenseNotRenewable)
	}
	if err := CanRenewLicense(license, store, s.now()); err != nil {
		return BrandLicense{}, err
	}
	version := license.Version
	license.EffectiveTo = license.EffectiveTo.Add(extension)
	if err := s.registry.SaveLicense(ctx, license, version); err != nil {
		return BrandLicense{}, fmt.Errorf("renew brand license: %w", err)
	}
	license.Version = version + 1
	return license, nil
}

func (s *Service) CompleteInspection(ctx context.Context, inspection Inspection, passScore int) (Inspection, error) {
	if inspection.CompletedAt.Before(inspection.StartedAt) || inspection.CompletedAt.After(s.now()) {
		return Inspection{}, fmt.Errorf("%w: inspection time range is invalid", ErrStoreNotCompliant)
	}
	score, passed, err := CalculateInspection(inspection.Sections, passScore)
	if err != nil {
		return Inspection{}, err
	}
	version := inspection.Version
	inspection.Score = score
	inspection.Passed = passed
	if err := s.registry.SaveInspection(ctx, inspection, version); err != nil {
		return Inspection{}, fmt.Errorf("complete inspection: %w", err)
	}
	inspection.Version = version + 1
	return inspection, nil
}

func (s *Service) PublishStoreCatalog(ctx context.Context, store StoreProfile, license BrandLicense, listings []ProductListing, lots map[string]IngredientLot) (CatalogSnapshot, error) {
	if err := ValidateLicenseAt(license, store, s.now()); err != nil {
		return CatalogSnapshot{}, err
	}
	for _, listing := range listings {
		if listing.StoreID != store.ID {
			return CatalogSnapshot{}, fmt.Errorf("%w: listing %s belongs to another store", ErrStoreNotCompliant, listing.ID)
		}
		if err := ValidateHighlandProduct(listing, lots); err != nil {
			return CatalogSnapshot{}, err
		}
		if err := ValidateListingPrices(listing); err != nil {
			return CatalogSnapshot{}, err
		}
	}
	snapshot := CatalogSnapshot{StoreID: store.ID, Listings: slices.Clone(listings), GeneratedAt: s.now().UTC()}
	if err := s.registry.PublishCatalog(ctx, snapshot); err != nil {
		return CatalogSnapshot{}, err
	}
	return CloneCatalog(snapshot), nil
}

func (s *Service) CloseRecall(ctx context.Context, recall Recall) (Recall, error) {
	if err := ctx.Err(); err != nil {
		return Recall{}, fmt.Errorf("close recall: %w", err)
	}
	now := s.now().UTC()
	if err := CanCloseRecall(recall, now); err != nil {
		return Recall{}, err
	}
	recall.ClosedAt = &now
	recall.AffectedStores = slices.Clone(recall.AffectedStores)
	recall.Acknowledged = slices.Clone(recall.Acknowledged)
	return recall, nil
}

func (s *Service) LaunchCampaign(ctx context.Context, campaign CultureCampaign, store StoreProfile, license BrandLicense) (CultureCampaign, error) {
	if err := ctx.Err(); err != nil {
		return CultureCampaign{}, fmt.Errorf("launch campaign: %w", err)
	}
	if err := ValidateLicenseAt(license, store, s.now()); err != nil {
		return CultureCampaign{}, err
	}
	if err := ValidateDestinationCoverage(campaign, license.RegionCodes); err != nil {
		return CultureCampaign{}, err
	}
	snapshot, ok, err := s.registry.Catalog(ctx, store.ID)
	if err != nil {
		return CultureCampaign{}, err
	}
	if !ok {
		return CultureCampaign{}, fmt.Errorf("%w: store catalog is not published", ErrCampaignNotApproved)
	}
	listings := make(map[string]ProductListing, len(snapshot.Listings))
	for _, listing := range snapshot.Listings {
		listings[listing.SKU] = listing
	}
	if err := ValidateCampaign(campaign, store, listings, s.now()); err != nil {
		return CultureCampaign{}, err
	}
	return campaign, nil
}
