package eth2node

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
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
	"sync"
	"time"
)

const connectionTimeout = time.Second * 10

// Minimum time a peer stays safe from the peer manager pruning
const pruneGrace = time.Second * 20

type subnetInfo struct {
	subscribedAt Slot
	sub          *pubsub.Subscription
}

type subnetKInfo struct {
	subnetInfo
	// when the subnet subscription should be rotated for something else
	expiry Slot
}

type Eth2Node struct {
	log *zap.SugaredLogger

	subProcesses struct {
		ctx    context.Context
		cancel context.CancelFunc
	}

	h    host.Host
	ps   *pubsub.PubSub
	disc Discovery
	conf ExpandedConfig

	// to kill main loop
	kill chan struct{}

	// Queue of requests to peer with others
	dialReq chan peer.ID

	// Set of validator indices that runs on this node
	localValidators map[ValidatorIndex]struct{}
	validatorsLock  sync.RWMutex

	// All SHARD_COUNT topics (joined but not necessarily subscribed)
	shardSubnets []*pubsub.Topic

	// All CHUNK_INDEX_SUBNETS topics (joined but not necessarily subscribed)
	dasSubnets []*pubsub.Topic

	// A singular topic used to message shard headers on.
	shardHeaders *pubsub.Topic

	// currently publicly joined dasSubnets
	pIndices map[DASSubnetIndex]*subnetInfo
	// currently randomly joined dasSubnets
	kIndices map[DASSubnetIndex]*subnetKInfo
}

func New(ctx context.Context, conf *Config, disc Discovery, log *zap.SugaredLogger) (*Eth2Node, error) {
	options := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.Security(noise.ID, noise.New),
		libp2p.NATManager(basichost.NewNATManager),
		libp2p.ConnectionManager(connmgr.NewConnManager(int(conf.PEER_COUNT_LO), int(conf.PEER_COUNT_HI), pruneGrace)),
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
		h:               h,
		ps:              ps,
		disc:            disc,
		conf:            expandedConf,
		dialReq:         make(chan peer.ID, 30), // don't try to schedule too many dials at a time.
		localValidators: make(map[ValidatorIndex]struct{}),
		pIndices:        make(map[DASSubnetIndex]*subnetInfo),
		kIndices:        make(map[DASSubnetIndex]*subnetKInfo),
		log:             log,
		kill:            make(chan struct{}),
	}
	if err := n.joinInitialTopics(); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *Eth2Node) RegisterValidators(indices ...ValidatorIndex) {
	n.validatorsLock.Lock()
	defer n.validatorsLock.Unlock()
	for _, i := range indices {
		n.localValidators[i] = struct{}{}
	}
}
func (n *Eth2Node) ListValidators() (out []ValidatorIndex) {
	n.validatorsLock.RLock()
	defer n.validatorsLock.RUnlock()
	out = make([]ValidatorIndex, len(n.localValidators))
	for i := range n.localValidators {
		out = append(out, i)
	}
	return
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
	n.log.With("validators", n.ListValidators()).Info("starting node!")
	if err := n.h.Network().Listen(mAddr); err != nil {
		return errors.Wrap(err, "failed to bind to network interface")
	}

	// This initializes the initial P and K subscriptions
	if err := n.initialSubscriptions(); err != nil {
		return errors.Wrap(err, "failed to open initial subscriptions")
	}
	go n.processLoop()
	return nil
}

func (n *Eth2Node) slotWithOffset(t time.Time, offset time.Duration) (slot Slot, preGenesis bool) {
	genesisTime := time.Unix(int64(n.conf.GENESIS_TIME), 0)
	sinceGenesis := t.Add(offset).Sub(genesisTime)
	slotFloat := math.Round(sinceGenesis.Seconds() / float64(n.conf.SECONDS_PER_SLOT))
	if slotFloat < 0 {
		return Slot(uint64(-slotFloat)), true
	} else {
		return Slot(uint64(slotFloat)), false
	}
}

