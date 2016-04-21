package parser

import (
//	"errors"
	"fmt"
	"strconv"
	"strings"
	"drepl/drepl"
)

type Type struct {
	ds	*Dataset
	name    string // can be ""
	primary bool   // true if primary type

	// matrix
	etype   *Type   // element type
	dimnum  int     // number of dimensions
	dimexpr []*Expr // size for each dimension, nil -- unlimited
	dim	[]int
	vidx	[]EVar	// used while processing the conversion expressions, variable for each dimension

	// struct
	fields	[]Field

	pos	Pos // position where defined

	size	int64	// size of an instance of the type (<0 if variable)
}

type VarDecl struct {
	name	string
	pos	Pos // position where defined
	t	*Type
//	dest	Destination

	blk	drepl.Block
}

type Field struct {
	name	string
	idx	int	// index in struct definition
	t	*Type
	offset	int64	// offset from the beginning of the structure
	pos	Pos
}

type ConstDecl struct {
	EVar
	pos  Pos // position where defined
}

type Dataset struct {
	types  map[string]*Type
	vars   map[string]*VarDecl
	consts map[string]*ConstDecl

	v	*drepl.View
	repl	*drepl.Replica
}

func (ds *Dataset) setPos(p, np *Pos) {
	if np != nil {
		*p = *np
	} else {
		p.fname = ""
	}
}

func (ds *Dataset) createConstDecl(name string, pos *Pos) (*ConstDecl, bool) {
	if c := ds.consts[name]; c != nil {
		return c, false
	}

	c := new(ConstDecl)
	c.name = name
	c.aux = c
	ds.setPos(&c.pos, pos)
	ds.consts[name] = c
	return c, true
}

func (ds *Dataset) getConstDecl(name string) *ConstDecl {
	return ds.consts[name]
}

func (ds *Dataset) createType(name string, pos *Pos) (*Type, bool) {
	if name != "" {
		if td := ds.types[name]; td != nil {
			return td, false
		}
	}

	td := new(Type)
	td.ds = ds
	td.name = name
	ds.setPos(&td.pos, pos)

	if strings.HasPrefix(name, "string") {
		n, err := strconv.Atoi(name[6:])
		if err != nil {
			return nil, false
		}

		td.etype = ds.types["int8"]
//		td.primary = true
		td.dimnum = 1
		td.dimexpr = make([]*Expr, 1)
		td.dimexpr[0] = new(Expr)
		td.dimexpr[0].val = int64(n+1)
	}

	if name != "" {
		ds.types[name] = td
	}

	return td, true
}

func (ds *Dataset) getType(name string) *Type {
	t := ds.types[name]
	if t != nil {
		return t
	}

	if strings.HasPrefix(name, "string") {
		n, err := strconv.Atoi(name[6:])
		if err != nil {
			return nil
		}

		t = ds.createPrimaryType(name, int64(n+1))
		t.dimnum = 1
		t.dimexpr = make([]*Expr, 1)
		t.dimexpr[0] = new(Expr)
		t.dimexpr[0].val = int64(n+1)
	}

	return t
}

func (ds *Dataset) createVarDecl(name string, pos *Pos) (*VarDecl, bool) {
	if vd := ds.vars[name]; vd != nil {
		return vd, false
	}

	vd := new(VarDecl)
	vd.name = name
	ds.setPos(&vd.pos, pos)
	ds.vars[name] = vd

	return vd, true
}

func (ds *Dataset) getVarDecl(name string) *VarDecl {
	return ds.vars[name]
}

func (t *Type) getField(name string) *Field {
	for i := 0; i < len(t.fields); i++ {
		if name == t.fields[i].name {
			return &t.fields[i]
		}
	}

	return nil
}

func (t *Type) addFields(names []string, ftype *Type, pos *Pos) bool {
	fs := make([]Field, len(t.fields)+len(names))
	copy(fs, t.fields)
	for i, n := 0, len(t.fields); i < len(names); i++ {
		if t.getField(names[i]) != nil {
			return false
		}

		f := &fs[i+n]
		f.name = names[i]
		f.idx = i+n
		f.pos = *pos
		f.t = ftype
	}

	t.fields = fs
	return true
}

// true if the type is defined
func (t *Type) isDefined() bool {
	return t.pos.fname != ""
}

func (t *Type) isArray() bool {
	return t.etype != nil && t.dimnum > 0
}

func (t *Type) isStaticArray() bool {
	if !t.isArray() {
		return false
	}

	for _, de := range t.dimexpr {
		if de == nil {
			return false
		}
	}

	return true
}

func (t *Type) isStruct() bool {
	return t.fields != nil
}

