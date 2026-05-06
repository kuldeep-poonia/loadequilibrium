package streaming

import (
	"github.com/loadequilibrium/loadequilibrium/internal/topology"
)

// TopologyDiff represents the delta between two consecutive topology snapshots.
// It allows clients to apply incremental updates rather than replacing the full graph.
type TopologyDiff struct {
	// Schema matches the parent TickPayload schema version.
	Schema uint32 `json:"schema"`

	// AddedNodes are nodes that appeared since the last snapshot.
	AddedNodes []topology.Node `json:"added_nodes,omitempty"`

	// RemovedNodeIDs are service IDs that disappeared since the last snapshot.
	RemovedNodeIDs []string `json:"removed_nodes,omitempty"`

	// UpdatedNodes are nodes whose NormalisedLoad changed by more than deltaThreshold.
	UpdatedNodes []topology.Node `json:"updated_nodes,omitempty"`

	// AddedEdges are edges that appeared or whose weight increased by >deltaThreshold.
	AddedEdges []topology.Edge `json:"added_edges,omitempty"`

	// RemovedEdgeKeys are "src:tgt" pairs for edges that disappeared.
	RemovedEdgeKeys []string `json:"removed_edges,omitempty"`

	// UpdatedEdges are edges whose weight changed by more than deltaThreshold.
	UpdatedEdges []topology.Edge `json:"updated_edges,omitempty"`

	// IsFull is true when the diff carries a full snapshot (e.g., first tick or
	// after the diff would be larger than a full snapshot).
	IsFull bool `json:"is_full"`
}

const topoDeltaThreshold = 0.02 // 2% change triggers update

// ComputeTopologyDiff computes the incremental delta between prev and curr snapshots.
// Returns a full diff when prev is empty or when too many changes occurred.
func ComputeTopologyDiff(prev, curr topology.GraphSnapshot) TopologyDiff {
	if len(prev.Nodes) == 0 {
		return fullDiff(curr)
	}

	diff := TopologyDiff{Schema: SchemaVersion}

	// Build lookup maps.
	prevNodes := make(map[string]topology.Node, len(prev.Nodes))
	for _, n := range prev.Nodes {
		prevNodes[n.ServiceID] = n
	}
	currNodes := make(map[string]topology.Node, len(curr.Nodes))
	for _, n := range curr.Nodes {
		currNodes[n.ServiceID] = n
	}

	// Additions and updates.
	for id, cn := range currNodes {
		if pn, ok := prevNodes[id]; !ok {
			diff.AddedNodes = append(diff.AddedNodes, cn)
		} else if abs64(cn.NormalisedLoad-pn.NormalisedLoad) > topoDeltaThreshold {
			diff.UpdatedNodes = append(diff.UpdatedNodes, cn)
		}
	}
	// Removals.
	for id := range prevNodes {
		if _, ok := currNodes[id]; !ok {
			diff.RemovedNodeIDs = append(diff.RemovedNodeIDs, id)
		}
	}

	// Edge diff.
	prevEdges := make(map[string]topology.Edge, len(prev.Edges))
	for _, e := range prev.Edges {
		prevEdges[e.Source+":"+e.Target] = e
	}
	currEdges := make(map[string]topology.Edge, len(curr.Edges))
	for _, e := range curr.Edges {
		currEdges[e.Source+":"+e.Target] = e
	}

	for key, ce := range currEdges {
		if pe, ok := prevEdges[key]; !ok {
			diff.AddedEdges = append(diff.AddedEdges, ce)
		} else if abs64(ce.Weight-pe.Weight) > topoDeltaThreshold {
			diff.UpdatedEdges = append(diff.UpdatedEdges, ce)
		}
	}
	for key := range prevEdges {
		if _, ok := currEdges[key]; !ok {
			diff.RemovedEdgeKeys = append(diff.RemovedEdgeKeys, key)
		}
	}

	// Fall back to full if diff is large (>50% of node count changed).
	totalChanges := len(diff.AddedNodes) + len(diff.RemovedNodeIDs) + len(diff.UpdatedNodes)
	if totalChanges > len(curr.Nodes)/2 {
		return fullDiff(curr)
	}

	return diff
}

func fullDiff(snap topology.GraphSnapshot) TopologyDiff {
	return TopologyDiff{
		Schema:     SchemaVersion,
		AddedNodes: snap.Nodes,
		AddedEdges: snap.Edges,
		IsFull:     true,
	}
}

func abs64(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
