package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"

	spec "github.com/astropods/astro-spec"
	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/deployment"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	"github.com/gin-gonic/gin"
)

type ListAccountVariablesResponse struct {
	Variables []accountvars.VariableMetadata `json:"variables"`
}

type CreateAccountVariableEntry struct {
	Name        string `json:"name" binding:"required"`
	Value       string `json:"value" binding:"required"`
	Secret      bool   `json:"secret"`
	Description string `json:"description"`
}

type CreateAccountVariablesRequest struct {
	Variables []CreateAccountVariableEntry `json:"variables" binding:"required,dive"`
}

type CreateVariableResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

type CreateAccountVariablesResponse struct {
	Results []CreateVariableResult `json:"results"`
}

type UpdateAccountVariableRequest struct {
	Value       *string `json:"value"`
	Secret      *bool   `json:"secret"`
	Description *string `json:"description"`
}

// ListAccountVariables returns metadata for all variables in the account.
// Secret variable values are never returned; plaintext variable values are included.
// GET /api/v1/accounts/:account/variables
func ListAccountVariables(log *logger.Logger, store *accountvars.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		vars, err := store.List(acct.ID)
		if err != nil {
			log.Error("account secrets: list account variables failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list variables"})
			return
		}
		if vars == nil {
			vars = []accountvars.VariableMetadata{}
		}

		c.JSON(http.StatusOK, ListAccountVariablesResponse{Variables: vars})
	}
}

// CreateAccountVariable stores one or more account variables (optionally encrypted).
// The request body is { "variables": [ ... ] }. Each entry is saved via upsert.
// POST /api/v1/accounts/:account/variables
func CreateAccountVariable(log *logger.Logger, store *accountvars.Store, vault *envelope.Vault) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		var req CreateAccountVariablesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
			return
		}

		if len(req.Variables) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one variable is required"})
			return
		}

		// Set up encryptor once if any entries are secrets.
		var enc *envelope.Encryptor
		for _, e := range req.Variables {
			if e.Secret {
				var err error
				enc, err = getAccountEncryptor(c, log, store, acct.ID, vault)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set up encryption"})
					return
				}
				break
			}
		}

		results := make([]CreateVariableResult, 0, len(req.Variables))
		for _, entry := range req.Variables {
			entry.Name = strings.TrimSpace(entry.Name)
			result := CreateVariableResult{Name: entry.Name}

			if !spec.IsValidVarName(entry.Name) {
				result.Status = "error"
				result.Error = "invalid variable name"
				results = append(results, result)
				continue
			}

			v := &accountvars.AccountVariable{
				AccountID:   acct.ID,
				Name:        entry.Name,
				Secret:      entry.Secret,
				Description: entry.Description,
			}

			if err := applyValue(v, entry.Value, enc); err != nil {
				log.Error("account secrets: encrypt variable failed", "error", err, "account_id", acct.ID, "name", entry.Name)
				result.Status = "error"
				result.Error = "failed to encrypt"
				results = append(results, result)
				continue
			}

			if err := store.Save(v); err != nil {
				log.Error("account secrets: save account variable failed", "error", err, "account_id", acct.ID, "name", entry.Name)
				result.Status = "error"
				result.Error = "failed to save"
				results = append(results, result)
				continue
			}
			result.Status = "created"
			results = append(results, result)
			log.Info("account secrets: account variable created", "account_id", acct.ID, "name", entry.Name, "secret", entry.Secret)
		}

		c.JSON(http.StatusOK, CreateAccountVariablesResponse{Results: results})
	}
}

// GetAccountVariable returns metadata for a single variable.
// For plaintext variables the value is included; for secrets it is omitted.
// GET /api/v1/accounts/:account/variables/:varName
func GetAccountVariable(log *logger.Logger, store *accountvars.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		varName := c.Param("varName")
		v, err := store.Get(acct.ID, varName)
		if err != nil {
			log.Error("account secrets: get account variable failed", "error", err, "account_id", acct.ID, "name", varName)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get variable"})
			return
		}
		if v == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "variable not found"})
			return
		}

		meta := accountvars.VariableMetadata{
			Name:        v.Name,
			Secret:      v.Secret,
			Description: v.Description,
			CreatedAt:   v.CreatedAt,
			UpdatedAt:   v.UpdatedAt,
		}
		if !v.Secret {
			meta.Value = &v.Value
		}
		c.JSON(http.StatusOK, meta)
	}
}

