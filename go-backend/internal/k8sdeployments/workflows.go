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
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting CreateServiceWorkflow", "serviceID", input.ServiceID, "repo", input.Repo, "commitSHA", input.CommitSHA)

	// Child workflow: build
	buildInput := BuildServiceWorkflowInput{
		ServiceID:      input.ServiceID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    input.GitProvider,
		InstallationID: input.InstallationID,
		CommitSHA:      input.CommitSHA,
	}
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: fmt.Sprintf("build-%s-%s", input.ServiceID, input.CommitSHA),
	})
	var buildResult BuildServiceWorkflowResult
	err := workflow.ExecuteChildWorkflow(childCtx, BuildServiceWorkflow, buildInput).Get(ctx, &buildResult)
	if err != nil {
		return CreateServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	// Deploy
	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})
	var activities *Activities

	deployInput := DeployInput{
		ServiceID:  input.ServiceID,
		ImageRef:   buildResult.ImageRef,
		CommitSHA:  buildResult.CommitSHA,
		AppsDomain: input.AppsDomain,
	}
	var deployResult DeployResult
	err = workflow.ExecuteActivity(actCtx, activities.Deploy, deployInput).Get(ctx, &deployResult)
	if err != nil {
		return CreateServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	// WaitForRollout
	waitInput := WaitForRolloutInput{
		Namespace:      deployResult.Namespace,
		DeploymentName: deployResult.DeploymentName,
	}
	var waitResult WaitForRolloutResult
	err = workflow.ExecuteActivity(actCtx, activities.WaitForRollout, waitInput).Get(ctx, &waitResult)
	if err != nil {
		return CreateServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	return CreateServiceWorkflowResult{
		ServiceID: input.ServiceID,
		Status:    waitResult.Status,
		URL:       deployResult.URL,
		CommitSHA: buildResult.CommitSHA,
	}, nil
}

func RedeployServiceWorkflow(ctx workflow.Context, input RedeployServiceWorkflowInput) (RedeployServiceWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting RedeployServiceWorkflow", "serviceID", input.ServiceID, "repo", input.Repo, "commitSHA", input.CommitSHA)

	buildInput := BuildServiceWorkflowInput{
		ServiceID:      input.ServiceID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    input.GitProvider,
		InstallationID: input.InstallationID,
		CommitSHA:      input.CommitSHA,
	}
	childCtx := workflow.WithChildOptions(ctx, workflow.ChildWorkflowOptions{
		WorkflowID: fmt.Sprintf("build-%s-%s", input.ServiceID, input.CommitSHA),
	})
	var buildResult BuildServiceWorkflowResult
	err := workflow.ExecuteChildWorkflow(childCtx, BuildServiceWorkflow, buildInput).Get(ctx, &buildResult)
	if err != nil {
		return RedeployServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 5 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})
	var activities *Activities

	deployInput := DeployInput{
		ServiceID:  input.ServiceID,
		ImageRef:   buildResult.ImageRef,
		CommitSHA:  buildResult.CommitSHA,
		AppsDomain: input.AppsDomain,
	}
	var deployResult DeployResult
	err = workflow.ExecuteActivity(actCtx, activities.Deploy, deployInput).Get(ctx, &deployResult)
	if err != nil {
		return RedeployServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	waitInput := WaitForRolloutInput{
		Namespace:      deployResult.Namespace,
		DeploymentName: deployResult.DeploymentName,
	}
	var waitResult WaitForRolloutResult
	err = workflow.ExecuteActivity(actCtx, activities.WaitForRollout, waitInput).Get(ctx, &waitResult)
	if err != nil {
		return RedeployServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	return RedeployServiceWorkflowResult{
		ServiceID: input.ServiceID,
		Status:    waitResult.Status,
		URL:       deployResult.URL,
		CommitSHA: buildResult.CommitSHA,
	}, nil
}

func BuildServiceWorkflow(ctx workflow.Context, input BuildServiceWorkflowInput) (BuildServiceWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting BuildServiceWorkflow", "serviceID", input.ServiceID, "repo", input.Repo)

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 10 * time.Minute,
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
		if cleanupErr := workflow.ExecuteActivity(actCtx, activities.CleanupSource, path).Get(ctx, nil); cleanupErr != nil {
			logger.Warn("CleanupSource failed", "sourcePath", path, "error", cleanupErr)
		}
	}

	for attempt := 1; attempt <= 2; attempt++ {
		if attempt > 1 {
			logger.Warn("Retrying build after missing source path", "serviceID", input.ServiceID, "attempt", attempt)
		}

		// 1. Clone
		cloneInput := CloneRepoInput{
			ServiceID:      input.ServiceID,
			Repo:           input.Repo,
			Branch:         input.Branch,
			GitProvider:    input.GitProvider,
			InstallationID: input.InstallationID,
			CommitSHA:      input.CommitSHA,
		}
		var cloneResult CloneRepoResult
		err := workflow.ExecuteActivity(actCtx, activities.CloneRepo, cloneInput).Get(ctx, &cloneResult)
		if err != nil {
			return BuildServiceWorkflowResult{}, fmt.Errorf("clone failed: %w", err)
		}

		// 2. Resolve build context
		resolveInput := ResolveBuildContextInput{
			ServiceID:  input.ServiceID,
			SourcePath: cloneResult.SourcePath,
			CommitSHA:  cloneResult.CommitSHA,
		}
		var resolveResult ResolveBuildContextResult
		err = workflow.ExecuteActivity(actCtx, activities.ResolveBuildContext, resolveInput).Get(ctx, &resolveResult)
		if err != nil {
			cleanupSource(cloneResult.SourcePath)
			if isSourcePathMissing(err) && attempt < 2 {
				continue
			}
			return BuildServiceWorkflowResult{}, fmt.Errorf("resolve failed: %w", err)
		}

		// 3. Build based on build pack
		buildInput := BuildImageInput{
			SourcePath: cloneResult.SourcePath,
			ImageRef:   resolveResult.ImageRef,
			BuildPack:  resolveResult.BuildPack,
			Name:       resolveResult.Name,
			Namespace:  resolveResult.Namespace,
			EnvVars:    resolveResult.EnvVars,
		}
		var buildResult BuildImageResult
		switch resolveResult.BuildPack {
		case "railpack":
			err = workflow.ExecuteActivity(actCtx, activities.RailpackBuild, buildInput).Get(ctx, &buildResult)
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
		}, nil
	}

	return BuildServiceWorkflowResult{}, fmt.Errorf("build failed: source path missing after re-clone")
}

func isSourcePathMissing(err error) bool {
	var appErr *temporal.ApplicationError
	return errors.As(err, &appErr) && appErr.Type() == "source_path_missing"
}

func DeleteServiceWorkflow(ctx workflow.Context, input DeleteServiceWorkflowInput) (DeleteServiceWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting DeleteServiceWorkflow", "serviceID", input.ServiceID, "namespace", input.Namespace, "name", input.Name)

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

	deleteInput := DeleteServiceInput{
		ServiceID: input.ServiceID,
		Namespace: input.Namespace,
		Name:      input.Name,
	}
	var deleteResult DeleteServiceResult
	err := workflow.ExecuteActivity(ctx, activities.DeleteService, deleteInput).Get(ctx, &deleteResult)
	if err != nil {
		return DeleteServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	status := deleteResult.Status
	if status == "" {
		status = StatusDeleted
	}

	return DeleteServiceWorkflowResult{
		ServiceID: input.ServiceID,
		Status:    status,
	}, nil
}
