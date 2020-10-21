package eth2node

import (
	"crypto/sha256"
	"encoding/binary"
	"github.com/libp2p/go-libp2p-core/peer"
)

// Public indices are sampled as:
//
// def das_public_peer_seed(peer_id: PeerID) -> Bytes32:
//   return H(PUBLIC_DAS_SUBNETS_DOMAIN ++ peer_id)
//
// def das_public_peer_slot_offset(peer_seed) -> Slot:
//   return Slot(peer_seed[:8] % SLOTS_PER_P_ROTATION)
//
// def das_public_subnet_slot_offset(i: DASSubnetIndex) -> Slot:
//   return Slot(i * SLOT_OFFSET_PER_P_INDEX) % SLOTS_PER_P_ROTATION
//
// def das_public_subnet_index(peer_seed: Bytes32, slot: Slot, i: uint64) -> DASSubnetIndex:
//   window_index = slot ~/ SLOTS_PER_P_ROTATION
//   return H(peer_seed ++ i ++ window_index)[:8] % CHUNK_INDEX_SUBNETS
//
// def das_public_subnet_indices(peer_id: PeerID, slot: Slot, das_p: uint64) -> Set[DASSubnetIndex]:
//   peer_seed = das_public_peer_seed(peer_id)
//   peer_offset = das_public_peer_slot_offset(peer_seed)
//   return set(
//  		das_public_subnet_index(
// 				peer_seed,
//				slot + peer_offset + das_public_subnet_slot_offset(i),
//				i,
//			) for i in range(das_p))
//
// p_indices = das_public_subnet_indices(peer_id, slot, P)
//
// Any overlap in p_indices is rare, and acceptable. The set will be a little smaller sometimes.
//

// DasPublicPeerSeed defines a seed specific to the peer.
// So that its selection of P subnets is different from other honest peers, and predictable.
func (conf *ExpandedConfig) DasPublicPeerSeed(id peer.ID) (out [32]byte) {
	h := sha256.New()
	h.Write([]byte("das domain"))
	h.Write([]byte(id))
	copy(out[:], h.Sum(nil))
	return out
}

// DasPublicPeerSlotOffset defines an offset specific to the peer.
// All nodes switching public subnets at the same time is bad,
// everyone should pic something predictable and somewhat unique.
// Otherwise honest nodes could accidentally create spikes of network overhead (subnet switches),
// Hence the offset based on peer ID (this can be cached for other peers too, but is cheap anyway)
func (conf *ExpandedConfig) DasPublicPeerSlotOffset(peerSeed [32]byte) Slot {
	return Slot(binary.LittleEndian.Uint64(peerSeed[:8]) % conf.SLOTS_PER_P_ROTATION)
}

// DasPublicSubnetSlotOffset defines an offset specific to each entry i of the P subnets,
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

// DasPublicSubnetIndices defines to which subnets a peer is publicly subscribed to at slot, given its index count (its P).
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
