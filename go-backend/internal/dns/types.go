package dns

// Workflow inputs and results

type ActivateZoneInput struct {
	ZoneID      string
	Zone        string
	Nameservers []string
	IngressIP   string
}

type ActivateZoneResult struct {
	Status       string
	ErrorMessage string
}

type DeactivateZoneInput struct {
	ZoneID    string
	Zone      string
	Namespace string
}

type DeactivateZoneResult struct {
	Status       string
	ErrorMessage string
}

type AttachSubdomainInput struct {
	ZoneRecordID string
	Zone         string
	Name         string
	IngressIP    string
	Namespace    string
	ServiceName  string
	ServicePort  int32
}

type AttachSubdomainResult struct {
	Status       string
	ErrorMessage string
}

type DetachSubdomainInput struct {
	Zone        string
	Name        string
	Namespace   string
	ServiceName string
}

type DetachSubdomainResult struct {
	Status       string
	ErrorMessage string
}

// Activity inputs

type CreateZoneInput struct {
	Zone        string
	Nameservers []string
	IngressIP   string
}

type DeleteZoneInput struct {
	Zone string
}

type UpsertRecordInput struct {
	Zone    string
	Name    string
	Type    string
	Content string
	TTL     int
}

type DeleteRecordInput struct {
	Zone string
	Name string
	Type string
}

type WaitForNSInput struct {
	Zone       string
	ExpectedNS []string
}

type ApplyWildcardCertInput struct {
	Zone      string
	Namespace string
}

type WaitForCertReadyInput struct {
	Zone      string
	Namespace string
}

type UpdateZoneStatusInput struct {
	ZoneID             string
	Status             string
	WildcardCertSecret string
}

type ApplySubdomainIngressInput struct {
	Namespace   string
	ServiceName string
	FQDN        string
	ServicePort int32
}

type DeleteIngressInput struct {
	Namespace   string
	IngressName string
}

type DeleteCertificateInput struct {
	Namespace       string
	CertificateName string
}

type ApplyCertLoaderInput struct {
	Zone       string
	SecretName string
}

type DeleteCertLoaderInput struct {
	Zone string
}

type EnsureRedirectMiddlewareInput struct {
	Namespace string
}
