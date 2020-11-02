package eth2node

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/libp2p/go-libp2p-core/peer"
)

// DasSlowPeerSeed defines a seed specific to the peer.
// So that its selection of SLOW_INDICES verticalSubnets is different from other honest peers, and predictable.
func (conf *ExpandedConfig) DasSlowPeerSeed(id peer.ID) (out [32]byte) {
	h := sha256.New()
	h.Write([]byte("das domain"))
	h.Write([]byte(id))
	copy(out[:], h.Sum(nil))
	return out
}

// DasSlowPeerSlotOffset defines an offset specific to the peer.
// All nodes switching public verticalSubnets at the same time is bad,
// everyone should pic something predictable and somewhat unique.
// Otherwise honest nodes could accidentally create spikes of network overhead (subnet switches),
// Hence the offset based on peer ID (this can be cached for other peers too, but is cheap anyway)
func (conf *ExpandedConfig) DasSlowPeerSlotOffset(peerSeed [32]byte) Slot {
	return Slot(binary.LittleEndian.Uint64(peerSeed[:8]) % conf.SLOTS_PER_SLOW_ROTATION)
}

// DasSlowSubnetSlotOffset defines an offset specific to each entry i of the SLOW_INDICES verticalSubnets,
// used to avoid sudden updates all at once.
func (conf *ExpandedConfig) DasSlowSubnetSlotOffset(i uint64) Slot {
	return Slot((i * conf.SLOT_OFFSET_PER_SLOW_INDEX) % conf.SLOTS_PER_SLOW_ROTATION)
}

// DasSlowSubnetIndex defines how a peer Seed, some slot (with offsets applied),
// and a SLOW_INDICES entry index map to a subnet choice.
func (conf *ExpandedConfig) DasSlowSubnetIndex(peerSeed [32]byte, slot Slot, i uint64) VerticalIndex {
	windowIndex := uint64(slot) / conf.SLOTS_PER_SLOW_ROTATION
	h := sha256.New()
	h.Write(peerSeed[:])
	binary.Write(h, binary.LittleEndian, i)
	binary.Write(h, binary.LittleEndian, windowIndex)
	return VerticalIndex(binary.LittleEndian.Uint64(h.Sum(nil)[:8]) % conf.SAMPLE_SUBNETS)
}

// DasSlowSubnetIndices defines to which verticalSubnets a peer is publicly subscribed to at slot, given its index count (its SLOW_INDICES).
// This could be called to describe remote nodes, with minimal information (just their peer ID and das_p from ENR/metadata)
func (conf *ExpandedConfig) DasSlowSubnetIndices(id peer.ID, slot Slot, dasP uint64) map[VerticalIndex]struct{} {
	peerSeed := conf.DasSlowPeerSeed(id)
	peerOffset := conf.DasSlowPeerSlotOffset(peerSeed)
	out := make(map[VerticalIndex]struct{})
	for i := uint64(0); i < dasP; i++ {
		subnetIndex := conf.DasSlowSubnetIndex(
			peerSeed,
			slot+peerOffset+conf.DasSlowSubnetSlotOffset(i),
			i)
		out[subnetIndex] = struct{}{}
	}
	return out
}
