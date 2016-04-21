package drepl

import (
	"fmt"
	"errors"
)

type Block interface {
	Offset() int64
	Size() int64
	Read(buf []byte, offset, base int64) (int64, error)
	Write(data []byte, offset, base int64, write, replicate bool) (int64, error)
//	Clone() Block				// doesn't clone the destinations
	ConnectDestinations()
	cloneConnect(blk1, blk2 Block, udest bool) Block
	isDest(d Block) bool
	xform(src, dst []byte)			// converts data described by the block to the data described to the (single) destination
	replicate(data []byte, offset, base int64) error
	export1(blks map[Block]int32)
	export2(data []byte, blks map[Block]int32, views map[*View]int32, visited map[Block]bool) []byte
}

// single block
type SBlock struct {
	view	*View
	offset	int64
	size	int64
	dests	[]*SBlock
	src	*SBlock		// if unmaterialized block, pointer to the block to read from

	// debugging stuff
	clonee	*SBlock		// if the block is a clone, the original
}

// array block
type ABlock struct {
	view	*View
	offset	int64
	size	int64

	dim	[]int64
	elsize	int64
	elnum	int64		// number of elements (product of all values in dim)
	elblk	Block
	dests	[]*ADest
	src	*ADest		// if unmaterialized block, pointer to the block to read from

	// debugging stuff
	clonee	*ABlock		// if the block is a clone, the original
}

type ADest struct {
	expr	[]PExpr
	arr	*ABlock
	el	Block
}

// used only during the construction of the block list
type TBlock struct {
	view	*View
	offset	int64
	size	int64
	bs	BlockSeq
	dests	[]*TBlock
	src	*TBlock		// if unmaterialized view, pointer to the block to read from

	// debugging stuff
	clonee	*TBlock		// if the block is a clone, the original
}

type Destination struct {
	fname	string
	offset	int64
}

var AsyncWrite = false

func (b *SBlock) Offset() int64 {
	return b.offset
}

func (b *SBlock) Size() int64 {
	return b.size
}

func (b *SBlock) AddDestination(dst *SBlock) {
	b.dests = append(b.dests, dst)
}

func (b *SBlock) AddSource(src *SBlock) {
	b.src = src
}

func (b *SBlock) String() string {
	dests := "("
	for _, d := range b.dests {
		dests = fmt.Sprintf("%s %s:%06d(%p) ", dests, d.view.repl.Name, d.offset, d)
	}
	dests += ")"

	return fmt.Sprintf("%06d SBlock %p size %d src %p dests %s", b.offset, b, b.size, b.src, dests)
}

func (b *SBlock) isDest(d Block) bool {
	for _, bb := range b.dests {
		if bb == d {
			return true
		}
	}

	return false
}

func (b *SBlock) Read(buf []byte, offset, base int64) (int64, error) {
	if b.view.repl != nil {
		return b.view.Read(buf, offset)
	}

	// Unmaterialized view
	// The execution can reach this point only if the SBlock is at top level,
	// i.e. is not part of TBlock or ABlock. In that case the block offset is
	// correct offset within the view and all destinations have valid offsets
	// in their views too
//	fmt.Printf("SBlock.Read %p offset %d count %d\n", b, offset, len(buf))
	sb := b.src
	return sb.Read(buf, offset - base - b.offset + sb.offset, 0)

}

func (b *SBlock) xform(src, dst []byte) {
	copy(dst, src)
}

func (b *SBlock) replicate(data []byte, offset, base int64) (err error) {
	// replicate the data to all destinations
	dbase := offset - base + b.offset
	doff := offset - b.offset
	for _, d := range b.dests {
		_, err := d.Write(data, doff + d.offset, dbase + d.offset, true, false)
		if err != nil {
			return err
		}
	}

	return nil
}

