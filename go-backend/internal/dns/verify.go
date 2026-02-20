package dns

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
)

// txtVerifyHost returns the TXT verification hostname for a zone.
// Always a child record under the zone, verified before NS delegation.
//
//	dogs.breacher.org → _dp-verify.dogs.breacher.org
//	mydomain.com      → _dp-verify.mydomain.com
func txtVerifyHost(zone string) string {
	return "_dp-verify." + NormalizeDomain(zone)
}

func VerifyTXT(zone, expectedToken string) (bool, error) {
	host := txtVerifyHost(zone)
	records, err := net.LookupTXT(host)
	if err != nil {
		return false, fmt.Errorf("TXT lookup failed for %s: %w", host, err)
	}

	needle := "dp-verify=" + expectedToken
	for _, r := range records {
		if strings.TrimSpace(r) == needle {
			return true, nil
		}
	}
	return false, nil
}

// VerifyNSRecords checks each expected NS record individually and returns
// a map of normalized NS hostname → verified status.
func VerifyNSRecords(zone string, expectedNS []string) map[string]bool {
	result := make(map[string]bool, len(expectedNS))
	for _, ns := range expectedNS {
		result[NormalizeDomain(ns)] = false
	}

	nsRecords, err := net.LookupNS(NormalizeDomain(zone))
	if err != nil {
		return result
	}

	found := make(map[string]bool)
	for _, ns := range nsRecords {
		found[NormalizeDomain(ns.Host)] = true
	}

	for _, expected := range expectedNS {
		result[NormalizeDomain(expected)] = found[NormalizeDomain(expected)]
	}
	return result
}

func GenerateVerificationToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func NormalizeDomain(d string) string {
	d = strings.TrimSpace(d)
	d = strings.ToLower(d)
	d = strings.TrimSuffix(d, ".")
	return d
}

func ValidateHostedZone(zone, platformDomain string) error {
	zone = NormalizeDomain(zone)

	if zone == "" {
		return fmt.Errorf("zone is required")
	}

	if strings.Contains(zone, "*") {
		return fmt.Errorf("wildcard zones are not supported")
	}

	if strings.HasSuffix(zone, "."+NormalizeDomain(platformDomain)) || zone == NormalizeDomain(platformDomain) {
		return fmt.Errorf("cannot delegate a %s subdomain", platformDomain)
	}

	if !strings.Contains(zone, ".") {
		return fmt.Errorf("invalid zone: must be a domain name (e.g. %s.com or apps.%s.com)", zone, zone)
	}

	parts := strings.Split(zone, ".")
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("invalid zone format")
		}
	}

	return nil
}
