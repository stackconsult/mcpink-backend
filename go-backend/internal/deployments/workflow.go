package deployments

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func DeployToCoolifyWorkflow(ctx workflow.Context, input DeployWorkflowInput) (DeployWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting DeployWorkflow",
		"serviceID", input.ServiceID,
		"repo", input.Repo,
		"branch", input.Branch)

	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    time.Minute,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	var activities *Activities

	// Step 1: Create app from private GitHub
	createAppInput := CreateAppInput{
		ServiceID:     input.ServiceID,
		GitHubAppUUID: input.GitHubAppUUID,
		Repo:          input.Repo,
		Branch:        input.Branch,
		Name:          input.Name,
		BuildPack:     input.BuildPack,
		Port:          input.Port,
	}

	var createAppResult CreateAppResult
	err := workflow.ExecuteActivity(ctx, activities.CreateAppFromPrivateGithub, createAppInput).Get(ctx, &createAppResult)
	if err != nil {
		logger.Error("Failed to create app", "error", err)
		_ = markServiceFailed(ctx, activities, input.ServiceID, err.Error())
		return DeployWorkflowResult{
			ServiceID:    input.ServiceID,
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	// Step 2: Bulk update environment variables (if any)
	if len(input.EnvVars) > 0 {
		bulkUpdateInput := BulkUpdateEnvsInput{
			CoolifyAppUUID: createAppResult.CoolifyAppUUID,
			EnvVars:        input.EnvVars,
		}

		err = workflow.ExecuteActivity(ctx, activities.BulkUpdateEnvs, bulkUpdateInput).Get(ctx, nil)
		if err != nil {
			logger.Error("Failed to set environment variables", "error", err)
			_ = markServiceFailed(ctx, activities, input.ServiceID, err.Error())
			return DeployWorkflowResult{
				ServiceID:    input.ServiceID,
				AppUUID:      createAppResult.CoolifyAppUUID,
				Status:       string(BuildStatusFailed),
				ErrorMessage: err.Error(),
			}, err
		}
	}

	// Step 3: Start the app (triggers deployment)
	startAppInput := StartAppInput{
		CoolifyAppUUID: createAppResult.CoolifyAppUUID,
	}

	var startAppResult StartAppResult
	err = workflow.ExecuteActivity(ctx, activities.StartApp, startAppInput).Get(ctx, &startAppResult)
	if err != nil {
		logger.Error("Failed to start app", "error", err)
		_ = markServiceFailed(ctx, activities, input.ServiceID, err.Error())
		return DeployWorkflowResult{
			ServiceID:    input.ServiceID,
			AppUUID:      createAppResult.CoolifyAppUUID,
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	logger.Info("Deployment triggered", "deploymentUUID", startAppResult.DeploymentUUID)

	// Step 4: Wait for app to be running (polls for 3min, retries 3x = 9min total)
	waitCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 4 * time.Minute,
		HeartbeatTimeout:    30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    5 * time.Second,
			BackoffCoefficient: 1.0,
			MaximumInterval:    5 * time.Second,
			MaximumAttempts:    3,
		},
	})

	waitInput := WaitForRunningInput{
		ServiceID:      input.ServiceID,
		CoolifyAppUUID: createAppResult.CoolifyAppUUID,
	}

	var waitResult WaitForRunningResult
	err = workflow.ExecuteActivity(waitCtx, activities.WaitForRunning, waitInput).Get(ctx, &waitResult)
	if err != nil {
		logger.Error("App failed to reach running state", "error", err)
		_ = markServiceFailed(ctx, activities, input.ServiceID, err.Error())
		return DeployWorkflowResult{
			ServiceID:    input.ServiceID,
			AppUUID:      createAppResult.CoolifyAppUUID,
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	logger.Info("DeployWorkflow completed successfully",
		"serviceID", input.ServiceID,
		"appUUID", createAppResult.CoolifyAppUUID,
		"fqdn", waitResult.FQDN)

	return DeployWorkflowResult{
		ServiceID: input.ServiceID,
		AppUUID:   createAppResult.CoolifyAppUUID,
		FQDN:      waitResult.FQDN,
		Status:    string(BuildStatusSuccess),
	}, nil
}

func markServiceFailed(ctx workflow.Context, activities *Activities, serviceID, errorMsg string) error {
	failedInput := UpdateServiceFailedInput{
		ServiceID:    serviceID,
		ErrorMessage: errorMsg,
	}
	return workflow.ExecuteActivity(ctx, activities.UpdateServiceFailed, failedInput).Get(ctx, nil)
}

func RegisterWorkflowsAndActivities(w worker.Worker, activities *Activities) {
	w.RegisterWorkflow(DeployToCoolifyWorkflow)
	w.RegisterActivity(activities.CreateAppFromPrivateGithub)
	w.RegisterActivity(activities.BulkUpdateEnvs)
	w.RegisterActivity(activities.StartApp)
	w.RegisterActivity(activities.WaitForRunning)
	w.RegisterActivity(activities.UpdateServiceFailed)
}