func (b *SBlock) Write(data []byte, offset, base int64, write, replicate bool) (n int64, err error) {
//	fmt.Printf("SBlock.Write %p r %v b.offset %d offset %d count %d\n", b, replicate, b.offset, offset, len(data))
	if write && b.view.repl != nil {
		n, err = b.view.Write(data, offset)
		if !replicate || err!=nil {
//			fmt.Printf("SBlock.Write %p return %d %v\n", b, n, err)
			return n, err
		}
	} else if !replicate {
//		fmt.Printf("SBlock.Write %p return %d %v\n", b, b.size, err)
		return b.size, nil
	}

	if !AsyncWrite {
		err = b.replicate(data, offset, base)
	} else {
		go b.replicate(data, offset, base)
	}

//	fmt.Printf("SBlock.Write %p return %d %v\n", b, len(data), err)
	return int64(len(data)), err
}

// Clones blk1 and connects it to blk2 recursively. Both blk1 and blk2 are destinations of b
func (b *SBlock) cloneConnect(blk1, blk2 Block, force bool) Block {
	b1 := blk1.(*SBlock)
	b2 := blk2.(*SBlock)
	nb := new(SBlock)
	*nb = *b1
	nb.clonee = b1
	nb.dests = nil

	if nb.view.readonly {
		force = false
	}

	fmt.Printf("SBlock.CloneConnect b1 view %s repl %p b2 view %s repl %p force %v\n", b1.view.Name, b1.view.repl, b2.view.Name, b2.view.repl, force)
	if b2.view.repl!=nil || force {
//		b2.AddDestination(nb)
		nb.AddDestination(b2)
	}

	if nb.view.repl == nil && b2.view == nb.view.dflt {
//		fmt.Printf("SBlock.CloneConnect %p source for %p\n", b2, nb)
		nb2 := (b.cloneConnect(blk2, blk1, true)).(*SBlock)
		nb.AddSource(nb2)
	}
		
//	fmt.Printf("SBlock.cloneConnect nb %p\n", nb)
	return nb
}

func (b *SBlock) ConnectDestinations() {
//	fmt.Printf("SBlock.ConnectDestinations %p view %s offset %v \n", b, b.view.Name, b.offset)
	for i, d1 := range b.dests {
		fmt.Printf("\td1 %p view %s offset %v\n", d1, d1.view.Name, d1.offset)
		if d1.view.repl == nil {
			// unmaterialized view
			fmt.Printf("SBlock.ConnectDestinations: unmaterialized block %p\n", d1)
			for _, d2 := range b.dests {
				if d2.view == d1.view.dflt {
//					fmt.Printf("SBlock.ConnectDestinations: %p source for %p\n", d2, d1)
					nd2 := d2
					if !d1.view.IsReadonly() {
						nd2 = (b.cloneConnect(d2, d1, false)).(*SBlock)
					}

					d1.AddSource(nd2)
					break
				}
			}
		} else if !d1.view.IsReadonly() {
			for j, d2 := range b.dests /*j := i + 1; j < len(b.dests); j++*/ {
//				d2 := b.dests[j]
				if i == j {
					continue
				}

				fmt.Printf("\t\td2 %p view %s repl %v offset %v\n", d2, d2.view.Name, d2.view.repl, d2.offset)
//				fmt.Printf("SBlock.ConnectDestinations d2 %p d2.view.repl %p\n", d2, d2.view.repl)
				if d2.view.repl!=nil {
					d1.AddDestination(d2)
				}
			}
		}
	}
}

/*
func (b *SBlock) Clone() Block {
	bb := new(SBlock)
	*bb = *b
	bb.dests = nil
	return bb
}
*/

func (b *ABlock) Offset() int64 {
	return b.offset
}

func (b *ABlock) Size() int64 {
	return b.size
}

func (b *ABlock) AddDestination(ab *ABlock, el Block, pe []PExpr) {
	ad := new(ADest)
	ad.arr = ab
	ad.el = el
	ad.expr = pe
	b.dests = append(b.dests, ad)
}

func (b *ABlock) AddSource(ab *ABlock, el Block, pe []PExpr) {
	as := new(ADest)
	as.arr = ab
	as.el = el
	as.expr = pe
	b.src = as
}

func (b *ABlock) Element() Block {
	return b.elblk
}

