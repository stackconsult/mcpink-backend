package k8sdeployments

import (
	"errors"
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

const (
	StatusRunning = "running"
	StatusFailed  = "failed"
	StatusDeleted = "deleted"
)

func CreateServiceWorkflow(ctx workflow.Context, input CreateServiceWorkflowInput) (CreateServiceWorkflowResult, error) {
	return deployService(ctx, input)
}

func RedeployServiceWorkflow(ctx workflow.Context, input RedeployServiceWorkflowInput) (RedeployServiceWorkflowResult, error) {
	return deployService(ctx, input)
}

func deployService(ctx workflow.Context, input DeployServiceInput) (DeployServiceResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting deploy", "serviceID", input.ServiceID, "deploymentID", input.DeploymentID, "repo", input.Repo, "commitSHA", input.CommitSHA)

	var activities *Activities

	statusCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Second,
		RetryPolicy:         &temporal.RetryPolicy{MaximumAttempts: 3},
	})

	markFailed := func(errMsg string) {
		_ = workflow.ExecuteActivity(statusCtx, activities.MarkDeploymentFailed, MarkDeploymentFailedInput{
			DeploymentID: input.DeploymentID,
			ErrorMessage: errMsg,
		}).Get(ctx, nil)
	}

	fail := func(err error) (DeployServiceResult, error) {
		markFailed(err.Error())
		return DeployServiceResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	// Mark deployment as building
	if err := workflow.ExecuteActivity(statusCtx, activities.UpdateDeploymentBuilding, UpdateDeploymentBuildingInput{
		DeploymentID: input.DeploymentID,
	}).Get(ctx, nil); err != nil {
		return DeployServiceResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: fmt.Sprintf("update deployment building: %v", err),
		}, fmt.Errorf("update deployment building: %w", err)
	}

	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: fmt.Sprintf("build-%s-%s", input.ServiceID, input.CommitSHA),
	})
	var buildResult BuildServiceWorkflowResult
	if err := workflow.ExecuteChildWorkflow(childCtx, BuildServiceWorkflow, BuildServiceWorkflowInput{
		ServiceID:      input.ServiceID,
		DeploymentID:   input.DeploymentID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    input.GitProvider,
		InstallationID: input.InstallationID,
		CommitSHA:      input.CommitSHA,
	}).Get(ctx, &buildResult); err != nil {
		return fail(err)
	}

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var deployResult DeployResult
	if err := workflow.ExecuteActivity(actCtx, activities.Deploy, DeployInput{
		ServiceID:  input.ServiceID,
		ImageRef:   buildResult.ImageRef,
		CommitSHA:  buildResult.CommitSHA,
		AppsDomain: input.AppsDomain,
		Port:       buildResult.Port,
	}).Get(ctx, &deployResult); err != nil {
		return fail(err)
	}

	// Mark deployment as deploying (waiting for rollout)
	if err := workflow.ExecuteActivity(statusCtx, activities.UpdateDeploymentDeploying, UpdateDeploymentDeployingInput{
		DeploymentID: input.DeploymentID,
	}).Get(ctx, nil); err != nil {
		logger.Warn("Failed to mark deployment deploying", "error", err)
	}

	rolloutCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 3 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
	})

	var waitResult WaitForRolloutResult
	if err := workflow.ExecuteActivity(rolloutCtx, activities.WaitForRollout, WaitForRolloutInput{
		Namespace:      deployResult.Namespace,
		DeploymentName: deployResult.DeploymentName,
	}).Get(ctx, &waitResult); err != nil {
		return fail(err)
	}

	// Mark deployment as active, supersede old, set pointer
	if err := workflow.ExecuteActivity(statusCtx, activities.MarkDeploymentActive, MarkDeploymentActiveInput{
		ServiceID:    input.ServiceID,
		DeploymentID: input.DeploymentID,
		URL:          deployResult.URL,
		CommitSHA:    buildResult.CommitSHA,
		ImageRef:     buildResult.ImageRef,
	}).Get(ctx, nil); err != nil {
		return DeployServiceResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: fmt.Sprintf("mark deployment active: %v", err),
		}, fmt.Errorf("mark deployment active: %w", err)
	}

	return DeployServiceResult{
		ServiceID: input.ServiceID,
		Status:    waitResult.Status,
		URL:       deployResult.URL,
		CommitSHA: buildResult.CommitSHA,
	}, nil
}

