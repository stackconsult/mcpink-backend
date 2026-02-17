package dns

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/clusters"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/delegatedzones"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/zonerecords"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient  client.Client
	delegatedZonesQ delegatedzones.Querier
	zoneRecordsQ    zonerecords.Querier
	servicesQ       services.Querier
	usersQ          users.Querier
	projectsQ       projects.Querier
	clusters        map[string]clusters.Cluster
	nameservers     []string
	logger          *slog.Logger
}

func NewService(
	temporalClient client.Client,
	delegatedZonesQ delegatedzones.Querier,
	zoneRecordsQ zonerecords.Querier,
	servicesQ services.Querier,
	usersQ users.Querier,
	projectsQ projects.Querier,
	clusters map[string]clusters.Cluster,
	cfg Config,
	logger *slog.Logger,
) *Service {
	return &Service{
		temporalClient:  temporalClient,
		delegatedZonesQ: delegatedZonesQ,
		zoneRecordsQ:    zoneRecordsQ,
		servicesQ:       servicesQ,
		usersQ:          usersQ,
		projectsQ:       projectsQ,
		clusters:        clusters,
		nameservers:     cfg.Nameservers,
		logger:          logger,
	}
}

func (s *Service) Nameservers() []string {
	return s.nameservers
}

type DelegateZoneParams struct {
	UserID string
	Zone   string
}

type DNSRecord struct {
	Host     string
	Type     string
	Value    string
	Verified bool
}

type DelegateZoneResult struct {
	ZoneID  string
	Zone    string
	Status  string
	Records []DNSRecord
}

func (s *Service) DelegateZone(ctx context.Context, params DelegateZoneParams) (*DelegateZoneResult, error) {
	zone := NormalizeDomain(params.Zone)

	if err := ValidateDelegatedZone(zone, "ml.ink"); err != nil {
		return nil, err
	}

	existing, err := s.delegatedZonesQ.FindOverlappingZone(ctx, zone)
	if err == nil {
		canReclaim := existing.Status == "failed" ||
			((existing.Status == "pending_verification" || existing.Status == "pending_delegation") &&
				existing.ExpiresAt.Valid && existing.ExpiresAt.Time.Before(time.Now()))
		if canReclaim {
			_ = s.delegatedZonesQ.Delete(ctx, existing.ID)
		} else {
			if existing.UserID == params.UserID {
				return nil, fmt.Errorf("you already have zone %s (status: %s)", existing.Zone, existing.Status)
			}
			return nil, fmt.Errorf("zone %s overlaps with an existing delegation", zone)
		}
	}

	token := GenerateVerificationToken()

	dz, err := s.delegatedZonesQ.Create(ctx, delegatedzones.CreateParams{
		UserID:            params.UserID,
		Zone:              zone,
		VerificationToken: token,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create delegated zone: %w", err)
	}

	records := []DNSRecord{
		{Host: txtVerifyHost(zone), Type: "TXT", Value: "dp-verify=" + token},
	}
	for _, ns := range s.nameservers {
		records = append(records, DNSRecord{Host: zone, Type: "NS", Value: ns})
	}

	return &DelegateZoneResult{
		ZoneID:  dz.ID,
		Zone:    zone,
		Status:  dz.Status,
		Records: records,
	}, nil
}

type VerifyDelegationParams struct {
	UserID string
	Zone   string
}

type VerifyDelegationResult struct {
	ZoneID  string
	Zone    string
	Status  string
	Message string
	Records []DNSRecord
}

func (s *Service) VerifyDelegation(ctx context.Context, params VerifyDelegationParams) (*VerifyDelegationResult, error) {
	zone := NormalizeDomain(params.Zone)

	dz, err := s.delegatedZonesQ.GetByZone(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("delegation not found for zone %s", zone)
	}

	if dz.UserID != params.UserID {
		return nil, fmt.Errorf("delegation not found for zone %s", zone)
	}

	// Check TXT and NS status for any non-terminal state
	txtVerified := dz.VerifiedAt.Valid // already passed TXT
	if !txtVerified && dz.Status != "active" {
		ok, _ := VerifyTXT(dz.Zone, dz.VerificationToken)
		txtVerified = ok
	}

	nsStatus := VerifyNSRecords(dz.Zone, s.nameservers)

	records := []DNSRecord{
		{
			Host:     txtVerifyHost(dz.Zone),
			Type:     "TXT",
			Value:    "dp-verify=" + dz.VerificationToken,
			Verified: txtVerified,
		},
	}
	for _, ns := range s.nameservers {
		records = append(records, DNSRecord{
			Host:     dz.Zone,
			Type:     "NS",
			Value:    ns,
			Verified: nsStatus[NormalizeDomain(ns)],
		})
	}

	if dz.Status == "active" {
		return &VerifyDelegationResult{
			ZoneID:  dz.ID,
			Zone:    dz.Zone,
			Status:  "active",
			Message: "Zone is already active",
			Records: records,
		}, nil
	}

	if dz.Status == "provisioning" {
		return &VerifyDelegationResult{
			ZoneID:  dz.ID,
			Zone:    dz.Zone,
			Status:  "provisioning",
			Message: "Zone is being provisioned; please wait",
			Records: records,
		}, nil
	}

	// TXT verification → provisioning (zone created in PowerDNS, waits for NS)
	if dz.Status == "pending_verification" || dz.Status == "pending_delegation" {
		if dz.Status == "pending_verification" {
			if !txtVerified {
				errMsg := fmt.Sprintf("TXT verification pending. Add TXT record: %s with value dp-verify=%s", txtVerifyHost(dz.Zone), dz.VerificationToken)
				s.delegatedZonesQ.UpdateError(ctx, delegatedzones.UpdateErrorParams{
					ID:        dz.ID,
					LastError: &errMsg,
				})
				return &VerifyDelegationResult{
					ZoneID:  dz.ID,
					Zone:    dz.Zone,
					Status:  dz.Status,
					Message: errMsg,
					Records: records,
				}, nil
			}
			s.delegatedZonesQ.UpdateTXTVerified(ctx, dz.ID)
		}

		return s.startActivation(ctx, dz, records)
	}

	// Retry failed zones
	if dz.Status == "failed" {
		return s.startActivation(ctx, dz, records)
	}

	return &VerifyDelegationResult{
		ZoneID:  dz.ID,
		Zone:    dz.Zone,
		Status:  dz.Status,
		Message: fmt.Sprintf("Zone is in unexpected status: %s", dz.Status),
		Records: records,
	}, nil
}

func (s *Service) startActivation(ctx context.Context, dz delegatedzones.DelegatedZone, records []DNSRecord) (*VerifyDelegationResult, error) {
	dz, err := s.delegatedZonesQ.UpdateProvisioning(ctx, dz.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update zone status: %w", err)
	}

	var cluster clusters.Cluster
	for _, c := range s.clusters {
		if c.Status == "active" {
			cluster = c
			break
		}
	}
	if cluster.Region == "" {
		return nil, fmt.Errorf("no active cluster available")
	}

	workflowID := fmt.Sprintf("activate-zone-%s", dz.ID)
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}, ActivateZoneWorkflow, ActivateZoneInput{
		ZoneID:      dz.ID,
		Zone:        dz.Zone,
		Nameservers: s.nameservers,
		IngressIP:   cluster.IngressIp,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start zone activation workflow: %w", err)
	}

	return &VerifyDelegationResult{
		ZoneID:  dz.ID,
		Zone:    dz.Zone,
		Status:  "provisioning",
		Message: "TXT verified! Provisioning started. Add NS records to complete activation.",
		Records: records,
	}, nil
}

