package eth2node

import "github.com/protolambda/zrnt/eth2/beacon"

type VerticalIndex uint64

// Aliases for ease of use
type ValidatorIndex = beacon.ValidatorIndex
type Root = beacon.Root
type Shard = beacon.Shard
type Slot = beacon.Slot
type Epoch = beacon.Epoch
type BLSSignature = beacon.BLSSignature

type ShardBlock struct {
	ShardParentRoot Root
	BeaconParentRoot Root
	Slot Slot
	Shard Shard
	ProposerIndex ValidatorIndex
	Body ShardBlockData
}

type SignedShardBlock struct {
	Message ShardBlock
	Signature BLSSignature
}

type ShardBlockHeader struct {
	ShardParentRoot Root
	BeaconParentRoot Root
	Slot Slot
	Shard Shard
	ProposerIndex ValidatorIndex
	BodyRoot Root
}

type SignedShardBlockHeader struct {
	Message ShardBlockHeader
	Signature BLSSignature
}

type ShardBlockData []byte

type ShardBlockDataChunk []byte

type DASMessage struct {
	Slot  Slot
	Chunk ShardBlockDataChunk
	Index VerticalIndex

	// TODO: does this need a ref to the shard block root, so it can be matched with the header easily?

	// Proof to show that the chunk is part of the vector commitment in the header. (correct?)
	KateProof [48]byte
}