func (b *ABlock) isDest(d Block) bool {
	for _, dd := range b.dests {
		if dd.arr == d {
			return true
		}
	}

	return false
}

func (b *ABlock) Read(data []byte, offset, base int64) (int64, error) {
	if b.view.repl != nil {
		return b.view.Read(data, offset)
	}

	// Unmaterialized view
//	fmt.Printf("ABlock.Read %p offset %d count %d\n", b, offset, len(data))
	offset -= base
	esz := b.elblk.Size()
	eidx := int64(offset + int64(len(data))) / esz
	sidx := int64(offset) / esz
	soffset := sidx * esz
//	eoffset := eidx * esz
//	fmt.Printf("ABlock.Read sidx %d eidx %d soffset %d esz %d\n", sidx, eidx, soffset, esz)
	n := esz - (offset - soffset) + (eidx - sidx - 1) * esz
	data = data[0:n]	// finish at whole element
	elo := b.view.elo
	idx := make([]int64, len(b.dim))
	didx := make([]int64, len(b.dim))
	off := offset - b.Offset()
	d := b.src
	buf := make([]byte, d.arr.elsize)

	for int64(len(data)) >= esz {
		elo.ToIdx(sidx, idx, b.dim)
//		fmt.Printf("ABlock.Read: source index: %v\n", idx)

		// calculate the indices in the destination array
		for i := 0; i < len(idx); i++ {
			q, r := d.expr[i].calc(idx)
			if r!=0 {
				// if there is a remainder, the element doesn't
				// belong to the destination array
				return 0, errors.New("default view doesn't have the requested element")
			}

			didx[i] = q
		}

		// base offset for the destination element
		doff := d.arr.view.elo.FromIdx(didx, d.arr.dim) * d.arr.elsize
//		fmt.Printf("ABlock.Read: destination offset %d index: %v esz %d data %d %v\n", doff, didx, esz, len(data), data[0:esz])
//		fmt.Printf("ABlock.Read: d.el %v\n", d.el)
		n, err := d.el.Read(buf, d.arr.offset + doff, d.arr.offset + doff)
//		fmt.Printf("ABlock.Read: result %d %v\n", n, err)
		if err != nil {
			return 0, err
		}

		if n != int64(len(buf)) {
//			fmt.Printf("ABlock.Read: short write: got %d instead of %d\n", n, b.elsize)
			return 0, errors.New("short write")
		}

		d.el.xform(buf, data[0:esz])
		data = data[esz:]
		off += esz
		sidx++
	}

	return n, nil
}

func (b *ABlock) xform(src, dst []byte) {
	idx := make([]int64, len(b.dim))
	didx := make([]int64, len(b.dim))
	elo := b.view.elo
	d := b.dests[0]		// single destination
l1:	for n := int64(0); n < b.elnum; n++ {
		elo.ToIdx(n, idx, b.dim)
		for i := 0; i < len(idx); i++ {
			q, r := d.expr[i].calc(idx)
			if r != 0 {
				// if there is a remainder, the element doesn't
				// belong to the destination array
				continue l1
			}

			didx[i] = q
		}

		dn := d.arr.view.elo.FromIdx(didx, d.arr.dim)
		soff := n*b.elsize
		doff := dn*d.arr.elsize
		b.elblk.xform(src[soff:soff + b.elsize], dst[doff:doff+d.arr.elsize])
	}
}