type RemoveDelegationParams struct {
	UserID string
	Zone   string
}

type RemoveDelegationResult struct {
	ZoneID  string
	Message string
}

func (s *Service) RemoveDelegation(ctx context.Context, params RemoveDelegationParams) (*RemoveDelegationResult, error) {
	zone := NormalizeDomain(params.Zone)

	dz, err := s.delegatedZonesQ.GetByZone(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("delegation not found for zone %s", zone)
	}

	if dz.UserID != params.UserID {
		return nil, fmt.Errorf("delegation not found for zone %s", zone)
	}

	if dz.Status == "active" || dz.Status == "provisioning" {
		workflowID := fmt.Sprintf("deactivate-zone-%s", dz.ID)
		_, _ = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: TaskQueue,
		}, DeactivateZoneWorkflow, DeactivateZoneInput{
			ZoneID: dz.ID,
			Zone:   dz.Zone,
		})
	}

	if err := s.delegatedZonesQ.Delete(ctx, dz.ID); err != nil {
		return nil, fmt.Errorf("failed to delete delegation: %w", err)
	}

	return &RemoveDelegationResult{
		ZoneID:  dz.ID,
		Message: fmt.Sprintf("Delegation for %s removed", zone),
	}, nil
}

func (s *Service) ListDelegations(ctx context.Context, userID string) ([]delegatedzones.DelegatedZone, error) {
	return s.delegatedZonesQ.ListByUserID(ctx, userID)
}

type AddCustomDomainParams struct {
	Name    string
	Project string
	UserID  string
	Domain  string
}

type AddCustomDomainResult struct {
	ServiceID string
	Domain    string
	Status    string
	Message   string
}