func (t *Type) calcSize() (err string) {
	if t==nil || t.size != 0 {
		return ""
	}

	if t.etype!=nil {
		// matrix
		err = t.etype.calcSize()
		if err!="" {
			return err
		}

		esz := t.etype.size
		if esz < 0 {
			t.size = -1
			return fmt.Sprintf("matrix with variable element size")
		}

		if t.dimnum == 0 {
			// type alias
			t.size = esz
			return ""
		}

		sz := int64(1)
		for _, n := range t.dim {
			if n==0 {
				sz = -1
				break
			}

			sz = sz * int64(n)
		}

		t.size = sz * esz
	} else if t.fields != nil && len(t.fields) > 0 {
		sz := int64(0)
		for i := 0; i < len(t.fields); i++ {
			f := &t.fields[i]
			err = f.t.calcSize()
			if err != "" {
				return err
			}

			// TODO: alignment?
			fsz := f.t.size
			if fsz < 0 {
				t.size = -1
				return "field with variable size"
			}

			f.offset = sz
			sz += fsz
		}

		t.size = sz
	} else {
		return fmt.Sprintf("%s: undefined type", t.name)
	}

	return ""
}

func (t *Type) EvalDims() (err string) {
	if t.dimnum > 0 && t.dim == nil {
		t.dim = make([]int, t.dimnum)
		t.vidx = make([]EVar, t.dimnum)
		for i := 0; i < t.dimnum; i++ {
			t.vidx[i].name = fmt.Sprintf("d%d", i)
			if t.dim[i]!=0 {
				continue
			}

			_, err = t.ds.evalExpr(t.dimexpr[i])
			if err != "" {
				return
			}

			var val interface{}
			if t.dimexpr[i] != nil {
				val = t.dimexpr[i].val
			}

			if val == nil {
				t.dim[i] = 0
			} else if n, ok := val.(int64); ok {
				t.dim[i] = int(n)
			} else {
				err = fmt.Sprintf("dimension not integer")
				return
			}
		}
	}

	if err=="" && t.etype!=nil {
		err = t.etype.EvalDims()
	}

	if err=="" && t.fields!=nil {
		for _, f := range t.fields {
			err = f.t.EvalDims()
			if err!="" {
				break
			}
		}
	}

	return
}

func (t *Type) createBlocks(bs *drepl.BlockSeq) (b drepl.Block, err string) {
	if t.etype!=nil && t.dimnum==0 {
		// handle type aliases
		t = t.etype
	}

	if t.etype!=nil {
		dim := make([]int64, t.dimnum)
		for i:=0; i < t.dimnum; i++ {
			dim[i] = int64(t.dim[i])
		}

		nbs := bs.View().NewBlockSeq()
		_, err = t.etype.createBlocks(nbs)
		if err!="" {
			return
		}

		nblks := nbs.Blocks()
		if len(nblks) != 1 {
			return nil, "internal error"
		}

		b = bs.NewABlock(t.etype.size, dim, nblks[0])
	} else if t.fields != nil {
		nbs := bs.View().NewBlockSeq()
		for _, f := range t.fields {
			_, err = f.t.createBlocks(nbs)
			if err != "" {
				return
			}
		}

		b = bs.NewTBlock(nbs)
	} else {
		// primary types
		b = bs.NewSBlock(t.size)
	}

	return
}


func (t *Type) String() string {
	s := t.name
	if t.dimnum != 0 && t.fields != nil {
		return "type error"
	}

	if t.dimnum==0 && t.etype != nil {
		t = t.etype
	}

	if t.dimnum!=0 {
		s = "["
		for i := 0; i < t.dimnum; i++ {
			s += fmt.Sprintf("%d", t.dim[i])
			if i+1 < t.dimnum {
				s += ","
			}
		}
		s += "]"

		s += t.etype.String()
	}

	if t.fields!=nil {
		s += "{"
		for _, f := range t.fields {
			s += fmt.Sprintf("%s %s; ", f.name, f.t)
		}
		s += "}"
	}

	return s
}

func (vd *VarDecl) isDefined() bool {
	return vd.pos.fname != ""
}

func (v *VarDecl) createBlocks(vv *drepl.View) (err string) {
	v.blk, err = v.t.createBlocks(vv.Blocks())
	return
}

func (cd *ConstDecl) isDefined() bool {
	return cd.pos.fname != ""
}

func (ds *Dataset) createPrimaryType(name string, sz int64) *Type {
	td, _ := ds.createType(name, nil)
	td.pos.fname = "built-in"
	td.primary = true
	td.size = sz
	return td
}

func NewDataset() *Dataset {
	ds := new(Dataset)
	ds.types = make(map[string]*Type)
	ds.vars = make(map[string]*VarDecl)
	ds.consts = make(map[string]*ConstDecl)
	ds.createPrimaryType("int8", 1)
	ds.createPrimaryType("int16", 2)
	ds.createPrimaryType("int32", 4)
	ds.createPrimaryType("int64", 8)
	ds.createPrimaryType("float32", 4)
	ds.createPrimaryType("float64", 8)
//	ds.createPrimaryType("string")

	return ds
}

