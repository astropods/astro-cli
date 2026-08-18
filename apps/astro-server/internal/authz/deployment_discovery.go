package authz

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/account"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"
)

const deploymentDiscoveryCacheTTL = 3 * time.Second

// DeploymentVisibility is the FGA portion of one deployment-list request.
// Accounts absent from FGAAccountIDs retain legacy membership visibility.
type DeploymentVisibility struct {
	FGAAccountIDs         []string `json:"fga_account_ids,omitempty"`
	ReadableDeploymentIDs []string `json:"readable_deployment_ids,omitempty"`
}

func (v DeploymentVisibility) EnforcesAccount(accountID string) bool {
	return slices.Contains(v.FGAAccountIDs, accountID)
}

type deploymentManagedAccountStore interface {
	AccountsWithManagedDeployments(context.Context, []string) ([]string, error)
}

type deploymentDiscoveryMemberStore interface {
	GetMember(accountID, userID string) (*account.AccountMember, error)
}

// DeploymentDiscovery resolves list visibility with one WorkOS resource-list
// request per selected, opted-in organization rather than one check per card.
type DeploymentDiscovery struct {
	active     bool
	discovery  ResourceDiscovery
	experiment AccountExperimentGate
	managed    deploymentManagedAccountStore
	members    deploymentDiscoveryMemberStore

	cacheMu         sync.Mutex
	cache           map[deploymentDiscoveryCacheKey]deploymentDiscoveryCacheEntry
	cacheGeneration map[string]uint64
	cacheGroup      singleflight.Group
}

type deploymentDiscoveryCacheKey struct {
	accountID      string
	membershipID   string
	organizationID string
	generation     uint64
}

type deploymentDiscoveryCacheEntry struct {
	resources []ResourceRef
	expiresAt time.Time
}

func NewDeploymentDiscovery(
	active bool,
	discovery ResourceDiscovery,
	experiment AccountExperimentGate,
	managed deploymentManagedAccountStore,
	members deploymentDiscoveryMemberStore,
) *DeploymentDiscovery {
	return &DeploymentDiscovery{
		active: active, discovery: discovery, experiment: experiment, managed: managed, members: members,
		cache:           make(map[deploymentDiscoveryCacheKey]deploymentDiscoveryCacheEntry),
		cacheGeneration: make(map[string]uint64),
	}
}

func (d *DeploymentDiscovery) Active() bool {
	return d != nil && d.active
}

// InvalidateAccount drops discovery results after an experiment change.
func (d *DeploymentDiscovery) InvalidateAccount(accountID string) {
	if d == nil || accountID == "" {
		return
	}
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	d.cacheGeneration[accountID]++
	for key := range d.cache {
		if key.accountID == accountID {
			delete(d.cache, key)
		}
	}
}

// Visible discovers deployment:read resources for selected organizations that
// have entered the PR4 lifecycle. Historical deployments without a lifecycle
// row retain legacy visibility until backfill; pending rows fail closed.
func (d *DeploymentDiscovery) Visible(
	ctx context.Context,
	userID string,
	accounts []account.AccountWithRole,
) (DeploymentVisibility, error) {
	if !d.Active() {
		return DeploymentVisibility{}, nil
	}
	if d.discovery == nil || d.experiment == nil || d.managed == nil || d.members == nil {
		return DeploymentVisibility{}, errors.New("deployment discovery is not configured")
	}

	accountIDs := make([]string, 0, len(accounts))
	byID := make(map[string]account.AccountWithRole, len(accounts))
	for _, acct := range accounts {
		if acct.Type != "organization" || acct.WorkOSOrganizationID == "" {
			continue
		}
		accountIDs = append(accountIDs, acct.ID)
		byID[acct.ID] = acct
	}
	if len(accountIDs) == 0 {
		return DeploymentVisibility{}, nil
	}
	managedIDs, err := d.managed.AccountsWithManagedDeployments(ctx, accountIDs)
	if err != nil {
		return DeploymentVisibility{}, fmt.Errorf("resolve FGA-managed deployment accounts: %w", err)
	}

	var mu sync.Mutex
	visibility := DeploymentVisibility{}
	// A failed organization stays in the enforced set with no discovered IDs.
	// That org fails closed without hiding healthy or legacy organizations.
	markEnforced := func(accountID string, resources []ResourceRef) {
		mu.Lock()
		defer mu.Unlock()
		visibility.FGAAccountIDs = append(visibility.FGAAccountIDs, accountID)
		for _, resource := range resources {
			if resource.Type == ResourceDeployment && resource.ExternalID != "" {
				visibility.ReadableDeploymentIDs = append(visibility.ReadableDeploymentIDs, resource.ExternalID)
			}
		}
	}
	var group errgroup.Group
	group.SetLimit(4)
	for _, accountID := range managedIDs {
		acct, ok := byID[accountID]
		if !ok {
			continue
		}
		group.Go(func() error {
			resources, enforced := d.visibleAccount(ctx, userID, acct)
			if enforced {
				markEnforced(acct.ID, resources)
			}
			return nil
		})
	}
	_ = group.Wait()

	visibility.FGAAccountIDs = sortedUnique(visibility.FGAAccountIDs)
	visibility.ReadableDeploymentIDs = sortedUnique(visibility.ReadableDeploymentIDs)
	return visibility, nil
}

