package k8sdeployments

import "go.temporal.io/sdk/worker"

func RegisterWorkflowsAndActivities(w worker.Worker, activities *Activities) {
	w.RegisterWorkflow(CreateServiceWorkflow)
	w.RegisterWorkflow(RedeployServiceWorkflow)
	w.RegisterWorkflow(DeleteServiceWorkflow)
	w.RegisterActivity(activities.CloneRepo)
	w.RegisterActivity(activities.BuildAndPush)
	w.RegisterActivity(activities.Deploy)
	w.RegisterActivity(activities.WaitForRollout)
	w.RegisterActivity(activities.DeleteService)
}
