package eth2node

import (
	verkle "github.com/protolambda/go-verkle"
	"github.com/protolambda/zrnt/eth2/beacon"
	"github.com/protolambda/ztyp/codec"
	"github.com/protolambda/ztyp/tree"
)

type VerticalIndex uint64

// Aliases for ease of use
type ValidatorIndex = beacon.ValidatorIndex
type Root = beacon.Root
type Shard = beacon.Shard
type Slot = beacon.Slot
type Epoch = beacon.Epoch
type BLSSignature = beacon.BLSSignature

type PointIndex uint64
type Point = verkle.Big
type RawPoint [POINT_SIZE]byte

type ShardBlock struct {
	ShardParentRoot  Root
	BeaconParentRoot Root
	Slot             Slot
	Shard            Shard
	ProposerIndex    ValidatorIndex
	Body             ShardBlockData
}

func (d *ShardBlock) Deserialize(dr *codec.DecodingReader) error {
	return dr.Container(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.Body)
}

func (d *ShardBlock) Serialize(w *codec.EncodingWriter) error {
	return w.Container(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.Body)
}

func (d *ShardBlock) ByteLength() uint64 {
	return codec.ContainerLength(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.Body)
}

func (d *ShardBlock) FixedLength() uint64 {
	return 0
}

func (d *ShardBlock) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.Body)
}

type SignedShardBlock struct {
	Message   ShardBlock
	Signature BLSSignature
}

func (d *SignedShardBlock) Deserialize(dr *codec.DecodingReader) error {
	return dr.Container(&d.Message, &d.Signature)
}

func (d *SignedShardBlock) Serialize(w *codec.EncodingWriter) error {
	return w.Container(&d.Message, &d.Signature)
}

func (d *SignedShardBlock) ByteLength() uint64 {
	return codec.ContainerLength(&d.Message, &d.Signature)
}

func (d *SignedShardBlock) FixedLength() uint64 {
	return 0
}

func (d *SignedShardBlock) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(&d.Message, &d.Signature)
}

type ShardBlockHeader struct {
	ShardParentRoot  Root
	BeaconParentRoot Root
	Slot             Slot
	Shard            Shard
	ProposerIndex    ValidatorIndex
	BodyRoot         Root
}

func (d *ShardBlockHeader) Deserialize(dr *codec.DecodingReader) error {
	return dr.FixedLenContainer(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.BodyRoot)
}

func (d *ShardBlockHeader) Serialize(w *codec.EncodingWriter) error {
	return w.FixedLenContainer(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.BodyRoot)
}

func (d *ShardBlockHeader) ByteLength() uint64 {
	return codec.ContainerLength(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.BodyRoot)
}

func (d *ShardBlockHeader) FixedLength() uint64 {
	return codec.ContainerLength(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.BodyRoot)
}

func (d *ShardBlockHeader) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(&d.ShardParentRoot, &d.BeaconParentRoot, &d.Slot, &d.Shard, &d.ProposerIndex, &d.BodyRoot)
}

type SignedShardBlockHeader struct {
	Message   ShardBlockHeader
	Signature BLSSignature
}

func (d *SignedShardBlockHeader) Deserialize(dr *codec.DecodingReader) error {
	return dr.FixedLenContainer(&d.Message, &d.Signature)
}

func (d *SignedShardBlockHeader) Serialize(w *codec.EncodingWriter) error {
	return w.FixedLenContainer(&d.Message, &d.Signature)
}

func (d *SignedShardBlockHeader) ByteLength() uint64 {
	return codec.ContainerLength(&d.Message, &d.Signature)
}

func (d *SignedShardBlockHeader) FixedLength() uint64 {
	return codec.ContainerLength(&d.Message, &d.Signature)
}

func (d *SignedShardBlockHeader) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(&d.Message, &d.Signature)
}

// TODO: normally would put this in spec config, but that's not up to date with prototype config options
const blockDataLimit = 1 << 20

type ShardBlockData []byte

func (d *ShardBlockData) Deserialize(dr *codec.DecodingReader) error {
	return dr.ByteList((*[]byte)(d), blockDataLimit)
}

func (d *ShardBlockData) Serialize(w *codec.EncodingWriter) error {
	return w.Write(*d)
}

func (d *ShardBlockData) ByteLength() uint64 {
	return uint64(len(*d))
}

func (d *ShardBlockData) FixedLength() uint64 {
	return 0
}

func (d *ShardBlockData) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.ByteListHTR(*d, blockDataLimit)
}

// TODO
const sampleByteLength = 1 << 20

// TODO naming
type ShardBlockDataChunk []byte

func (d *ShardBlockDataChunk) Deserialize(dr *codec.DecodingReader) error {
	return dr.ByteVector((*[]byte)(d), sampleByteLength)
}

func (d *ShardBlockDataChunk) Serialize(w *codec.EncodingWriter) error {
	return w.Write(*d)
}

func (d *ShardBlockDataChunk) ByteLength() uint64 {
	return uint64(len(*d))
}

func (d *ShardBlockDataChunk) FixedLength() uint64 {
	return uint64(len(*d))
}

func (d *ShardBlockDataChunk) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.ByteVectorHTR(*d)
}

type DASMessage struct {
	Slot  Slot
	Chunk ShardBlockDataChunk
	Index VerticalIndex

	// TODO: does this need a ref to the shard block root, so it can be matched with the header easily?
	ShardHeaderRoot Root

	// Proof to show that the chunk is part of the vector commitment in the header.
	KateProof beacon.BLSPubkey
}

func (d *DASMessage) Deserialize(dr *codec.DecodingReader) error {
	return dr.FixedLenContainer(&d.Slot, &d.Chunk, &d.ShardHeaderRoot, &d.KateProof)
}

func (d *DASMessage) Serialize(w *codec.EncodingWriter) error {
	return w.FixedLenContainer(&d.Slot, &d.Chunk, &d.ShardHeaderRoot, &d.KateProof)
}

func (d *DASMessage) ByteLength() uint64 {
	return codec.ContainerLength(&d.Slot, &d.Chunk, &d.ShardHeaderRoot, &d.KateProof)
}

func (d *DASMessage) FixedLength() uint64 {
	return codec.ContainerLength(&d.Slot, &d.Chunk, &d.ShardHeaderRoot, &d.KateProof)
}

func (d *DASMessage) HashTreeRoot(hFn tree.HashFn) Root {
	return hFn.HashTreeRoot(&d.Slot, &d.Chunk, &d.ShardHeaderRoot, &d.KateProof)
}