func (s *Service) AddCustomDomain(ctx context.Context, params AddCustomDomainParams) (*AddCustomDomainResult, error) {
	domain := NormalizeDomain(params.Domain)

	svc, err := s.resolveService(ctx, params.UserID, params.Name, params.Project)
	if err != nil {
		return nil, err
	}

	// Try exact zone match first (domain == zone apex)
	dz, err := s.delegatedZonesQ.GetByZone(ctx, domain)
	if err != nil || dz.UserID != params.UserID || dz.Status != "active" {
		// Try subdomain match
		dz, err = s.delegatedZonesQ.FindMatchingZoneForDomain(ctx, delegatedzones.FindMatchingZoneForDomainParams{
			UserID: params.UserID,
			Lower:  domain,
		})
		if err != nil {
			return nil, fmt.Errorf("no active delegated zone found for domain %s — delegate the zone first using delegate_zone", domain)
		}
	}

	var name string
	if domain == dz.Zone {
		name = "@"
	} else {
		suffix := "." + dz.Zone
		if !strings.HasSuffix(domain, suffix) {
			return nil, fmt.Errorf("domain %s is not under zone %s", domain, dz.Zone)
		}
		name = strings.TrimSuffix(domain, suffix)
		if name == "" || strings.Contains(name, ".") {
			return nil, fmt.Errorf("only single-level subdomains are supported (e.g. api.%s)", dz.Zone)
		}
	}

	_, err = s.zoneRecordsQ.GetByZoneAndName(ctx, zonerecords.GetByZoneAndNameParams{
		ZoneID: dz.ID,
		Lower:  name,
	})
	if err == nil {
		return nil, fmt.Errorf("subdomain %s.%s already exists", name, dz.Zone)
	}

	zr, err := s.zoneRecordsQ.Create(ctx, zonerecords.CreateParams{
		ZoneID:    dz.ID,
		ServiceID: svc.ID,
		Name:      name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create zone record: %w", err)
	}

	cluster, ok := s.clusters[svc.Region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q for service %s", svc.Region, svc.ID)
	}

	user, err := s.usersQ.GetUserByID(ctx, svc.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	proj, err := s.projectsQ.GetProjectByID(ctx, svc.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	namespace := k8sdeployments.NamespaceName(user.ID, proj.Ref)
	serviceName := k8sdeployments.ServiceName(*svc.Name)
	port := k8sdeployments.EffectivePort(svc.BuildPack, svc.Port, svc.BuildConfig)

	workflowID := fmt.Sprintf("attach-dz-%s", zr.ID)
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}, AttachSubdomainWorkflow, AttachSubdomainInput{
		ZoneRecordID: zr.ID,
		Zone:         dz.Zone,
		Name:         name,
		IngressIP:    cluster.IngressIp,
		Namespace:    namespace,
		ServiceName:  serviceName,
		ServicePort:  port,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start subdomain attach workflow: %w", err)
	}

	return &AddCustomDomainResult{
		ServiceID: svc.ID,
		Domain:    domain,
		Status:    "provisioning",
		Message:   fmt.Sprintf("Subdomain %s will be live in seconds", domain),
	}, nil
}

type RemoveCustomDomainParams struct {
	Name    string
	Project string
	UserID  string
}

type RemoveCustomDomainResult struct {
	ServiceID string
	Message   string
}

func (s *Service) RemoveCustomDomain(ctx context.Context, params RemoveCustomDomainParams) (*RemoveCustomDomainResult, error) {
	svc, err := s.resolveService(ctx, params.UserID, params.Name, params.Project)
	if err != nil {
		return nil, err
	}

	records, err := s.zoneRecordsQ.ListByServiceID(ctx, svc.ID)
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("no custom domain configured for service %s", params.Name)
	}

	zr := records[0]
	dz, err := s.delegatedZonesQ.GetByID(ctx, zr.ZoneID)
	if err != nil {
		return nil, fmt.Errorf("zone not found: %w", err)
	}

	user, err := s.usersQ.GetUserByID(ctx, svc.UserID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	proj, err := s.projectsQ.GetProjectByID(ctx, svc.ProjectID)
	if err != nil {
		return nil, fmt.Errorf("project not found: %w", err)
	}

	namespace := k8sdeployments.NamespaceName(user.ID, proj.Ref)
	serviceName := k8sdeployments.ServiceName(*svc.Name)

	workflowID := fmt.Sprintf("detach-dz-%s", zr.ID)
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}, DetachSubdomainWorkflow, DetachSubdomainInput{
		Zone:        dz.Zone,
		Name:        zr.Name,
		Namespace:   namespace,
		ServiceName: serviceName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start detach workflow: %w", err)
	}

	if err := s.zoneRecordsQ.Delete(ctx, zr.ID); err != nil {
		return nil, fmt.Errorf("failed to delete zone record: %w", err)
	}

	return &RemoveCustomDomainResult{
		ServiceID: svc.ID,
		Message:   fmt.Sprintf("Custom domain %s.%s removed", zr.Name, dz.Zone),
	}, nil
}

func (s *Service) GetCustomDomainForService(ctx context.Context, serviceID string) (*zonerecords.ZoneRecord, *delegatedzones.DelegatedZone, error) {
	records, err := s.zoneRecordsQ.ListByServiceID(ctx, serviceID)
	if err != nil || len(records) == 0 {
		return nil, nil, fmt.Errorf("no custom domain")
	}
	zr := records[0]
	dz, err := s.delegatedZonesQ.GetByID(ctx, zr.ZoneID)
	if err != nil {
		return nil, nil, err
	}
	return &zr, &dz, nil
}

func (s *Service) resolveService(ctx context.Context, userID, name, project string) (*services.Service, error) {
	if project == "" {
		project = "default"
	}
	svc, err := s.servicesQ.GetServiceByNameAndUserProject(ctx, services.GetServiceByNameAndUserProjectParams{
		Name:   &name,
		UserID: userID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s in project %s", name, project)
	}
	return &svc, nil
}
