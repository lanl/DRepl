package parser

import (
	"flag"
	"fmt"
	"os"
	"drepl/drepl"
)

type DRepl struct {
	Dataset		*Dataset
	Views		map[string] *View
	Replicas	map[string] *Repl
}

type ErrPrinter struct {
	errs		string
}

func Parse(file string) (dr *DRepl, errs string) {
	var f *os.File

	fi, err := os.Stat(flag.Arg(0))
	if err!=nil {
		errs = fmt.Sprintf("Error: %v\n", err)
		return
	}

	buf := make([]byte, fi.Size())
	f, err = os.Open(flag.Arg(0))
	if err!=nil {
		errs = fmt.Sprintf("Error: %v\n", err)
		return
	}


	for b:=buf; len(b)>0; {
		var n int

		n, err = f.Read(b)
		if err!=nil {
			errs = fmt.Sprintf("Error: %v\n", err)
			return
		}

		b = b[n:]
	}
	f.Close()

	return NewDRepl(buf)
}

func NewDRepl(descr []byte) (dr *DRepl, errs string) {
	dr = new(DRepl)
	dr.Dataset = NewDataset()
	dr.Views = make(map[string] *View)
	dr.Replicas = make(map[string] *Repl)

	ep := new(ErrPrinter)
	sc := NewScanner(flag.Arg(0), descr, ep, InsertSemis)
	ps := NewParser(dr, sc, ep)
	ps.Parse()
	if ep.errs != "" {
		errs = ep.errs
		return
	}

	errs = dr.Dataset.EvalConsts()
	if errs!="" {
		return
	}

	errs = dr.Dataset.EvalDims()
	if errs!="" {
		return
	}

	errs = dr.Dataset.calcTypeSizes()
	if errs!="" {
		return
	}

	errs = dr.Dataset.calcVarOffsets()
	if errs!="" {
		return
	}

//	for _, t := range dr.Dataset.types {
//		fmt.Printf("type %s sz %d: %v\n", t.name, t.size, t)
//	}

//	for _, v := range dr.Dataset.vars {
//		fmt.Printf("var %s sz %d '%v'\n", v.name, v.t.size, v.t)
//	}

	for _, v := range dr.Views {
		if err:=v.process(); err!="" {
			errs = err
			return
		}
	}

	return dr, errs
}

func (dr *DRepl) CreateTransformationRules() (repls []*drepl.Replica, views []*drepl.View, errs string) {
	errs = dr.Dataset.createBlocks()
	if errs!="" {
		return
	}


	var defaultView *drepl.View
	vmap := make(map[*View] *drepl.View);
	for _, v := range dr.Views {
		var elo drepl.ElementOrder 

		switch (v.flags & 0x3F) {
		case Vrowmajor:
			elo = drepl.RowMajorOrder
		case Vrowminor:
			elo = drepl.RowMinorOrder
		default:
			return nil, nil, fmt.Sprintf("invalid order: %d", v.flags & 0x7F)
		}

		vv := drepl.NewView(v.Name, elo, v.flags & Vreadonly != 0)
		
		if v.flags & Vdefault != 0 {
			defaultView = vv
		}

		views = append(views, vv)
		vmap[v] = vv;
	}

	for _, r := range dr.Replicas {
		rr := drepl.NewReplica(r.Name, r.filename)
		
		for _, v := range r.views {
			rr.AddView(vmap[v])
		}

		repls = append(repls, rr)
	}

	for _, v := range dr.Views {
		vv := vmap[v]

		// make sure that the unmaterialized views have default view to read from
		if !vv.Materialized() {
			vv.SetDefaultView(defaultView)
		}

		errs = v.createBlocks(vv)
		if errs != "" {
			return
		}
	}

	errs = dr.Dataset.fixBlocks()
	if errs != "" {
		return
	}

	dr.Dataset.reset()
	for _, v := range dr.Views {
		v.reset()
	}
	for _, r := range dr.Replicas {
		r.reset()
	}


	return repls, views, ""
/*
	for _, v := range dr.Dataset.vars {
		fmt.Printf("*** %s %v\n", v.name, v.blk)
	}

	for _, v := range dr.Views {
		for _, vv := range v.vars {
			fmt.Printf("=== %s %v\n", vv.name, vv.blk)
		}
	}
*/

	return
}

func (dr *DRepl) AddView(descr []byte) (errs string) {
	oldviews := make(map[*View] bool)
	for _, v := range dr.Views {
		oldviews[v] = true
	}

	ep := new(ErrPrinter)
	sc := NewScanner(flag.Arg(0), descr, ep, InsertSemis)
	ps := NewParser(dr, sc, ep)
	ps.Parse()
	if ep.errs != "" {
		errs = ep.errs
	}

	for _, v := range dr.Views {
		if !oldviews[v] {
			v.process()
		}
	}

	return
}

func (dr *DRepl) RemoveView(name string) (errs string) {
	if v := dr.Views[name]; v == nil {
		return "view not found"
	}

	delete(dr.Views, name)
	return
}

func (dr *DRepl) AddReplica(descr []byte) (errs string) {
	ep := new(ErrPrinter)
	sc := NewScanner(flag.Arg(0), descr, ep, InsertSemis)
	ps := NewParser(dr, sc, ep)
	ps.Parse()
	if ep.errs != "" {
		errs = ep.errs
	}

	// TODO: process the replica ???

	return
}

func (dr *DRepl) RemoveReplica(name string) (errs string) {
	if r := dr.Replicas[name]; r == nil {
		return "replica not found"
	}

	delete(dr.Replicas, name)
	return
}

func (e *ErrPrinter) Error(pos *Pos, msg string) {
	e.errs = fmt.Sprintf("%sError: %s:%d: %s\n", e.errs, pos.fname, pos.line, msg)
//	panic("boo")
}

func (dr *DRepl) createView(name string, flags int) *View {
	v := dr.Views[name]
	if v!=nil {
		return nil
	}

	v = NewView(name, flags)
	dr.Views[name] = v
	return v
}

func (dr *DRepl) findView(name string) *View {
	v, found := dr.Views[name]
	if !found {
		return nil
	}

	return v
}

func (dr *DRepl) createReplica(name string, fname string, flags int) *Repl {
	r, found := dr.Replicas[name]
	if found {
		return nil
	}

	r = NewReplica(name, fname, flags)
	dr.Replicas[name] = r
	return r
}

func (dr *DRepl) findReplica(name string) *Repl {
	r, found := dr.Replicas[name]
	if !found {
		return nil
	}

	return r
}
