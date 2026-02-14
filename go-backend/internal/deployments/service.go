package deployments

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/augustdev/autoclip/internal/dnsverify"
	"github.com/augustdev/autoclip/internal/k8sdeployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/clusters"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/customdomains"
	deploymentsdb "github.com/augustdev/autoclip/internal/storage/pg/generated/deployments"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/githubcreds"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/projects"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/services"
	"github.com/augustdev/autoclip/internal/storage/pg/generated/users"
	"github.com/lithammer/shortuuid/v4"
	enumspb "go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/client"
)

type Service struct {
	temporalClient client.Client
	servicesQ      services.Querier
	deploymentsQ   deploymentsdb.Querier
	projectsQ      projects.Querier
	usersQ         users.Querier
	ghCredsQ       githubcreds.Querier
	customDomainsQ customdomains.Querier
	clusters       map[string]clusters.Cluster
	logger         *slog.Logger
}

func NewService(
	temporalClient client.Client,
	servicesQ services.Querier,
	deploymentsQ deploymentsdb.Querier,
	projectsQ projects.Querier,
	usersQ users.Querier,
	ghCredsQ githubcreds.Querier,
	customDomainsQ customdomains.Querier,
	clusters map[string]clusters.Cluster,
	logger *slog.Logger,
) *Service {
	return &Service{
		temporalClient: temporalClient,
		servicesQ:      servicesQ,
		deploymentsQ:   deploymentsQ,
		projectsQ:      projectsQ,
		usersQ:         usersQ,
		ghCredsQ:       ghCredsQ,
		customDomainsQ: customDomainsQ,
		clusters:       clusters,
		logger:         logger,
	}
}

type CreateServiceInput struct {
	UserID           string
	ProjectRef       string
	GitHubAppUUID    string
	Repo             string
	Branch           string
	Name             string
	BuildPack        string
	Port             string
	EnvVars          []EnvVar
	GitProvider      string // "github" or "gitea"
	Memory           string
	VCPUs            string
	BuildCommand     string
	StartCommand     string
	InstallationID   int64
	PublishDirectory string
	RootDirectory    string
	DockerfilePath   string
	Region           string
}

type CreateServiceResult struct {
	ServiceID    string
	DeploymentID string
	Name         string
	Status       string
	Repo         string
	WorkflowID   string
}

