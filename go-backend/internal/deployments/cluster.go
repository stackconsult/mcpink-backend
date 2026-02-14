package deployments

import (
	"context"
	"fmt"

	"github.com/augustdev/autoclip/internal/storage/pg/generated/clusters"
)

type ClusterResolver struct {
	clustersQ clusters.Querier
}

func NewClusterResolver(clustersQ clusters.Querier) *ClusterResolver {
	return &ClusterResolver{clustersQ: clustersQ}
}

// SelectCluster picks a cluster for new services.
// Today: first active cluster. Future: region preference, capacity.
func (r *ClusterResolver) SelectCluster(ctx context.Context) (*clusters.Cluster, error) {
	active, err := r.clustersQ.ListActiveClusters(ctx)
	if err != nil {
		return nil, fmt.Errorf("list active clusters: %w", err)
	}
	if len(active) == 0 {
		return nil, fmt.Errorf("no active clusters available")
	}
	return &active[0], nil
}

func (r *ClusterResolver) GetClusterForService(ctx context.Context, serviceID string) (*clusters.Cluster, error) {
	c, err := r.clustersQ.GetClusterByServiceID(ctx, serviceID)
	if err != nil {
		return nil, fmt.Errorf("get cluster for service %s: %w", serviceID, err)
	}
	return &c, nil
}
