package parser

import (
//	"drepl/drepl"
//	"fmt"
//	"log"
//	"os"
)

const (
	Rcomplete	= 1<<iota		// has all data in the dataset
	Rreadonly				// read-only
	Rdefault				// default replica to read unmaterialized views from
)

type Repl struct {
	Name		string
	filename	string
	flags		int
	views		[] *View
}

func NewReplica(name string, fname string, flags int) *Repl {
	r := new(Repl)
	r.Name = name
	r.filename = fname
	r.flags = flags

	return r
}

func (r *Repl) addView(v *View) {
	r.views = append(r.views, v)
}

func (r *Repl) reset() {
}

/*
func (r *Repl) process() (repl *drepl.Replica, err string) {
	repl = drepl.NewReplica(r.Name, r.filename)
	for _, v := range r.views {
		var elo drepl.ElementOrder

		switch (v.flags & 0x7F) {
		case Vrowmajor:
			elo = drepl.RowMajorOrder
		case Vrowminor:
			elo = drepl.RowMinorOrder
		default:
			return nil, fmt.Sprintf("invalid order: %d", v.flags & 0x7F)
		}

		vv := repl.NewView(v.Name, elo)
		err = v.createBlocks(vv)
		if err!="" {
			return nil, err
		}
	}

	return
}
*/
