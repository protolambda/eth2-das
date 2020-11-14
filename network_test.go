package main

import (
	"context"
	"fmt"
	"github.com/libp2p/go-libp2p-core/peer"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/protolambda/eth2-das/eth2node"
	"github.com/protolambda/zrnt/eth2/beacon"
	"go.uber.org/zap"
	"net"
	"strings"
	"testing"
	"time"
)

func TestDAS(t *testing.T) {
	// TODO parameters
	conf := &eth2node.Config{
		FAST_INDICES:                16,
		SLOW_INDICES:                4,
		MAX_SAMPLES_PER_SHARD_BLOCK: 16,
		POINTS_PER_SAMPLE:           16,
		SLOTS_PER_FAST_ROTATION_MAX: 32,
		SLOTS_PER_SLOW_ROTATION:     2048,
		SLOT_OFFSET_PER_SLOW_INDEX:  512,
		SHARD_COUNT:                 4, // smaller, just testing here, lower resources.
		SECONDS_PER_SLOT:            12,
		VALIDATOR_COUNT:             1500, // TODO
		ForkDigest:                  [4]byte{0xaa, 0xbb, 0xcc, 0xdd},
		TARGET_PEERS_PER_DAS_SUB:    6,
		PEER_COUNT_LO:               120,
		PEER_COUNT_HI:               200,
		GENESIS_TIME:                uint64(time.Now().Add(time.Second * 40).Unix()), // TODO
		SHUFFLE_ROUND_COUNT:         90,
		// TODO gossipsub score tuning (ignored for now)
		VERT_SUBNET_TOPIC_SCORE_PARAMS:   nil,
		HORZ_SUBNET_TOPIC_SCORE_PARAMS:   nil,
		SHARD_HEADERS_TOPIC_SCORE_PARAMS: nil,
		GOSSIP_GLOBAL_SCORE_PARAMS:       nil,
		GOSSIP_GLOBAL_SCORE_THRESHOLDS:   nil,
		// for test speed, tweak network layer
		ENABLE_NAT:                 false,
		DISABLE_TRANSPORT_SECURITY: true,
	}
	// TODO: use Testground sync to learn all peer IDs and their addresses

	log, err := zap.NewDevelopment()
	if err != nil {
		t.Fatal(err)
	}
	slog := log.Sugar()
	nodeCount := uint64(128)
	disc := &eth2node.MockDiscovery{
		Peers: make(map[peer.ID][]ma.Multiaddr),
	}

	ctx, cancel := context.WithCancel(context.Background())

	mkNode := func(nodeIndex uint64) (*eth2node.Eth2Node, error) {
		n, err := eth2node.New(ctx, conf, disc, slog.With("node", nodeIndex))
		if err != nil {
			return nil, errors.Wrap(err, "failed to start eth2 node")
		}
		start := conf.VALIDATOR_COUNT * nodeIndex / nodeCount
		end := conf.VALIDATOR_COUNT * (nodeIndex + 1) / nodeCount
		count := end - start
		indices := make([]beacon.ValidatorIndex, count, count)
		for i := uint64(0); i < count; i++ {
			indices[i] = beacon.ValidatorIndex(start + i)
		}
		// TODO: generate interop BLS keys for validators maybe?
		n.RegisterValidators(indices...)
		if err := n.Start(net.IPv4zero, 9000+uint16(nodeIndex)); err != nil {
			return nil, errors.Wrap(err, "failed to start node")
		}
		return n, nil
	}

	var nodes []*eth2node.Eth2Node
	for i := uint64(0); i < nodeCount; i++ {
		n, err := mkNode(i)
		if err != nil {
			t.Fatalf("node %d failed to start: %v", i, err)
		}
		nodes = append(nodes, n)
	}

	for _, n := range nodes {
		id, addrs := n.DiscInfo()
		disc.Peers[id] = addrs
	}

	// Log useful global information in slot loop, avoid logging duplicate info on each peer.
	go func() {
		expConf := conf.Expand()
		allSubnets := make(map[eth2node.VerticalIndex]struct{})
		for i := eth2node.VerticalIndex(0); i < eth2node.VerticalIndex(expConf.SAMPLE_SUBNETS); i++ {
			allSubnets[i] = struct{}{}
		}
		slotDuration := time.Second * time.Duration(conf.SECONDS_PER_SLOT)
		ticker := conf.TickerWithOffset(slotDuration, 0)
		defer ticker.Stop()
		for {
			select {
			case ts := <-ticker.C:
				slot, preGenesis := conf.SlotWithOffset(ts, 0)
				if preGenesis {
					slog.With("genesis_time", conf.GENESIS_TIME, "slots", slot).Info("Genesis countdown...")
					continue
				}
				backbone := disc.FindPublic(&expConf, slot, allSubnets)
				var slotsStats strings.Builder
				slotsStats.WriteString("backbone:\n")
				for i := eth2node.VerticalIndex(0); i < eth2node.VerticalIndex(expConf.SAMPLE_SUBNETS); i++ {
					slotsStats.WriteString(fmt.Sprintf("%3d ", len(backbone[i])))
				}
				slotsStats.WriteString("\npeer counts:\n")
				for _, node := range nodes {
					peerCount := node.Stats()
					slotsStats.WriteString(fmt.Sprintf("%3d ", peerCount))
				}
				slog.With("slot", slot).Debug(slotsStats.String())
			case <-ctx.Done():
				return
			}
		}
	}()

	// run network for test duration
	time.Sleep(time.Second * time.Duration(conf.SECONDS_PER_SLOT*32*10))

	for _, n := range nodes {
		if err := n.Close(); err != nil {
			t.Log("shutdown err", err)
		}
	}
	cancel()
}
