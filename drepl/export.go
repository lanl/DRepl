package drepl

// import "fmt"

// Header
// 	nrepl	int32
// 	nview	int32
// 	nblk	int32
// 
// Replica
// 	id	int32
// 	name	string
// 	fname	string
// 
// View
// 	id	int32
// 	name	string
//	flags	int32		// bit 0: 1 == async
// 	replid	int32
// 	offset	int64
// 	elo	int32
// 	dfltid	int32
// 	nblks	int32
// 	blkids	*int32
// 
// Expr
// 	a	int64
// 	b	int64
// 	c	int64
// 	d	int64
// 	idx	int32
// 
// Dest
// 	nexpr	int32
// 	expr	*Expr
// 	arrid	int32
// 	elid	int32
// 
// Block
// 	id	int32
// 	viewid	int32
// 	offset	int64
// 	size	int64
// 	src	Dest
// 	ndest	int32
// 	dests	*Dest
// 
// 	// ablock
// 	ndim	int32
// 	dim	*int64
// 	elsize	int64
// 	elnum	int64
// 	elid	int32
// 
// 	// tblock
// 	nfld	int32
// 	fldid	*int32

type Exporter struct {
	repls	map[*Replica]int32
	views	map[*View]int32
}

func NewExporter() *Exporter {
	e := new(Exporter)
	e.repls = make(map[*Replica]int32)
	e.views = make(map[*View]int32)

	return e
}

func (e *Exporter) AddReplica(r *Replica) {
	id := e.repls[r]
	if id > 0 {
		return
	}

	id = int32(len(e.repls) + 1)
	e.repls[r] = id
}

func (e *Exporter) AddView(v *View) {
	id := e.views[v]
	if id > 0 {
		return
	}

	id = int32(len(e.views) + 1)
	e.views[v] = id
}

func (e *Exporter) Data(flags uint32) []byte {

	// first get list of all blocks
	blks := make(map[Block]int32)
	for v, _ := range e.views {
		for _, b := range v.bs.blks {
			b.export1(blks)
		}
	}

	// header
	p := pint32(nil, int32(len(e.repls)))
	p = pint32(p, int32(len(e.views)))
	p = pint32(p, int32(len(blks)))

	// replicas
	for r, id := range e.repls {
		p = pint32(p, id)
		p = pstr(p, r.Name)
		p = pstr(p, r.FileName)
	}

	// views
	for v, id := range e.views {
		p = pint32(p, id)
		p = pstr(p, v.Name)
		p = pint32(p, int32(flags))
		p = pint32(p, e.repls[v.repl])
		p = pint64(p, v.offset)
		p = pint32(p, v.elo.Id())
		p = pint32(p, e.views[v.dflt])
		p = pint32(p, int32(len(v.bs.blks)))
		for _, b := range v.bs.blks {
			p = pint32(p, blks[b])
		}
	}

	// blocks
	visited := make(map[Block]bool)
	for b, _ := range blks {
		p = b.export2(p, blks, e.views, visited)
	}

	return p
}

func (b *SBlock) export1(blks map[Block]int32) {
	if b==nil {
		return
	}

	id := blks[b]
	if id>0 {
		return
	}

	id = int32(len(blks) + 1)
	blks[b] = id
	if b.src!=nil {
		b.src.export1(blks)
	}

	for _, bb := range b.dests {
		bb.export1(blks)
	}
}

func (b *ABlock) export1(blks map[Block]int32) {
	if b==nil {
		return
	}

	id := blks[b]
	if id>0 {
		return
	}

	id = int32(len(blks) + 1)
	blks[b] = id
	b.elblk.export1(blks)
	if b.src!=nil {
		b.src.arr.export1(blks)
		b.src.el.export1(blks)
	}

	for _, d := range b.dests {
		d.arr.export1(blks)
		d.el.export1(blks)
	}
}

func (b *TBlock) export1(blks map[Block]int32) {
	if b==nil {
		return
	}

	id := blks[b]
	if id>0 {
		return
	}

	id = int32(len(blks) + 1)
	blks[b] = id
	for _, bb := range b.bs.blks {
		bb.export1(blks)
	}

	if b.src!=nil {
		b.src.export1(blks)
	}

	for _, bb := range b.dests {
		bb.export1(blks)
	}
}

func (b *SBlock) export2(data []byte, blks map[Block]int32, views map[*View]int32, visited map[Block]bool) []byte {
	if b==nil {
		return data
	}

	if visited[b] {
		return data
	}

	visited[b] = true
	id := blks[b]
	p := pint32(data, id)
	p = pint32(p, views[b.view])
	p = pint64(p, b.offset)
	p = pint64(p, b.size)

	// src
	p = pint32(p, 0)		// nexpr
	p = pblk(p, blks, b.src)	// arrid
	p = pblk(p, blks, nil)		// elid


	p = pint32(p, int32(len(b.dests)))
	for _, bb := range b.dests {
		p = pint32(p, 0)	// nexpr
		p = pblk(p, blks, bb)	// arrid
		p = pblk(p, blks, nil)	// elid
	}

	// empty ablock
	p = pint32(p, 0)	// ndim
	p = pint64(p, 0)	// elsize
	p = pint64(p, 0)	// elnum
	p = pint32(p, 0)	// elid

	// empty tblock
	p = pint32(p, 0)

//	fmt.Printf("\tblock %d offset %v\n", blks[b] - 1, b.offset);
//	for _, bb := range b.dests {
//		fmt.Printf("\t\tdest arr %d el -1\n", blks[bb] - 1);
//	}

	return p
}

