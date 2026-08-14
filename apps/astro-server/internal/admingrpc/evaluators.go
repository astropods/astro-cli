package admingrpc

import (
	"context"
	"time"

	"github.com/astropods/astro/apps/astro-server/internal/deployeval"
	adminv1 "github.com/astropods/astro/packages/astro-proto/admin/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// SetEvaluators wires the deployment-configuration-drift evaluators
// (internal/deployeval) and their tracking store behind ListEvaluators,
// RunEvaluatorSweep, ListEvaluatorDrift, and FixDeploymentDrift.
func (s *Server) SetEvaluators(store *deployeval.Store, evaluators []deployeval.Evaluator) {
	s.evaluatorStore = store
	s.evaluators = evaluators
}

func (s *Server) requireEvaluators() error {
	if s.evaluatorStore == nil {
		return status.Error(codes.FailedPrecondition, "evaluators not configured")
	}
	return nil
}

func (s *Server) findEvaluator(id string) (deployeval.Evaluator, error) {
	for _, ev := range s.evaluators {
		if ev.ID() == id {
			return ev, nil
		}
	}
	return nil, status.Errorf(codes.NotFound, "evaluator %q not found", id)
}

// ListEvaluators returns the registered evaluator catalog with each
// evaluator's aggregate state across every deployment it's been run against.
func (s *Server) ListEvaluators(ctx context.Context, _ *adminv1.ListEvaluatorsRequest) (*adminv1.ListEvaluatorsResponse, error) {
	if err := s.requireEvaluators(); err != nil {
		return nil, err
	}
	out := make([]*adminv1.EvaluatorSummary, 0, len(s.evaluators))
	for _, ev := range s.evaluators {
		sum, err := s.evaluatorStore.Summarize(ctx, ev.ID())
		if err != nil {
			return nil, status.Errorf(codes.Internal, "summarize evaluator %q: %v", ev.ID(), err)
		}
		last := ""
		if sum.LastCheckedAt != nil {
			last = sum.LastCheckedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, &adminv1.EvaluatorSummary{
			ID:             ev.ID(),
			Name:           ev.Name(),
			Description:    ev.Description(),
			OKCount:        int32(sum.OKCount),        //nolint:gosec
			DriftedCount:   int32(sum.DriftedCount),   //nolint:gosec
			FixFailedCount: int32(sum.FixFailedCount), //nolint:gosec
			LastCheckedAt:  last,
		})
	}
	return &adminv1.ListEvaluatorsResponse{Evaluators: out}, nil
}

// RunEvaluatorSweep runs one evaluator's check against every active
// deployment and persists the result.
func (s *Server) RunEvaluatorSweep(ctx context.Context, req *adminv1.RunEvaluatorSweepRequest) (*adminv1.RunEvaluatorSweepResponse, error) {
	if err := s.requireEvaluators(); err != nil {
		return nil, err
	}
	ev, err := s.findEvaluator(req.EvaluatorID)
	if err != nil {
		return nil, err
	}
	result, err := deployeval.RunSweep(ctx, ev, s.deployStore, s.evaluatorStore, s.log)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "run evaluator sweep: %v", err)
	}
	return &adminv1.RunEvaluatorSweepResponse{
		EvaluatorID:  req.EvaluatorID,
		CheckedCount: int32(result.CheckedCount), //nolint:gosec
		DriftedCount: int32(result.DriftedCount), //nolint:gosec
	}, nil
}

// ListEvaluatorDrift returns every deployment currently drifted or
// fix_failed for one evaluator, enriched with deployment/account identity.
func (s *Server) ListEvaluatorDrift(ctx context.Context, req *adminv1.ListEvaluatorDriftRequest) (*adminv1.ListEvaluatorDriftResponse, error) {
	if err := s.requireEvaluators(); err != nil {
		return nil, err
	}
	if _, err := s.findEvaluator(req.EvaluatorID); err != nil {
		return nil, err
	}

	rows, err := s.evaluatorStore.ListDrifted(ctx, req.EvaluatorID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list evaluator drift: %v", err)
	}

	deps, err := s.deployStore.ListAllWithAccount()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "resolve deployments for evaluator drift: %v", err)
	}
	type identity struct{ agent, accountID, accountName string }
	byID := make(map[string]identity, len(deps))
	for _, d := range deps {
		byID[d.ID] = identity{agent: d.AgentName, accountID: d.AccountID, accountName: d.AccountName}
	}

	out := make([]*adminv1.EvaluatorDriftRow, 0, len(rows))
	for _, r := range rows {
		id := byID[r.DeploymentID]
		row := &adminv1.EvaluatorDriftRow{
			DeploymentID: r.DeploymentID,
			AgentName:    id.agent,
			AccountID:    id.accountID,
			AccountName:  id.accountName,
			Status:       string(r.Status),
			Detail:       r.Detail,
			CheckedAt:    r.CheckedAt.UTC().Format(time.RFC3339),
		}
		if r.FixedAt != nil {
			row.FixedAt = r.FixedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, row)
	}
	return &adminv1.ListEvaluatorDriftResponse{Rows: out}, nil
}

// FixDeploymentDrift runs one evaluator's fix for a single deployment, then
// re-checks it so the response reflects the real post-fix state.
func (s *Server) FixDeploymentDrift(ctx context.Context, req *adminv1.FixDeploymentDriftRequest) (*adminv1.FixDeploymentDriftResponse, error) {
	if err := s.requireEvaluators(); err != nil {
		return nil, err
	}
	if req.DeploymentID == "" {
		return nil, status.Error(codes.InvalidArgument, "deployment_id is required")
	}
	ev, err := s.findEvaluator(req.EvaluatorID)
	if err != nil {
		return nil, err
	}

	dep, err := s.deployStore.GetDeploymentByID(req.DeploymentID)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get deployment: %v", err)
	}
	if dep == nil {
		return nil, status.Error(codes.NotFound, "deployment not found")
	}

	fixErr := deployeval.FixDeployment(ctx, ev, dep, s.evaluatorStore)
	resp := &adminv1.FixDeploymentDriftResponse{}
	if fixErr != nil {
		resp.Error = fixErr.Error()
	}

	rows, listErr := s.evaluatorStore.ListDrifted(ctx, req.EvaluatorID)
	if listErr == nil {
		for _, r := range rows {
			if r.DeploymentID == req.DeploymentID {
				resp.Status = string(r.Status)
				resp.Detail = r.Detail
				break
			}
		}
	}
	if resp.Status == "" && fixErr == nil {
		resp.Status = string(deployeval.StatusOK)
	}
	return resp, nil
}
