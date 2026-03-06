package main

import (
	"testing"
)

func TestBuildBPFFilter(t *testing.T) {
	tests := []struct {
		name     string
		filters  []Filter
		expected string
	}{
		{
			name: "Single CIDR, Portrange, Protocol, Destination",
			filters: []Filter{
				{
					Service:    "test_service",
					Direction:  "dst",
					Cidrs:      []string{"192.168.1.0/24"},
					Portranges: []string{"80-90"},
					Protocols:  []string{"tcp"},
				},
			},
			expected: "((dst net 192.168.1.0/24) and (dst portrange 80-90) and (tcp))",
		},
		{
			name: "Source and Both Directions",
			filters: []Filter{
				{
					Service:   "src_service",
					Direction: "src",
					Cidrs:     []string{"10.0.0.0/8"},
				},
				{
					Service:   "both_service",
					Direction: "both",
					Portranges: []string{"53"},
				},
			},
			expected: "((src net 10.0.0.0/8)) or ((portrange 53))",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			act := BuildBPFFilter(tt.filters)
			if act != tt.expected {
				t.Errorf("BuildBPFFilter() = %q, want %q", act, tt.expected)
			}
		})
	}
}
