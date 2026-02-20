package dataloader

import (
	"context"
	"fmt"
	"net/http"

	deploymentsdb "github.com/augustdev/autoclip/internal/storage/pg/generated/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsdb"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/vikstrous/dataloadgen"
)

type ctxKey struct{}

type CustomDomainInfo struct {
	Domain string
	Status string
}

type LoaderDeps struct {
	ServiceQueries    services.Querier
	DeploymentQueries deploymentsdb.Querier
	DnsQueries        dnsdb.Querier
}

type Loaders struct {
	ServicesByProjectID         *dataloadgen.Loader[string, []services.Service]
	LatestDeploymentByServiceID *dataloadgen.Loader[string, *deploymentsdb.Deployment]
	CustomDomainByServiceID     *dataloadgen.Loader[string, *CustomDomainInfo]
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
		rows, err := deps.DnsQueries.ListCustomDomainsByServiceIDs(ctx, keys)
		if err != nil {
			return nil, []error{fmt.Errorf("ListCustomDomainsByServiceIDs: %w", err)}
		}

		// Pick first record per service
		firstByService := make(map[string]*CustomDomainInfo, len(rows))
		for _, row := range rows {
			sid := ""
			if row.ServiceID != nil {
				sid = *row.ServiceID
			}
			if sid == "" {
				continue
			}
			if _, ok := firstByService[sid]; !ok {
				firstByService[sid] = &CustomDomainInfo{
					Domain: row.Name + "." + row.Zone,
					Status: row.Status,
				}
			}
		}

		results := make([]*CustomDomainInfo, len(keys))
		for i, k := range keys {
			results[i] = firstByService[k]
		}
		return results, nil
	}
}
