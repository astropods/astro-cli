package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

const (
	maxListQueryLen           = 100
	defaultBlueprintListLimit = 50
	maxBlueprintListLimit     = 100
	maxBlueprintListOffset    = 10_000
)

// BlueprintListFilters holds optional filters for blueprint list endpoints.
type BlueprintListFilters struct {
	Query      string
	Tag        string
	Visibility string
	Sort       string
	Limit      int
	Offset     int
}

// ParseBlueprintListFilters reads blueprint list query params from the request.
func ParseBlueprintListFilters(c *gin.Context) (BlueprintListFilters, error) {
	f := BlueprintListFilters{
		Query: strings.TrimSpace(c.Query("q")),
		Tag:   strings.TrimSpace(c.Query("tag")),
		Sort:  strings.TrimSpace(c.Query("sort")),
	}
	if vis := strings.TrimSpace(c.Query("visibility")); vis != "" {
		if vis != "public" && vis != "private" {
			return f, fmt.Errorf("visibility must be 'public' or 'private'")
		}
		f.Visibility = vis
	}
	if len(f.Query) > maxListQueryLen {
		return f, fmt.Errorf("q must be at most %d characters", maxListQueryLen)
	}
	if len(f.Tag) > maxListQueryLen {
		return f, fmt.Errorf("tag must be at most %d characters", maxListQueryLen)
	}
	if f.Sort == "" {
		f.Sort = "name"
	}
	switch f.Sort {
	case "name", "newest":
	default:
		return f, fmt.Errorf("sort must be 'name' or 'newest'")
	}

	limit, offset, err := parseListPagination(c)
	if err != nil {
		return f, err
	}
	f.Limit = limit
	f.Offset = offset

	return f, nil
}

func parseListPagination(c *gin.Context) (int, int, error) {
	limit := defaultBlueprintListLimit
	if limitStr := strings.TrimSpace(c.Query("limit")); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			return 0, 0, fmt.Errorf("limit must be a positive integer")
		}
		if parsed > maxBlueprintListLimit {
			parsed = maxBlueprintListLimit
		}
		limit = parsed
	}

	offset := 0
	if offsetStr := strings.TrimSpace(c.Query("offset")); offsetStr != "" {
		parsed, err := strconv.Atoi(offsetStr)
		if err != nil || parsed < 0 {
			return 0, 0, fmt.Errorf("offset must be a non-negative integer")
		}
		if parsed > maxBlueprintListOffset {
			return 0, 0, fmt.Errorf("offset must be at most %d", maxBlueprintListOffset)
		}
		offset = parsed
	}
	return limit, offset, nil
}

func writeListFilterError(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func writeBlueprintListInternalError(c *gin.Context, message string) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": message})
}
