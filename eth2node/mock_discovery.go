package eth2node

import (
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// peer ID -> list of multi Addresses (excl. /p2p/ component)
type MockDiscovery struct {
	Peers map[peer.ID][]ma.Multiaddr
}

func (m *MockDiscovery) FindPublic(conf *ExpandedConfig, slot Slot, subnets map[VerticalIndex]struct{}) map[VerticalIndex][]peer.ID {
	candidates := make(map[VerticalIndex][]peer.ID, len(subnets))
	for id := range m.Peers {
		// TODO: does everyone have the same SLOW_INDICES parameter, or may it vary per node?
		// TODO: also, in a real Eth2 scenario, the ENR needs to at least say "yes/no DAS subnet user"
		remoteSubs := conf.DasSlowSubnetIndices(id, slot, conf.SLOW_INDICES)
		for s := range remoteSubs {
			if _, ok := subnets[s]; ok {
				candidates[s] = append(candidates[s], id)
			}
		}
	}
	return candidates
}

// Get addrs of a peer, may be nil if the peer is unknown.
func (m *MockDiscovery) Addrs(id peer.ID) []ma.Multiaddr {
	return m.Peers[id]
}

type Discovery interface {
	FindPublic(conf *ExpandedConfig, slot Slot, subnets map[VerticalIndex]struct{}) map[VerticalIndex][]peer.ID
	Addrs(id peer.ID) []ma.Multiaddr
}