// visibleAccount returns enforced=true for opted-in accounts even when
// membership or WorkOS discovery fails, keeping that account fail-closed.
func (d *DeploymentDiscovery) visibleAccount(
	ctx context.Context,
	userID string,
	acct account.AccountWithRole,
) ([]ResourceRef, bool) {
	generation := d.accountCacheGeneration(acct.ID)
	enabled, err := d.experiment.Enabled(ctx, acct.ID)
	if err != nil {
		return nil, true
	}
	if !enabled {
		return nil, false
	}
	member, err := d.members.GetMember(acct.ID, userID)
	if err != nil || member.WorkOSMembershipID == "" {
		return nil, true
	}
	resources, err := d.cachedResources(
		ctx, acct.ID, member.WorkOSMembershipID, acct.WorkOSOrganizationID, generation,
	)
	if err != nil {
		return nil, true
	}
	return resources, true
}

func (d *DeploymentDiscovery) cachedResources(
	ctx context.Context,
	accountID string,
	membershipID string,
	organizationID string,
	generation uint64,
) ([]ResourceRef, error) {
	key := deploymentDiscoveryCacheKey{
		accountID: accountID, membershipID: membershipID, organizationID: organizationID, generation: generation,
	}
	if resources, ok := d.cached(key); ok {
		return resources, nil
	}

	flightKey := accountID + "\x00" + membershipID + "\x00" + organizationID + "\x00" + strconv.FormatUint(generation, 10)
	value, err, _ := d.cacheGroup.Do(flightKey, func() (any, error) {
		if resources, ok := d.cached(key); ok {
			return resources, nil
		}
		resources, err := d.discovery.ListResources(
			ctx,
			membershipID,
			ActionDeploymentRead,
			ResourceRef{Type: ResourceOrganization, ExternalID: organizationID},
		)
		if err != nil {
			return nil, err
		}
		d.storeCached(key, resources)
		return slices.Clone(resources), nil
	})
	if err != nil {
		return nil, err
	}
	return value.([]ResourceRef), nil
}

func (d *DeploymentDiscovery) cached(key deploymentDiscoveryCacheKey) ([]ResourceRef, bool) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	entry, ok := d.cache[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(d.cache, key)
		return nil, false
	}
	return slices.Clone(entry.resources), true
}

func (d *DeploymentDiscovery) storeCached(key deploymentDiscoveryCacheKey, resources []ResourceRef) {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	if d.cacheGeneration[key.accountID] != key.generation {
		return
	}
	now := time.Now()
	for cachedKey, entry := range d.cache {
		if now.After(entry.expiresAt) {
			delete(d.cache, cachedKey)
		}
	}
	d.cache[key] = deploymentDiscoveryCacheEntry{
		resources: slices.Clone(resources),
		expiresAt: now.Add(deploymentDiscoveryCacheTTL),
	}
}

func (d *DeploymentDiscovery) accountCacheGeneration(accountID string) uint64 {
	d.cacheMu.Lock()
	defer d.cacheMu.Unlock()
	return d.cacheGeneration[accountID]
}

func sortedUnique(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}