func (s *Service) CreateService(ctx context.Context, input CreateServiceInput) (*CreateServiceResult, error) {
	var projectID string
	if input.ProjectRef != "" {
		project, err := s.getOrCreateProject(ctx, input.UserID, input.ProjectRef)
		if err != nil {
			return nil, err
		}
		projectID = project.ID
	} else {
		project, err := s.projectsQ.GetDefaultProject(ctx, input.UserID)
		if err != nil {
			return nil, fmt.Errorf("default project not found for user")
		}
		projectID = project.ID
	}

	if input.Name != "" {
		_, err := s.servicesQ.GetServiceByNameAndProject(ctx, services.GetServiceByNameAndProjectParams{
			Name:      &input.Name,
			ProjectID: projectID,
		})
		if err == nil {
			return nil, fmt.Errorf("service %q already exists in this project", input.Name)
		}
	}

	svcID := shortuuid.New()
	deploymentID := shortuuid.New()
	workflowID := fmt.Sprintf("deploy-%s", deploymentID)

	gitProvider := input.GitProvider
	if gitProvider == "" {
		gitProvider = "github"
	}

	envVarsJSON, _ := json.Marshal(input.EnvVars)

	buildConfigJSON, _ := json.Marshal(k8sdeployments.BuildConfig{
		RootDirectory:    input.RootDirectory,
		DockerfilePath:   input.DockerfilePath,
		PublishDirectory: input.PublishDirectory,
		BuildCommand:     input.BuildCommand,
		StartCommand:     input.StartCommand,
	})

	memory := input.Memory
	if memory == "" {
		memory = "256Mi"
	}
	vcpus := input.VCPUs
	if vcpus == "" {
		vcpus = "0.5"
	}

	region := input.Region
	if region == "" {
		region = "eu-central-1"
	}
	cluster, ok := s.clusters[region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q", region)
	}
	if cluster.Status != "active" {
		return nil, fmt.Errorf("region %q is not available (status=%s)", region, cluster.Status)
	}

	// Create service row (no deployment state)
	_, err := s.servicesQ.CreateService(ctx, services.CreateServiceParams{
		ID:          svcID,
		UserID:      input.UserID,
		ProjectID:   projectID,
		Repo:        input.Repo,
		Branch:      input.Branch,
		ServerUuid:  "k8s",
		Name:        &input.Name,
		BuildPack:   input.BuildPack,
		Port:        input.Port,
		EnvVars:     envVarsJSON,
		GitProvider: gitProvider,
		BuildConfig: buildConfigJSON,
		Memory:      memory,
		Vcpus:       vcpus,
		Region:      cluster.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create service record: %w", err)
	}

	// Create deployment row with config snapshot
	_, err = s.deploymentsQ.CreateDeployment(ctx, deploymentsdb.CreateDeploymentParams{
		ID:              deploymentID,
		ServiceID:       svcID,
		WorkflowID:      workflowID,
		BuildPack:       input.BuildPack,
		BuildConfig:     buildConfigJSON,
		EnvVarsSnapshot: envVarsJSON,
		Memory:          memory,
		Vcpus:           vcpus,
		Port:            input.Port,
		Trigger:         "api",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	workflowInput := k8sdeployments.CreateServiceWorkflowInput{
		ServiceID:      svcID,
		DeploymentID:   deploymentID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    gitProvider,
		InstallationID: input.InstallationID,
		AppsDomain:     cluster.AppsDomain,
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:                                       workflowID,
		TaskQueue:                                cluster.TaskQueue,
		WorkflowIDConflictPolicy:                 enumspb.WORKFLOW_ID_CONFLICT_POLICY_FAIL,
		WorkflowIDReusePolicy:                    enumspb.WORKFLOW_ID_REUSE_POLICY_REJECT_DUPLICATE,
		WorkflowExecutionErrorWhenAlreadyStarted: true,
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.CreateServiceWorkflow, workflowInput)
	if err != nil {
		s.logger.Error("failed to start deploy workflow",
			"workflowID", workflowID,
			"error", err)
		return nil, fmt.Errorf("failed to start deploy workflow: %w", err)
	}

	runID := run.GetRunID()
	if err := s.deploymentsQ.UpdateDeploymentWorkflowRunID(ctx, deploymentsdb.UpdateDeploymentWorkflowRunIDParams{
		ID:            deploymentID,
		WorkflowRunID: &runID,
	}); err != nil {
		s.logger.Warn("failed to persist workflow run id",
			"deploymentID", deploymentID,
			"workflowID", workflowID,
			"runID", runID,
			"error", err)
	}

	s.logger.Info("started deploy workflow",
		"workflowID", workflowID,
		"runID", run.GetRunID())

	return &CreateServiceResult{
		ServiceID:    svcID,
		DeploymentID: deploymentID,
		Name:         input.Name,
		Status:       "queued",
		Repo:         input.Repo,
		WorkflowID:   workflowID,
	}, nil
}

func (s *Service) ListServices(ctx context.Context, userID string, limit, offset int32) ([]services.Service, error) {
	svcList, err := s.servicesQ.ListServicesByUserID(ctx, services.ListServicesByUserIDParams{
		UserID: userID,
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}
	return svcList, nil
}

func (s *Service) GetProjectByRef(ctx context.Context, userID, ref string) (*projects.Project, error) {
	return s.getOrCreateProject(ctx, userID, ref)
}

func (s *Service) getOrCreateProject(ctx context.Context, userID, ref string) (*projects.Project, error) {
	project, err := s.projectsQ.GetProjectByRef(ctx, projects.GetProjectByRefParams{
		UserID: userID,
		Ref:    ref,
	})
	if err == nil {
		return &project, nil
	}

	s.logger.Info("auto-creating project", "user_id", userID, "ref", ref)
	newProject, err := s.projectsQ.CreateProject(ctx, projects.CreateProjectParams{
		UserID: userID,
		Name:   ref,
		Ref:    ref,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create project: %w", err)
	}
	return &newProject, nil
}

func (s *Service) GetServiceByNameAndProject(ctx context.Context, name, projectID string) (*services.Service, error) {
	svc, err := s.servicesQ.GetServiceByNameAndProject(ctx, services.GetServiceByNameAndProjectParams{
		Name:      &name,
		ProjectID: projectID,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s", name)
	}
	return &svc, nil
}

type GetServiceByNameParams struct {
	Name    string
	Project string // "default" uses user's default project
	UserID  string
}

func (s *Service) GetServiceByName(ctx context.Context, params GetServiceByNameParams) (*services.Service, error) {
	project := params.Project
	if project == "" {
		project = "default"
	}

	svc, err := s.servicesQ.GetServiceByNameAndUserProject(ctx, services.GetServiceByNameAndUserProjectParams{
		Name:   &params.Name,
		UserID: params.UserID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s in project %s", params.Name, project)
	}
	return &svc, nil
}

// GetCurrentDeployment returns the current deployment for a service, or nil if none.
func (s *Service) GetCurrentDeployment(ctx context.Context, serviceID string) (*deploymentsdb.Deployment, error) {
	svc, err := s.servicesQ.GetServiceByID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if svc.CurrentDeploymentID == nil {
		return nil, nil
	}
	dep, err := s.deploymentsQ.GetDeploymentByID(ctx, *svc.CurrentDeploymentID)
	if err != nil {
		return nil, err
	}
	return &dep, nil
}

// GetLatestDeployment returns the latest deployment for a service.
func (s *Service) GetLatestDeployment(ctx context.Context, serviceID string) (*deploymentsdb.Deployment, error) {
	dep, err := s.deploymentsQ.GetLatestDeploymentByServiceID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	return &dep, nil
}

func (s *Service) RedeployService(ctx context.Context, svcID string) (string, error) {
	return s.redeployWithTrigger(ctx, svcID, "manual", "")
}

func (s *Service) RedeployFromGitHubPush(ctx context.Context, svcID, afterSHA, deliveryID string) (string, error) {
	triggerRef := strings.TrimSpace(afterSHA)
	if triggerRef == "" || triggerRef == "0000000000000000000000000000000000000000" {
		triggerRef = strings.TrimSpace(deliveryID)
	}
	return s.redeployWithTrigger(ctx, svcID, "git_push", triggerRef)
}

func (s *Service) RedeployFromInternalGitPush(ctx context.Context, svcID, afterSHA string) (string, error) {
	triggerRef := strings.TrimSpace(afterSHA)
	if triggerRef == "" || triggerRef == "0000000000000000000000000000000000000000" {
		triggerRef = ""
	}
	return s.redeployWithTrigger(ctx, svcID, "git_push", triggerRef)
}

func (s *Service) redeployWithTrigger(ctx context.Context, svcID, trigger, triggerRef string) (string, error) {
	svc, err := s.servicesQ.GetServiceByID(ctx, svcID)
	if err != nil {
		return "", fmt.Errorf("service not found: %w", err)
	}

	cluster, ok := s.clusters[svc.Region]
	if !ok {
		return "", fmt.Errorf("unknown region %q for service %s", svc.Region, svcID)
	}

	deploymentID := shortuuid.New()
	workflowID := fmt.Sprintf("deploy-%s", deploymentID)

	// Cancel in-flight deployments
	cancelledWorkflows, err := s.deploymentsQ.CancelInFlightDeployments(ctx, deploymentsdb.CancelInFlightDeploymentsParams{
		ServiceID: svcID,
		ID:        deploymentID,
	})
	if err != nil {
		s.logger.Warn("failed to cancel in-flight deployments", "serviceID", svcID, "error", err)
	}
	for _, wfID := range cancelledWorkflows {
		if cancelErr := s.temporalClient.CancelWorkflow(ctx, wfID, ""); cancelErr != nil {
			s.logger.Warn("failed to cancel Temporal workflow", "workflowID", wfID, "error", cancelErr)
		}
	}

	// Snapshot current service config into deployment
	envVarsSnapshot := svc.EnvVars
	if len(envVarsSnapshot) == 0 {
		envVarsSnapshot = []byte("[]")
	}
	buildConfig := svc.BuildConfig
	if len(buildConfig) == 0 {
		buildConfig = []byte("{}")
	}

	var triggerRefPtr *string
	if triggerRef != "" {
		triggerRefPtr = &triggerRef
	}
	var commitHashPtr *string
	if triggerRef != "" && trigger == "git_push" {
		commitHashPtr = &triggerRef
	}

	_, err = s.deploymentsQ.CreateDeployment(ctx, deploymentsdb.CreateDeploymentParams{
		ID:              deploymentID,
		ServiceID:       svcID,
		WorkflowID:      workflowID,
		BuildPack:       svc.BuildPack,
		BuildConfig:     buildConfig,
		EnvVarsSnapshot: envVarsSnapshot,
		Memory:          svc.Memory,
		Vcpus:           svc.Vcpus,
		Port:            svc.Port,
		Trigger:         trigger,
		TriggerRef:      triggerRefPtr,
		CommitHash:      commitHashPtr,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create deployment record: %w", err)
	}

	var installationID int64
	if svc.GitProvider == "github" {
		creds, err := s.ghCredsQ.GetGitHubCredsByUserID(ctx, svc.UserID)
		if err == nil && creds.GithubAppInstallationID != nil {
			installationID = *creds.GithubAppInstallationID
		}
	}

	commitSHA := ""
	if triggerRef != "" && trigger == "git_push" {
		commitSHA = triggerRef
	}

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: cluster.TaskQueue,
	}

	we, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.RedeployServiceWorkflow, k8sdeployments.RedeployServiceWorkflowInput{
		ServiceID:      svcID,
		DeploymentID:   deploymentID,
		Repo:           svc.Repo,
		Branch:         svc.Branch,
		GitProvider:    svc.GitProvider,
		InstallationID: installationID,
		CommitSHA:      commitSHA,
		AppsDomain:     cluster.AppsDomain,
	})
	if err != nil {
		s.logger.Error("failed to start redeploy workflow",
			"workflowID", workflowID,
			"error", err)
		return "", fmt.Errorf("failed to start redeploy workflow: %w", err)
	}

	runID := we.GetRunID()
	if err := s.deploymentsQ.UpdateDeploymentWorkflowRunID(ctx, deploymentsdb.UpdateDeploymentWorkflowRunIDParams{
		ID:            deploymentID,
		WorkflowRunID: &runID,
	}); err != nil {
		s.logger.Warn("failed to persist workflow run id", "deploymentID", deploymentID, "error", err)
	}

	s.logger.Info("started redeploy workflow",
		"workflowID", workflowID,
		"runID", we.GetRunID())

	return we.GetID(), nil
}

type DeleteServiceParams struct {
	Name    string
	Project string
	UserID  string
}

type DeleteServiceResult struct {
	ServiceID  string
	Name       string
	WorkflowID string
}

func (s *Service) DeleteService(ctx context.Context, params DeleteServiceParams) (*DeleteServiceResult, error) {
	project := params.Project
	if project == "" {
		project = "default"
	}

	svc, err := s.servicesQ.GetServiceByNameAndUserProject(ctx, services.GetServiceByNameAndUserProjectParams{
		Name:   &params.Name,
		UserID: params.UserID,
		Ref:    project,
	})
	if err != nil {
		return nil, fmt.Errorf("service not found: %s in project %s", params.Name, project)
	}

	cluster, ok := s.clusters[svc.Region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q for service %s", svc.Region, svc.ID)
	}

	var name string
	if svc.Name != nil {
		name = *svc.Name
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
	serviceName := k8sdeployments.ServiceName(name)

	workflowID := fmt.Sprintf("delete-svc-%s", svc.ID)

	workflowOptions := client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: cluster.TaskQueue,
	}

	input := k8sdeployments.DeleteServiceWorkflowInput{
		ServiceID: svc.ID,
		Namespace: namespace,
		Name:      serviceName,
	}

	run, err := s.temporalClient.ExecuteWorkflow(ctx, workflowOptions, k8sdeployments.DeleteServiceWorkflow, input)
	if err != nil {
		return nil, fmt.Errorf("failed to start delete workflow: %w", err)
	}

	s.logger.Info("started delete service workflow",
		"service_id", svc.ID,
		"name", name,
		"workflow_id", run.GetID())

	return &DeleteServiceResult{
		ServiceID:  svc.ID,
		Name:       name,
		WorkflowID: run.GetID(),
	}, nil
}

type AddCustomDomainParams struct {
	Name    string
	Project string
	UserID  string
	Domain  string
}

type AddCustomDomainResult struct {
	ServiceID    string
	Domain       string
	Status       string
	Instructions string
}

func (s *Service) AddCustomDomain(ctx context.Context, params AddCustomDomainParams) (*AddCustomDomainResult, error) {
	domain := dnsverify.NormalizeDomain(params.Domain)

	svc, err := s.GetServiceByName(ctx, GetServiceByNameParams{
		Name:    params.Name,
		Project: params.Project,
		UserID:  params.UserID,
	})
	if err != nil {
		return nil, err
	}

	cluster, ok := s.clusters[svc.Region]
	if !ok {
		return nil, fmt.Errorf("unknown region %q for service %s", svc.Region, svc.ID)
	}

	if err := dnsverify.ValidateCustomDomain(domain, cluster.AppsDomain); err != nil {
		return nil, err
	}

	// Anti-squat: clean up expired/failed records so the real owner can claim the domain
	existing, err := s.customDomainsQ.GetByDomain(ctx, domain)
	if err == nil {
		canReclaim := existing.Status == "failed" ||
			(existing.Status == "pending_dns" && existing.ExpiresAt.Valid && existing.ExpiresAt.Time.Before(time.Now()))
		if canReclaim {
			_ = s.customDomainsQ.Delete(ctx, existing.ID)
		} else {
			return nil, fmt.Errorf("domain %s is already attached to a service", existing.Domain)
		}
	}

	_, err = s.customDomainsQ.GetByServiceID(ctx, svc.ID)
	if err == nil {
		return nil, fmt.Errorf("service %s already has a custom domain; remove it first", params.Name)
	}

	verificationToken := dnsverify.GenerateVerificationToken()
	perServiceTarget := *svc.Name + "." + cluster.CnameTarget

	cd, err := s.customDomainsQ.CreateCustomDomain(ctx, customdomains.CreateCustomDomainParams{
		ServiceID:            svc.ID,
		Domain:               domain,
		ExpectedRecordTarget: perServiceTarget,
		VerificationToken:    verificationToken,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create custom domain record: %w", err)
	}

	instructions := dnsverify.DNSInstructions(domain, perServiceTarget, verificationToken)

	s.logger.Info("custom domain added",
		"service_id", svc.ID,
		"domain", domain,
		"custom_domain_id", cd.ID)

	return &AddCustomDomainResult{
		ServiceID:    svc.ID,
		Domain:       domain,
		Status:       cd.Status,
		Instructions: instructions,
	}, nil
}

type VerifyCustomDomainParams struct {
	Name    string
	Project string
	UserID  string
}

type VerifyCustomDomainResult struct {
	ServiceID string
	Domain    string
	Status    string
	Message   string
}

func (s *Service) VerifyCustomDomain(ctx context.Context, params VerifyCustomDomainParams) (*VerifyCustomDomainResult, error) {
	svc, err := s.GetServiceByName(ctx, GetServiceByNameParams{
		Name:    params.Name,
		Project: params.Project,
		UserID:  params.UserID,
	})
	if err != nil {
		return nil, err
	}

	cd, err := s.customDomainsQ.GetByServiceID(ctx, svc.ID)
	if err != nil {
		return nil, fmt.Errorf("no custom domain configured for service %s", params.Name)
	}

	if cd.Status == "active" {
		return &VerifyCustomDomainResult{
			ServiceID: svc.ID,
			Domain:    cd.Domain,
			Status:    "active",
			Message:   "Custom domain is already active",
		}, nil
	}

	if cd.Status == "provisioning" {
		return &VerifyCustomDomainResult{
			ServiceID: svc.ID,
			Domain:    cd.Domain,
			Status:    "provisioning",
			Message:   "Custom domain is being provisioned; please wait",
		}, nil
	}

	// Step 1: Verify TXT ownership record
	txtOK, txtErr := dnsverify.VerifyTXT(cd.Domain, cd.VerificationToken)
	if txtErr != nil || !txtOK {
		errMsg := fmt.Sprintf("TXT ownership verification failed. Add a TXT record: _dp-verify.%s with value dp-verify=%s", cd.Domain, cd.VerificationToken)
		if txtErr != nil {
			errMsg = txtErr.Error()
		}
		s.customDomainsQ.UpdateError(ctx, customdomains.UpdateErrorParams{
			ID:        cd.ID,
			LastError: &errMsg,
		})
		return &VerifyCustomDomainResult{
			ServiceID: svc.ID,
			Domain:    cd.Domain,
			Status:    cd.Status,
			Message:   errMsg,
		}, nil
	}

	// Step 2: Verify CNAME routing record
	cnameOK, cnameErr := dnsverify.VerifyCNAME(cd.Domain, cd.ExpectedRecordTarget)
	if cnameErr != nil || !cnameOK {
		errMsg := fmt.Sprintf("CNAME record not found or does not match. Expected CNAME %s -> %s", cd.Domain, cd.ExpectedRecordTarget)
		if cnameErr != nil {
			errMsg = cnameErr.Error()
		}
		s.customDomainsQ.UpdateError(ctx, customdomains.UpdateErrorParams{
			ID:        cd.ID,
			LastError: &errMsg,
		})
		return &VerifyCustomDomainResult{
			ServiceID: svc.ID,
			Domain:    cd.Domain,
			Status:    cd.Status,
			Message:   errMsg,
		}, nil
	}

	s.customDomainsQ.UpdateStatus(ctx, customdomains.UpdateStatusParams{
		ID:     cd.ID,
		Status: "provisioning",
	})

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

	workflowID := fmt.Sprintf("attach-cd-%s", cd.ID)

	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: cluster.TaskQueue,
	}, k8sdeployments.AttachCustomDomainWorkflow, k8sdeployments.AttachCustomDomainWorkflowInput{
		CustomDomainID: cd.ID,
		ServiceID:      svc.ID,
		Namespace:      namespace,
		ServiceName:    serviceName,
		CustomDomain:   cd.Domain,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start attach workflow: %w", err)
	}

	return &VerifyCustomDomainResult{
		ServiceID: svc.ID,
		Domain:    cd.Domain,
		Status:    "provisioning",
		Message:   "DNS verified! Provisioning TLS certificate...",
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
	svc, err := s.GetServiceByName(ctx, GetServiceByNameParams{
		Name:    params.Name,
		Project: params.Project,
		UserID:  params.UserID,
	})
	if err != nil {
		return nil, err
	}

	cd, err := s.customDomainsQ.GetByServiceID(ctx, svc.ID)
	if err != nil {
		return nil, fmt.Errorf("no custom domain configured for service %s", params.Name)
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

	workflowID := fmt.Sprintf("detach-cd-%s", cd.ID)

	// Start cleanup workflow BEFORE deleting DB record to prevent orphaned K8s resources
	_, err = s.temporalClient.ExecuteWorkflow(ctx, client.StartWorkflowOptions{
		ID:        workflowID,
		TaskQueue: cluster.TaskQueue,
	}, k8sdeployments.DetachCustomDomainWorkflow, k8sdeployments.DetachCustomDomainWorkflowInput{
		CustomDomainID: cd.ID,
		ServiceID:      svc.ID,
		Namespace:      namespace,
		ServiceName:    serviceName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to start detach workflow: %w", err)
	}

	if err := s.customDomainsQ.Delete(ctx, cd.ID); err != nil {
		return nil, fmt.Errorf("failed to delete custom domain record: %w", err)
	}

	return &RemoveCustomDomainResult{
		ServiceID: svc.ID,
		Message:   fmt.Sprintf("Custom domain %s removed", cd.Domain),
	}, nil
}

func (s *Service) GetCustomDomainByServiceID(ctx context.Context, serviceID string) (*customdomains.CustomDomain, error) {
	cd, err := s.customDomainsQ.GetByServiceID(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	return &cd, nil
}

