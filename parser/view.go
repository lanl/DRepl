package parser

import (
	"fmt"
	"drepl/drepl"
)

/* const (
	// view flags
	Vrowmajor,				// multi-dimensional arrays are row-major
	Vcolmajor,				// multi-dimensional arrays are column-major
	Vhilbert,				// multi-dimensional array elements are stored in hilbert-curve order
	Vzorder,				// multi-dimensional array elements are stored in Z curve order
)*/

const (
	Vrowmajor = iota
	Vrowminor
	Vreadonly = 0x40
	Vdefault = 0x80				// default view to use to read unmaterialized views
)

type VType struct {
	name	string
	dt	*Type		// corresponding dataset type (if any)
	temps	map[string] *VTemp// temporary variables used to define a slice

	// matrix
	etype	*VType		// element type
	dim	[]*Expr		// size for each dimension, can only use constants and temps

	// struct
	fields	[]*VField

	sz	int64		// size of the type
	vdim	[]VDim		// description of the slice after process is called
	vidx	[]EVar		// used while processing expressions, variable for each dimension

	pos	Pos		// position where defined
}

// used in slice definitions
type VTemp struct {
	EVar
	expr	*Expr		// transformed expression
	ep	**Expr		// the right-side part in expr
	ot	*VTemp		// the corresponding temp (same name) on the other side of the definition
	idx	int		// dimension where the temp is used
	min	int64
	max	int64
	pos	Pos
}

type VField struct {
	name	string
	vt	*VType
	f	*Field		// corresponding field in the dataset
	pos	Pos
}

type VDim struct {
	min	int64
	max	int64

	// left side
	lt	*VTemp		// temp used in the expression
	le	*Expr		// expression transformed to lt
	lep	**Expr		// pointer into le

	// right side
	rt	*VTemp		// temp used in the expression
	re	*Expr		// expression transformed to rt
	rep	**Expr		// pointer into re

	// index conversion
	v2d	*Expr		// expression that uses vidx and calculates the data index
	d2v	*Expr		// expression that uses didx and calculates the view index
	pv2d	drepl.PExpr	// same as v2d, but as PExpr
	pd2v	drepl.PExpr	// same as d2v, but as PExpr
}

type VVarDecl struct {
	name	string
	
	v	*VarDecl	// original variable from the dataset
	lt, rt	*VType		// left and right type definitions
	dim	[]VDim		// dimension stuff
	blk	drepl.Block

	pos	Pos		// position where defined
}

type View struct {
	Name	string
	flags	int
	vars	[] *VVarDecl
	types	map[string] *VType
	vmap	map[string] *VVarDecl
	dv	*drepl.View
}

func NewView(name string, flags int) *View {
	v := new(View)
	v.Name = name
	v.flags = flags
	v.types = make(map[string] *VType)
	v.vmap = make(map[string] *VVarDecl)

	return v
}

func (vw *View) setPos(p, np *Pos) {
	if np!=nil {
		*p = *np
	} else {
		p.fname = ""
	}
}

func (vw *View) createType(name string, pos *Pos) (*VType, bool) {
	if name!="" {
		if t:=vw.types[name]; t!=nil {
			return t, false
		}
	}

	t := new(VType)
	t.name = name
	t.temps = make(map[string] *VTemp)

	vw.setPos(&t.pos, pos)
	if name != "" {
//		fmt.Printf("add vtype %s\n", name)
		vw.types[name] = t
	}

	return t, true
}

func (vw *View) addType(name string, t *VType) bool {
	if vw.types[name] != nil {
		return false
	}

	t.name = name
	vw.types[name] = t
	return true
}

func (vw *View) getType(name string) (*VType) {
	return vw.types[name]
}

func (vw *View) createVarDecl(name string, pos *Pos) (*VVarDecl, bool) {
	if v:=vw.vmap[name]; v!=nil {
		return v, false
	}

	v := new(VVarDecl)
	v.name = name
	vw.setPos(&v.pos, pos)
	vw.vmap[name] = v
	vw.vars = append(vw.vars, v)

	return v, true
}

func (vw *View) getVarDecl(name string) (*VVarDecl) {
	return vw.vmap[name]
}

func  (t *VType) getField(name string) *VField {
	for i := 0; i < len(t.fields); i++ {
		if name == t.fields[i].name {
			return t.fields[i]
		}
	}

	return nil
}

func (t *VType) addFields(names []string, ftype *VType, pos *Pos) bool {
	fs := make([]*VField, len(t.fields) + len(names))
	copy(fs, t.fields)
	for i, n:=0, len(t.fields); i<len(names); i++ {
		if t.getField(names[i]) != nil {
			return false
		}

		f := new(VField)
		fs[i+n] = f
		f.name = names[i]
		f.pos = *pos
		f.vt = ftype
	}

	t.fields = fs
	return true
}

