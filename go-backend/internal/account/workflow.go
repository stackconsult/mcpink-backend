package account

import (
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

const TaskQueue = "default"

func SetupAccountWorkflow(ctx workflow.Context, input SetupAccountInput) (SetupAccountResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting SetupAccountWorkflow", "userID", input.UserID)

	activityOptions := workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	}
	ctx = workflow.WithActivityOptions(ctx, activityOptions)

	var activities *Activities

	createProjectInput := CreateDefaultProjectInput(input)

	var createProjectResult CreateDefaultProjectResult
	err := workflow.ExecuteActivity(ctx, activities.CreateDefaultProject, createProjectInput).Get(ctx, &createProjectResult)
	if err != nil {
		logger.Error("Failed to create default project", "error", err)
		return SetupAccountResult{}, err
	}

	logger.Info("SetupAccountWorkflow completed",
		"userID", input.UserID,
		"defaultProjectID", createProjectResult.ProjectID)

	return SetupAccountResult{
		DefaultProjectID: createProjectResult.ProjectID,
	}, nil
}

func RegisterWorkflowsAndActivities(w worker.Worker, activities *Activities) {
	w.RegisterWorkflow(SetupAccountWorkflow)
	w.RegisterActivity(activities.CreateDefaultProject)
}
