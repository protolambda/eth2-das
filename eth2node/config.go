package eth2node

import (
	"fmt"
	"github.com/protolambda/zrnt/eth2/beacon"
)

// New subnet index type
type DASSubnetIndex uint64

// Aliases for ease of use
type ValidatorIndex = beacon.ValidatorIndex
type Shard = beacon.Shard
type Slot = beacon.Slot
type Epoch = beacon.Epoch

type Config struct {
	// Subset of indices that nodes sample, privately
	K uint64
	// Subset of indices that nodes sample, publicly
	P uint64

	// Number of bytes per chunk (index)
	CHUNK_SIZE uint64
	// Number of bytes in a block
	MAX_BLOCK_SIZE uint64
	// Number of bytes in a shard header
	SHARD_HEADER_SIZE uint64
	// Number of shards
	SHARD_COUNT uint64
	// Number of seconds in each slot
	SECONDS_PER_SLOT uint64

	// Maximum of how frequently one of the K dasSubnets are randomly swapped.
	// Rotations of a subnet in K can happen any time between 1 and SLOTS_PER_K_ROTATION_MAX (incl) slots.
	SLOTS_PER_K_ROTATION_MAX uint64

	// How frequently each of the P dasSubnets are randomly swapped.
	// (deterministic on peer ID, so public and predictable)
	// Offset logic is applied to avoid all P updating at once at an SLOTS_PER_P_ROTATION interval.
	SLOTS_PER_P_ROTATION uint64

	// SLOT_OFFSET_PER_P_INDEX is the time for an index of P to wait for the previous index to rotate.
	// If P > SLOTS_PER_P_ROTATION / SLOT_OFFSET_PER_P_INDEX then multiple indices
	// may be rotating closer together / at once. This would be considered super-node territory,
	// not normal, and not a benefit, nor a big negative.
	SLOT_OFFSET_PER_P_INDEX uint64

	// how frequently shard subnet subscriptions are kept
	EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION uint64

	// Number of active validators
	VALIDATOR_COUNT uint64
	// Physical validator nodes in the p2p network
	NODE_COUNT uint64

	// Fork digest, put in the topic names
	ForkDigest [4]byte

	// To coordinate work between all nodes
	GENESIS_TIME uint64
}

func (c *Config) Expand() ExpandedConfig {
	subnets := c.MAX_BLOCK_SIZE / c.CHUNK_SIZE
	if c.K+c.P > subnets {
		panic("invalid configuration! Need K + P <= CHUNK_INDEX_SUBNETS")
	}
	return ExpandedConfig{
		Config:                      *c,
		CHUNK_INDEX_SUBNETS:         subnets,
		AVERAGE_VALIDATORS_PER_NODE: c.VALIDATOR_COUNT / c.NODE_COUNT,
	}
}

type ExpandedConfig struct {
	Config
	// Number of chunks that make up a block
	CHUNK_INDEX_SUBNETS uint64
	// validators per node
	AVERAGE_VALIDATORS_PER_NODE uint64
}

func (conf *ExpandedConfig) ShardHeadersTopic() string {
	return fmt.Sprintf("/eth2/%x/shard_headers/ssz", conf.ForkDigest[:])
}

func (conf *ExpandedConfig) VertTopic(i DASSubnetIndex) string {
	return fmt.Sprintf("/eth2/%x/das_vert_%d/ssz", conf.ForkDigest[:], i)
}

func (conf *ExpandedConfig) ShardTopic(i Shard) string {
	return fmt.Sprintf("/eth2/%x/shard_%d/ssz", conf.ForkDigest[:], i)
}
