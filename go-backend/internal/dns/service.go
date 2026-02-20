package dns

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/powerdns"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/clusters"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/dnsdb"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/lithammer/shortuuid/v4"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient client.Client
	dnsQ           dnsdb.Querier
	servicesQ      services.Querier
	usersQ         users.Querier
	projectsQ      projects.Querier
	clusters       map[string]clusters.Cluster
	nameservers    []string
	logger         *slog.Logger
	pdns           *powerdns.Client
}

func NewService(
	temporalClient client.Client,
	dnsQ dnsdb.Querier,
	servicesQ services.Querier,
	usersQ users.Querier,
	projectsQ projects.Querier,
	clusters map[string]clusters.Cluster,
	cfg Config,
	logger *slog.Logger,
	pdns *powerdns.Client,
) *Service {
	return &Service{
		temporalClient: temporalClient,
		dnsQ:           dnsQ,
		servicesQ:      servicesQ,
		usersQ:         usersQ,
		projectsQ:      projectsQ,
		clusters:       clusters,
		nameservers:    cfg.Nameservers,
		logger:         logger,
		pdns:           pdns,
	}
}

func (s *Service) Nameservers() []string {
	return s.nameservers
}

type CreateHostedZoneParams struct {
	UserID string
	Zone   string
}

type DNSRecord struct {
	Host     string
	Type     string
	Value    string
	Verified bool
}

type CreateHostedZoneResult struct {
	ZoneID  string
	Zone    string
	Status  string
	Records []DNSRecord
}

