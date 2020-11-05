# Phase 1 DAS Validator

This document describes how the validator behavior is extended for data-availaibility-sampling (DAS).

## Table of contents

<!-- TOC -->
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
TODO TOC
<!-- END doctoc generated TOC please keep comment here to allow auto update -->
<!-- /TOC -->


| `MAX_DATA_SIZE` | `BYTES_PER_DATA_POINT * POINTS_PER_SAMPLE * MAX_SAMPLES_PER_SHARD_BLOCK // 2` | bytes | Max bytes per (unextended) block data blob |

### Mapping shard data to points

```python
def shard_data_to_points(input_bytes: bytes) -> Sequence[Point]:
    assert len(input_bytes) <= MAX_DATA_SIZE
    
    input_points = [deserialize_point(input_bytes[offset:min(offset+BYTES_PER_DATA_POINT, len(input_bytes))])
                     for offset in range(0, len(input_bytes), BYTES_PER_DATA_POINT)]
    padded_width = next_power_of_two(len(input_points))
    padded_points = input_points + [Point(0)] * (padded_width - len(input_points))

    # original points, but in reverse bit order. Simplifies some proofs over the data.
    even_points = [padded_points[i] for i in reverse_bit_order(padded_width.bitlength())]

    extended_width = len(padded_width) * 2
    # TODO: allow for variable powers of two in block size?
    assert extended_width == POINTS_PER_SAMPLE * MAX_SAMPLES_PER_SHARD_BLOCK

    domain = domain_for_size(extended_width)
    inverse_domain = [modular_inverse(d, MODULUS) for d in domain]  # Or simply reverse the domain (except first 1)
    odd_points = das_extension(even_points, MODULUS, domain, inverse_domain)
    
    return [even_points[i // 2] if i % 2 == 0 else odd_points[i // 2] for i in range(extended_width)]

def reverse_bit_order(bits):
    if bits == 0:
        return [0]
    return (
        [x*2 for x in reverse_bit_order(bits-1)] +
        [x*2+1 for x in reverse_bit_order(bits-1)] 
    )
```

### Mapping points to shard data

```python
def points_to_shard_data() -> bytes:
    ... # TODO reverse process
```

### Unextending points

First undo the reverse-bitorder reorg of the points.
Then take all the even-index points.

```python
def unextend_points(extended_points: Sequence[Point]) -> Sequence[Point]:
    ... # TODO
```
### Recovering points

Define how validator recovers points (to then map to shard data with above function),
 using the the recovery method outlined in `data_as_points.md`.

```python
def recover_points(samples: Dict[VerticalIndex, Sample]) -> Sequence[Point]:
    ... # TODO
```

### Mapping samples to DAS subnets

TODO. `hash(sample_index, shard, slot) % subnet_count = subnet index`

Randomized to spread load better. Skewed load would be limited in case of attack (manipulating sample count), but inconvenient.
Sample counts are also only powers of 2, so there is little room for manipulation there.

Shared subnets + high sample count = all subnets used, empty subnets should not be a problem.
