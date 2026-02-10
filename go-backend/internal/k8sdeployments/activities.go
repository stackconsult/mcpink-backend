package k8sdeployments

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
)

var ErrNotImplemented = errors.New("k8s deployment activity not implemented")

type Activities struct {
	logger *slog.Logger
}

func NewActivities(logger *slog.Logger) *Activities {
	return &Activities{logger: logger}
}

func (a *Activities) CloneRepo(ctx context.Context, input CloneRepoInput) (*CloneRepoResult, error) {
	a.logger.Info("CloneRepo activity invoked",
		"serviceID", input.ServiceID,
		"repo", input.Repo,
		"commitSHA", input.CommitSHA)
	return nil, notImplementedError("CloneRepo")
}

func (a *Activities) BuildAndPush(ctx context.Context, input BuildAndPushInput) (*BuildAndPushResult, error) {
	a.logger.Info("BuildAndPush activity invoked",
		"serviceID", input.ServiceID,
		"commitSHA", input.CommitSHA)
	return nil, notImplementedError("BuildAndPush")
}

func (a *Activities) Deploy(ctx context.Context, input DeployInput) (*DeployResult, error) {
	a.logger.Info("Deploy activity invoked",
		"serviceID", input.ServiceID,
		"imageRef", input.ImageRef)
	return nil, notImplementedError("Deploy")
}

func (a *Activities) WaitForRollout(ctx context.Context, input WaitForRolloutInput) (*WaitForRolloutResult, error) {
	a.logger.Info("WaitForRollout activity invoked",
		"namespace", input.Namespace,
		"deployment", input.DeploymentName)
	return nil, notImplementedError("WaitForRollout")
}

func (a *Activities) DeleteService(ctx context.Context, input DeleteServiceInput) (*DeleteServiceResult, error) {
	a.logger.Info("DeleteService activity invoked",
		"serviceID", input.ServiceID,
		"namespace", input.Namespace,
		"name", input.Name)
	return nil, notImplementedError("DeleteService")
}

func notImplementedError(activityName string) error {
	return fmt.Errorf("%w: %s", ErrNotImplemented, activityName)
}
