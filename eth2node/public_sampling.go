package eth2node

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/libp2p/go-libp2p-core/peer"
)

// DasPublicPeerSeed defines a seed specific to the peer.
// So that its selection of P dasSubnets is different from other honest peers, and predictable.
func (conf *ExpandedConfig) DasPublicPeerSeed(id peer.ID) (out [32]byte) {
	h := sha256.New()
	h.Write([]byte("das domain"))
	h.Write([]byte(id))
	copy(out[:], h.Sum(nil))
	return out
}

// DasPublicPeerSlotOffset defines an offset specific to the peer.
// All nodes switching public dasSubnets at the same time is bad,
// everyone should pic something predictable and somewhat unique.
// Otherwise honest nodes could accidentally create spikes of network overhead (subnet switches),
// Hence the offset based on peer ID (this can be cached for other peers too, but is cheap anyway)
func (conf *ExpandedConfig) DasPublicPeerSlotOffset(peerSeed [32]byte) Slot {
	return Slot(binary.LittleEndian.Uint64(peerSeed[:8]) % conf.SLOTS_PER_P_ROTATION)
}

// DasPublicSubnetSlotOffset defines an offset specific to each entry i of the P dasSubnets,
// used to avoid sudden updates all at once.
func (conf *ExpandedConfig) DasPublicSubnetSlotOffset(i uint64) Slot {
	return Slot((i * conf.SLOT_OFFSET_PER_P_INDEX) % conf.SLOTS_PER_P_ROTATION)
}

// DasPublicSubnetIndex defines how a peer Seed, some slot (with offsets applied),
// and a P entry index map to a subnet choice.
func (conf *ExpandedConfig) DasPublicSubnetIndex(peerSeed [32]byte, slot Slot, i uint64) DASSubnetIndex {
	windowIndex := uint64(slot) / conf.SLOTS_PER_P_ROTATION
	h := sha256.New()
	h.Write(peerSeed[:])
	binary.Write(h, binary.LittleEndian, i)
	binary.Write(h, binary.LittleEndian, windowIndex)
	return DASSubnetIndex(binary.LittleEndian.Uint64(h.Sum(nil)[:8]) % conf.CHUNK_INDEX_SUBNETS)
}

// DasPublicSubnetIndices defines to which dasSubnets a peer is publicly subscribed to at slot, given its index count (its P).
// This could be called to describe remote nodes, with minimal information (just their peer ID and das_p from ENR/metadata)
func (conf *ExpandedConfig) DasPublicSubnetIndices(id peer.ID, slot Slot, dasP uint64) map[DASSubnetIndex]struct{} {
	peerSeed := conf.DasPublicPeerSeed(id)
	peerOffset := conf.DasPublicPeerSlotOffset(peerSeed)
	out := make(map[DASSubnetIndex]struct{})
	for i := uint64(0); i < dasP; i++ {
		subnetIndex := conf.DasPublicSubnetIndex(
			peerSeed,
			slot+peerOffset+conf.DasPublicSubnetSlotOffset(i),
			i)
		out[subnetIndex] = struct{}{}
	}
	return out
}