func (b *ABlock) replicate(data []byte, offset, base int64) (err error) {
	// replicate the data to all destinations
	esz := b.elblk.Size()
	idx := make([]int64, len(b.dim))
	didx := make([]int64, len(b.dim))
	off := offset - base
	elo := b.view.elo
	o := off - (off/b.elsize)*b.elsize
	for int64(len(data)) >= esz {
		elo.ToIdx(off / b.elsize, idx, b.dim)
//		fmt.Printf("ABlock.Write: source index: %v\n", idx)

l1:		for _, d := range b.dests {
			// calculate the indices in the destination array
			for i := 0; i < len(idx); i++ {
				q, r := d.expr[i].calc(idx)
				if r!=0 {
					// if there is a remainder, the element doesn't
					// belong to the destination array
					goto l1
				}

				didx[i] = q
			}

			// base offset for the destination element
			doff := d.arr.view.elo.FromIdx(didx, d.arr.dim) * d.arr.elsize
//			fmt.Printf("ABlock.Write: destination offset %d index: %v esz %d data %d %v\n", doff, didx, esz, len(data), data[0:esz])
			err := d.el.replicate(data[0:esz], d.arr.offset + doff + o, d.arr.offset + doff)
			if err != nil {
				return err
			}

			if esz==0 {
				panic("zero element size")
			}

		}

		if int64(len(data)) < b.elsize {
			break
		}

		o = 0
		data = data[b.elsize:]
		off += b.elsize
	}

	return err
}

func (b *ABlock) Write(data []byte, offset, base int64, write, replicate bool) (n int64, err error) {
//	fmt.Printf("ABlock.Write %p offset %d count %d %v\n", b, offset, len(data), data)

	// make sure that we process only full elements
	esz := b.elblk.Size()
	enum := int64(len(data)) / esz
	data = data[0:enum*esz]

	if write && b.view.repl != nil {
		n, err = b.view.Write(data, offset)
		if !replicate || err!=nil {
//			fmt.Printf("ABlock.Write %p return %d %v\n", b, n, err)
			return n, err
		}
	} else if !replicate {
//		fmt.Printf("ABlock.Write %p return %d %v\n", b, b.size, err)
		return b.size, nil
	}


//	fmt.Printf("ABlock.Write %s:%p return %d\n", b.view.repl.Name, b, n)
	if !AsyncWrite {
		err = b.replicate(data, offset, base)
	} else {
		go b.replicate(data, offset, base)
	}

//	fmt.Printf("SBlock.Write %p return %d %v\n", b, len(data), err)
	return int64(len(data)), err
}

// Clones blk1 and connects it to blk2 recursively. Both blk1 and blk2 are destinations of b
func (b *ABlock) cloneConnect(blk1, blk2 Block, force bool) Block {
	b1 := blk1.(*ABlock)
	b2 := blk2.(*ABlock)
	nb := new(ABlock)
	*nb = *b1
	nb.clonee = b1
	nb.dests = nil
//	fmt.Printf("ABlock.CloneConnect b %p b1 %p b2 %p nb %p force %v\n", b, b1, b2, nb, force)

	if b2.view.readonly {
		force = false
	}

	// recursively connect the array elements
	el := b.elblk.cloneConnect(b1.elblk, b2.elblk, force)

	// find destinations b1 and b2 belong to
	var d1, d2 *ADest
	for _, d := range b.dests {
		if d.arr == b1 {
			d1 = d
		} else if d.arr == b2 {
			d2 = d
		}
	}

	// calculate the conversion expression
	ae := make([]PExpr, len(d1.expr))
	for n := 0; n < len(d1.expr); n++ {
		e1 := &ae[n]
		d1 := &d1.expr[n]
		d2 := &d2.expr[n]

		e1.A = d1.D*d2.A - d1.B*d2.C
		e1.B = d1.D*d2.B - d1.B*d2.D
		e1.C = d1.A*d2.C - d1.C*d2.A
		e1.D = d1.A*d2.D - d1.C*d2.B
	}

//	fmt.Printf("ABlock.CloneConnect b2.view.repl %p\n", b2.view.repl)
	if b2.view.repl!=nil || force {
		nb.AddDestination(b2, el, ae)
	}

	if nb.view.repl==nil && b2.view == nb.view.dflt {
//		fmt.Printf("ABlock.CloneConnect: %p source for %p\n", b2, nb)
		nb2 := b.cloneConnect(b2, nb, true).(*ABlock)
		nb.AddSource(nb2, el, ae)
	}

	return nb
}

