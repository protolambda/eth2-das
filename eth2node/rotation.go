package eth2node

import "go.uber.org/zap"

// rotatePSubnets rotates the subscriptions for topics we are on, called by the main loop every slot.
// It can handle skipped slots, and will robustly match slot expectations
func (n *Eth2Node) rotatePSubnets(slot Slot) {
	newSubnets := n.publicDasSubset(slot)

	// cancel subscriptions of topics we left.
	for subnet, info := range n.pIndices {
		if _, ok := newSubnets[subnet]; !ok {
			// TODO Possible optimization:
			// check pIndices of slot + 32, and see if we need so sub to the topic again.
			// If so, then don't cancel, avoid a quick leave/graft
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
			delete(n.kIndices, subnet)
			n.pIndices[subnet] = v
			continue
		}
		// and sometimes we really do have to open a new subscription
		t := n.subnets[subnet]
		sub, err := t.Subscribe()
		if err != nil {
			n.log.With(zap.Error(err)).Error("failed to subscribe to das subnet as part of public duties")
		} else {
			n.pIndices[subnet] = &subnetInfo{
				subscribedAt: slot,
				sub:          sub,
			}
		}
	}
}

func (n *Eth2Node) rotateKSubnets(slot Slot) {
	// swap a 1/K randomly, with something that is not already in our public subnet indices
	// TODO
}
