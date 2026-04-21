package handlers

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/astropods/astro/apps/astro-server/internal/accountvars"
	"github.com/astropods/astro/apps/astro-server/internal/config"
	"github.com/astropods/astro/apps/astro-server/internal/envelope"
	"github.com/astropods/astro/apps/astro-server/internal/logger"
	"github.com/astropods/astro/apps/astro-server/internal/middleware"
	spec "github.com/astropods/astro/packages/astro-spec"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/gin-gonic/gin"
)

// validVarName matches letters, digits, and underscores; must start with a letter or underscore.
var validVarName = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

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
			log.Error("Failed to list account variables", "error", err, "account_id", acct.ID)
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
func CreateAccountVariable(log *logger.Logger, store *accountvars.Store, cfg *config.Config) gin.HandlerFunc {
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
				enc, err = getAccountEncryptor(c, log, store, acct.ID, cfg)
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

			if !validVarName.MatchString(entry.Name) {
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

			if entry.Secret {
				if enc != nil {
					encValue, nonce, err := enc.Encrypt([]byte(entry.Value))
					if err != nil {
						log.Error("Failed to encrypt variable", "error", err, "account_id", acct.ID, "name", entry.Name)
						result.Status = "error"
						result.Error = "failed to encrypt"
						results = append(results, result)
						continue
					}
					v.Value = base64.StdEncoding.EncodeToString(encValue)
					v.Nonce = nonce
				} else {
					// KMS not configured — store plaintext
					v.Value = entry.Value
				}
			} else {
				v.Value = entry.Value
			}

			if err := store.Save(v); err != nil {
				log.Error("Failed to save account variable", "error", err, "account_id", acct.ID, "name", entry.Name)
				result.Status = "error"
				result.Error = "failed to save"
				results = append(results, result)
				continue
			}

			result.Status = "created"
			results = append(results, result)
			log.Info("Account variable created", "account_id", acct.ID, "name", entry.Name, "secret", entry.Secret)
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

		v, err := store.Get(acct.ID, c.Param("varName"))
		if err != nil {
			log.Error("Failed to get account variable", "error", err, "account_id", acct.ID)
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
func UpdateAccountVariable(log *logger.Logger, store *accountvars.Store, cfg *config.Config) gin.HandlerFunc {
	return func(c *gin.Context) {
		acct, ok := middleware.GetAccountFromContext(c)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "account not resolved"})
			return
		}

		varName := c.Param("varName")

		existing, err := store.Get(acct.ID, varName)
		if err != nil {
			log.Error("Failed to get variable for update", "error", err, "account_id", acct.ID)
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
			if isSecret {
				encValue, nonce, err := encryptVariableValue(c, log, store, acct.ID, cfg, []byte(*req.Value))
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt variable"})
					return
				}
				existing.Value = base64.StdEncoding.EncodeToString(encValue)
				existing.Nonce = nonce
			} else {
				existing.Value = *req.Value
				existing.Nonce = nil
			}
		} else if req.Secret != nil && existing.Secret != isSecret {
			// Secret flag changed without a new value — re-encrypt or decrypt existing
			// This is tricky so we require a new value when changing the secret flag
			c.JSON(http.StatusBadRequest, gin.H{"error": "value must be provided when changing the secret flag"})
			return
		}

		if err := store.Save(existing); err != nil {
			log.Error("Failed to update account variable", "error", err, "account_id", acct.ID)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update variable"})
			return
		}

		log.Info("Account variable updated", "account_id", acct.ID, "name", varName)
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
			log.Error("Failed to delete account variable", "error", err, "account_id", acct.ID, "name", varName)
			c.JSON(http.StatusNotFound, gin.H{"error": "variable not found"})
			return
		}

		log.Info("Account variable deleted", "account_id", acct.ID, "name", varName)
		c.JSON(http.StatusOK, gin.H{"message": "variable deleted"})
	}
}

