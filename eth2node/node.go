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

type subnetFastInfo struct {
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
	horizontalSubnets []*pubsub.Topic

	// Shard subscriptions
	horizontalSubs map[Shard]*pubsub.Subscription

	// All SAMPLE_SUBNETS topics (joined but not necessarily subscribed)
	verticalSubnets []*pubsub.Topic

	// A singular topic used to message shard headers on.
	shardHeaders *pubsub.Topic

	// currently publicly joined verticalSubnets, slowly rotates
	slowIndices map[VerticalIndex]*subnetInfo
	// currently randomly joined verticalSubnets, quickly rotates
	fastIndices map[VerticalIndex]*subnetFastInfo

	// current shard committees. Shaped as shard -> committee
	// TODO: currently these don't change, as in real-world these would be changing super slow (days)
	// So keep things simple.
	shard2Vals [][]ValidatorIndex
	val2Shard  []Shard
}

func New(ctx context.Context, conf *Config, disc Discovery, log *zap.SugaredLogger) (*Eth2Node, error) {
	options := []libp2p.Option{
		libp2p.Transport(tcp.NewTCPTransport),
		libp2p.Muxer("/mplex/6.7.0", mplex.DefaultTransport),
		libp2p.ConnectionManager(connmgr.NewConnManager(int(conf.PEER_COUNT_LO), int(conf.PEER_COUNT_HI), pruneGrace)),
		// memory peerstore by default
		// random identity by default
		libp2p.Ping(false),
		libp2p.DisableRelay(),
	}
	if conf.ENABLE_NAT {
		options = append(options, libp2p.NATManager(basichost.NewNATManager))
	}
	if !conf.DISABLE_TRANSPORT_SECURITY {
		options = append(options, libp2p.Security(noise.ID, noise.New))
	}

	h, err := libp2p.New(ctx, options...)
	if err != nil {
		return nil, errors.Wrap(err, "failed host init")
	}
	psOptions := []pubsub.Option{
		pubsub.WithNoAuthor(),
		pubsub.WithMessageSignaturePolicy(pubsub.StrictNoSign),
		pubsub.WithMessageIdFn(MsgIDFunction),
		//pubsub.WithDiscovery(), // TODO could immplement this, may be smoother than current connection work
	}
	if conf.GOSSIP_GLOBAL_SCORE_PARAMS != nil && conf.GOSSIP_GLOBAL_SCORE_THRESHOLDS != nil {
		psOptions = append(psOptions, pubsub.WithPeerScore(conf.GOSSIP_GLOBAL_SCORE_PARAMS, conf.GOSSIP_GLOBAL_SCORE_THRESHOLDS))
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
		horizontalSubs:  make(map[Shard]*pubsub.Subscription),
		slowIndices:     make(map[VerticalIndex]*subnetInfo),
		fastIndices:     make(map[VerticalIndex]*subnetFastInfo),
		log:             log,
		kill:            make(chan struct{}),
	}

	n.shard2Vals, n.val2Shard = n.shardCommitteeShuffling(0)

	if err := n.joinInitialTopics(); err != nil {
		return nil, err
	}
	return n, nil
}

func (n *Eth2Node) Stats() (peerCount uint64) {
	return uint64(len(n.h.Network().Peers()))
}

func (n *Eth2Node) DiscInfo() (peer.ID, []ma.Multiaddr) {
	return n.h.ID(), n.h.Addrs()
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
	out = make([]ValidatorIndex, 0, len(n.localValidators))
	for i := range n.localValidators {
		out = append(out, i)
	}
	return
}

func (n *Eth2Node) Start(ip net.IP, port uint16) error {
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

	// This initializes the initial SLOW_INDICES and FAST_INDICES subscriptions
	if err := n.initialSubscriptions(); err != nil {
		return errors.Wrap(err, "failed to open initial subscriptions")
	}
	go n.processLoop()
	return nil
}