// true if the type is defined
func (t *VType) isDefined() bool {
	return t.pos.fname != ""
}

func (t *VType) createTemp(name string, pos *Pos) (*VTemp, bool) {
	tmp := t.temps[name]
	if tmp!=nil {
		return tmp, false
	}

	tmp = new(VTemp)
	tmp.name = name
	tmp.pos = *pos
	tmp.aux = tmp
	t.temps[name] = tmp

	return tmp, true
}

func processVArray(lt, rt *VType, dt *Type) (err string) {
	var vt, retype *VType

//	fmt.Printf("processVArray lt %p rt %p dt %p\n", lt, rt, dt)
	if rt!=nil && lt!=nil && len(rt.dim) != len(lt.dim) {
		return "left and right types don't match"
	}

	if dt.dimnum != len(lt.dim) {
//		for i, e := range lt.dim {
//			fmt.Printf("%d %v\n", i, e)
//		}

		return fmt.Sprintf("view and data types don't match: vdim %d, dim %d", len(lt.dim), dt.dimnum)
	}

	if rt!=nil {
		retype = rt.etype
	}

	vt, err = processVType(lt.etype, retype, dt.etype)
	if err!="" {
		return err
	}

	lt.etype = vt
	dim := make([]VDim, len(rt.dim))
	vidx := make([]EVar, len(rt.dim))
	for i := 0; i < len(dim); i++ {
		// first transform the expressions on the right side so we can find the ranges for the temps
		var n interface{}

		vidx[i].name = fmt.Sprintf("v%d", i)

		ev, ne, ep, err := rt.dim[i].transform()
		if err != "" {
			return err
		}

		if ev==nil {
			// constant or empty expression
			dim[i].min = 0
			if rt.dim[i] == nil {
				if lt!=nil && lt.dim[i]!=nil {
					return fmt.Sprintf("empty dimension on the right side should correspond to empty expression on the left side: lt %p", lt)
				}

				ltmp := new(VTemp)
				rtmp := new(VTemp)
				ltmp.ot = rtmp
				rtmp.ot = ltmp
				ltmp.min = 0
				ltmp.max = int64(dt.dim[i])
				rtmp.min = ltmp.min
				rtmp.max = ltmp.max
				ltmp.idx = i
				rtmp.idx = i
				dim[i].max = ltmp.max

				dim[i].re = nil
				dim[i].rt = rtmp
				dim[i].rep = nil

				lt.dim[i] = VarExpr(&ltmp.EVar)
				rt.dim[i] = VarExpr(&rtmp.EVar)

				dim[i].le = nil				
				dim[i].lt = ltmp
				dim[i].lep = nil
			} else {
				dim[i].max = 1
			}

			continue
		}

		t, ok := ev.aux.(*VTemp)
		if !ok {
			return "expression variable not a temp"
		}

		dim[i].re = ne
		dim[i].rep = ep
		dim[i].rt = t

		t.expr = ne
		t.ep = ep
		t.idx = i
		if ep!=nil {
			re := new(Expr)
			re.val = int64(0)
			*ep = re
			n, err = ne.eval()
			if err != "" {
				return err
			}

			if nn, ok :=n.(int64); !ok {
				return fmt.Sprintf("expected int64, got: %v", n)
			} else {
				t.min = nn
			}

			re.val = int64(dt.dim[i])
			n, err = ne.eval()
			if err != "" {
				return err
			}

			*ep = nil
			if nn, ok :=n.(int64); !ok {
				return fmt.Sprintf("expected int64, got: %v", n)
			} else {
				t.max = nn
			}
		} else {
			t.min = 0
			t.max = int64(dt.dim[i])
		}

		if t.min > t.max {
			n := t.min
			t.min = t.max
			t.max = n
		}

		if lt==nil || lt.dim[i]==nil {
			return "left index expression empty while right isn't"
		}

		// temp vars in the left and right types are different
		ltmp := lt.temps[t.name]
		if ltmp != nil {
			// connect both temps
			t.ot = ltmp
			ltmp.ot = t
			ltmp.min = t.min
			ltmp.max = t.max
		}

		// transform the expression on the left side
		ev, ne, ep, err = lt.dim[i].transform()
		if err != "" {
			return err
		}

		if ev==nil {
			// constant expression
			dim[i].min = 0
			dim[i].max = 1
			continue
		}

		t, ok = ev.aux.(*VTemp)
		if !ok {
			return "expression variable not a temp"
		}

		dim[i].le = ne
		dim[i].lep = ep
		dim[i].lt = t
		t.expr = ne
		t.ep = ep
		t.idx = i
	}

	for i := 0; i < len(dim); i++ {
		d := &dim[i]

		// evaluate the min and max values for the dimension, using
		// the min and the max values for the temp (FIX)
		t := d.lt

		var nn, xx int64
		
		if lt!=nil && lt.dim[i]!=nil {
			t.val = t.min
			n, err := lt.dim[i].eval()
			if err != "" {
				return err
			}

			var x interface{}
			t.val = t.max
			x, err = lt.dim[i].eval()
			if err != "" {
				return err
			}

			t.val = nil

			nn = n.(int64)
			xx = x.(int64)
		} else {
			nn = 0
			xx = int64(dt.dim[i])
		}

		if nn > xx {
			tt := nn
			nn = xx
			xx = tt
		}

		if nn<0 {
			nn = 0
		}

		d.min = nn
		d.max = xx

		// create the conversion expressions
		ve := new(Expr)
		ridx := d.lt.ot.idx
		ve.val = &dt.vidx[ridx]
		e, ep := dim[ridx].rt.expr.clone(dim[ridx].rt.ep)
		if ep!=nil {
			*ep = ve
		} else {
			e = ve
		}

		d.d2v, _ = lt.dim[i].clone(nil)
		d.d2v, _ = d.d2v.replace(&d.lt.EVar, e)

		ve = new(Expr)
		lidx := d.rt.ot.idx
		ve.val = &vidx[lidx]
		e, ep = dim[lidx].lt.expr.clone(dim[lidx].lt.ep)
		if ep!=nil {
			*ep = ve
		} else {
			e = ve
		}

		d.v2d, _ = rt.dim[i].clone(nil)
		d.v2d, _ = d.v2d.replace(&d.rt.EVar, e)

		if !d.d2v.toPExpr(&d.pd2v, dt.vidx) {
			return fmt.Sprintf("can't convert %v to PExpr\n", d.d2v)
		}

		if !d.v2d.toPExpr(&d.pv2d, vidx) {
			return fmt.Sprintf("can't convert %v to PExpr\n", d.v2d)
		}
	}

/*
	fmt.Printf("view var %s[", v.name)
	for i := 0; i < len(dim); i++ {
		fmt.Printf("%d,", dim[i].max - dim[i].min)
	}
	fmt.Printf("]\n")

	for i := 0; i < len(dim); i++ {
		fmt.Printf("%d d2v %v\n", i, dim[i].d2v)
		fmt.Printf("%d v2d %v\n", i, dim[i].v2d)
	}
*/

	sz := int64(1)
	for i := 0; i < len(dim); i++ {
		sz *= (dim[i].max - dim[i].min)
	}

	lt.sz = sz * lt.etype.sz
	lt.vdim = dim
	lt.vidx = vidx

	return err
}