// getAccountEncryptor returns an Encryptor for the account, creating the data key via KMS if needed.
// Returns nil if KMS is not configured.
func getAccountEncryptor(c *gin.Context, log *logger.Logger, store *accountvars.Store, accountID string, cfg *config.Config) (*envelope.Encryptor, error) {
	if cfg.Deployment.KMSKeyARN == "" {
		return nil, nil
	}

	ctx := c.Request.Context()
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx)
	if err != nil {
		log.Error("Failed to load AWS config for KMS", "error", err)
		return nil, err
	}
	kmsClient := kms.NewFromConfig(awsCfg)

	ek, err := store.GetEncryptionKey(accountID)
	if err != nil {
		log.Error("Failed to get account encryption key", "error", err, "account_id", accountID)
		return nil, err
	}

	if ek == nil {
		enc, err := envelope.NewEncryptor(ctx, kmsClient, cfg.Deployment.KMSKeyARN)
		if err != nil {
			log.Error("Failed to generate KMS data key", "error", err, "account_id", accountID)
			return nil, err
		}
		if err := store.SaveEncryptionKey(accountID, enc.EncryptedDataKey, enc.KMSKeyARN); err != nil {
			log.Error("Failed to save account encryption key", "error", err, "account_id", accountID)
			return nil, err
		}
		return enc, nil
	}

	kmsOut, err := kmsClient.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: ek.EncryptedDataKey,
	})
	if err != nil {
		log.Error("Failed to KMS decrypt account data key", "error", err, "account_id", accountID)
		return nil, err
	}

	enc, err := envelope.NewEncryptorFromPlaintext(kmsOut.Plaintext, ek.EncryptedDataKey, ek.KMSKeyARN)
	for i := range kmsOut.Plaintext {
		kmsOut.Plaintext[i] = 0
	}
	if err != nil {
		log.Error("Failed to create encryptor from plaintext key", "error", err, "account_id", accountID)
		return nil, err
	}

	return enc, nil
}

// encryptVariableValue encrypts a plaintext value using the account's shared data key.
// If the account doesn't have a data key yet, one is generated via KMS.
func encryptVariableValue(c *gin.Context, log *logger.Logger, store *accountvars.Store, accountID string, cfg *config.Config, plaintext []byte) (ciphertext, nonce []byte, err error) {
	enc, err := getAccountEncryptor(c, log, store, accountID, cfg)
	if err != nil {
		return nil, nil, err
	}
	if enc == nil {
		return plaintext, nil, nil
	}
	return enc.Encrypt(plaintext)
}

// resolveVarReferences resolves variables with a ref field by looking up
// account variables and populating the value (decrypting secrets as needed).
// It returns the original refs map (variable name → account variable name) so
// callers can persist the refs for later use (e.g. prefilled deployment templates).
func resolveVarReferences(c *gin.Context, log *logger.Logger, submittedSpec *spec.AstroDeploymentSpec, accountID string, store *accountvars.Store, cfg *config.Config) (map[string]string, error) {
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
		log.Error("Failed to fetch account variables for deployment", "error", err, "account_id", accountID)
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
		return nil, fmt.Errorf("unresolved variable references: %s", strings.Join(missing, "; "))
	}

	// Set up decryptor if any referenced variables are secrets
	var decryptor *envelope.Decryptor
	for _, acctVarName := range refs {
		if varMap[acctVarName].Secret {
			ek, err := store.GetEncryptionKey(accountID)
			if err != nil {
				log.Error("Failed to get account encryption key", "error", err, "account_id", accountID)
				return nil, fmt.Errorf("failed to decrypt account variables")
			}
			if ek != nil && cfg.Deployment.KMSKeyARN != "" {
				ctx := c.Request.Context()
				awsCfg, awsErr := awsconfig.LoadDefaultConfig(ctx)
				if awsErr != nil {
					log.Error("Failed to load AWS config for variable resolution", "error", awsErr)
					return nil, fmt.Errorf("failed to decrypt account variables")
				}
				kmsClient := kms.NewFromConfig(awsCfg)
				decryptor, err = envelope.NewDecryptor(ctx, kmsClient, ek.EncryptedDataKey)
				if err != nil {
					log.Error("Failed to create decryptor for account variables", "error", err, "account_id", accountID)
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
					log.Error("Failed to decode variable ciphertext", "error", err, "variable", acctVarName)
					return nil, fmt.Errorf("failed to decrypt variable %q", acctVarName)
				}
				pt, err := decryptor.Decrypt(ciphertext, av.Nonce)
				if err != nil {
					log.Error("Failed to decrypt account variable", "error", err, "variable", acctVarName)
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

		log.Info("Resolved account variable reference", "variable", varKey, "account_var", acctVarName, "account_id", accountID)
	}

	return refs, nil
}