func BuildServiceWorkflow(ctx workflow.Context, input BuildServiceWorkflowInput) (BuildServiceWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting build", "serviceID", input.ServiceID, "repo", input.Repo)

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    2,
		},
	})
	var activities *Activities

	cleanupSource := func(path string) {
		if path == "" {
			return
		}
		if err := workflow.ExecuteActivity(actCtx, activities.CleanupSource, path).Get(ctx, nil); err != nil {
			logger.Warn("CleanupSource failed", "sourcePath", path, "error", err)
		}
	}

	trySkipBuild := func(commitSHA string) (*BuildServiceWorkflowResult, bool) {
		if commitSHA == "" {
			return nil, false
		}

		var resolveResult ResolveImageRefResult
		if err := workflow.ExecuteActivity(actCtx, activities.ResolveImageRef, ResolveImageRefInput{
			ServiceID: input.ServiceID,
			CommitSHA: commitSHA,
		}).Get(ctx, &resolveResult); err != nil {
			logger.Warn("ResolveImageRef failed; continuing with full build",
				"serviceID", input.ServiceID, "commitSHA", commitSHA, "error", err)
			return nil, false
		}

		var exists bool
		if err := workflow.ExecuteActivity(actCtx, activities.ImageExists, resolveResult.ImageRef).Get(ctx, &exists); err != nil {
			logger.Warn("ImageExists check failed; continuing with full build",
				"imageRef", resolveResult.ImageRef, "error", err)
			return nil, false
		}
		if !exists {
			return nil, false
		}

		logger.Info("Skipping build; image already exists",
			"serviceID", input.ServiceID, "imageRef", resolveResult.ImageRef, "commitSHA", commitSHA)
		return &BuildServiceWorkflowResult{
			ImageRef:  resolveResult.ImageRef,
			CommitSHA: commitSHA,
			// Port unknown in skip path — Deploy will re-read from DB
		}, true
	}

	if result, ok := trySkipBuild(input.CommitSHA); ok {
		return *result, nil
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			logger.Warn("Retrying build after missing source path", "serviceID", input.ServiceID, "attempt", attempt)
		}

		var cloneResult CloneRepoResult
		err := workflow.ExecuteActivity(actCtx, activities.CloneRepo, CloneRepoInput{
			ServiceID:      input.ServiceID,
			Repo:           input.Repo,
			Branch:         input.Branch,
			GitProvider:    input.GitProvider,
			InstallationID: input.InstallationID,
			CommitSHA:      input.CommitSHA,
		}).Get(ctx, &cloneResult)
		if err != nil {
			return BuildServiceWorkflowResult{}, fmt.Errorf("clone failed: %w", err)
		}

		var resolveResult ResolveBuildContextResult
		err = workflow.ExecuteActivity(actCtx, activities.ResolveBuildContext, ResolveBuildContextInput{
			ServiceID:  input.ServiceID,
			SourcePath: cloneResult.SourcePath,
			CommitSHA:  cloneResult.CommitSHA,
		}).Get(ctx, &resolveResult)
		if err != nil {
			cleanupSource(cloneResult.SourcePath)
			if isSourcePathMissing(err) && attempt < 2 {
				continue
			}
			return BuildServiceWorkflowResult{}, fmt.Errorf("resolve failed: %w", err)
		}

		var imageExists bool
		err = workflow.ExecuteActivity(actCtx, activities.ImageExists, resolveResult.ImageRef).Get(ctx, &imageExists)
		if err != nil {
			logger.Warn("ImageExists check failed after resolve; continuing with build",
				"imageRef", resolveResult.ImageRef, "error", err)
		} else if imageExists {
			cleanupSource(cloneResult.SourcePath)
			logger.Info("Skipping build after resolve; image already exists",
				"serviceID", input.ServiceID, "imageRef", resolveResult.ImageRef, "commitSHA", cloneResult.CommitSHA)
			return BuildServiceWorkflowResult{
				ImageRef:  resolveResult.ImageRef,
				CommitSHA: cloneResult.CommitSHA,
				Port:      resolveResult.Port,
			}, nil
		}

		buildInput := BuildImageInput{
			SourcePath:       resolveResult.EffectiveSourcePath,
			ImageRef:         resolveResult.ImageRef,
			BuildPack:        resolveResult.BuildPack,
			Name:             resolveResult.Name,
			Namespace:        resolveResult.Namespace,
			EnvVars:          resolveResult.EnvVars,
			PublishDirectory: resolveResult.PublishDirectory,
			DockerfilePath:   resolveResult.DockerfilePath,
			BuildCommand:     resolveResult.BuildCommand,
			StartCommand:     resolveResult.StartCommand,
		}
		var buildResult BuildImageResult
		switch resolveResult.BuildPack {
		case "railpack":
			if resolveResult.PublishDirectory != "" {
				err = workflow.ExecuteActivity(actCtx, activities.RailpackStaticBuild, buildInput).Get(ctx, &buildResult)
			} else {
				err = workflow.ExecuteActivity(actCtx, activities.RailpackBuild, buildInput).Get(ctx, &buildResult)
			}
		case "dockerfile":
			err = workflow.ExecuteActivity(actCtx, activities.DockerfileBuild, buildInput).Get(ctx, &buildResult)
		case "static":
			err = workflow.ExecuteActivity(actCtx, activities.StaticBuild, buildInput).Get(ctx, &buildResult)
		default:
			cleanupSource(cloneResult.SourcePath)
			return BuildServiceWorkflowResult{}, fmt.Errorf("unsupported build pack: %s", resolveResult.BuildPack)
		}
		cleanupSource(cloneResult.SourcePath)
		if err != nil {
			if isSourcePathMissing(err) && attempt < 2 {
				continue
			}
			return BuildServiceWorkflowResult{}, fmt.Errorf("build failed (%s): %w", resolveResult.BuildPack, err)
		}

		return BuildServiceWorkflowResult{
			ImageRef:  buildResult.ImageRef,
			CommitSHA: cloneResult.CommitSHA,
			Port:      resolveResult.Port,
		}, nil
	}

	return BuildServiceWorkflowResult{}, fmt.Errorf("build failed: source path missing after re-clone")
}

