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
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"go.uber.org/zap"
	"math"
	"net"
	"time"
)

type subnetInfo struct {
	subscribedAt Slot
	sub          *pubsub.Subscription
}

type Eth2Node struct {
	log *zap.SugaredLogger

	subProcesses struct {
		ctx    context.Context
		cancel context.CancelFunc
	}

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

	subCtx, subCancel := context.WithCancel(context.Background())

	expandedConf := conf.Expand()
	n := &Eth2Node{
		subProcesses: struct {
			ctx    context.Context
			cancel context.CancelFunc
		}{ctx: subCtx, cancel: subCancel},
		h:        h,
		ps:       ps,
		conf:     expandedConf,
		pIndices: make(map[DASSubnetIndex]*subnetInfo),
		kIndices: make(map[DASSubnetIndex]*subnetInfo),
		log:      log,
		kill:     make(chan struct{}),
	}
	if err := n.joinInitialTopics(); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *Eth2Node) Start(ip net.IP, port uint64) error {
	ipScheme := "ip4"
	if ip4 := ip.To4(); ip4 == nil {
		ipScheme = "ip6"
	} else {
		ip = ip4
	}
	mAddr, err := ma.NewMultiaddr(fmt.Sprintf("/%s/%s/tcp/%d", ipScheme, ip.String(), port))
	if err != nil {
		return errors.Wrap(err, "could not construct multi addr")
	}
	n.log.Info("starting node!")
	if err := n.h.Network().Listen(mAddr); err != nil {
		return errors.Wrap(err, "failed to bind to network interface")
	}
	go n.processLoop()
	return nil
}

func (n *Eth2Node) processLoop() {
	slotDuration := time.Second * time.Duration(n.conf.SECONDS_PER_SLOT)

	genesisTime := time.Unix(int64(n.conf.GENESIS_TIME), 0)

	// adjust the timer to be exactly at the slot boundary
	d := time.Since(genesisTime) % slotDuration
	if d < 0 {
		d += slotDuration
	}

	// creates a ticker, aligned with genesis slot, ticking every interval, and with some offset to genesis.
	tickerWithOffset := func(interval time.Duration, offset time.Duration) *time.Ticker {
		ticker := time.NewTicker(interval)
		ticker.Reset(d + offset)
		return ticker
	}

	// Note that slot ticker can adjust itself and drop slots, if the receiver is slow (i.e. when under heavy load)
	slotTicker := tickerWithOffset(slotDuration, 0)
	defer slotTicker.Stop()

	// TODO schedule work publishing etc.
	//workTicker := tickerWithOffset(slotDuration, slotDuration / 3)

	for {
		select {
		case _, _ = <-n.kill:
			n.log.Info("stopping work, goodbye!")
			return
		case <-slotTicker.C: // schedules genesis
			t := time.Since(genesisTime)
			slotFloat := math.Round(t.Seconds() / float64(n.conf.SECONDS_PER_SLOT))
			if slotFloat < 0 {
				// waiting for genesis, countdown every slot near the end.
				if remaining := uint64(-slotFloat); remaining%10 == 0 || remaining < 10 {
					n.log.With("genesis_time", n.conf.GENESIS_TIME, "slots", remaining).Info("Genesis countdown...")
				}
				continue
			}
			slot := Slot(slotFloat)
			n.log.With("slot", slot).Debug("slot event!")

			n.rotatePSubnets(slot)
			n.rotateKSubnets(slot)
		}
	}
}

func (n *Eth2Node) Close() error {
	// close processing loop with a channel join
	n.kill <- struct{}{}
	close(n.kill)
	n.subProcesses.cancel()
	return n.h.Close()
}

func (n *Eth2Node) publicDasSubset(slot Slot) map[DASSubnetIndex]struct{} {
	return n.conf.DasPublicSubnetIndices(n.h.ID(), slot, n.conf.P)
}

// joinInitialTopics does not subscribe to topics, it just makes the handles to them available for later use.
func (n *Eth2Node) joinInitialTopics() error {
	n.subnets = make([]*pubsub.Topic, 0, n.conf.CHUNK_INDEX_SUBNETS)
	for i := DASSubnetIndex(0); i < DASSubnetIndex(n.conf.CHUNK_INDEX_SUBNETS); i++ {
		name := n.conf.VertTopic(i)
		if err := n.ps.RegisterTopicValidator(name, n.subnetValidator(i)); err != nil {
			return fmt.Errorf("failed to create validator for das subnet topic %d: %w", i, err)
		}
		t, err := n.ps.Join(name)
		if err != nil {
			return fmt.Errorf("failed to prepare das subnet topic %d: %w", i, err)
		}
		go n.handleTopicEvents(name, t)
		n.subnets = append(n.subnets, t)
	}
	return nil
}

func (n *Eth2Node) handleTopicEvents(name string, t *pubsub.Topic) {
	evs, err := t.EventHandler()
	if err != nil {
		n.log.With("topic", name).With(zap.Error(err)).Warn("failed to listen for topic events")
		return
	}
	for {
		ev, err := evs.NextPeerEvent(n.subProcesses.ctx)
		if err != nil {
			return // context timeout/cancel
		}
		switch ev.Type {
		case pubsub.PeerJoin:
			n.log.With("peer", ev.Peer.String(), "topic", name).Debug("peer JOIN")
		case pubsub.PeerLeave:
			n.log.With("peer", ev.Peer.String(), "topic", name).Debug("peer LEAVE")
		default:
			n.log.With("peer", ev.Peer.String(), "topic", name).Warn("unknown topic event")
		}
	}
}

func MsgIDFunction(pmsg *pubsub_pb.Message) string {
	h := sha256.New()
	// never errors, see crypto/sha256 Go doc
	_, _ = h.Write(pmsg.Data) // TODO:  do we want snappy compressed messages?
	id := h.Sum(nil)[:20]
	return base64.URLEncoding.EncodeToString(id)
}
