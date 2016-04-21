package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"drepl/drepl"
	"drepl/parser"
)

var vlegacy = "view legacy {\nvar a [i]{ a } = data[i]\nvar b [i]{ b } = data[i]\nvar c [i]{ c } = data[i]\n}"
var rlegacy = "replica legacy \"legacy\" {\nview legacy\n}"
var vb = "view b {\nvar b [i]{ b } = data[i]\n}";
var rb = "replica b \"b\" {view b\n}";

var vb1 = "view b {\nvar b = b\n}";
var rb1 = "replica b \"b\" {view b\n}";

func outputGraph(dr *parser.DRepl, fname string) {
	_, views, err := dr.CreateTransformationRules()
	if err != "" {
		fmt.Printf("%s\n", err)
		return
	}

	g := drepl.NewGraph()
	for _, v := range views {
		g.AddView(v)
	}

	f, oerr := os.Create(fname)
	if oerr != nil {
		fmt.Printf("Error: %v\n", oerr)
		return
	}

	g.Output(f, false)
	f.Close()
}

func main() {
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Printf("invalid arguments\n")
		return
	}

	dr, err := parser.Parse(flag.Arg(0))
	if err != "" {
		fmt.Printf("%s\n", err)
		return
	}

	outputGraph(dr, path.Base(flag.Arg(0)) + "0.dot")
/*
	dr.AddView([]byte(vb1))
	outputGraph(dr, path.Base(flag.Arg(0)) + "1.dot")

	dr.AddReplica([]byte(rb1))
	outputGraph(dr, path.Base(flag.Arg(0)) + "2.dot")
*/

/*
	dr.AddView([]byte(vlegacy))
	dr.AddReplica([]byte(rlegacy))
	outputGraph(dr, path.Base(flag.Arg(0)) + "2.dot")

	dr.RemoveReplica("legacy")
	dr.RemoveView("legacy")
	dr.RemoveReplica("b")
	outputGraph(dr, path.Base(flag.Arg(0)) + "3.dot")
	dr.RemoveView("b")
	outputGraph(dr, path.Base(flag.Arg(0)) + "4.dot")
*/
}