func processVStruct(lt, rt *VType, dt *Type) (err string) {
	var vt *VType

	if rt!=nil {
		panic("substruct error")
	}

	if dt.fields==nil || len(dt.fields)==0 {
		return fmt.Sprintf("data type needs to be struct: %v", dt.etype)
	}

	sz := int64(0)
	for _, vf := range(lt.fields) {
		var f *Field

		s := vf.name
		for _, df := range(dt.fields) {
			if s == df.name {
				f = &df
				break
			}
		}

		if f==nil {
			return fmt.Sprintf("field '%s' not found", s)
		}

		vf.f = f
		vt, err = processVType(vf.vt, nil, f.t)
		if err!="" {
			return ""
		}

		vf.vt = vt
		sz += vf.vt.sz
	}

	lt.sz = sz
	return ""
}

func processVType(lt, rt *VType, dt *Type) (vt *VType, err string) {
	vt = lt
	if dt!=nil {
		for dt.dimnum==0 && dt.etype!=nil {
			// handle type aliases
			dt = dt.etype
		}
	}

//	fmt.Printf("processVType lt %p rt %p dt %v\n", lt, rt, dt)
	if lt==nil {
		if rt==nil {
			lt = new(VType)
			lt.dt = dt
			lt.sz = dt.size
			if dt.etype != nil {
				lt.dim = make([]*Expr, dt.dimnum)
				rt = new(VType)
				rt.dim = make([]*Expr, dt.dimnum)
				err = processVArray(lt, rt, dt)
			} else if dt.fields != nil {
				fnames := make([]string, len(dt.fields))
				for i, f := range dt.fields {
					fnames[i] = f.name
				}

				lt.addFields(fnames, nil, &lt.pos)
				err = processVStruct(lt, rt, dt)
			}

			return lt, err
		}

		lt = rt
		rt = nil
	}

	if lt.dim!=nil {
		err = processVArray(lt, rt, dt)
	} else if lt.fields!=nil && len(lt.fields) > 0 || dt.fields!=nil {
		if lt.fields==nil && dt.fields != nil {
			fnames := make([]string, len(dt.fields))
			for i, f := range dt.fields {
				fnames[i] = f.name
			}

			lt.addFields(fnames, nil, &lt.pos)
		}

		err = processVStruct(lt, rt, dt)
	} else if dt != nil {
		lt.sz = dt.size
	}

//	fmt.Printf("--- processVType: lt %v\n", lt)
	return vt, err
}

