package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"path"
	"syscall"
//	"unsafe"
	"github.com/lionkov/go9p/p"
	"github.com/lionkov/go9p/p/srv"
	"drepl/drepl"
	"drepl/parser"
)

type Drfs struct {
	srv.Fsrv
}

type ViewFile struct {
	srv.File
	v	*drepl.View
}

var addr = flag.String("addr", ":5640", "network address")
var debug = flag.Bool("d", false, "print debug messages")
var debugall = flag.Bool("D", false, "print packets as well as debug messages")

var sync = flag.Bool("sync", true, "sync the other view in the background")
var domsync = flag.Bool("msync", false, "msync after each write")
var graph = flag.Bool("g", false, "output a dot graph file")

var Ewrite = &p.Error{"invalid offset", uint32(syscall.EIO)}

func toError(err error) *p.Error {
	return &p.Error{err.Error(), uint32(syscall.EIO)}
}

func (v *ViewFile) Read(fid *srv.FFid, data []byte, offset uint64) (count int, err error) {
//	fmt.Printf("ViewFile.Read\n")
	count = len(data)
	if offset > v.Length {
		count = 0
	} else if offset+uint64(count) > v.Length {
		count = int(v.Length - offset)
	}

	if count == 0 {
		return 0, nil
	}

	data = data[0:count]
	blks := v.v.Search(int64(offset), int64(len(data)))
//	fmt.Printf("ViewFile.Read blks %v\n", blks)
	count = 0
	for _, b := range blks {
		n, err := b.Read(data, int64(offset), b.Offset())
//		fmt.Printf("ViewFile.Read: n %d err %v\n", n, err)
		if err != nil {
			return count, err
		}

		if n==0 {
			break
		}

		offset += uint64(n)
		count += int(n)
		data = data[n:]
	}

	return int(count), err
}

func (v *ViewFile) Write(fid *srv.FFid, data []byte, offset uint64) (count int, err error) {
	blks := v.v.Search(int64(offset), int64(len(data)))
//	fmt.Printf("ViewFile.Write blks %v\n", blks)
	for _, b := range blks {
		n, err := b.Write(data, int64(offset), b.Offset(), true, true)
//		fmt.Printf("ViewFile.Write: n %d err %v\n", n, err)
		if err != nil {
			return count, err
		}

		offset += uint64(n)
		count += int(n)
		data = data[n:]
	}

	if len(data)!=0 && count==0 {
//		fmt.Printf("short write: %d instead of %d\n", count, len(data))
		return 0, errors.New("short write")
	}

	if (*domsync) {
		err = v.v.Sync()
	}

	return count, err
}

func (*Drfs) ConnOpened(c *srv.Conn) {
}

func (*Drfs) ConnClosed(c *srv.Conn) {
	os.Exit(0)
}

func main() {
	var oerr error
	var s *Drfs
	var root *srv.File
	var g *drepl.Graph

	user := p.OsUsers.Uid2User(os.Geteuid())
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Printf("invalid arguments")
		return
	}

	dr, err := parser.Parse(flag.Arg(0))
	if err != "" {
		fmt.Printf("Error: %s\n", err);
		return
	}

	repls, views, err := dr.CreateTransformationRules()
	if err != "" {
		fmt.Printf("Error: %s\n", err)
		return
	}

	root = new(srv.File)
	oerr = root.Add(nil, "/", user, nil, p.DMDIR|0555, nil)
	if oerr != nil {
		goto oerror
	}

	if *graph {
		g = drepl.NewGraph()
	}

	for _, r := range repls {
		f, err := os.Create(r.FileName)
		if err != nil {
			oerr = err
			goto oerror
		}

		r.SetFile(f)
	}

	for _, v := range views {
		if g != nil {
			g.AddView(v)
		}

		pf := new(ViewFile)
		pf.v = v
//		fmt.Printf("unmaterialized view %s %v\n", v.Name, v)
		oerr = pf.Add(root, v.Name, user, nil, 0666, pf)
		if oerr != nil {
			goto oerror
		}
		pf.Length = uint64(v.Size())
	}

	drepl.AsyncWrite = !*sync

	s = new(Drfs)
	s.Root = root
	root.Parent = root
	s.Dotu = true
	s.Msize = 32736 + p.IOHDRSZ

	if *debug {
		s.Debuglevel = 1
	}

	if *debugall {
		s.Debuglevel = 2
	}

	if g != nil {
		f, oerr := os.Create(path.Base(flag.Arg(0)) + ".dot")
		if oerr != nil {
			fmt.Printf("Error: %v\n", oerr)
			return
		}

		g.Output(f, true)
		f.Close()
	}

	s.Start(s)
	oerr = s.StartNetListener("tcp", *addr)
	if oerr != nil {
		goto oerror
	}

	return

oerror:
	log.Println(fmt.Sprintf("Error: %v", oerr))
	return
}
