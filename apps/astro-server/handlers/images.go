package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/gin-gonic/gin"
	"github.com/postman/astro/apps/astro-server/internal/logger"
)

// ImageInfo represents information about a container image
type ImageInfo struct {
	Repository string   `json:"repository"`
	Namespace  string   `json:"namespace"`
	Name       string   `json:"name"`
	Tags       []string `json:"tags"`
}

// ListImagesResponse represents the response for listing images
type ListImagesResponse struct {
	Images []ImageInfo `json:"images"`
	Count  int         `json:"count"`
}

// ListImages handles GET /api/v1/images
// Lists all container images in ECR with tenant-* prefix
func ListImages(log *logger.Logger, region string) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()

		log.Info("Listing ECR images with tenant-* prefix")

		// Load AWS config
		opts := []func(*config.LoadOptions) error{}
		if region != "" {
			opts = append(opts, config.WithRegion(region))
		}

		cfg, err := config.LoadDefaultConfig(ctx, opts...)
		if err != nil {
			log.Error("Failed to load AWS config", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to connect to AWS",
				"details": err.Error(),
			})
			return
		}

		// Create ECR client
		client := ecr.NewFromConfig(cfg)

		// List repositories with tenant- prefix
		images, err := listTenantRepositories(ctx, client, log)
		if err != nil {
			log.Error("Failed to list ECR repositories", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Failed to list images from ECR",
				"details": err.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, ListImagesResponse{
			Images: images,
			Count:  len(images),
		})
	}
}

// listTenantRepositories lists all ECR repositories with tenant-* prefix
func listTenantRepositories(ctx context.Context, client *ecr.Client, log *logger.Logger) ([]ImageInfo, error) {
	var images []ImageInfo
	var nextToken *string

	for {
		// List repositories
		input := &ecr.DescribeRepositoriesInput{
			NextToken: nextToken,
		}

		output, err := client.DescribeRepositories(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, repo := range output.Repositories {
			repoName := *repo.RepositoryName

			// Filter for tenant-* repositories
			if !strings.HasPrefix(repoName, "tenant-") {
				continue
			}

			// Parse namespace and image name from repository name
			// Format: tenant-{namespace}/{image} or tenant-{namespace}
			namespace, imageName := parseRepositoryName(repoName)

			// Get tags for this repository
			tags, err := getRepositoryTags(ctx, client, repoName)
			if err != nil {
				log.Warn("Failed to get tags for repository",
					"repository", repoName,
					"error", err,
				)
				tags = []string{}
			}

			images = append(images, ImageInfo{
				Repository: repoName,
				Namespace:  namespace,
				Name:       imageName,
				Tags:       tags,
			})
		}

		// Check for more pages
		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return images, nil
}

// parseRepositoryName parses a repository name into namespace and image name
// Input: tenant-user123/myapp -> namespace: user123, name: myapp
// Input: tenant-user123 -> namespace: user123, name: (empty)
func parseRepositoryName(repoName string) (namespace, imageName string) {
	// Strip tenant- prefix
	withoutPrefix := strings.TrimPrefix(repoName, "tenant-")

	// Split by /
	parts := strings.SplitN(withoutPrefix, "/", 2)
	namespace = parts[0]

	if len(parts) > 1 {
		imageName = parts[1]
	}

	return namespace, imageName
}

// getRepositoryTags gets all tags for a repository
func getRepositoryTags(ctx context.Context, client *ecr.Client, repoName string) ([]string, error) {
	var tags []string
	var nextToken *string

	for {
		input := &ecr.ListImagesInput{
			RepositoryName: &repoName,
			NextToken:      nextToken,
		}

		output, err := client.ListImages(ctx, input)
		if err != nil {
			return nil, err
		}

		for _, imageID := range output.ImageIds {
			if imageID.ImageTag != nil {
				tags = append(tags, *imageID.ImageTag)
			}
		}

		if output.NextToken == nil {
			break
		}
		nextToken = output.NextToken
	}

	return tags, nil
}
