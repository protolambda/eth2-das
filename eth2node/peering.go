package eth2node

import (
	"github.com/libp2p/go-libp2p-core/network"
)

func (n *Eth2Node) peersUpdate(slot Slot) {
	// determine set of subnets we are on
	subnets := make(map[VerticalIndex]struct{}, n.conf.SLOW_INDICES+n.conf.FAST_INDICES)
	for subnet := range n.slowIndices {
		subnets[subnet] = struct{}{}
	}
	for subnet := range n.fastIndices {
		subnets[subnet] = struct{}{}
	}
	want := make(map[VerticalIndex]struct{})
	// Now check our peers on each of these topics
	for subnet := range subnets {
		topicPeers := n.ps.ListPeers(n.conf.VertTopic(subnet))
		if uint64(len(topicPeers)) < n.conf.TARGET_PEERS_PER_DAS_SUB {
			want[subnet] = struct{}{}
		}
	}
	// if there's anything we want to change
	if len(want) > 0 {
		// find the backbone peer mapping.
		// TODO: inefficient local re-computation. Can be cached or partial data may be enough.
		backbone := n.disc.FindPublic(&n.conf, slot, want)
		for subnet := range want {
			// complement with whatever peers we can find in the backbone and we're not connected to already
			currentPeerCount := uint64(len(n.ps.ListPeers(n.conf.VertTopic(subnet))))
			backbonePeers := backbone[subnet]
			if len(backbonePeers) == 0 {
				n.log.With("topic_peers", currentPeerCount, "subnet", subnet).Warn("Searching for more topic peers, but backbone does not have any for us")
				break
			}
			// TODO: short-term it's more "optimal" to select the backbone peers with similar interests as us.
			//  However: seems more fragile, more complex, and lessens our chance of having sufficient peers after future rotations,
			//  since we rotate out the need for the majority of current peers, and left with little.

			dials := uint64(0)
			// TODO: could shuffle backbone peers, but should be mostly random already anyway.
			for _, id := range backbonePeers {
				if id == n.h.ID() { // don't dial ourselves.
					continue
				}
				if currentPeerCount+dials >= n.conf.TARGET_PEERS_PER_DAS_SUB {
					break
				}
				switch n.h.Network().Connectedness(id) {
				case network.Connected, network.CannotConnect:
					continue
				case network.NotConnected, network.CanConnect:
					// try connect to them
					select { // try to schedule a dial, but too many may already be queued, in which case we skip.
					case n.dialReq <- id:
						dials++
					default:
						break
					}
				}
			}
			if currentPeerCount+dials < n.conf.TARGET_PEERS_PER_DAS_SUB {
				n.log.With("topic_peers", currentPeerCount, "dials", dials).Warn("failed to find enough peers to get target")
				break
			}
		}
	}
}

// TODO: using the connection-manager tagging mechanism, we could:
// - tag peers with their subnet user labels
// - protect/unprotect peers that are on the same subnets as we like to have
// This would avoid bad connection manager situations where useful peers get kicked unexpectedly.