// UpdateAccountVariable updates an existing account variable.
// PUT /api/v1/accounts/:account/variables/:varName
func UpdateAccountVariable(log *logger.Logger, store *accountvars.Store, vault *envelope.Vault) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		varName := c.Param("varName")

		existing, err := store.Get(acct.ID, varName)
		if err != nil {
			log.Error("account secrets: get variable for update failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update variable"})
			return
		}
		if existing == nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "variable not found"})
			return
		}

		var req UpdateAccountVariableRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body", "details": err.Error()})
			return
		}

		if req.Value == nil && req.Description == nil && req.Secret == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "at least one of value, secret, or description must be provided"})
			return
		}

		if req.Description != nil {
			existing.Description = *req.Description
		}

		// Determine the effective secret flag
		isSecret := existing.Secret
		if req.Secret != nil {
			isSecret = *req.Secret
			existing.Secret = isSecret
		}

		if req.Value != nil {
			enc, err := getAccountEncryptor(c, log, store, acct.ID, vault)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to set up encryption"})
				return
			}
			if err := applyValue(existing, *req.Value, enc); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt variable"})
				return
			}
		} else if req.Secret != nil && existing.Secret != isSecret {
			// Secret flag changed without a new value — re-encrypt or decrypt existing
			// This is tricky so we require a new value when changing the secret flag
			c.JSON(http.StatusBadRequest, gin.H{"error": "value must be provided when changing the secret flag"})
			return
		}

		if err := store.Save(existing); err != nil {
			log.Error("account secrets: update account variable failed", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update variable"})
			return
		}

		log.Info("account secrets: account variable updated", "account_id", acct.ID, "name", varName)
		c.JSON(http.StatusOK, gin.H{"name": varName, "message": "variable updated"})
	}
}

// DeleteAccountVariable removes an account variable.
// DELETE /api/v1/accounts/:account/variables/:varName
func DeleteAccountVariable(log *logger.Logger, store *accountvars.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		varName := c.Param("varName")

		if err := store.Delete(acct.ID, varName); err != nil {
			log.Error("account secrets: delete account variable failed", "error", err, "account_id", acct.ID, "name", varName)
			c.JSON(http.StatusNotFound, gin.H{"error": "variable not found"})
			return
		}
		log.Info("account secrets: account variable deleted", "account_id", acct.ID, "name", varName)
		c.JSON(http.StatusOK, gin.H{"message": "variable deleted"})
	}
}

func getAccountEncryptor(c *gin.Context, log *logger.Logger, store *accountvars.Store, accountID string, vault *envelope.Vault) (*envelope.Encryptor, error) {
	ctx := c.Request.Context()

	ek, err := store.GetEncryptionKey(accountID)
	if err != nil {
		log.Error("account secrets: get account encryption key failed", "error", err, "account_id", accountID)
		return nil, err
	}

	if ek == nil {
		enc, err := vault.Encryptor(ctx)
		if err != nil {
			log.Error("account secrets: generate KMS data key failed", "error", err, "account_id", accountID)
			return nil, err
		}
		if err := store.SaveEncryptionKey(accountID, enc.EncryptedDataKey, enc.KMSKeyARN); err != nil {
			log.Error("account secrets: save account encryption key failed", "error", err, "account_id", accountID)
			return nil, err
		}
		return enc, nil
	}

	enc, err := vault.EncryptorFor(ctx, ek.EncryptedDataKey)
	if err != nil {
		log.Error("account secrets: create encryptor from stored data key failed", "error", err, "account_id", accountID)
		return nil, err
	}

	return enc, nil
}

// applyValue sets v.Value and v.Nonce from plaintext.
// With KMS (enc != nil): encrypts and base64-encodes, sets nonce.
// Without KMS (enc == nil): stores plaintext, clears nonce.
func applyValue(v *accountvars.AccountVariable, plaintext string, enc *envelope.Encryptor) error {
	if v.Secret && enc != nil {
		ciphertext, nonce, err := enc.Encrypt([]byte(plaintext))
		if err != nil {
			return err
		}
		v.Value = base64.StdEncoding.EncodeToString(ciphertext)
		v.Nonce = nonce
	} else {
		v.Value = plaintext
		v.Nonce = nil
	}
	return nil
}