func (b *ABlock) ConnectDestinations() {
//	fmt.Printf("ABlock.ConnectDestinations %p\n", b)

	for i, ad1 := range b.dests {
		v1 := ad1.arr.view

		for j := i + 1; j < len(b.dests); j++ {
			ad2 := b.dests[j]
			v2 := ad2.arr.view

			// calculate the conversion expression
			ae1 := make([]PExpr, len(ad1.expr))
			ae2 := make([]PExpr, len(ad1.expr))
			for n := 0; n < len(ad1.expr); n++ {
				e1 := &ae1[n]
				e2 := &ae2[n]
				d1 := &ad1.expr[n]
				d2 := &ad2.expr[n]
				
				e1.A = d1.D*d2.A - d1.B*d2.C
				e1.B = d1.D*d2.B - d1.B*d2.D
				e1.C = d1.A*d2.C - d1.C*d2.A
				e1.D = d1.A*d2.D - d1.C*d2.B
				e2.A = -e1.D
				e2.B = e1.B
				e2.C = e1.C
				e2.D = -e1.A
			}

//			fmt.Printf("ABlock.ConnectDestinations ad2.arr.view.repl %p\n", ad2.arr.view.repl)
			if v2.repl!=nil && !v1.IsReadonly() {
				el1 := b.elblk.cloneConnect(ad1.arr.elblk, ad2.arr.elblk, false)
				ad1.arr.AddDestination(ad2.arr, el1, ae1)
			} else if v2.dflt == v1 {
//				fmt.Printf("ABlock.ConnectDestinations %p source for %p\n", ad1.arr, ad2.arr)
				na1 := ad1.arr
				nel1 := ad1.arr.elblk

				if !v2.IsReadonly() {
					nel1 = b.elblk.cloneConnect(ad1.arr.elblk, ad2.arr.elblk, true)
					na1 = b.cloneConnect(ad1.arr, ad2.arr, true).(*ABlock)
				}

				ad2.arr.AddSource(na1, nel1, ae1)
			}

//			fmt.Printf("ABlock.ConnectDestinations ad1.arr.view.repl %p\n", ad1.arr.view.repl)
			if v1.repl!=nil && !v2.IsReadonly() {
				el2 := b.elblk.cloneConnect(ad2.arr.elblk, ad1.arr.elblk, false)
				ad2.arr.AddDestination(ad1.arr, el2, ae2)
			} else if v1.dflt == v2 {
//				fmt.Printf("ABlock.ConnectDestinations %p source for %p\n", ad2.arr, ad1.arr)
				nel2 := ad2.arr.elblk
				na2 := ad2.arr

				if !v1.IsReadonly() {
					nel2 = b.elblk.cloneConnect(ad2.arr.elblk, ad1.arr.elblk, true)
					na2 = b.cloneConnect(ad2.arr, ad1.arr, true).(*ABlock)
				}

				ad1.arr.AddSource(na2, nel2, ae2)
			}
		}
	}
}

func (b *ABlock) String() string {
	s := fmt.Sprintf("%06d ABlock %p elnum %d elsize %d dim %v elblk (%v) src (%v) dests:\n", b.offset, b, b.elnum, b.elsize, b.dim, b.elblk, b.src)
	for _, d := range b.dests {
		s += fmt.Sprintf("\t\t\t\t%v\n", d)
	}

	return s
}

func (b *TBlock) AddBlock(bb Block) {
	b.bs.blks = append(b.bs.blks, bb)
	b.size += bb.Size()
}

func (b *TBlock) Offset() int64 {
	return b.offset
}

func (b *TBlock) Size() int64 {
	return b.size
}

func (b *TBlock) isDest(d Block) bool {
	for _, bb := range b.dests {
		if bb == d {
			return true
		}
	}

	return false
}

func (b *TBlock) Read(buf []byte, offset, base int64) (int64, error) {
//	fmt.Printf("TBlock.Read %p offset %d count %d\n", b, offset, len(buf))
	if b.view.repl != nil {
		return b.view.Read(buf, offset)
	}

	// Unmaterialized view
	// The execution can reach this point only if the SBlock is at top level,
	// i.e. is not part of TBlock or ABlock. In that case the block offset is
	// correct offset within the view and all destinations have valid offsets
	// in their views too
	sb := b.src
	sbuf := make([]byte, sb.size)
	dbuf := make([]byte, b.size)

	n, err := b.src.Read(sbuf, sb.offset, sb.offset)
	if err!=nil {
		return 0, err
	}

	if n!=sb.size {
		return 0, errors.New("short read")
	}

	b.xform(sbuf, dbuf)
	return int64(copy(buf, dbuf[offset - base:])), nil
}

