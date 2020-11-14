package eth2node

import (
	"fmt"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
)

const BYTES_PER_DATA_POINT = 31
const BYTES_PER_FULL_POINT = 32

type Config struct {
	// Sampling configuration
	// ----------------------------------

	// Subset of indices that nodes sample, privately chosen, and replaced quickly
	FAST_INDICES uint64
	// Subset of indices that nodes sample, publicly determined, and replaced slowly
	SLOW_INDICES uint64

	// Maximum number of samples in which the extended points are split into
	MAX_SAMPLES_PER_SHARD_BLOCK uint64
	// Number of points per sample
	POINTS_PER_SAMPLE uint64

	// Maximum of how frequently a fast vertical subnet subscriptions is randomly swapped.
	// Rotations of a subnet can happen any time between 1 and SLOTS_PER_FAST_ROTATION_MAX (incl) slots.
	SLOTS_PER_FAST_ROTATION_MAX uint64

	// How frequently a slow vertical subnet subscriptions is randomly swapped.
	// (deterministic on peer ID, so public and predictable)
	SLOTS_PER_SLOW_ROTATION uint64

	// The time for a slow vertical subscription to wait for the previous index to rotate, for stagger effect..
	// If SLOW_INDICES > SLOTS_PER_SLOW_ROTATION / SLOT_OFFSET_PER_SLOW_INDEX then multiple indices
	// may be rotating closer together / at once. This would be considered super-node territory,
	// not normal, and not a benefit, nor a big negative.
	SLOT_OFFSET_PER_SLOW_INDEX uint64

	// General configuration
	// ----------------------------------

	// Number of shards
	SHARD_COUNT uint64
	// Number of seconds in each slot
	SECONDS_PER_SLOT uint64

	// Number of active validators
	VALIDATOR_COUNT uint64

	// Fork digest, put in the topic names
	ForkDigest [4]byte

	// How many peers we should try to maintain for each of the SLOW_INDICES and FAST_INDICES subnets, when to hit discovery.
	// Gossipsub will do some peer management for us too, but may not find peers as quickly.
	TARGET_PEERS_PER_DAS_SUB uint64

	// The Low water should be at least TARGET_PEERS_PER_DAS_SUB * (SLOW_INDICES + FAST_INDICES)
	// if CHUNK_INDEX_SUBNETS is very large compared to (SLOW_INDICES + FAST_INDICES).
	// However, peers may cover multiple of our SLOW_INDICES and FAST_INDICES topics, so it is OK to have a little less.
	// We could try and optimize by selecting good short-term peers from the backbone that cover multiple topic needs,
	// but that seems fragile.
	PEER_COUNT_LO uint64
	// When HI is hit, peers will be pruned back to LO. This pruning happens on a long interval, and is not a hard limit.
	PEER_COUNT_HI uint64

	// To coordinate work between all nodes
	GENESIS_TIME uint64

	// for shuffling shard committees
	SHUFFLE_ROUND_COUNT uint8

	VERT_SUBNET_TOPIC_SCORE_PARAMS   *pubsub.TopicScoreParams
	HORZ_SUBNET_TOPIC_SCORE_PARAMS   *pubsub.TopicScoreParams
	SHARD_HEADERS_TOPIC_SCORE_PARAMS *pubsub.TopicScoreParams
	GOSSIP_GLOBAL_SCORE_PARAMS       *pubsub.PeerScoreParams
	GOSSIP_GLOBAL_SCORE_THRESHOLDS   *pubsub.PeerScoreThresholds

	// Network settings
	ENABLE_NAT                 bool
	DISABLE_TRANSPORT_SECURITY bool
}

func (c *Config) Expand() ExpandedConfig {
	subnets := c.MAX_SAMPLES_PER_SHARD_BLOCK * c.SHARD_COUNT
	if c.FAST_INDICES+c.SLOW_INDICES > subnets {
		panic("invalid configuration! Need FAST_INDICES + SLOW_INDICES <= subnets")
	}
	return ExpandedConfig{
		Config:         *c,
		SAMPLE_SUBNETS: subnets,
		MAX_DATA_SIZE:  BYTES_PER_DATA_POINT * c.POINTS_PER_SAMPLE * c.MAX_SAMPLES_PER_SHARD_BLOCK / 2,
	}
}

type ExpandedConfig struct {
	Config

	// Total number of vertical subnets
	SAMPLE_SUBNETS uint64

	// Max bytes per (unextended) block data blob
	MAX_DATA_SIZE uint64
}

func (conf *ExpandedConfig) ShardHeadersTopic() string {
	return fmt.Sprintf("/eth2/%x/shard_headers/ssz", conf.ForkDigest[:])
}

func (conf *ExpandedConfig) VertTopic(i VerticalIndex) string {
	return fmt.Sprintf("/eth2/%x/das_vert_%d/ssz", conf.ForkDigest[:], i)
}

func (conf *ExpandedConfig) HorzTopic(i Shard) string {
	return fmt.Sprintf("/eth2/%x/das_horz_%d/ssz", conf.ForkDigest[:], i)
}
