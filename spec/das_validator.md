# Phase 1 DAS Validator

This document describes how the validator behavior is extended for data-availaibility-sampling (DAS).

## Table of contents

<!-- TOC -->
<!-- START doctoc generated TOC please keep comment here to allow auto update -->
<!-- DON'T EDIT THIS SECTION, INSTEAD RE-RUN doctoc TO UPDATE -->
TODO TOC
<!-- END doctoc generated TOC please keep comment here to allow auto update -->
<!-- /TOC -->


| `MAX_DATA_SIZE` | `POINT_SIZE * POINTS_PER_SAMPLE * MAX_SAMPLES_PER_SHARD_BLOCK // 2` | bytes | max bytes per (unextended) block data blob |

### Mapping shard data to points

```python
def shard_data_to_points(input_bytes: bytes) -> Sequence[Point]:
    assert len(input_bytes) <= MAX_DATA_SIZE

    padded_bytes = input_bytes + "\x00" * (sample_target_byte_length - len(input_bytes))
    
    raw_points = [raw_point(padded_bytes, i) for i in range(MAX_DATA_SIZE // POINT_SIZE)]
    assert len(raw_points) == next_power_of_2(len(raw_points))

    input_points = [deserialize_point(p) for p in raw_points]
    
    extended_width = len(input_points) * 2
    assert extended_width == POINTS_PER_SAMPLE * MAX_SAMPLES_PER_SHARD_BLOCK

    domain = domain_for_size(extended_width)
    inverse_domain = [modular_inverse(d, MODULUS) for d in domain]  # Or simply reverse the domain (except first 1)
    odd_points = das_extension(input_points, MODULUS, domain, inverse_domain)
    
    extended_points = [input_points[i // 2] if i % 2 == 0 else odd_points[i // 2] for i in range(extended_width)]
    
    # TODO: changing it to bit-reverse order, so that points are grouped more efficiently for sample proofs
    reorganized_extended_points = bit_reverse_reorg(extended_points)
    
    return reorganized_extended_points
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