func (ds *Dataset) evalExpr(e *Expr) (val interface{}, err string) {
	if e == nil {
		return nil, ""
	}

	val = e.val
	err = ""
	if val != nil {
		if c, ok := val.(*ConstDecl); ok {
			val, err = ds.evalConst(c)
		} else if b, ok := val.([]byte); ok {
			var oe error

			s := string(b)
			//			fmt.Printf("evalExpr: %p %d\n", e, e.op)
			switch e.op {
			case INT:
				val, oe = strconv.ParseInt(s, 0, 64)
				if oe == strconv.ErrRange {
					val, oe = strconv.ParseUint(s, 0, 64)
				}

				if oe != nil {
					return nil, oe.Error()
				}
				break

			case FLOAT:
				val, oe = strconv.ParseFloat(s, 64)
				if oe != nil {
					return nil, oe.Error()
				}

			case STRING:
				val, oe = strconv.Unquote(s)
				if oe != nil {
					return nil, oe.Error()
				}
			}
		}

		// store the value so we don't evaluate it next time
		e.val = val
		return
	}

	switch e.op {
	case ADD, SUB, MUL, QUO, REM:
		lv, le := ds.evalExpr(e.left)
		rv, re := ds.evalExpr(e.right)
		if le != "" {
			return nil, le
		}

		if re != "" {
			return nil, re
		}

		// first try int64 result
		if val, err = evalInt64(lv, rv, e.op); val != nil || err != "" {
			e.val = val
			return
		}

		// then uint64
		if val, err = evalUint64(lv, rv, e.op); val != nil || err != "" {
			e.val = val
			return
		}

		if val, err = evalFloat(lv, rv, e.op); val != nil || err != "" {
			e.val = val
			return
		}

		err = "invalid operand type(s)"

	default:
		err = "invalid operator"
	}

	return
}

func (ds *Dataset) evalConst(c *ConstDecl) (val interface{}, err string) {
	if c.eval {
		return nil, fmt.Sprintf("%v: constant %s used while evaluating it", &c.pos, c.name)
	}

	if c.expr == nil {
		return nil, fmt.Sprintf("constant %s not defined", c.name)
	}

	c.eval = true
	val, err = ds.evalExpr(c.expr)
	c.val = val
	c.eval = false
	return
}

func (ds *Dataset) EvalConsts() (err string) {
	for _, c := range ds.consts {
		_, err = ds.evalConst(c)
		if err != "" {
			return
		}

//		fmt.Printf("const %s = %v\n", c.name, c.expr.val)
	}

	return
}

func (ds *Dataset) EvalDims() (err string) {
	for _, v := range ds.vars {
		err = v.t.EvalDims()
		if err!="" {
			return fmt.Sprintf("%v: %s", &v.pos, err)
			break
		}
	}

	return
}

func (ds *Dataset) calcTypeSizes() (err string) {
	for _, t := range ds.types {
		err = t.calcSize()
		if err!="" {
			return
		}
	}

	return ""
}

func (ds *Dataset) calcVarOffsets() (err string) {
//	fname := "ds"
	n := 0
	offset := int64(0)
	for _, v := range ds.vars {
//		v.dest.fname = fname
//		v.dest.offset = offset

		err = v.t.calcSize()
		if err!="" {
			return
		}

		if v.t.size > 0 {
			offset += v.t.size
		} else {
//			fname = fmt.Sprintf("ds%d", n)
			n++
			offset = 0
		}
	}

	return ""
}

func (ds *Dataset) createBlocks() (err string) {
	ds.repl = drepl.NewReplica("*default*", "")
	ds.v = drepl.NewView("*default", drepl.RowMajorOrder, false)
	ds.repl.AddView(ds.v)

	for _, v := range ds.vars {
		err = v.createBlocks(ds.v)
		if err != "" {
			return err
		}
	}

	return
}

// connect each view with each other view
func (ds *Dataset) fixBlocks() (err string) {
	for _, v := range ds.vars {
//		fmt.Printf("connect destinations for %s\n", v.name)
		v.blk.ConnectDestinations()
	}
	
	return
}

func (ds *Dataset) GetBlocks() (bs []drepl.Block) {
	for _, v := range ds.vars {
		bs = append(bs, v.blk)
	}

	return
}

func (t *Type) reset() {
}

func (v *VarDecl) reset() {
	v.blk = nil
}

func (ds *Dataset) reset() {
	for _, t := range ds.types {
		t.reset()
	}

	for _, v := range ds.vars {
		v.reset()
	}

	ds.v = nil
	ds.repl = nil
}
