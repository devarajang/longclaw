package network

import (
	"testing"

	"github.com/devarajang/longclaw/internal/domain"
)

func TestNodeTargets(t *testing.T) {
	conns := []*IsoConnection{{}, {}, {}}
	index := func(c *IsoConnection) int {
		for i, x := range conns {
			if x == c {
				return i
			}
		}
		return -1
	}

	// advertised: every step goes to every node
	next := 0
	for step := 0; step < 3; step++ {
		if got := nodeTargets(domain.NodeTestModeAdvertised, conns, &next); len(got) != 3 {
			t.Fatalf("advertised step %d: %d targets, want 3", step, len(got))
		}
	}

	// round_robin: one node per step, in turn
	next = 0
	var order []int
	for step := 0; step < 7; step++ {
		got := nodeTargets(domain.NodeTestModeRoundRobin, conns, &next)
		if len(got) != 1 {
			t.Fatalf("round_robin step %d: %d targets, want 1", step, len(got))
		}
		order = append(order, index(got[0]))
	}
	want := []int{0, 1, 2, 0, 1, 2, 0}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("round_robin order = %v, want %v", order, want)
		}
	}

	// A node leaving mid-test: rotation continues over the nodes that remain
	if got := nodeTargets(domain.NodeTestModeRoundRobin, conns[:2], &next); len(got) != 1 {
		t.Errorf("after a node left: %d targets, want 1", len(got))
	}
	if got := nodeTargets(domain.NodeTestModeRoundRobin, nil, &next); len(got) != 0 {
		t.Errorf("no nodes: %d targets, want 0", len(got))
	}
}