func (n *Eth2Node) processLoop() {
	slotDuration := time.Second * time.Duration(n.conf.SECONDS_PER_SLOT)

	// Note that slot ticker can adjust itself and drop slots, if the receiver is slow (i.e. when under heavy load)
	slotTicker := n.conf.TickerWithOffset(slotDuration, 0)
	defer slotTicker.Stop()

	// TODO schedule work publishing etc.
	workTicker := n.conf.TickerWithOffset(slotDuration, slotDuration/3*2)

	for {
		select {
		case _, _ = <-n.kill:
			n.log.Info("stopping work, goodbye!")
			return
		case t := <-slotTicker.C: // schedules genesis
			slot, preGenesis := n.conf.SlotWithOffset(t, 0)

			if preGenesis {
				// waiting for genesis, countdown every slot near the end.
				//if slot%10 == 0 || slot < 10 {
				//	n.log.With("genesis_time", n.conf.GENESIS_TIME, "slots", slot).Info("Genesis countdown...")
				//}
				// every 4 slots pre-genesis, look for peers to start with
				if slot%4 == 0 {
					n.peersUpdate(0)
				}
				continue
			}
			//n.log.With("slot", slot).Debug("slot event!")

			n.rotateSlowVertSubnets(slot)
			n.rotateFastVertSubnets(slot)
			n.peersUpdate(slot)
		case t := <-workTicker.C:
			// 1/3 before every slot, prepare and schedule shard blocks
			slot, preGenesis := n.conf.SlotWithOffset(t, slotDuration/3)
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

func (n *Eth2Node) publicDasSubset(slot Slot) map[VerticalIndex]struct{} {
	return n.conf.DasSlowSubnetIndices(n.h.ID(), slot, n.conf.SLOW_INDICES)
}

func (n *Eth2Node) initialSubscriptions() error {
	sub, err := n.shardHeaders.Subscribe()
	if err != nil {
		return errors.Wrap(err, "failed to subscribe to shard headers topic")
	}
	go n.shardHeaderHandler(sub)
	slot, preGenesis := n.conf.SlotNow()
	if preGenesis { // if pre-genesis, just register with the topics we'll have to be around at first
		slot = 0
	}
	n.rotateSlowVertSubnets(slot)
	n.rotateFastVertSubnets(slot)
	n.subscribeHorzSubnets()
	return nil
}

// joinInitialTopics does not subscribe to topics, it just makes the handles to them available for later use.
func (n *Eth2Node) joinInitialTopics() error {

	// init DAS subnets
	n.verticalSubnets = make([]*pubsub.Topic, 0, n.conf.SAMPLE_SUBNETS)
	for i := VerticalIndex(0); i < VerticalIndex(n.conf.SAMPLE_SUBNETS); i++ {
		name := n.conf.VertTopic(i)
		if err := n.ps.RegisterTopicValidator(name, n.vertSubnetValidator(i)); err != nil {
			return fmt.Errorf("failed to create validator for das subnet topic %d: %w", i, err)
		}
		t, err := n.ps.Join(name)
		if err != nil {
			return fmt.Errorf("failed to prepare das subnet topic %d: %w", i, err)
		}
		go n.handleTopicEvents(name, t)
		if n.conf.VERT_SUBNET_TOPIC_SCORE_PARAMS != nil {
			if err := t.SetScoreParams(n.conf.VERT_SUBNET_TOPIC_SCORE_PARAMS); err != nil {
				return errors.Wrap(err, "bad vertical subnet score params")
			}
		}
		n.verticalSubnets = append(n.verticalSubnets, t)
	}

	// init shard subnets
	n.horizontalSubnets = make([]*pubsub.Topic, 0, n.conf.SHARD_COUNT)
	for i := Shard(0); i < Shard(n.conf.SHARD_COUNT); i++ {
		name := n.conf.HorzTopic(i)
		if err := n.ps.RegisterTopicValidator(name, n.horzSubnetValidator(i)); err != nil {
			return fmt.Errorf("failed to create validator for shard subnet topic %d: %w", i, err)
		}
		t, err := n.ps.Join(name)
		if err != nil {
			return fmt.Errorf("failed to prepare shard subnet topic %d: %w", i, err)
		}
		go n.handleTopicEvents(name, t)
		if n.conf.HORZ_SUBNET_TOPIC_SCORE_PARAMS != nil {
			if err := t.SetScoreParams(n.conf.HORZ_SUBNET_TOPIC_SCORE_PARAMS); err != nil {
				return errors.Wrap(err, "bad horizontal subnet score params")
			}
		}
		n.horizontalSubnets = append(n.horizontalSubnets, t)
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
		if n.conf.SHARD_HEADERS_TOPIC_SCORE_PARAMS != nil {
			if err := t.SetScoreParams(n.conf.SHARD_HEADERS_TOPIC_SCORE_PARAMS); err != nil {
				return errors.Wrap(err, "bad shard headers score params")
			}
		}
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
			//n.log.With("peer", ev.Peer.String(), "topic", name).Debug("peer JOIN")
		case pubsub.PeerLeave:
			//n.log.With("peer", ev.Peer.String(), "topic", name).Debug("peer LEAVE")
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
