package drepl

import (
	"fmt"
)

type View struct {
	repl		*Replica
	Name		string
	offset		int64		// offset in the replica the view belongs to
	elo		ElementOrder	// order of the elements in a multidimensional arrays
	bs		BlockSeq
	dflt		*View		// default view for unmaterialized views
	readonly	bool
}

type BlockSeq struct {
	v 		*View
	blks		[]Block
}

// creates a view that doesn't belong to a replica (unmaterialized view)
func NewView(name string, elo ElementOrder, readonly bool) *View {
	v := new(View)
	v.offset = 0
	v.Name = name
	v.elo = elo
	v.readonly = readonly
	v.bs.v = v

	return v
}

func (v *View) Size() int64 {
	return v.bs.Size()
}

func (v *View) Materialized() bool {
	return v.repl != nil
}

func (v *View) SetDefaultView(dv *View) {
	v.dflt = dv
}

func (v *View) IsReadonly() bool {
	return v.readonly
}

func (v *View) Blocks() *BlockSeq {
	return &v.bs
}

func (v *View) NewBlockSeq() *BlockSeq {
	bs := new(BlockSeq)
	bs.v = v
//	fmt.Printf("NewBlockSeq %p view %p\n", bs, v)
	return bs
}

func (v *View) Read(buf []byte, offset int64) (int64, error) {
	return v.repl.Read(buf, offset + v.offset)
}

func (v *View) Write(data []byte, offset int64) (int64, error) {
//	fmt.Printf("View.Write %s offset %d count %d\n", v.Name, offset, len(data))
	return v.repl.Write(data, offset + v.offset)
}

func (v *View) Sync() error {
	return v.repl.Sync()
}

func (v *View) String() string {
	s := fmt.Sprintf("View '%s' offset %d size %d\n", v.Name, v.offset, v.Size())
	for _, b := range v.bs.blks {
		s = fmt.Sprintf("%s\t\t%v\n", s, b)
	}

	return s
}

func (bs *BlockSeq) View() *View {
	return bs.v
}

func (bs *BlockSeq) Size() (sz int64) {
	if bs.blks != nil && len(bs.blks) > 0 {
		b := bs.blks[len(bs.blks) - 1]
		sz = b.Offset() + b.Size()
	}

	return
}

func (bs *BlockSeq) NewSBlock(size int64) *SBlock {
	b := new(SBlock)
	b.view = bs.v
	b.offset = bs.Size()
	b.size = size
	bs.blks = append(bs.blks, b)

	return b
}

func (bs *BlockSeq) Blocks() []Block {
	return bs.blks
}

func (bs *BlockSeq) NewABlock(elsize int64, dim []int64, el Block) *ABlock {
	ab := new(ABlock)
	ab.view = bs.v
	ab.offset = bs.Size()
	ab.elsize = elsize
	ab.dim = dim
	ab.elnum = 1
	for _, m := range(dim) {
		ab.elnum *= m
	}

	ab.size = ab.elnum * elsize
	ab.elblk = el
	bs.blks = append(bs.blks, ab)

	return ab
}

func (bs *BlockSeq) NewTBlock(bbs *BlockSeq) *TBlock {
	tb := new(TBlock)
	tb.view = bs.v
	tb.offset = bs.Size()
	tb.bs = *bbs
	tb.size = bbs.Size()
	bs.blks = append(bs.blks, Block(tb))

	return tb
}

func (bs *BlockSeq) String() string {
	s := "["
	for _, b := range bs.blks {
		s = fmt.Sprintf("%s (%v)", s, b)
	}

	return s + "]"
}

