package app

import "testing"

func TestRealityTargetsRejectUnsafeRanges(t *testing.T) {
	for _, input := range []string{"127.0.0.1", "10.0.0.1", "100.64.0.1", "192.168.0.0/28", "203.0.113.4", "8.8.0.0/16", "localhost"} {
		if _, err := realityTargets(input); err == nil {
			t.Fatalf("unsafe target %q was accepted", input)
		}
	}
}

func TestRealityTargetsAcceptDomainAndSmallPublicCIDR(t *testing.T) {
	targets, err := realityTargets("www.microsoft.com")
	if err != nil || len(targets) != 1 || targets[0] != "www.microsoft.com" {
		t.Fatalf("domain rejected: %v %#v", err, targets)
	}
	targets, err = realityTargets("8.8.8.0/30")
	if err != nil || len(targets) != 4 {
		t.Fatalf("small public CIDR rejected: %v %#v", err, targets)
	}
}
