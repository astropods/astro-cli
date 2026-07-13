package admingrpc

import (
	"context"

	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/astropods/astro/apps/astro-server/internal/auditlog"
)

// RefreshMessagingCache evicts the messaging sidecar's ECR Docker Hub
// pull-through cache tag so the next agent pull re-imports it from Docker Hub,
// bypassing ECR's ~24h upstream-check window. Running agents pick up the new
// sidecar on their next restart/redeploy; this does not restart pods.
func (s *Server) RefreshMessagingCache(ctx context.Context, _ *adminv1.RefreshMessagingCacheRequest) (*adminv1.RefreshMessagingCacheResponse, error) {
	if s.imageRefresher == nil {
		return nil, status.Error(codes.Unavailable, "image cache refresh is not configured on this server")
	}

	image, err := s.imageRefresher.RefreshMessaging(ctx)
	if err != nil {
		s.log.Error("Admin messaging cache refresh failed", "image", image, "error", err)
		return nil, status.Errorf(codes.Internal, "refresh messaging cache: %v", err)
	}

	if s.auditStore != nil {
		evt := auditlog.ForAdmin("", "grpc")
		evt.Action = auditlog.ImageCacheRefreshMessaging
		evt.ResourceType = "image_cache"
		evt.ResourceID = image
		evt.Description = "Admin force-refreshed the messaging pull-through cache image"
		evt.Metadata = map[string]any{"image": image}
		s.auditStore.LogAsync(s.log, evt)
	}

	s.log.Info("Admin force-refreshed messaging cache", "image", image)

	return &adminv1.RefreshMessagingCacheResponse{
		Image:   image,
		Message: "Evicted messaging pull-through cache tag. Agents pick up the new sidecar on their next restart or redeploy.",
	}, nil
}
