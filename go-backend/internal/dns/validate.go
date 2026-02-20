package dns

import (
	"fmt"
	"net"
	"strings"
)

func ValidateDnsRecord(rrtype, content string) error {
	switch strings.ToUpper(rrtype) {
	case "A":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() == nil {
			return fmt.Errorf("invalid A record: %q is not a valid IPv4 address", content)
		}
	case "AAAA":
		ip := net.ParseIP(content)
		if ip == nil || ip.To4() != nil {
			return fmt.Errorf("invalid AAAA record: %q is not a valid IPv6 address", content)
		}
	case "CNAME":
		if content == "" || !strings.Contains(content, ".") {
			return fmt.Errorf("invalid CNAME record: %q is not a valid hostname", content)
		}
	case "MX":
		parts := strings.SplitN(content, " ", 2)
		if len(parts) != 2 || parts[1] == "" {
			return fmt.Errorf("invalid MX record: expected format \"priority hostname\" (e.g. \"10 mail.example.com\")")
		}
	case "TXT":
		if content == "" {
			return fmt.Errorf("invalid TXT record: content cannot be empty")
		}
	case "CAA":
		parts := strings.SplitN(content, " ", 3)
		if len(parts) != 3 || parts[2] == "" {
			return fmt.Errorf("invalid CAA record: expected format \"flags tag value\" (e.g. '0 issue \"letsencrypt.org\"')")
		}
	default:
		return fmt.Errorf("unsupported record type: %s (supported: A, AAAA, CNAME, MX, TXT, CAA)", rrtype)
	}
	return nil
}