func (t *VType) createBlocks(bs *drepl.BlockSeq, dblk drepl.Block) (b drepl.Block, err string) {
//	fmt.Printf("VType.createBlocks (%v) dblk (%v)\n", t, dblk)
	if t.etype!=nil {
		dab := dblk.(*drepl.ABlock)
		dim := make([]int64, len(t.vdim))
		for i := 0; i < len(dim); i++ {
			dim[i] = t.vdim[i].max - t.vdim[i].min
		}

		nbs := bs.View().NewBlockSeq()
		elblk := dab.Element()
		_, err = t.etype.createBlocks(nbs, elblk)
		if err != "" {
			return
		}

		nblks := nbs.Blocks()
		if len(nblks) != 1 {
			return nil, "internal error"
		}

		ab := bs.NewABlock(t.etype.sz, dim, nblks[0])
		v2d := make([]drepl.PExpr, len(t.vdim))
		for i, d := range t.vdim {
			v2d[i] = d.pv2d
		}

		dab.AddDestination(ab, nil, v2d)
		b = ab
	} else if t.fields != nil {
		dtb := dblk.(*drepl.TBlock)
		nbs := bs.View().NewBlockSeq()
		dblks := dtb.Blocks()
		for _, f := range t.fields {
			db := dblks[f.f.idx]
			_, err = f.vt.createBlocks(nbs, db)
			if err != "" {
				return
			}
		}

		tb := bs.NewTBlock(nbs)
		dtb.AddDestination(tb)
		b = tb
	} else {
		db := dblk.(*drepl.SBlock)
		sb := bs.NewSBlock(t.sz)
		db.AddDestination(sb)
		b = sb
	}

	return
}

func (t *VType) String() string {
	if t.etype != nil {
		return fmt.Sprintf("'%s' dt |%v| etype %v dim %v", t.name, t.dt, t.etype, t.dim)
	} else if t.fields != nil {
		return fmt.Sprintf("'%s' dt |%v| |%v|", t.name, t.dt, t.fields)
	}

	return fmt.Sprintf("'%s' dt |%v|", t.name, t.dt)
}

func (f *VField) String() string {
	return fmt.Sprintf("(%s %v)", f.name, f.vt)
}

func (v *VVarDecl) process() (err string) {
	vt, err := processVType(v.lt, v.rt, v.v.t)
	if err!="" {
		return err
	}

	v.lt = vt
	v.rt = nil

//	fmt.Printf("view var %s sz %d\n", v.name, v.lt.sz)
	return ""
}

func (v *VVarDecl) createBlocks(dv *drepl.View) (err string) {
	var b drepl.Block

	b, err = v.lt.createBlocks(dv.Blocks(), v.v.blk)
	v.blk = b
	return
}

func (v *View) process() (err string) {
	for _, v := range v.vars {
		err = v.process()
		if err!="" {
			return err
		}
	}

	return ""
}

func (v *View) createBlocks(vv *drepl.View) (err string) {
	v.dv = vv
	for _, vvar := range v.vars {
		err = vvar.createBlocks(v.dv)
		if err != "" {
			return err
		}
	}

	return ""
}

func (v *View) GetBlocks() (bs []drepl.Block) {
	return v.dv.Blocks().Blocks()
}

func (vt *VType) reset() {
	if vt == nil || vt.vdim == nil {
		return
	}

	for _, vdm := range vt.vdim {
		vdm.reset()
	}
}

func (vdm *VDim) reset() {
	if vdm != nil {
//		vdm.pv2d = nil
//		vdm.pd2v = nil
	}
}

func (vv *VVarDecl) reset() {
	vv.lt.reset()
	vv.rt.reset()
	vv.blk = nil
}

// resets all drepl.* fields that are related to creating the transformation rules
func (v *View) reset() {
	for _, vt := range v.types {
		vt.reset()
	}

	for _, vv := range v.vars {
		vv.reset()
	}

	v.dv = nil
}