func (b *TBlock) xform(src, dst []byte) {
//	fmt.Printf("TBlock.xform b %p\n", b)
	n := int64(0)
	db := b.dests[0]
	for _, bb := range b.bs.blks {
		m := int64(0)

		// ugly, but will do for now
		for _, dbb := range db.bs.blks {
			if bb.isDest(dbb) {
		       	bb.xform(src[n:n+bb.Size()], dst[m:m+dbb.Size()])
				break
			}

			m += dbb.Size()
		}

		n += bb.Size()
	}
}

func (b *TBlock) replicate(data []byte, offset, base int64) (err error) {
//	eoffset := offset + int64(len(buf))
	count := int64(0)
//	fmt.Printf("TBlock.Write %s:%p offset %d count %d\n", b.view.repl.Name, b, offset, len(b))
	for _, bb := range b.bs.blks {
		err := bb.replicate(data[0:bb.Size()], offset, base)
		if err != nil {
			return err
		}

		n := bb.Size()
		offset += n
		base += n
		count += n
		data = data[n:]

		if len(data) == 0 {
			break
		}
	}

//	fmt.Printf("TBlock.Write %p exit %d\n", b, count)
	return nil
}

func (b *TBlock) Write(buf []byte, offset, base int64, write, replicate bool) (n int64, err error) {
//	fmt.Printf("TBlock.Write %p offset %d count %d\n", b, offset, len(buf))
	if write && b.view.repl != nil {
		n, err = b.view.Write(buf, offset)
		if !replicate || err != nil {
//			fmt.Printf("TBlock.Write %p return %d %v\n", b, n, err)
			return n, err
		}
	} else if !replicate {
//		fmt.Printf("TBlock.Write %p return %d %v\n", b, b.size, err)
		return b.size, nil
	}

	if !AsyncWrite {
		err = b.replicate(buf, offset, base)
	} else {
		go b.replicate(buf, offset, base)
	}

//	fmt.Printf("TBlock.Write %p return %d %v\n", b, len(buf), err)
	return int64(len(buf)), err
}

func (b *TBlock) Blocks() []Block {
	return b.bs.blks
}

func (b *TBlock) AddDestination(dst *TBlock) {
	b.dests = append(b.dests, dst)
}

func (b *TBlock) AddSource(src *TBlock) {
	b.src = src
}

/*
func (b *TBlock) Clone() Block {
	bb := new(TBlock)
	*bb = *b
	bb.dests = nil
	return bb
}
*/

