package dataloader

import (
	"context"
	"fmt"
	"net/http"

	deploymentsdb "github.com/augustdev/autoclip/internal/storage/pg/generated/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/delegatedzones"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/zonerecords"
	"github.com/vikstrous/dataloadgen"
)

type ctxKey struct{}

type CustomDomainInfo struct {
	Domain string
	Status string
}

type LoaderDeps struct {
	ServiceQueries       services.Querier
	DeploymentQueries    deploymentsdb.Querier
	ZoneRecordQueries    zonerecords.Querier
	DelegatedZoneQueries delegatedzones.Querier
}

type Loaders struct {
	ServicesByProjectID          *dataloadgen.Loader[string, []services.Service]
	LatestDeploymentByServiceID  *dataloadgen.Loader[string, *deploymentsdb.Deployment]
	CustomDomainByServiceID      *dataloadgen.Loader[string, *CustomDomainInfo]
}

func NewLoaders(deps *LoaderDeps) *Loaders {
	return &Loaders{
		ServicesByProjectID:         dataloadgen.NewLoader(newServicesByProjectIDFn(deps)),
		LatestDeploymentByServiceID: dataloadgen.NewLoader(newLatestDeploymentFn(deps)),
		CustomDomainByServiceID:     dataloadgen.NewLoader(newCustomDomainFn(deps)),
	}
}

func Middleware(deps *LoaderDeps) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			loaders := NewLoaders(deps)
			ctx := context.WithValue(r.Context(), ctxKey{}, loaders)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func For(ctx context.Context) *Loaders {
	return ctx.Value(ctxKey{}).(*Loaders)
}

// --- batch functions ---

func newServicesByProjectIDFn(deps *LoaderDeps) func(ctx context.Context, keys []string) ([][]services.Service, []error) {
	return func(ctx context.Context, keys []string) ([][]services.Service, []error) {
		rows, err := deps.ServiceQueries.ListServicesByProjectIDs(ctx, keys)
		if err != nil {
			return nil, []error{fmt.Errorf("ListServicesByProjectIDs: %w", err)}
		}

		byProject := make(map[string][]services.Service, len(keys))
		for i := range rows {
			pid := rows[i].ProjectID
			byProject[pid] = append(byProject[pid], rows[i])
		}

		results := make([][]services.Service, len(keys))
		for i, k := range keys {
			results[i] = byProject[k]
			if results[i] == nil {
				results[i] = []services.Service{}
			}
		}
		return results, nil
	}
}

func newLatestDeploymentFn(deps *LoaderDeps) func(ctx context.Context, keys []string) ([]*deploymentsdb.Deployment, []error) {
	return func(ctx context.Context, keys []string) ([]*deploymentsdb.Deployment, []error) {
		rows, err := deps.DeploymentQueries.GetLatestDeploymentsByServiceIDs(ctx, keys)
		if err != nil {
			return nil, []error{fmt.Errorf("GetLatestDeploymentsByServiceIDs: %w", err)}
		}

		byService := make(map[string]*deploymentsdb.Deployment, len(rows))
		for i := range rows {
			byService[rows[i].ServiceID] = &rows[i]
		}

		results := make([]*deploymentsdb.Deployment, len(keys))
		for i, k := range keys {
			results[i] = byService[k] // nil if not found, which is fine
		}
		return results, nil
	}
}

func newCustomDomainFn(deps *LoaderDeps) func(ctx context.Context, keys []string) ([]*CustomDomainInfo, []error) {
	return func(ctx context.Context, keys []string) ([]*CustomDomainInfo, []error) {
		zoneRecords, err := deps.ZoneRecordQueries.ListByServiceIDs(ctx, keys)
		if err != nil {
			return nil, []error{fmt.Errorf("ListByServiceIDs: %w", err)}
		}

		if len(zoneRecords) == 0 {
			return make([]*CustomDomainInfo, len(keys)), nil
		}

		// Collect unique zone IDs and pick the first zone record per service
		type zrInfo struct {
			Name   string
			ZoneID string
		}
		firstByService := make(map[string]zrInfo, len(zoneRecords))
		zoneIDSet := make(map[string]struct{})
		for _, zr := range zoneRecords {
			if _, ok := firstByService[zr.ServiceID]; !ok {
				firstByService[zr.ServiceID] = zrInfo{Name: zr.Name, ZoneID: zr.ZoneID}
				zoneIDSet[zr.ZoneID] = struct{}{}
			}
		}

		zoneIDs := make([]string, 0, len(zoneIDSet))
		for id := range zoneIDSet {
			zoneIDs = append(zoneIDs, id)
		}

		zones, err := deps.DelegatedZoneQueries.GetByIDs(ctx, zoneIDs)
		if err != nil {
			return nil, []error{fmt.Errorf("GetByIDs: %w", err)}
		}

		zoneByID := make(map[string]*delegatedzones.DelegatedZone, len(zones))
		for i := range zones {
			zoneByID[zones[i].ID] = &zones[i]
		}

		results := make([]*CustomDomainInfo, len(keys))
		for i, k := range keys {
			info, ok := firstByService[k]
			if !ok {
				continue
			}
			dz, ok := zoneByID[info.ZoneID]
			if !ok {
				continue
			}
			results[i] = &CustomDomainInfo{
				Domain: info.Name + "." + dz.Zone,
				Status: dz.Status,
			}
		}
		return results, nil
	}
}
