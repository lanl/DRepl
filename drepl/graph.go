package drepl

import (
	"fmt"
	"io"
)

type Graph struct {
	blks	map[Block] string
}

func NewGraph() *Graph {
	g := new(Graph)
	g.blks = make(map[Block] string)

	return g
}

func (g *Graph) AddView(v *View) {
	for _, b := range v.bs.blks {
		g.AddBlock(b)
	}
}

func (g *Graph) AddBlock(bb Block) {
	if bb == nil {
		return
	}

	if g.blks[bb] != "" {
		return
	}

	switch b := bb.(type) {
	default:
		panic("unknown block type")

	case *SBlock:
		if b==nil {
			return
		}
		g.blks[b] = b.view.Name
		g.AddBlock(b.clonee)
		g.AddBlock(b.src)
		for _, bb := range b.dests {
			g.AddBlock(bb)
		}

	case *ABlock:
		if b==nil {
			return
		}
		g.blks[b] = b.view.Name
		g.AddBlock(b.elblk)
		g.AddBlock(b.clonee)
		if b.src != nil {
			g.AddBlock(b.src.el)
			g.AddBlock(b.src.arr)
		}

		for _, d := range b.dests {
			g.AddBlock(d.el)
			g.AddBlock(d.arr)
		}

	case *TBlock:
		if b==nil {
			return
		}
		g.blks[b] = b.view.Name
		g.AddBlock(b.clonee)
		g.AddBlock(b.src)
		for _, bb := range b.bs.blks {
			g.AddBlock(bb)
		}
		for _, bb := range b.dests {
			g.AddBlock(bb)
		}
	}
}

func (g *Graph) outputBlock(showptr bool, bb Block) (nodes, links string) {
	ptr := ""
	if showptr {
		ptr = fmt.Sprintf("%p|", bb)
	}

	switch b := bb.(type) {
	default:
		panic("unknown block type")

	case *SBlock:
		c := "black"
		if b.clonee != nil {
			c = "blue"
		}
		nodes = fmt.Sprintf("node%p [\nlabel = \"%sS %s:%06d\"\nshape = \"record\"\ncolor=%s\nfontcolor=%s];\n", b, ptr, b.view.Name, b.offset, c, c)
		if b.clonee != nil {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=blue\nfontcolor=blue\nlabel=clonee\n]\n", b, b.clonee)
		}
		if b.src != nil {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=cyan4\nfontcolor=cyan4\nlabel=src\n]\n", b, b.src)
		}
		for i, bb := range b.dests {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=black\nlabel=dest%d\n]\n", b, bb, i)
		}

	case *ABlock:
		c := "black"
		if b.clonee != nil {
			c = "blue"
		}
		nodes = fmt.Sprintf("node%p [\nlabel = \"%sA %s:%06d\"\nshape = \"record\"\ncolor = %s\nfontcolor = %s\n];\n", b, ptr, b.view.Name, b.offset, c, c)
		if b.src != nil {
			nodes += fmt.Sprintf("dest%p [\nlabel=\"<arr> arr | <el> el\"\nshape = \"record\"\ncolor=cyan4\nfontcolor=cyan4\n];\n", b.src)
			links += fmt.Sprintf("node%p -> dest%p [\ncolor=cyan4\nfontcolor=cyan4\nlabel=src\n]\n", b, b.src)
			links += fmt.Sprintf("dest%p:arr -> node%p [\ncolor=cyan4\nfontcolor=cyan4\nlabel=arr\n]\n", b.src, b.src.arr)
			links += fmt.Sprintf("dest%p:el -> node%p [\ncolor=cyan4\nfontcolor=cyan4\nlabel=el\n]\n", b.src, b.src.el)
		}
		for i, d := range b.dests {
			nodes += fmt.Sprintf("dest%p [\nlabel=\"<arr> arr | <el> el\"\nshape = \"record\"\ncolor=green\n];\n", d)
			links += fmt.Sprintf("node%p -> dest%p [\ncolor=black\nlabel=dest%d\n]\n", b, d, i)
			links += fmt.Sprintf("dest%p:arr -> node%p [\ncolor=green\nfontcolor=green\nlabel=arr\n]\n", d, d.arr)
			links += fmt.Sprintf("dest%p:el -> node%p [\ncolor=green\nfontcolor=green\nlabel=el\n]\n", d, d.el)
		}
		if b.clonee != nil {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=blue\nfontcolor=blue\nlabel=clonee\n]\n", b, b.clonee)
		}
		links += fmt.Sprintf("node%p -> node%p [\ncolor=orange\nfontcolor=orange\nlabel=el\n]\n", b, b.elblk)

	case *TBlock:
		c := "black"
		if b.clonee != nil {
			c = "blue"
		}
		nodes = fmt.Sprintf("node%p [\nlabel = \"%sT %s:%06d\"\nshape = \"record\"\ncolor = %s\nfontcolor=%s];\n", b, ptr, b.view.Name, b.offset, c, c)
		if b.clonee != nil {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=blue\nfontcolor=blue\nlabel=clonee\n]\n", b, b.clonee)
		}
		if b.src != nil {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=cyan4\nfontcolor=cyan4\nlabel=src\n]\n", b, b.src)
		}
		for i, bb := range b.bs.blks {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=violet\nfontcolor=violet\nlabel=field%d\n]\n", b, bb, i)
		}
		for i, bb := range b.dests {
			links += fmt.Sprintf("node%p -> node%p [\ncolor=black\nlabel=dest%d\n]\n", b, bb, i)
		}
	}

	return
}

func (g *Graph) Output(f io.Writer, showptr bool) (err error) {
	nodes := make(map[string] string)
	links := make(map[string] string)

	// get the nodes and links per view
	for b, v := range g.blks {
		n, l := g.outputBlock(showptr, b)
		nodes[v] += n
		links[v] += l
	}

	fmt.Fprintf(f, "digraph drepl {\nrankdir=LR;\nsize=\"8,5\"\nrotate=90\n")

	for v, n := range nodes {
		_, err = fmt.Fprintf(f, "subgraph cluster%s {\nlabel=\"%s\";\n", v, v)
		if err != nil {
			return
		}

		_, err = fmt.Fprintf(f, "%s", n)
		if err != nil {
			return
		}

		_, err = fmt.Fprintf(f, "}\n")
		if err != nil {
			return
		}

	}

	for _, l := range links {
		_, err = fmt.Fprintf(f, "%s", l)
		if err != nil {
			return
		}
	}

	_, err = fmt.Fprintf(f, "\n}\n")
	return
}
