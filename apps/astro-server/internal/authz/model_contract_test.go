package authz_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/astropods/astro/apps/astro-server/internal/authz"
)

// modelPath is the file the WorkOS Authorization model is applied from. A slug
// Astro checks but WorkOS does not have fails closed, which is why these are
// compared rather than trusted.
const modelPath = "../../../../scripts/workos-fga/model.json"

type workosModel struct {
	Permissions []struct {
		Slug         string `json:"slug"`
		ResourceType string `json:"resourceType"`
	} `json:"permissions"`
	Roles []struct {
		Slug         string   `json:"slug"`
		ResourceType string   `json:"resourceType"`
		Permissions  []string `json:"permissions"`
	} `json:"roles"`
}

// modelledTypes are the resource types Astro registers roles for. Audience and
// knowledge store exist in WorkOS but have no Astro role yet.
var modelledTypes = []authz.ResourceType{
	authz.ResourceAccount,
	authz.ResourceBlueprint,
	authz.ResourceDeployment,
}

func loadWorkOSModel(t *testing.T) workosModel {
	t.Helper()
	raw, err := os.ReadFile(filepath.Clean(modelPath))
	if err != nil {
		t.Fatalf("read WorkOS model: %v", err)
	}
	var model workosModel
	if err := json.Unmarshal(raw, &model); err != nil {
		t.Fatalf("parse WorkOS model: %v", err)
	}
	if len(model.Permissions) == 0 || len(model.Roles) == 0 {
		t.Fatal("WorkOS model is empty")
	}
	return model
}

func TestRoleBundlesMatchWorkOSModel(t *testing.T) {
	t.Parallel()

	model := loadWorkOSModel(t)
	wanted := make(map[authz.RoleSlug][]string, len(model.Roles))
	for _, role := range model.Roles {
		wanted[authz.RoleSlug(role.Slug)] = role.Permissions
	}

	for _, resourceType := range modelledTypes {
		roles := authz.ResourceRoles(resourceType)
		if len(roles) == 0 {
			t.Fatalf("no roles registered for %q", resourceType)
		}
		for _, role := range roles {
			want, ok := wanted[role.Slug]
			if !ok {
				t.Errorf("role %q is not in the WorkOS model", role.Slug)
				continue
			}
			got := make([]string, 0, len(role.Actions))
			for _, action := range role.Actions {
				got = append(got, string(action))
			}
			slices.Sort(got)
			want = slices.Clone(want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Errorf("role %q permissions = %v, want %v", role.Slug, got, want)
			}
		}
	}
}

// Every role WorkOS defines for a modelled type has an Astro counterpart, so a
// role added in the dashboard cannot go unnoticed.
func TestEveryModelledWorkOSRoleIsRegistered(t *testing.T) {
	t.Parallel()

	model := loadWorkOSModel(t)
	for _, role := range model.Roles {
		resourceType := authz.ResourceType(role.ResourceType)
		if !slices.Contains(modelledTypes, resourceType) {
			continue
		}
		if _, ok := authz.AccessLevelForRole(resourceType, authz.RoleSlug(role.Slug)); !ok {
			t.Errorf("WorkOS role %q on %q is not registered in the catalog", role.Slug, role.ResourceType)
		}
	}
}

// Every permission slug Astro can name exists in WorkOS, and every WorkOS
// permission has a constant, so the two vocabularies stay one vocabulary.
func TestActionVocabularyMatchesWorkOSModel(t *testing.T) {
	t.Parallel()

	model := loadWorkOSModel(t)
	inModel := make(map[string]struct{}, len(model.Permissions))
	for _, permission := range model.Permissions {
		inModel[permission.Slug] = struct{}{}
	}

	inAstro := make(map[string]struct{}, len(inModel))
	for _, resourceType := range modelledTypes {
		for _, role := range authz.ResourceRoles(resourceType) {
			for _, action := range role.Actions {
				inAstro[string(action)] = struct{}{}
				if _, ok := inModel[string(action)]; !ok {
					t.Errorf("Astro action %q is not a WorkOS permission", action)
				}
			}
		}
	}

	// account-admin holds every permission, so the two sets are the same size.
	for slug := range inModel {
		if _, ok := inAstro[slug]; !ok {
			t.Errorf("WorkOS permission %q has no Astro action", slug)
		}
	}
}
