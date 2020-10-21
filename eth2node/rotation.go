package eth2node

import (
	"crypto/rand"
	"encoding/binary"
	"go.uber.org/zap"
)

// rotatePSubnets rotates the subscriptions for topics we are on, called by the main loop every slot.
// It can handle skipped slots, and will robustly match slot expectations
func (n *Eth2Node) rotatePSubnets(slot Slot) {
	newSubnets := n.publicDasSubset(slot)
	futureSubnets := n.publicDasSubset(slot + 32)

	// cancel subscriptions of topics we left.
	for subnet, info := range n.pIndices {
		if _, ok := newSubnets[subnet]; !ok {
			// Optimization:
			// check pIndices of slot + 32, and see if we need so sub to the topic again.
			// If so, then don't cancel, avoid a quick leave/graft
			if _, ok := futureSubnets[subnet]; ok {
				continue
			}

			// TODO: technically one of the random K rotations might hit it soon, re-subscribing to the same topic.
			// Resubscribing shouldn't be a big issue if peer connections are maintained between the leave and join.
			// (un)-subscriptions are also ignored for gossip scoring currently.
			info.sub.Cancel()
		}
	}

	// get subscriptions for topics we joined.
	for subnet := range newSubnets {
		// not everything rotates all the time
		if _, ok := n.pIndices[subnet]; ok {
			continue
		}
		// sometimes we are already subscribed to it privately. Move it over then.
		if v, ok := n.kIndices[subnet]; ok {
			// Remove from K subset, add to P subset
			delete(n.kIndices, subnet)
			// just get the regular subnet info, strip out the rotation that was part of the K subnets logic.
			n.pIndices[subnet] = &v.subnetInfo
			continue
		}
		// and sometimes we really do have to open a new subscription
		t := n.dasSubnets[subnet]
		sub, err := t.Subscribe()
		if err != nil {
			n.log.With(zap.Error(err)).Error("failed to subscribe to das subnet as part of public duties")
		} else {
			n.pIndices[subnet] = &subnetInfo{
				subscribedAt: slot,
				sub:          sub,
			}
			go n.dasHandleSubnet(subnet, sub)
		}
	}
}

func (n *Eth2Node) rotateKSubnets(slot Slot) {
	// Swap things as scheduled, with something that is not already in our public subnet indices

	// collect everything that is getting rotated out
	old := make(map[DASSubnetIndex]*subnetInfo)
	// rotate out the expired
	for subnet, info := range n.kIndices {
		// time to unsubscribe here yet?
		if info.expiry <= slot {
			old[subnet] = &info.subnetInfo
			delete(n.kIndices, subnet)
		}
	}
	// now fill back up to K with random subnets
	randomUint64 := func() uint64 {
		var indexBuf [8]byte
		if _, err := rand.Read(indexBuf[:]); err != nil {
			panic("could not read random bytes")
		}
		return binary.LittleEndian.Uint64(indexBuf[:])
	}

	randomSubnet := func() DASSubnetIndex {
		return DASSubnetIndex(randomUint64() % n.conf.CHUNK_INDEX_SUBNETS)
	}

	randomExpiry := func() Slot {
		return slot + 1 + Slot(randomUint64()%n.conf.SLOTS_PER_K_ROTATION_MAX)
	}

	for i := uint64(len(n.kIndices)); i < n.conf.K; i++ {
		for { // try random subnets until we find one that works
			// (given a small K and P, and large CHUNK_INDEX_SUBNETS, trial-and-error should be fast)
			subnet := randomSubnet()
			// must not be part of P already
			if _, ok := n.pIndices[subnet]; ok {
				continue
			}
			// must not be part of K already
			if _, ok := n.kIndices[subnet]; ok {
				continue
			}
			// if we were subscribed to it previously, then fine, we'll stay on it.
			if v, ok := old[subnet]; ok {
				delete(old, subnet)

				n.kIndices[subnet] = &subnetKInfo{
					subnetInfo: *v,             // same old subnet
					expiry:     randomExpiry(), // new expiry time
				}
			} else {
				t := n.dasSubnets[subnet]
				sub, err := t.Subscribe() // the new subnet!
				if err != nil {
					n.log.With(zap.Error(err)).Error("failed to subscribe to das subnet as part of private duties")
				} else {
					n.kIndices[subnet] = &subnetKInfo{
						subnetInfo: subnetInfo{
							subscribedAt: slot,
							sub:          sub,
						},
						expiry: randomExpiry(), // new expiry time
					}
					go n.dasHandleSubnet(subnet, sub)
				}
			}
			break
		}
	}
}
