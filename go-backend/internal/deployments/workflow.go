package deployments

import (
	"encoding/json"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

func DeployToCoolifyWorkflow(ctx workflow.Context, input DeployWorkflowInput) (DeployWorkflowResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting DeployWorkflow",
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

	// Step 1: Create app record in database
	envVarsJSON, _ := json.Marshal(input.EnvVars)
	createRecordInput := CreateAppRecordInput{
		UserID:     input.UserID,
		WorkflowID: workflow.GetInfo(ctx).WorkflowExecution.ID,
		Repo:       input.Repo,
		Branch:     input.Branch,
		Name:       input.Name,
		BuildPack:  input.BuildPack,
		Port:       input.Port,
		EnvVars:    envVarsJSON,
	}

	var createRecordResult CreateAppRecordResult
	err := workflow.ExecuteActivity(ctx, activities.CreateAppRecord, createRecordInput).Get(ctx, &createRecordResult)
	if err != nil {
		logger.Error("Failed to create app record", "error", err)
		return DeployWorkflowResult{
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	appID := createRecordResult.AppID
	logger.Info("Created app record", "appID", appID)

	// Step 2: Create app in Coolify from private GitHub
	createAppInput := CoolifyAppInput{
		AppID:         appID,
		GitHubAppUUID: input.GitHubAppUUID,
		Repo:          input.Repo,
		Branch:        input.Branch,
		Name:          input.Name,
		BuildPack:     input.BuildPack,
		Port:          input.Port,
	}

	var createAppResult CoolifyAppResult
	err = workflow.ExecuteActivity(ctx, activities.CreateAppFromPrivateGithub, createAppInput).Get(ctx, &createAppResult)
	if err != nil {
		logger.Error("Failed to create app", "error", err)
		_ = markAppFailed(ctx, activities, appID, err.Error())
		return DeployWorkflowResult{
			AppID:        appID,
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	// Step 3: Bulk update environment variables (if any)
	if len(input.EnvVars) > 0 {
		bulkUpdateInput := BulkUpdateEnvsInput{
			CoolifyAppUUID: createAppResult.CoolifyAppUUID,
			EnvVars:        input.EnvVars,
		}

		err = workflow.ExecuteActivity(ctx, activities.BulkUpdateEnvs, bulkUpdateInput).Get(ctx, nil)
		if err != nil {
			logger.Error("Failed to set environment variables", "error", err)
			_ = markAppFailed(ctx, activities, appID, err.Error())
			return DeployWorkflowResult{
				AppID:        appID,
				AppUUID:      createAppResult.CoolifyAppUUID,
				Status:       string(BuildStatusFailed),
				ErrorMessage: err.Error(),
			}, err
		}
	}

	// Step 4: Start the app (triggers deployment)
	startAppInput := StartAppInput{
		CoolifyAppUUID: createAppResult.CoolifyAppUUID,
	}

	var startAppResult StartAppResult
	err = workflow.ExecuteActivity(ctx, activities.StartApp, startAppInput).Get(ctx, &startAppResult)
	if err != nil {
		logger.Error("Failed to start app", "error", err)
		_ = markAppFailed(ctx, activities, appID, err.Error())
		return DeployWorkflowResult{
			AppID:        appID,
			AppUUID:      createAppResult.CoolifyAppUUID,
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	logger.Info("Deployment triggered", "deploymentUUID", startAppResult.DeploymentUUID)

	// Step 5: Wait for app to be running (polls for 3min, retries 3x = 9min total)
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
		AppID:          appID,
		CoolifyAppUUID: createAppResult.CoolifyAppUUID,
	}

	var waitResult WaitForRunningResult
	err = workflow.ExecuteActivity(waitCtx, activities.WaitForRunning, waitInput).Get(ctx, &waitResult)
	if err != nil {
		logger.Error("App failed to reach running state", "error", err)
		_ = markAppFailed(ctx, activities, appID, err.Error())
		return DeployWorkflowResult{
			AppID:        appID,
			AppUUID:      createAppResult.CoolifyAppUUID,
			Status:       string(BuildStatusFailed),
			ErrorMessage: err.Error(),
		}, err
	}

	logger.Info("DeployWorkflow completed successfully",
		"appID", appID,
		"appUUID", createAppResult.CoolifyAppUUID,
		"fqdn", waitResult.FQDN)

	return DeployWorkflowResult{
		AppID:   appID,
		AppUUID: createAppResult.CoolifyAppUUID,
		FQDN:    waitResult.FQDN,
		Status:  string(BuildStatusSuccess),
	}, nil
}

func markAppFailed(ctx workflow.Context, activities *Activities, appID, errorMsg string) error {
	failedInput := UpdateAppFailedInput{
		AppID:        appID,
		ErrorMessage: errorMsg,
	}
	return workflow.ExecuteActivity(ctx, activities.UpdateAppFailed, failedInput).Get(ctx, nil)
}

func RegisterWorkflowsAndActivities(w worker.Worker, activities *Activities) {
	w.RegisterWorkflow(DeployToCoolifyWorkflow)
	w.RegisterActivity(activities.CreateAppRecord)
	w.RegisterActivity(activities.CreateAppFromPrivateGithub)
	w.RegisterActivity(activities.BulkUpdateEnvs)
	w.RegisterActivity(activities.StartApp)
	w.RegisterActivity(activities.WaitForRunning)
	w.RegisterActivity(activities.UpdateAppFailed)
}
