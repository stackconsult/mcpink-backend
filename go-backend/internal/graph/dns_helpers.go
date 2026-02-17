package graph

import (
	"github.com/augustdev/autoclip/internal/dns"
	"github.com/augustdev/autoclip/internal/graph/model"
)

func toGQLDNSRecords(records []dns.DNSRecord) []*model.DNSRecord {
	out := make([]*model.DNSRecord, len(records))
	for i, r := range records {
		out[i] = &model.DNSRecord{
			Host:     r.Host,
			Type:     r.Type,
			Value:    r.Value,
			Verified: r.Verified,
		}
	}
	return out
}
