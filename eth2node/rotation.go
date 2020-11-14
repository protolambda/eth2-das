package eth2node

import (
	"crypto/rand"
	"encoding/binary"
	"go.uber.org/zap"
)

// rotateSlowVertSubnets rotates the subscriptions for topics we are on, called by the main loop every slot.
// It can handle skipped slots, and will robustly match slot expectations
func (n *Eth2Node) rotateSlowVertSubnets(slot Slot) {
	newSubnets := n.publicDasSubset(slot)
	futureSubnets := n.publicDasSubset(slot + 32)

	// cancel subscriptions of topics we left.
	for subnet, info := range n.slowIndices {
		if _, ok := newSubnets[subnet]; !ok {
			// Optimization:
			// check slowIndices of slot + 32, and see if we need so sub to the topic again.
			// If so, then don't cancel, avoid a quick leave/graft
			if _, ok := futureSubnets[subnet]; ok {
				continue
			}

			// TODO: technically one of the random FAST_INDICES rotations might hit it soon, re-subscribing to the same topic.
			// Resubscribing shouldn't be a big issue if peer connections are maintained between the leave and join.
			// (un)-subscriptions are also ignored for gossip scoring currently.
			info.sub.Cancel()
		}
	}

	// get subscriptions for topics we joined.
	for subnet := range newSubnets {
		// not everything rotates all the time
		if _, ok := n.slowIndices[subnet]; ok {
			continue
		}
		// sometimes we are already subscribed to it privately. Move it over then.
		if v, ok := n.fastIndices[subnet]; ok {
			// Remove from FAST_INDICES subset, add to SLOW_INDICES subset
			delete(n.fastIndices, subnet)
			// just get the regular subnet info, strip out the rotation that was part of the FAST_INDICES subnets logic.
			n.slowIndices[subnet] = &v.subnetInfo
			continue
		}
		// and sometimes we really do have to open a new subscription
		t := n.verticalSubnets[subnet]
		sub, err := t.Subscribe()
		if err != nil {
			n.log.With(zap.Error(err)).Error("failed to subscribe to das subnet as part of public duties")
		} else {
			n.slowIndices[subnet] = &subnetInfo{
				subscribedAt: slot,
				sub:          sub,
			}
			go n.vertHandleSubnet(subnet, sub)
		}
	}
}

// rotateFastVertSubnets, like rotateSlowVertSubnets, rotates subnets and does so robustly.
// But instead of deterministically and publicly predictable subscribing, it is based on private local randomness.
// Subscriptions in FAST_INDICES won't overlap with those in SLOW_INDICES, to avoid double work.
func (n *Eth2Node) rotateFastVertSubnets(slot Slot) {
	// Swap things as scheduled, with something that is not already in our public subnet indices

	// collect everything that is getting rotated out
	old := make(map[VerticalIndex]*subnetInfo)
	// rotate out the expired
	for subnet, info := range n.fastIndices {
		// time to unsubscribe here yet?
		if info.expiry <= slot {
			old[subnet] = &info.subnetInfo
			delete(n.fastIndices, subnet)
		}
	}
	// now fill back up to FAST_INDICES with random subnets
	randomUint64 := func() uint64 {
		var indexBuf [8]byte
		if _, err := rand.Read(indexBuf[:]); err != nil {
			panic("could not read random bytes")
		}
		return binary.LittleEndian.Uint64(indexBuf[:])
	}

	randomSubnet := func() VerticalIndex {
		return VerticalIndex(randomUint64() % n.conf.SAMPLE_SUBNETS)
	}

	randomExpiry := func() Slot {
		return slot + 1 + Slot(randomUint64()%n.conf.SLOTS_PER_FAST_ROTATION_MAX)
	}

	for i := uint64(len(n.fastIndices)); i < n.conf.FAST_INDICES; i++ {
		for { // try random subnets until we find one that works
			// (given a small FAST_INDICES and SLOW_INDICES, and large CHUNK_INDEX_SUBNETS, trial-and-error should be fast)
			subnet := randomSubnet()
			// must not be part of SLOW_INDICES already
			if _, ok := n.slowIndices[subnet]; ok {
				continue
			}
			// must not be part of FAST_INDICES already
			if _, ok := n.fastIndices[subnet]; ok {
				continue
			}
			// if we were subscribed to it previously, then fine, we'll stay on it.
			if v, ok := old[subnet]; ok {
				delete(old, subnet)

				//n.log.With("subnet", subnet, "slot", slot).Debug("re-subscribing to FAST_INDICES subnet")
				n.fastIndices[subnet] = &subnetFastInfo{
					subnetInfo: *v,             // same old subnet
					expiry:     randomExpiry(), // new expiry time
				}
			} else {
				//n.log.With("subnet", subnet, "slot", slot).Debug("subscribing to new FAST_INDICES subnet")
				t := n.verticalSubnets[subnet]
				sub, err := t.Subscribe() // the new subnet!
				if err != nil {
					n.log.With(zap.Error(err)).Error("failed to subscribe to das subnet as part of private duties")
				} else {
					n.fastIndices[subnet] = &subnetFastInfo{
						subnetInfo: subnetInfo{
							subscribedAt: slot,
							sub:          sub,
						},
						expiry: randomExpiry(), // new expiry time
					}
					go n.vertHandleSubnet(subnet, sub)
				}
			}
			break
		}
	}

	// for any old subnets that were not reused, unsubscribe
	for _, info := range old {
		//n.log.With("subnet", subnet, "slot", slot).Debug("unsubscribing from FAST_INDICES subnet")
		info.sub.Cancel()
	}
}