// Clones blk1 and connects it to blk2 recursively. Both blk1 and blk2 are destinations of b
func (b *TBlock) cloneConnect(blk1, blk2 Block, force bool) Block {
	b1 := blk1.(*TBlock)
	b2 := blk2.(*TBlock)
	nb := new(TBlock)
	*nb = *b1
	nb.clonee = b1
	nb.dests = nil
	nb.bs.blks = make([]Block, len(b1.bs.blks))
	copy(nb.bs.blks, b1.bs.blks)
//	fmt.Printf("TBlock.CloneConnect b %p b1 %p b2 %p nb %p force %v\n", b, b1, b2, nb, force)
//	fmt.Printf("TBlock.cloneConnect blks before %v\n", nb.bs.blks)

//	fmt.Printf("b dests ")
//	for _, db := range b.dests {
//		fmt.Printf("%p ", db)
//	}
//	fmt.Printf("\n")

	if b2.view.readonly {
		force = false
	}

	connected := false
	for i, bb1 := range b1.bs.blks {
		var bb, bb2 Block

//		fmt.Printf("TBlock.CloneConnect bb1 %p %v\n", bb1, bb1)
		// find the sub-block where bb1 belongs to
		for _, tb := range b.bs.blks {
//			fmt.Printf("TBlock.CloneConnect: check bb %v\n", tb)
			if tb.isDest(bb1) {
				bb = tb
				break
			}
		}

		if bb==nil {
			panic("can't find bb")
		}

		// find the sub-block that should connect to bb1
		for _, tb := range b2.bs.blks {
			if bb.isDest(tb) {
				bb2 = tb
				break
			}
		}

		// if there is no connection for bb1, keep going
		if bb2 == nil {
			continue
		}

		connected = true
		nbb1 := bb.cloneConnect(bb1, bb2, force)
		nb.bs.blks[i] = nbb1
	}

	if connected {
		if b2.view.repl != nil || force {
			nb.AddDestination(b2)
		}

		if nb.view.repl == nil && nb.view.dflt == b2.view {
//			fmt.Printf("TBlock.CloneConnect %p source for %p\n", b2, nb)
			nb2 := b.cloneConnect(b2, nb, true).(*TBlock)
			nb.AddSource(nb2)
		}
	}

//	fmt.Printf("TBlock.CloneConnect nb.blks %v b1.blks %v exit\n", nb.bs.blks, b1.bs.blks)
//	fmt.Printf("TBlock.cloneConnect blks after %v\n", nb.bs.blks)
	return nb
}

func (b *TBlock) ConnectDestinations() {
//	fmt.Printf("TBlock.ConnectDestinations %p\n", b)

	for _, bb := range b.bs.blks {
		bb.ConnectDestinations()
	}

	for i, d1 := range b.dests {
		v1 := d1.view

		for j := i + 1; j < len(b.dests); j++ {
			d2 := b.dests[j]
			v2 := d2.view

//			fmt.Printf("TBlock.ConnectDestinations v1 %s readonly %v v2 %s readonly %v\n", v1.Name, v1.IsReadonly(), v2.Name, v2.IsReadonly())
			if v2.repl!=nil && !v1.IsReadonly() {
//				fmt.Printf("\t add destination d1 -> d2\n")
				d1.AddDestination(d2)
			} else if d2.view.dflt == d1.view {
//				fmt.Printf("TBlock.ConnectDestinations %p source for %p\n", d1, d2)
				d2.AddSource(d1)
			}

			if v1.repl!=nil && !v2.IsReadonly() {
//				fmt.Printf("\t add destination d2 -> d1\n")
				d2.AddDestination(d1)
			} else if d1.view.dflt == d2.view {
//				fmt.Printf("TBlock.ConnectDestinations %p source for %p\n", d2, d1)
				d1.AddSource(d2)
			}
		}
	}
}

func (b *TBlock) String() string {
	bs := ""
	for _, bb := range b.bs.blks {
		bs += fmt.Sprintf("%p ", bb)
	}

	return fmt.Sprintf("%06d TBlock %p src %p bs %s", b.offset, b, b.src, bs)
}

func (d *ADest) String() string {
	return fmt.Sprintf("(pexpr %v arr %s:%d el <%v>)", d.expr, d.arr.view.repl.Name, d.arr.offset, d.el)
}

// returns slice with all blocks that describe data in the specified interval
func (v *View) Search(offset, count int64) []Block {

//	fmt.Printf("View.Search offset %d count %d\n", offset, count)
	blks := v.bs.blks
	start := offset
	end := offset + count

	// find the first block
	n := 0
	b := 0
	t := len(blks)
	for b < t {
		n = (b + t) / 2
		o := blks[n].Offset()
//		fmt.Printf("View.Search b %d t %d n %d o %d\n", b, t, n, o)
		if o == start {
			b = n + 1
			break
		} else if o < start {
			b = n + 1
		} else {
			t = n
		}
	}

	if b > 0 {
		b--
	}

//	fmt.Printf("View.Search b %d\n", b)
	// find the last block
	for i := b; i < len(blks); i++ {
		if blks[i].Offset() > end {
			return blks[b:i]
		}
	}

	if b > len(blks) {
		return nil
	}

	return blks[b:]
}
