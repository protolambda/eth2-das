package eth2node

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	mplex "github.com/libp2p/go-libp2p-mplex"
	noise "github.com/libp2p/go-libp2p-noise"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	pubsub_pb "github.com/libp2p/go-libp2p-pubsub/pb"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	"github.com/libp2p/go-tcp-transport"
	"github.com/pkg/errors"
	"go.uber.org/zap"
)

type DASSubnetIndex uint64
type Slot uint64

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

	// Maximum of how frequently one of the K subnets are randomly swapped.
	// Rotations of a subnet in K can happen any time between 1 and SLOTS_PER_K_ROTATION_MAX slots.
	SLOTS_PER_K_ROTATION_MAX uint64

	// How frequently each of the P subnets are randomly swapped.
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
}

func (c *Config) Expand() ExpandedConfig {
	return ExpandedConfig{
		Config:                      *c,
		CHUNK_INDEX_SUBNETS:         c.MAX_BLOCK_SIZE / c.CHUNK_SIZE,
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

type subnetInfo struct {
	subscribedAt Slot
	sub          *pubsub.Subscription
}

type Eth2Node struct {
	log  *zap.SugaredLogger
	h    host.Host
	ps   *pubsub.PubSub
	conf ExpandedConfig

	// to kill main loop
	kill chan struct{}

	// TODO track shard topics

	// All CHUNK_INDEX_SUBNETS topics (joined but not subscribed)
	subnets []*pubsub.Topic
	// currently publicly joined subnets
	pIndices map[DASSubnetIndex]*subnetInfo
	// currently randomly joined subnets
	kIndices map[DASSubnetIndex]*subnetInfo
}

func New(ctx context.Context, conf *Config, log *zap.SugaredLogger) (*Eth2Node, error) {
	options := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.NATManager(basichost.NewNATManager),
		// memory peerstore by default
		// random identity by default
		libp2p.Ping(false),
		libp2p.DisableRelay(),
	}

	h, err := libp2p.New(ctx, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed host init")
	}
	psOptions := []pubsub.Option{
		pubsub.WithNoAuthor(),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithMessageIdFn(MsgIDFunction),
	}

	ps, err := pubsub.NewGossipSub(ctx, h, psOptions...)
	if err != nil {
		return nil, errors.Wrap(err, "failed gossipsub init")
	}

	expandedConf := conf.Expand()
	n := &Eth2Node{
		h:        h,
		ps:       ps,
		conf:     expandedConf,
		pIndices: make(map[DASSubnetIndex]*subnetInfo),
		kIndices: make(map[DASSubnetIndex]*subnetInfo),
		log:      log,
	}
	if err := n.joinInitialTopics(); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *Eth2Node) Start() {

}

func (n *Eth2Node) Close() error {
	// close processing loop with a channel join
	n.kill <- struct{}{}
	return n.h.Close()
}

func (n *Eth2Node) publicDasSubset(slot Slot) map[DASSubnetIndex]struct{} {
	return n.conf.DasPublicSubnetIndices(n.h.ID(), slot, n.conf.P)
}

func (n *Eth2Node) onSlot(slot uint64) {

}

// joinInitialTopics does not subscribe to topics, it just makes the handles to them available for later use.
func (n *Eth2Node) joinInitialTopics() error {
	n.subnets = make([]*pubsub.Topic, 0, n.conf.CHUNK_INDEX_SUBNETS)
	for i := uint64(0); i < n.conf.CHUNK_INDEX_SUBNETS; i++ {
		t, err := n.ps.Join(fmt.Sprintf("/eth2/%x/das_vert/ssz", n.conf.ForkDigest[:]))
		if err != nil {
			return fmt.Errorf("failed to prepare das subnet topic %d: %w", i, err)
		}
		n.subnets = append(n.subnets, t)
	}
	return nil
}

func MsgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc
	_, _ = h.Write(pmsg.Data) // TODO:  do we want snappy compressed messages?
	id := h.Sum(nil)[:20]
	return base64.URLEncoding.EncodeToString(id)
}
