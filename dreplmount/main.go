package main

import (
	"flag"
	"fmt"
	"os"
	"syscall"
	"unsafe"
	"drepl/drepl"
	"drepl/parser"
)

var sync = flag.Bool("s", true, "synchronous writes")

func main() {
	var e *drepl.Exporter
	var flags uint32

	flag.Parse()
	if flag.NArg() != 2 {
		fmt.Printf("invalid arguments")
		return
	}

	if *sync {
		flags = 1
	}

	dr, err := parser.Parse(flag.Arg(0))
	if err != "" {
		fmt.Printf("%s\n", err)
		return
	}

	repls, views, err := dr.CreateTransformationRules()
	if err != "" {
		fmt.Printf("%s\n", err)
		return
	}

	e = drepl.NewExporter()
	for _, r := range repls {
		f, err := os.Create(r.FileName)
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}

		err = f.Truncate(r.Size())
		if err != nil {
			fmt.Printf("%s\n", err)
			return
		}

		f.Close()
		e.AddReplica(r)
//		fmt.Printf("%v\n", r);
	}

	for _, v := range views {
		e.AddView(v)
//		fmt.Printf("%v\n", v);
	}

	data := e.Data(flags)
	buf := []byte(nil)
	len := uint32(len(data))
	buf = append(buf, uint8(len))
	buf = append(buf, uint8(len>>8))
	buf = append(buf, uint8(len>>16))
	buf = append(buf, uint8(len>>24))

	addr := uint64(uintptr(unsafe.Pointer(&data[0])))
	buf = append(buf, uint8(addr))
	buf = append(buf, uint8(addr>>8))
	buf = append(buf, uint8(addr>>16))
	buf = append(buf, uint8(addr>>24))
	buf = append(buf, uint8(addr>>32))
	buf = append(buf, uint8(addr>>40))
	buf = append(buf, uint8(addr>>48))
	buf = append(buf, uint8(addr>>56))


	oe := syscall.Mount2("", flag.Arg(1), "dreplfs", 0, buf)
	fmt.Printf("mount: %v\n", oe)

	return
}