func (s *Service) CreateHostedZone(ctx context.Context, params CreateHostedZoneParams) (*CreateHostedZoneResult, error) {
	zone := NormalizeDomain(params.Zone)

	if err := ValidateHostedZone(zone, "ml.ink"); err != nil {
		return nil, err
	}

	existing, err := s.dnsQ.FindOverlappingHostedZone(ctx, zone)
	if err == nil {
		canReclaim := existing.Status == "failed" ||
			((existing.Status == "pending_verification" || existing.Status == "pending_delegation") &&
				existing.ExpiresAt.Valid && existing.ExpiresAt.Time.Before(time.Now()))
		if canReclaim {
			_ = s.dnsQ.DeleteHostedZone(ctx, existing.ID)
		} else {
			if existing.UserID == params.UserID {
				return nil, fmt.Errorf("you already have zone %s (status: %s)", existing.Zone, existing.Status)
			}
			return nil, fmt.Errorf("zone %s overlaps with an existing delegation", zone)
		}
	}

	token := GenerateVerificationToken()

	hz, err := s.dnsQ.CreateHostedZone(ctx, dnsdb.CreateHostedZoneParams{
		ID:                shortuuid.New(),
		UserID:            params.UserID,
		Zone:              zone,
		VerificationToken: token,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create hosted zone: %w", err)
	}

	records := []DNSRecord{
		{Host: txtVerifyHost(zone), Type: "TXT", Value: "dp-verify=" + token},
	}
	for _, ns := range s.nameservers {
		records = append(records, DNSRecord{Host: zone, Type: "NS", Value: ns})
	}

	return &CreateHostedZoneResult{
		ZoneID:  hz.ID,
		Zone:    zone,
		Status:  hz.Status,
		Records: records,
	}, nil
}

type VerifyHostedZoneParams struct {
	UserID string
	Zone   string
}

type VerifyHostedZoneResult struct {
	ZoneID  string
	Zone    string
	Status  string
	Message string
	Records []DNSRecord
}

func (s *Service) VerifyHostedZone(ctx context.Context, params VerifyHostedZoneParams) (*VerifyHostedZoneResult, error) {
	zone := NormalizeDomain(params.Zone)

	hz, err := s.dnsQ.GetHostedZoneByZone(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("hosted zone not found for zone %s", zone)
	}

	if hz.UserID != params.UserID {
		return nil, fmt.Errorf("hosted zone not found for zone %s", zone)
	}

	txtVerified := hz.VerifiedAt.Valid
	if !txtVerified && hz.Status != "active" {
		ok, _ := VerifyTXT(hz.Zone, hz.VerificationToken)
		txtVerified = ok
	}

	nsStatus := VerifyNSRecords(hz.Zone, s.nameservers)

	records := []DNSRecord{
		{
			Host:     txtVerifyHost(hz.Zone),
			Type:     "TXT",
			Value:    "dp-verify=" + hz.VerificationToken,
			Verified: txtVerified,
		},
	}
	for _, ns := range s.nameservers {
		records = append(records, DNSRecord{
			Host:     hz.Zone,
			Type:     "NS",
			Value:    ns,
			Verified: nsStatus[NormalizeDomain(ns)],
		})
	}

	if hz.Status == "active" {
		return &VerifyHostedZoneResult{
			ZoneID:  hz.ID,
			Zone:    hz.Zone,
			Status:  "active",
			Message: "Zone is already active",
			Records: records,
		}, nil
	}

	if hz.Status == "provisioning" {
		return &VerifyHostedZoneResult{
			ZoneID:  hz.ID,
			Zone:    hz.Zone,
			Status:  "provisioning",
			Message: "Zone is being provisioned; please wait",
			Records: records,
		}, nil
	}

	if hz.Status == "pending_verification" {
		if !txtVerified {
			errMsg := fmt.Sprintf("TXT verification pending. Add TXT record: %s with value dp-verify=%s", txtVerifyHost(hz.Zone), hz.VerificationToken)
			s.dnsQ.UpdateHostedZoneError(ctx, dnsdb.UpdateHostedZoneErrorParams{
				ID:        hz.ID,
				LastError: &errMsg,
			})
			return &VerifyHostedZoneResult{
				ZoneID:  hz.ID,
				Zone:    hz.Zone,
				Status:  hz.Status,
				Message: errMsg,
				Records: records,
			}, nil
		}
		s.dnsQ.UpdateHostedZoneTXTVerified(ctx, hz.ID)

		if cluster, ok := s.activeCluster(); ok {
			workflowID := fmt.Sprintf("precreate-zone-%s", hz.ID)
			_, _ = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
				ID:        workflowID,
				TaskQueue: TaskQueue,
			}, PreCreateZoneWorkflow, CreateZoneInput{
				Zone:        hz.Zone,
				Nameservers: s.nameservers,
				IngressIP:   cluster.IngressIp,
			})
		}

		return &VerifyHostedZoneResult{
			ZoneID:  hz.ID,
			Zone:    hz.Zone,
			Status:  "pending_delegation",
			Message: "TXT verified! Now add the NS records at your registrar.",
			Records: records,
		}, nil
	}

	if hz.Status == "pending_delegation" {
		allNS := true
		for _, ns := range s.nameservers {
			if !nsStatus[NormalizeDomain(ns)] {
				allNS = false
				break
			}
		}
		if !allNS {
			return &VerifyHostedZoneResult{
				ZoneID:  hz.ID,
				Zone:    hz.Zone,
				Status:  hz.Status,
				Message: "NS records not yet detected. Please add them at your registrar and try again.",
				Records: records,
			}, nil
		}
		return s.startActivation(ctx, hz, records)
	}

	if hz.Status == "failed" {
		return s.startActivation(ctx, hz, records)
	}

	return &VerifyHostedZoneResult{
		ZoneID:  hz.ID,
		Zone:    hz.Zone,
		Status:  hz.Status,
		Message: fmt.Sprintf("Zone is in unexpected status: %s", hz.Status),
		Records: records,
	}, nil
}

func (s *Service) activeCluster() (clusters.Cluster, bool) {
	for _, c := range s.clusters {
		if c.Status == "active" {
			return c, true
		}
	}
	return clusters.Cluster{}, false
}

