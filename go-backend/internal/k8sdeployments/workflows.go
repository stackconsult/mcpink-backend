package k8sdeployments

import (
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

	cloneInput := CloneRepoInput{
		ServiceID:      input.ServiceID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    input.GitProvider,
		InstallationID: input.InstallationID,
		CommitSHA:      input.CommitSHA,
	}
	var cloneResult CloneRepoResult
	err := workflow.ExecuteActivity(ctx, activities.CloneRepo, cloneInput).Get(ctx, &cloneResult)
	if err != nil {
		return CreateServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	buildInput := BuildAndPushInput{
		ServiceID:  input.ServiceID,
		SourcePath: cloneResult.SourcePath,
		CommitSHA:  cloneResult.CommitSHA,
	}
	var buildResult BuildAndPushResult
	err = workflow.ExecuteActivity(ctx, activities.BuildAndPush, buildInput).Get(ctx, &buildResult)
	if err != nil {
		return CreateServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	deployInput := DeployInput{
		ServiceID: input.ServiceID,
		ImageRef:  buildResult.ImageRef,
		CommitSHA: input.CommitSHA,
	}
	var deployResult DeployResult
	err = workflow.ExecuteActivity(ctx, activities.Deploy, deployInput).Get(ctx, &deployResult)
	if err != nil {
		return CreateServiceWorkflowResult{
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
	err = workflow.ExecuteActivity(ctx, activities.WaitForRollout, waitInput).Get(ctx, &waitResult)
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
		CommitSHA: input.CommitSHA,
	}, nil
}

func RedeployServiceWorkflow(ctx workflow.Context, input RedeployServiceWorkflowInput) (RedeployServiceWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting RedeployServiceWorkflow", "serviceID", input.ServiceID, "repo", input.Repo, "commitSHA", input.CommitSHA)

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

	cloneInput := CloneRepoInput{
		ServiceID:      input.ServiceID,
		Repo:           input.Repo,
		Branch:         input.Branch,
		GitProvider:    input.GitProvider,
		InstallationID: input.InstallationID,
		CommitSHA:      input.CommitSHA,
	}
	var cloneResult CloneRepoResult
	err := workflow.ExecuteActivity(ctx, activities.CloneRepo, cloneInput).Get(ctx, &cloneResult)
	if err != nil {
		return RedeployServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	buildInput := BuildAndPushInput{
		ServiceID:  input.ServiceID,
		SourcePath: cloneResult.SourcePath,
		CommitSHA:  cloneResult.CommitSHA,
	}
	var buildResult BuildAndPushResult
	err = workflow.ExecuteActivity(ctx, activities.BuildAndPush, buildInput).Get(ctx, &buildResult)
	if err != nil {
		return RedeployServiceWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       StatusFailed,
			ErrorMessage: err.Error(),
		}, err
	}

	deployInput := DeployInput{
		ServiceID: input.ServiceID,
		ImageRef:  buildResult.ImageRef,
		CommitSHA: input.CommitSHA,
	}
	var deployResult DeployResult
	err = workflow.ExecuteActivity(ctx, activities.Deploy, deployInput).Get(ctx, &deployResult)
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
	err = workflow.ExecuteActivity(ctx, activities.WaitForRollout, waitInput).Get(ctx, &waitResult)
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
		CommitSHA: input.CommitSHA,
	}, nil
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