// resolveVarReferences resolves variables with a ref field by looking up
// account variables and populating the value (decrypting secrets as needed).
// It returns the original refs map (variable name → account variable name) so
// callers can persist the refs for later use (e.g. prefilled deployment templates).
func resolveVarReferences(c *gin.Context, log *logger.Logger, submittedSpec *deployment.AstroDeploymentSpec, accountID string, store *accountvars.Store, vault *envelope.Vault) (map[string]string, error) {
	if len(submittedSpec.Variables) == 0 {
		return nil, nil
	}

	// Collect all ref'd variable names
	refs := make(map[string]string) // spec variable key → account variable name
	for key, v := range submittedSpec.Variables {
		if v.Ref != "" {
			refs[key] = v.Ref
		}
	}
	if len(refs) == 0 {
		return nil, nil
	}

	// Deduplicate names
	nameSet := make(map[string]bool)
	for _, name := range refs {
		nameSet[name] = true
	}
	names := make([]string, 0, len(nameSet))
	for name := range nameSet {
		names = append(names, name)
	}

	// Fetch variables from DB
	acctVars, err := store.GetByNames(accountID, names)
	if err != nil {
		log.Error("account secrets: fetch account variables for deployment failed", "error", err, "account_id", accountID)
		return nil, fmt.Errorf("failed to fetch account variables")
	}

	// Build lookup map
	varMap := make(map[string]*accountvars.AccountVariable, len(acctVars))
	for i := range acctVars {
		varMap[acctVars[i].Name] = &acctVars[i]
	}

	// Check all referenced variables exist
	var missing []string
	for varKey, acctVarName := range refs {
		if _, ok := varMap[acctVarName]; !ok {
			missing = append(missing, fmt.Sprintf("variables.%s: account variable %q not found", varKey, acctVarName))
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		return nil, fmt.Errorf("unresolved variable references: %s", strings.Join(missing, "; "))
	}

	// A secret account variable may only resolve a secret deployment field, and
	// likewise for plaintext variables. Checking before decryption prevents a
	// secret value from being copied into a non-secret field and persisted as
	// ordinary deployment configuration.
	var incompatible []string
	for varKey, acctVarName := range refs {
		accountVarIsSecret := varMap[acctVarName].Secret
		deploymentVarIsSecret := submittedSpec.Variables[varKey].Secret
		if accountVarIsSecret == deploymentVarIsSecret {
			continue
		}

		accountVarType := "plain"
		if accountVarIsSecret {
			accountVarType = "secret"
		}
		deploymentVarType := "plain"
		if deploymentVarIsSecret {
			deploymentVarType = "secret"
		}
		incompatible = append(
			incompatible,
			fmt.Sprintf(
				"variables.%s: %s account variable %q cannot resolve a %s deployment variable",
				varKey,
				accountVarType,
				acctVarName,
				deploymentVarType,
			),
		)
	}
	if len(incompatible) > 0 {
		sort.Strings(incompatible)
		return nil, fmt.Errorf("incompatible variable references: %s", strings.Join(incompatible, "; "))
	}

	// Set up decryptor if any referenced variables are secrets
	var decryptor *envelope.Decryptor
	for _, acctVarName := range refs {
		if varMap[acctVarName].Secret {
			ek, err := store.GetEncryptionKey(accountID)
			if err != nil {
				log.Error("account secrets: get account encryption key failed", "error", err, "account_id", accountID)
				return nil, fmt.Errorf("failed to decrypt account variables")
			}
			if ek != nil {
				decryptor, err = vault.Decryptor(c.Request.Context(), ek.EncryptedDataKey)
				if err != nil {
					log.Error("account secrets: create decryptor for account variables failed", "error", err, "account_id", accountID)
					return nil, fmt.Errorf("failed to decrypt account variables")
				}
			}
			break
		}
	}

	// Resolve references
	for varKey, acctVarName := range refs {
		av := varMap[acctVarName]
		var plaintext string

		if av.Secret {
			if decryptor != nil && av.Nonce != nil {
				ciphertext, err := base64.StdEncoding.DecodeString(av.Value)
				if err != nil {
					log.Error("account secrets: decode variable ciphertext failed", "error", err, "variable", acctVarName)
					return nil, fmt.Errorf("failed to decrypt variable %q", acctVarName)
				}
				pt, err := decryptor.Decrypt(ciphertext, av.Nonce)
				if err != nil {
					log.Error("account secrets: decrypt account variable failed", "error", err, "variable", acctVarName)
					return nil, fmt.Errorf("failed to decrypt variable %q", acctVarName)
				}
				plaintext = string(pt)
			} else {
				plaintext = av.Value
			}
		} else {
			plaintext = av.Value
		}

		v := submittedSpec.Variables[varKey]
		v.Value = plaintext
		v.Ref = "" // clear ref after resolution so validation and K8s see only the value
		submittedSpec.Variables[varKey] = v

		log.Info("account secrets: resolved account variable reference", "variable", varKey, "account_var", acctVarName, "account_id", accountID)
	}

	return refs, nil
}
