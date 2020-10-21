# Eth2 Data Availability Sampling - Testground plan

This repository is a test plan to import into [Testground](https://github.com/testground/testground).
It tests the viability of various networking ideas for Phase 1 of Eth2.

## DAS experiment

Background:
- [Original write-up of Vitalik with different approaches](https://notes.ethereum.org/@vbuterin/r1v8VCULP)
- [Expansion on approach 2 by protolambda, using gossip subnets](https://notes.ethereum.org/McLGvrWgSX6Cpg60ewMYeQ)

The idea is to implement a proof of concept in Go, and run it on Testground with different parameters.
**Work in progress, highly experimental**

### The `eth2node` package

A mock Eth2 node with the Phase 1 functionality, and instrumentation for gossip tests.

- register topics:
  - `CHUNK_INDEX_SUBNETS` DAS subnets
  - `SHARD_COUNT` Shard subnets
  - Shard headers global topic
- Subscribe to `P` DAS subnets randomly based on publicly known seed, unique to peer.
  - For stability
  - Find other peers via discovery (TODO add discovery)
- Subscribe to `K` DAS subnets randomly 
  - The random sampling necessary to make DAS work secure enough
  - Verify if all the seen chunks are correct
    - TODO: keep track of what has been seen, *and what has not*
      - Adjust forkchoice to steer clear of unavailable shard data
- Subscribe randomly to a shard block subnet for every Validator that runs
  - Rotate out on random `[EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION, 2 * EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION]` interval (per shard)
  - For any seen block, split it up in chunks, and for each DAS subnet we are on, propagate the relevant chunks to it.
- Subscribe to shard headers topic
- Validate DAS subnets:
  - TODO
- Validate Shard subnets:
  - TODO
- Validate Shard headers topic:
  - TODO

### Constants / configurables

Don't take values as canonical. The point is to find good values.

#### Configurables

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `K` | `16` | indices | Subset of indices that nodes sample, privately  |
| `P` | `4` | indices | Subset of indices that nodes sample, publicly |
| `CHUNK_SIZE` | `512` | bytes | Number of bytes per chunk (index) |
| `MAX_BLOCK_SIZE` | `524288` | bytes | Number of bytes in a block |
| `SHARD_HEADER_SIZE` | `256` | bytes | Number of bytes in a shard header | 
| `SHARD_COUNT` | `64` | shards | Number of shards |
| `SECONDS_PER_SLOT` | `12` | seconds | Number of seconds in each slot |

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `SLOTS_PER_K_ROTATION_MAX` | `32` | slots | Maximum of how frequently one of the K dasSubnets are randomly swapped. Rotations of a subnet in K can happen any time between 1 and SLOTS_PER_K_ROTATION_MAX (incl) slots. |
| `SLOTS_PER_P_ROTATION` | `320` | slots | How frequently each of the P dasSubnets are randomly swapped. (deterministic on peer ID, so public and predictable) |
| `SLOT_OFFSET_PER_P_INDEX` | `16` | slots | SLOT_OFFSET_PER_P_INDEX is the time for an index of P to wait for the previous index to rotate. |
| `EPOCHS_PER_RANDOM_SUBNET_SUBSCRIPTION` | `256` | epochs | how frequently shard subnet subscriptions are kept |

#### Estimates for emulation purposes

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `VALIDATOR_COUNT` | `150000` | validators | Number of active validators |
| `NODE_COUNT` | `10000` | nodes | Physical validator nodes in the p2p network |


#### Derived configurable value

| Name | Planned Value | Unit | Description |
| - | - | - | - |
| `CHUNK_INDEX_SUBNETS` | `MAX_BLOCK_SIZE / CHUNK_SIZE = 1024` | subnets | Number of chunks that make up a block | 
| `AVERAGE_VALIDATORS_PER_NODE` | `VALIDATOR_COUNT / NODE_COUNT` | validators per node | 


### Public DAS subnet sampling, a.k.a. `P`

Public indices are sampled as:
```python
def das_public_peer_seed(peer_id: PeerID) -> Bytes32:
  return H(PUBLIC_DAS_SUBNETS_DOMAIN ++ peer_id)

def das_public_peer_slot_offset(peer_seed) -> Slot:
  return Slot(peer_seed[:8] % SLOTS_PER_P_ROTATION)

def das_public_subnet_slot_offset(i: DASSubnetIndex) -> Slot:
  return Slot(i * SLOT_OFFSET_PER_P_INDEX) % SLOTS_PER_P_ROTATION

def das_public_subnet_index(peer_seed: Bytes32, slot: Slot, i: uint64) -> DASSubnetIndex:
  window_index = slot ~/ SLOTS_PER_P_ROTATION
  return H(peer_seed ++ i ++ window_index)[:8] % CHUNK_INDEX_SUBNETS

def das_public_subnet_indices(peer_id: PeerID, slot: Slot, das_p: uint64) -> Set[DASSubnetIndex]:
  peer_seed = das_public_peer_seed(peer_id)
  peer_offset = das_public_peer_slot_offset(peer_seed)
  return set(
 		das_public_subnet_index(
				peer_seed,
				slot + peer_offset + das_public_subnet_slot_offset(i),
				i,
			) for i in range(das_p))

p_indices = das_public_subnet_indices(peer_id, slot, P)
```

Any overlap in `p_indices` is rare, and acceptable. The set will be a little smaller sometimes.

The idea of making `P` slow, public and predictable is:
- As backbone, for subnet stability
- Easy to discover peers through
- By using a seed based on peer ID, no data has to be tracked on discovery. No blow up in ENR size.
- Predictable enough to prepare subnet peering in advance
- Small value, other sampling can be unpredictable and rotate quicker.

### Private DAS subnet sampling, a.k.a. `K`

For the random sampling, `K` subnets are joined randomly,
selected out of the total `CHUNK_INDEX_SUBNETS`, except for what is already joined publicly (`P`).

Then on random intervals, each of the `K` subnet subscriptions is swapped for another subnet.

This effectively creates a PUSH model where validators 
register themselves on short notice to be sampling a random subset.
The shard blocks are split up in chunks, and should reach the validators with relatively low latency.

**Note: depending on test results, a larger K may be set, but for any topics that are subscribed to in e.g. 
the last few slots won't count as heavy towards bad data availability,
as it may just be an issue to subscribe to the topic fast enough.**


```python
class KInfo:
    sub: Subscription
    expiry: Slot

def random_subnet() -> DASSubnetIndex:
    return DASSubnetIndex(random_uint64() % CHUNK_INDEX_SUBNETS)

def random_expiry(now: Slot) -> Slot:
    return now + 1 + Slot(random_uint64() % SLOTS_PER_K_ROTATION_MAX)

def das_private_subnets_update(
    current_k: Dict[DASSubnetIndex, KInfo],
    current_p: Dict[DASSubnetIndex, Subscription],
    slot: Slot):

    old: Dict[DASSubnetIndex, Subscription] = {}
    for subnet, info in current_k.items():
        if info.expiry <= slot:
            old[subnet] = info.sub
            current_k.remove(subnet)
    # By trial and error, backfill k
    while len(current_k) < K:
        while True:
            subnet = random_subnet()
            if subnet in current_k:
                continue
            if subnet in current_p:
                continue
            if subnet in old:
                old.remove(subnet)
                current_k[subnet] = KInfo(sub=old[subnet], expiry=random_expiry(slot))
            else:
                sub = subcribe(subnet)
                current_k[subnet] = KInfo(sub=sub, expiry=random_expiry(slot))
            break
    # unsubscribe from remaining old topics
    for sub in old:
        sub.unsubscribe()
```

### DAS subnet discovery

The node uses a simple discovery interface:
```python
class Discovery(Protocol):
	def find_public(conf: Config, slot: Slot, subnets: Set[DASSubnetIndex]) -> Dict[DASSubnetIndex, Set[PeerID]]: ...
	def addrs(id: PeerID) -> List[Multiaddr]: ...

```

It lists possible candidates for each subnet, based on public subnet info known of the peers: their `P` subset.

In preparation of rotation of subnets, the Eth2 node can then try to peer with peers that complement their current peers,
 to allow gossipsub to build a mesh for the topic.

## License

MIT, see [`LICENSE`](./LICENSE) file.