func (b *ABlock) export2(data []byte, blks map[Block]int32, views map[*View]int32, visited map[Block]bool) []byte {
	if b==nil {
		return data
	}

	if visited[b] {
		return data
	}

	visited[b] = true
	id := blks[b]
	p := pint32(data, id)
	p = pint32(p, views[b.view])
	p = pint64(p, b.offset)
	p = pint64(p, b.size)

	// src
	if b.src != nil {
		p = pint32(p, int32(len(b.src.expr)))	// nexpr
		for i := 0; i < len(b.src.expr); i++ {
			e := &b.src.expr[i]
			p = pint64(p, e.A)
			p = pint64(p, e.B)
			p = pint64(p, e.C)
			p = pint64(p, e.D)
			p = pint32(p, int32(e.Xidx))
		}

		p = pblk(p, blks, b.src.arr)	// arrid
		p = pblk(p, blks, b.src.el)	// elid
	} else {
		p = pint32(p, 0)
		p = pblk(p, blks, nil)
		p = pblk(p, blks, nil)
	}

	p = pint32(p, int32(len(b.dests)))
	for _, d := range b.dests {
		p = pint32(p, int32(len(d.expr)))	// nexpr
		for i := 0; i < len(d.expr); i++ {
			e := &d.expr[i]
			p = pint64(p, e.A)
			p = pint64(p, e.B)
			p = pint64(p, e.C)
			p = pint64(p, e.D)
			p = pint32(p, int32(e.Xidx))
		}

		p = pblk(p, blks, d.arr)	// arrid
		p = pblk(p, blks, d.el)		// elid
	}

	// ablock
	p = pint32(p, int32(len(b.dim)))
	for _, d := range b.dim {
		p = pint64(p, d)
	}

	p = pint64(p, b.elsize)
	p = pint64(p, b.elnum)
	p = pblk(p, blks, b.elblk)

	// empty tblock
	p = pint32(p, 0)

//	fmt.Printf("\tblock %d view %d offset %v\n", blks[b] - 1, views[b.view] - 1, b.offset);
//	fmt.Printf("\t\telement id %d\n", blks[b.elblk] - 1);
//	for _, d := range b.dests {
//		arr := d.arr
//		el := d.el
//		fmt.Printf("\t\tdest arr %d el %d\n", blks[arr] - 1, blks[el] - 1);
//	}

	return p
}

func (b *TBlock) export2(data []byte, blks map[Block]int32, views map[*View]int32, visited map[Block]bool) []byte {
	if b==nil {
		return data
	}

	if visited[b] {
		return data
	}

	visited[b] = true
	id := blks[b]
	p := pint32(data, id)
	p = pint32(p, views[b.view])
	p = pint64(p, b.offset)
	p = pint64(p, b.size)

	// src
	p = pint32(p, 0)	// nexpr
	p = pblk(p, blks, b.src)// arrid
	p = pblk(p, blks, nil)	// elid


	p = pint32(p, int32(len(b.dests)))
	for _, bb := range b.dests {
		p = pint32(p, 0)	// nexpr
		p = pblk(p, blks, bb)	// arrid
		p = pblk(p, blks, nil)	// elid
	}

	// empty ablock
	p = pint32(p, 0)	// ndim
	p = pint64(p, 0)	// elsize
	p = pint64(p, 0)	// elnum
	p = pint32(p, 0)	// elid

	p = pint32(p, int32(len(b.bs.blks)))
	for _, bb := range b.bs.blks {
		p = pblk(p, blks, bb)
	}

//	fmt.Printf("\tblock %d offset %v\n", blks[b] - 1, b.offset);
//	for _, bb := range b.bs.blks {
//		fmt.Printf("\t\tfield id %d\n", blks[bb] - 1);
//	}
//	for _, bb := range b.dests {
//		fmt.Printf("\t\tdest arr %d el -1\n", blks[bb] - 1);
//	}

	return p
}


func pint32(buf []byte, val int32) []byte {
	buf = append(buf, uint8(val))
	buf = append(buf, uint8(val>>8))
	buf = append(buf, uint8(val>>16))
	buf = append(buf, uint8(val>>24))

	return buf
}

func pint64(buf []byte, val int64) []byte {
	buf = append(buf, uint8(val))
	buf = append(buf, uint8(val>>8))
	buf = append(buf, uint8(val>>16))
	buf = append(buf, uint8(val>>24))
	buf = append(buf, uint8(val>>32))
	buf = append(buf, uint8(val>>40))
	buf = append(buf, uint8(val>>48))
	buf = append(buf, uint8(val>>56))

	return buf
}

func pstr(buf []byte, val string) []byte {
	buf = pint32(buf, int32(len(val)))
	for i:=0; i < len(val); i++ {
		buf = append(buf, val[i])
	}

	return buf
}

func pblk(buf []byte, blks map[Block]int32, b Block) []byte {
	id := blks[b]
//	if id == 0 && b != nil {
//		fmt.Printf("Error: block %p not in blks\n", b)
//	}

	return pint32(buf, id)
}