func isSourcePathMissing(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "source_path_missing"
}

func AttachCustomDomainWorkflow(ctx workflow.Context, input AttachCustomDomainWorkflowInput) (AttachCustomDomainWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting attach custom domain",
		"customDomainID", input.CustomDomainID,
		"serviceName", input.ServiceName,
		"domain", input.CustomDomain)

	shortCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	waitCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 1.0,
			MaximumAttempts:    1,
		},
	})

	var activities *Activities

	markFailed := func(errMsg string) (AttachCustomDomainWorkflowResult, error) {
		_ = workflow.ExecuteActivity(shortCtx, activities.UpdateCustomDomainDBStatus, UpdateCustomDomainStatusInput{
			CustomDomainID: input.CustomDomainID,
			Status:         "failed",
		}).Get(ctx, nil)
		return AttachCustomDomainWorkflowResult{
			Status:       "failed",
			ErrorMessage: errMsg,
		}, fmt.Errorf("attach custom domain failed: %s", errMsg)
	}

	// Phase 1: Create Certificate CR (no Ingress yet → no Traefik 308 redirect → HTTP-01 works)
	certName := input.ServiceName + "-cd"
	if err := workflow.ExecuteActivity(shortCtx, activities.ApplyCustomDomainCertificate, ApplyCustomDomainCertificateInput{
		Namespace:   input.Namespace,
		ServiceName: input.ServiceName,
		Domain:      input.CustomDomain,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("failed to apply certificate: %v", err))
	}

	// Phase 2: Wait for certificate to be ready
	if err := workflow.ExecuteActivity(waitCtx, activities.WaitForCertificateReady, WaitForCertificateReadyInput{
		Namespace:       input.Namespace,
		CertificateName: certName,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("certificate provisioning failed: %v", err))
	}

	// Phase 3: Create Ingress WITH TLS (cert exists now, Traefik 308 redirect is fine)
	if err := workflow.ExecuteActivity(shortCtx, activities.ApplyCustomDomainIngress, ApplyCustomDomainIngressInput{
		Namespace:   input.Namespace,
		ServiceName: input.ServiceName,
		Domain:      input.CustomDomain,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("failed to apply ingress: %v", err))
	}

	// Phase 4: Mark active
	if err := workflow.ExecuteActivity(shortCtx, activities.UpdateCustomDomainDBStatus, UpdateCustomDomainStatusInput{
		CustomDomainID: input.CustomDomainID,
		Status:         "active",
	}).Get(ctx, nil); err != nil {
		return AttachCustomDomainWorkflowResult{
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("ingress applied but failed to update status: %v", err),
		}, err
	}

	return AttachCustomDomainWorkflowResult{Status: "active"}, nil
}

func DetachCustomDomainWorkflow(ctx workflow.Context, input DetachCustomDomainWorkflowInput) (DetachCustomDomainWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting detach custom domain",
		"customDomainID", input.CustomDomainID,
		"serviceName", input.ServiceName)

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var activities *Activities

	if err := workflow.ExecuteActivity(actCtx, activities.DeleteCustomDomainIngress, DeleteCustomDomainIngressInput{
		Namespace:   input.Namespace,
		ServiceName: input.ServiceName,
	}).Get(ctx, nil); err != nil {
		return DetachCustomDomainWorkflowResult{
			Status:       "failed",
			ErrorMessage: err.Error(),
		}, err
	}

	return DetachCustomDomainWorkflowResult{Status: "deleted"}, nil
}

func DeleteServiceWorkflow(ctx workflow.Context, input DeleteServiceWorkflowInput) (DeleteServiceWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting delete", "serviceID", input.ServiceID, "namespace", input.Namespace, "name", input.Name)

	ctx = workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var activities *Activities
	var deleteResult DeleteServiceResult
	if err := workflow.ExecuteActivity(ctx, activities.DeleteService, DeleteServiceInput(input)).Get(ctx, &deleteResult); err != nil {
		return DeleteServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	if err := workflow.ExecuteActivity(ctx, activities.SoftDeleteService, input.ServiceID).Get(ctx, nil); err != nil {
		logger.Error("Failed to soft-delete service record", "serviceID", input.ServiceID, "error", err)
		return DeleteServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: fmt.Sprintf("k8s resources deleted but failed to soft-delete service record: %v", err),
		}, fmt.Errorf("soft-delete service record: %w", err)
	}

	return DeleteServiceWorkflowResult{
		ServiceID: input.ServiceID,
		Status:    StatusDeleted,
	}, nil
}