func (s *Service) startActivation(ctx context.Context, hz dnsdb.HostedZone, records []DNSRecord) (*VerifyHostedZoneResult, error) {
	hz, err := s.dnsQ.UpdateHostedZoneProvisioning(ctx, hz.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to update zone status: %w", err)
	}

	cluster, ok := s.activeCluster()
	if !ok {
		return nil, fmt.Errorf("no active cluster available")
	}

	workflowID := fmt.Sprintf("activate-zone-%s", hz.ID)
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}, ActivateZoneWorkflow, ActivateZoneInput{
		ZoneID:      hz.ID,
		Zone:        hz.Zone,
		Nameservers: s.nameservers,
		IngressIP:   cluster.IngressIp,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start zone activation workflow: %w", err)
	}

	return &VerifyHostedZoneResult{
		ZoneID:  hz.ID,
		Zone:    hz.Zone,
		Status:  "provisioning",
		Message: "TXT verified! Provisioning started. Add NS records to complete activation.",
		Records: records,
	}, nil
}

type DeleteHostedZoneParams struct {
	UserID string
	Zone   string
}

type DeleteHostedZoneResult struct {
	ZoneID  string
	Message string
}

func (s *Service) DeleteHostedZone(ctx context.Context, params DeleteHostedZoneParams) (*DeleteHostedZoneResult, error) {
	zone := NormalizeDomain(params.Zone)

	hz, err := s.dnsQ.GetHostedZoneByZone(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("hosted zone not found for zone %s", zone)
	}

	if hz.UserID != params.UserID {
		return nil, fmt.Errorf("hosted zone not found for zone %s", zone)
	}

	if hz.Status == "active" || hz.Status == "provisioning" || hz.Status == "pending_delegation" {
		workflowID := fmt.Sprintf("deactivate-zone-%s", hz.ID)
		_, _ = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
			ID:        workflowID,
			TaskQueue: TaskQueue,
		}, DeactivateZoneWorkflow, DeactivateZoneInput{
			ZoneID: hz.ID,
			Zone:   hz.Zone,
		})
	}

	if err := s.dnsQ.DeleteHostedZone(ctx, hz.ID); err != nil {
		return nil, fmt.Errorf("failed to delete hosted zone: %w", err)
	}

	return &DeleteHostedZoneResult{
		ZoneID:  hz.ID,
		Message: fmt.Sprintf("Hosted zone for %s removed", zone),
	}, nil
}

