package dns

import (
	"fmt"
	"time"

	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func ActivateZoneWorkflow(ctx workflow.Context, input ActivateZoneInput) (ActivateZoneResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting zone activation", "zoneID", input.ZoneID, "zone", input.Zone)

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

	var a *Activities

	markFailed := func(errMsg string) (ActivateZoneResult, error) {
		_ = workflow.ExecuteActivity(shortCtx, a.UpdateZoneStatus, UpdateZoneStatusInput{
			ZoneID: input.ZoneID,
			Status: "failed",
		}).Get(ctx, nil)
		return ActivateZoneResult{
			Status:       "failed",
			ErrorMessage: errMsg,
		}, fmt.Errorf("activate zone failed: %s", errMsg)
	}

	if err := workflow.ExecuteActivity(shortCtx, a.CreateZone, CreateZoneInput{
		Zone:        input.Zone,
		Nameservers: input.Nameservers,
		IngressIP:   input.IngressIP,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("failed to create zone: %v", err))
	}

	certNamespace := "dp-system"
	if err := workflow.ExecuteActivity(shortCtx, a.ApplyWildcardCert, ApplyWildcardCertInput{
		Zone:      input.Zone,
		Namespace: certNamespace,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("failed to apply wildcard certificate: %v", err))
	}

	if err := workflow.ExecuteActivity(waitCtx, a.WaitForCertReady, WaitForCertReadyInput{
		Zone:      input.Zone,
		Namespace: certNamespace,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("wildcard certificate provisioning failed: %v", err))
	}

	certSecret := wildcardSecretName(input.Zone)

	// Create cert-loader IngressRoute in dp-system so Traefik loads the
	// wildcard cert into its global TLS pool for SNI-based selection.
	if err := workflow.ExecuteActivity(shortCtx, a.ApplyCertLoader, ApplyCertLoaderInput{
		Zone:       input.Zone,
		SecretName: certSecret,
	}).Get(ctx, nil); err != nil {
		return markFailed(fmt.Sprintf("failed to create cert loader: %v", err))
	}

	if err := workflow.ExecuteActivity(shortCtx, a.UpdateZoneStatus, UpdateZoneStatusInput{
		ZoneID:             input.ZoneID,
		Status:             "active",
		WildcardCertSecret: certSecret,
	}).Get(ctx, nil); err != nil {
		return ActivateZoneResult{
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("zone activated but failed to update status: %v", err),
		}, err
	}

	return ActivateZoneResult{Status: "active"}, nil
}

func AttachSubdomainWorkflow(ctx workflow.Context, input AttachSubdomainInput) (AttachSubdomainResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting subdomain attach",
		"zone", input.Zone, "name", input.Name, "serviceName", input.ServiceName)

	shortCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 30 * time.Second,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var a *Activities

	fqdn := input.Zone
	if input.Name != "@" {
		fqdn = input.Name + "." + input.Zone
	}

	if err := workflow.ExecuteActivity(shortCtx, a.UpsertRecord, UpsertRecordInput{
		Zone:    input.Zone,
		Name:    fqdn,
		Type:    "A",
		Content: input.IngressIP,
		TTL:     60,
	}).Get(ctx, nil); err != nil {
		return AttachSubdomainResult{
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("failed to create A record: %v", err),
		}, err
	}

	if err := workflow.ExecuteActivity(shortCtx, a.EnsureRedirectMiddleware, EnsureRedirectMiddlewareInput{
		Namespace: input.Namespace,
	}).Get(ctx, nil); err != nil {
		return AttachSubdomainResult{
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("failed to create redirect middleware: %v", err),
		}, err
	}

	if err := workflow.ExecuteActivity(shortCtx, a.ApplySubdomainIngress, ApplySubdomainIngressInput{
		Namespace:   input.Namespace,
		ServiceName: input.ServiceName,
		FQDN:        fqdn,
		ServicePort: input.ServicePort,
	}).Get(ctx, nil); err != nil {
		return AttachSubdomainResult{
			Status:       "failed",
			ErrorMessage: fmt.Sprintf("failed to apply ingress: %v", err),
		}, err
	}

	return AttachSubdomainResult{Status: "active"}, nil
}

func DetachSubdomainWorkflow(ctx workflow.Context, input DetachSubdomainInput) (DetachSubdomainResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting subdomain detach",
		"zone", input.Zone, "name", input.Name, "serviceName", input.ServiceName)

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var a *Activities

	ingressName := input.ServiceName + "-dz"
	if err := workflow.ExecuteActivity(actCtx, a.DeleteIngress, DeleteIngressInput{
		Namespace:   input.Namespace,
		IngressName: ingressName,
	}).Get(ctx, nil); err != nil {
		return DetachSubdomainResult{
			Status:       "failed",
			ErrorMessage: err.Error(),
		}, err
	}

	fqdn := input.Zone
	if input.Name != "@" {
		fqdn = input.Name + "." + input.Zone
	}
	if err := workflow.ExecuteActivity(actCtx, a.DeleteRecord, DeleteRecordInput{
		Zone: input.Zone,
		Name: fqdn,
		Type: "A",
	}).Get(ctx, nil); err != nil {
		return DetachSubdomainResult{
			Status:       "failed",
			ErrorMessage: err.Error(),
		}, err
	}

	return DetachSubdomainResult{Status: "deleted"}, nil
}

func DeactivateZoneWorkflow(ctx workflow.Context, input DeactivateZoneInput) (DeactivateZoneResult, error) {
	logger := workflow.GetLogger(ctx)
	logger.Info("Starting zone deactivation", "zoneID", input.ZoneID, "zone", input.Zone)

	actCtx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: 2 * time.Minute,
		RetryPolicy: &temporal.RetryPolicy{
			InitialInterval:    time.Second,
			BackoffCoefficient: 2.0,
			MaximumInterval:    30 * time.Second,
			MaximumAttempts:    3,
		},
	})

	var a *Activities

	// Delete the cert-loader IngressRoute from dp-system
	_ = workflow.ExecuteActivity(actCtx, a.DeleteCertLoader, DeleteCertLoaderInput{
		Zone: input.Zone,
	}).Get(ctx, nil)

	certName := wildcardCertName(input.Zone)
	certNamespace := "dp-system"
	if input.Namespace != "" {
		certNamespace = input.Namespace
	}
	_ = workflow.ExecuteActivity(actCtx, a.DeleteCertificate, DeleteCertificateInput{
		Namespace:       certNamespace,
		CertificateName: certName,
	}).Get(ctx, nil)

	if err := workflow.ExecuteActivity(actCtx, a.DeleteZone, DeleteZoneInput{
		Zone: input.Zone,
	}).Get(ctx, nil); err != nil {
		return DeactivateZoneResult{
			Status:       "failed",
			ErrorMessage: err.Error(),
		}, err
	}

	return DeactivateZoneResult{Status: "deleted"}, nil
}