func (n *Eth2Node) slotNow() (slot Slot, preGenesis bool) {
	return n.slotWithOffset(time.Now(), 0)
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
	workTicker := tickerWithOffset(slotDuration, slotDuration/3*2)

	for {
		select {
		case _, _ = <-n.kill:
			n.log.Info("stopping work, goodbye!")
			return
		case t := <-slotTicker.C: // schedules genesis
			slot, preGenesis := n.slotWithOffset(t, 0)

			if preGenesis {
				// waiting for genesis, countdown every slot near the end.
				if slot%10 == 0 || slot < 10 {
					n.log.With("genesis_time", n.conf.GENESIS_TIME, "slots", slot).Info("Genesis countdown...")
				}
				continue
			}
			n.log.With("slot", slot).Debug("slot event!")

			n.rotatePSubnets(slot)
			n.rotateKSubnets(slot)
			n.peersUpdate(slot)
		case t := <-workTicker.C:
			// 1/3 before every slot, prepare and schedule shard blocks
			slot, preGenesis := n.slotWithOffset(t, slotDuration/3)
			if preGenesis {
				continue
			}
			n.scheduleShardProposalsMaybe(slot)
		case id := <-n.dialReq:
			addrs := n.disc.Addrs(id)
			addrInfo := peer.AddrInfo{Addrs: addrs, ID: id}
			connectCtx, _ := context.WithTimeout(n.subProcesses.ctx, connectionTimeout)
			go func(ctx context.Context, addrInfo peer.AddrInfo) {
				n.log.With("peer_id", addrInfo.ID).Debug("connecting to peer")
				err := n.h.Connect(ctx, addrInfo)
				if err != nil {
					n.log.With("peer_id", addrInfo.ID, zap.Error(err)).Warn("failed to connect to peer")
				} else {
					n.log.With("peer_id", addrInfo.ID).Debug("success, connected to peer")
				}
			}(connectCtx, addrInfo)
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

func (n *Eth2Node) initialSubscriptions() error {
	sub, err := n.shardHeaders.Subscribe()
	if err != nil {
		return errors.Wrap(err, "failed to subscribe to shard headers topic")
	}
	go n.shardHeaderHandler(sub)
	slot, preGenesis := n.slotNow()
	if preGenesis { // if pre-genesis, just register with the topics we'll have to be around at first
		slot = 0
	}
	n.rotatePSubnets(slot)
	n.rotateKSubnets(slot)
	return nil
}

// joinInitialTopics does not subscribe to topics, it just makes the handles to them available for later use.
func (n *Eth2Node) joinInitialTopics() error {

	// init DAS subnets
	n.dasSubnets = make([]*pubsub.Topic, 0, n.conf.CHUNK_INDEX_SUBNETS)
	for i := DASSubnetIndex(0); i < DASSubnetIndex(n.conf.CHUNK_INDEX_SUBNETS); i++ {
		name := n.conf.VertTopic(i)
		if err := n.ps.RegisterTopicValidator(name, n.dasSubnetValidator(i)); err != nil {
			return fmt.Errorf("failed to create validator for das subnet topic %d: %w", i, err)
		}
		t, err := n.ps.Join(name)
		if err != nil {
			return fmt.Errorf("failed to prepare das subnet topic %d: %w", i, err)
		}
		go n.handleTopicEvents(name, t)
		n.dasSubnets = append(n.dasSubnets, t)
	}

	// init shard subnets
	n.shardSubnets = make([]*pubsub.Topic, 0, n.conf.SHARD_COUNT)
	for i := Shard(0); i < Shard(n.conf.SHARD_COUNT); i++ {
		name := n.conf.ShardTopic(i)
		if err := n.ps.RegisterTopicValidator(name, n.shardSubnetValidator(i)); err != nil {
			return fmt.Errorf("failed to create validator for shard subnet topic %d: %w", i, err)
		}
		t, err := n.ps.Join(name)
		if err != nil {
			return fmt.Errorf("failed to prepare shard subnet topic %d: %w", i, err)
		}
		go n.handleTopicEvents(name, t)
		n.shardSubnets = append(n.shardSubnets, t)
	}

	// init shard headers topic
	{
		name := n.conf.ShardHeadersTopic()
		if err := n.ps.RegisterTopicValidator(name, n.shardHeaderValidator()); err != nil {
			return fmt.Errorf("failed to create validator for shard headers topic: %w", err)
		}
		t, err := n.ps.Join(name)
		if err != nil {
			return fmt.Errorf("failed to prepare shard headers topic: %w", err)
		}
		go n.handleTopicEvents(name, t)
		n.shardHeaders = t
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