func (s *Service) ListHostedZones(ctx context.Context, userID string) ([]dnsdb.HostedZone, error) {
	return s.dnsQ.ListHostedZonesByUserID(ctx, userID)
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

	hz, err := s.dnsQ.GetHostedZoneByZone(ctx, domain)
	if err != nil || hz.UserID != params.UserID || hz.Status != "active" {
		hz, err = s.dnsQ.FindMatchingHostedZoneForDomain(ctx, dnsdb.FindMatchingHostedZoneForDomainParams{
			UserID: params.UserID,
			Lower:  domain,
		})
		if err != nil {
			return nil, fmt.Errorf("no active hosted zone found for domain %s — create the hosted zone first", domain)
		}
	}

	var name string
	if domain == hz.Zone {
		name = "@"
	} else {
		suffix := "." + hz.Zone
		if !strings.HasSuffix(domain, suffix) {
			return nil, fmt.Errorf("domain %s is not under zone %s", domain, hz.Zone)
		}
		name = strings.TrimSuffix(domain, suffix)
		if name == "" || strings.Contains(name, ".") {
			return nil, fmt.Errorf("only single-level subdomains are supported (e.g. api.%s)", hz.Zone)
		}
	}

	existing, _ := s.dnsQ.ListDnsRecordsByZoneAndName(ctx, dnsdb.ListDnsRecordsByZoneAndNameParams{
		ZoneID: hz.ID,
		Lower:  name,
	})
	for _, rec := range existing {
		if rec.Managed && rec.Rrtype == "A" {
			return nil, fmt.Errorf("subdomain %s.%s already has a managed record", name, hz.Zone)
		}
	}

	cluster, ok := s.clusters[svc.Region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q for service %s", svc.Region, svc.ID)
	}

	serviceID := svc.ID
	dr, err := s.dnsQ.CreateDnsRecord(ctx, dnsdb.CreateDnsRecordParams{
		ID:        shortuuid.New(),
		ZoneID:    hz.ID,
		Name:      name,
		Rrtype:    "A",
		Content:   cluster.IngressIp,
		Ttl:       300,
		Managed:   true,
		ServiceID: &serviceID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create dns record: %w", err)
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

	workflowID := fmt.Sprintf("attach-dz-%s", dr.ID)
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}, AttachSubdomainWorkflow, AttachSubdomainInput{
		ZoneRecordID: dr.ID,
		Zone:         hz.Zone,
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

	serviceID := svc.ID
	records, err := s.dnsQ.ListDnsRecordsByServiceID(ctx, &serviceID)
	if err != nil || len(records) == 0 {
		return nil, fmt.Errorf("no custom domain configured for service %s", params.Name)
	}

	dr := records[0]
	hz, err := s.dnsQ.GetHostedZoneByID(ctx, dr.ZoneID)
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

	workflowID := fmt.Sprintf("detach-dz-%s", dr.ID)
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: TaskQueue,
	}, DetachSubdomainWorkflow, DetachSubdomainInput{
		Zone:        hz.Zone,
		Name:        dr.Name,
		Namespace:   namespace,
		ServiceName: serviceName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start detach workflow: %w", err)
	}

	if err := s.dnsQ.DeleteDnsRecord(ctx, dr.ID); err != nil {
		return nil, fmt.Errorf("failed to delete dns record: %w", err)
	}

	return &RemoveCustomDomainResult{
		ServiceID: svc.ID,
		Message:   fmt.Sprintf("Custom domain %s.%s removed", dr.Name, hz.Zone),
	}, nil
}

func (s *Service) GetCustomDomainForService(ctx context.Context, serviceID string) (*dnsdb.DnsRecord, *dnsdb.HostedZone, error) {
	records, err := s.dnsQ.ListDnsRecordsByServiceID(ctx, &serviceID)
	if err != nil || len(records) == 0 {
		return nil, nil, fmt.Errorf("no custom domain")
	}
	dr := records[0]
	hz, err := s.dnsQ.GetHostedZoneByID(ctx, dr.ZoneID)
	if err != nil {
		return nil, nil, err
	}
	return &dr, &hz, nil
}

// --- User DNS record management ---

type AddDnsRecordParams struct {
	UserID  string
	Zone    string
	Name    string
	Rrtype  string
	Content string
	TTL     int
}

type AddDnsRecordResult struct {
	Record dnsdb.DnsRecord
}

func (s *Service) AddDnsRecord(ctx context.Context, params AddDnsRecordParams) (*AddDnsRecordResult, error) {
	zone := NormalizeDomain(params.Zone)

	hz, err := s.dnsQ.GetHostedZoneByZone(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("hosted zone not found: %s", zone)
	}
	if hz.UserID != params.UserID {
		return nil, fmt.Errorf("hosted zone not found: %s", zone)
	}
	if hz.Status != "active" {
		return nil, fmt.Errorf("hosted zone %s is not active (status: %s)", zone, hz.Status)
	}

	if err := ValidateDnsRecord(params.Rrtype, params.Content); err != nil {
		return nil, err
	}

	name := NormalizeDomain(params.Name)
	if name == "" || name == zone {
		name = "@"
	} else {
		suffix := "." + zone
		if strings.HasSuffix(name, suffix) {
			name = strings.TrimSuffix(name, suffix)
		}
	}

	// Check conflicts
	existing, _ := s.dnsQ.ListDnsRecordsByZoneAndName(ctx, dnsdb.ListDnsRecordsByZoneAndNameParams{
		ZoneID: hz.ID,
		Lower:  name,
	})

	for _, rec := range existing {
		if rec.Managed {
			return nil, fmt.Errorf("cannot add record at %s: managed record exists (attached to a service)", name)
		}
	}

	// CNAME exclusivity: CNAME can't coexist with other records at same name
	if params.Rrtype == "CNAME" && len(existing) > 0 {
		return nil, fmt.Errorf("cannot add CNAME at %s: other records already exist at this name", name)
	}
	for _, rec := range existing {
		if rec.Rrtype == "CNAME" {
			return nil, fmt.Errorf("cannot add %s record at %s: CNAME already exists at this name", params.Rrtype, name)
		}
	}

	// CNAME not allowed at apex
	if params.Rrtype == "CNAME" && name == "@" {
		return nil, fmt.Errorf("CNAME records are not allowed at the zone apex")
	}

	ttl := int32(300)
	if params.TTL > 0 {
		ttl = int32(params.TTL)
	}

	dr, err := s.dnsQ.CreateDnsRecord(ctx, dnsdb.CreateDnsRecordParams{
		ID:      shortuuid.New(),
		ZoneID:  hz.ID,
		Name:    name,
		Rrtype:  params.Rrtype,
		Content: params.Content,
		Ttl:     ttl,
		Managed: false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create dns record: %w", err)
	}

	// Sync to PowerDNS
	fqdn := name + "." + zone
	if name == "@" {
		fqdn = zone
	}
	if err := s.pdns.UpsertRecord(zone, fqdn, params.Rrtype, params.Content, int(ttl)); err != nil {
		// Best-effort: record is in DB, PowerDNS sync failed
		s.logger.Error("failed to sync dns record to PowerDNS", "error", err, "zone", zone, "name", name)
	}

	return &AddDnsRecordResult{Record: dr}, nil
}

type DeleteDnsRecordParams struct {
	UserID   string
	Zone     string
	RecordID string
}

func (s *Service) DeleteDnsRecord(ctx context.Context, params DeleteDnsRecordParams) error {
	zone := NormalizeDomain(params.Zone)

	hz, err := s.dnsQ.GetHostedZoneByZone(ctx, zone)
	if err != nil {
		return fmt.Errorf("hosted zone not found: %s", zone)
	}
	if hz.UserID != params.UserID {
		return fmt.Errorf("hosted zone not found: %s", zone)
	}

	dr, err := s.dnsQ.GetDnsRecordByID(ctx, params.RecordID)
	if err != nil {
		return fmt.Errorf("dns record not found: %s", params.RecordID)
	}
	if dr.ZoneID != hz.ID {
		return fmt.Errorf("dns record does not belong to zone %s", zone)
	}
	if dr.Managed {
		return fmt.Errorf("cannot delete managed record (attached to a service — use remove_custom_domain instead)")
	}

	// Delete from PowerDNS first
	fqdn := dr.Name + "." + zone
	if dr.Name == "@" {
		fqdn = zone
	}
	if err := s.pdns.DeleteRecord(zone, fqdn, dr.Rrtype); err != nil {
		s.logger.Error("failed to delete dns record from PowerDNS", "error", err, "zone", zone, "name", dr.Name)
	}

	if err := s.dnsQ.DeleteDnsRecord(ctx, dr.ID); err != nil {
		return fmt.Errorf("failed to delete dns record: %w", err)
	}

	return nil
}

type ListDnsRecordsParams struct {
	UserID string
	Zone   string
}

func (s *Service) ListDnsRecords(ctx context.Context, params ListDnsRecordsParams) ([]dnsdb.DnsRecord, error) {
	zone := NormalizeDomain(params.Zone)

	hz, err := s.dnsQ.GetHostedZoneByZone(ctx, zone)
	if err != nil {
		return nil, fmt.Errorf("hosted zone not found: %s", zone)
	}
	if hz.UserID != params.UserID {
		return nil, fmt.Errorf("hosted zone not found: %s", zone)
	}

	return s.dnsQ.ListDnsRecordsByZoneID(ctx, hz.ID)
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
