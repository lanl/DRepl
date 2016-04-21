// Copyright 2009 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// View1:
//	a	float[arraylen]
//	b	float[arraylen]
//	c	float[arraylen]
//
// View2:
//	ds	[arraylen] struct {
//			a	float
//			b	float
//			c	float
//		}
//
// View3:
//	b	float[arraylen]
//

package main

import (
	"code.google.com/p/go9p/p"
	"code.google.com/p/go9p/p/srv"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"
	"unsafe"
)

type Dataset struct {
	acnt   int // array count
	filesz uint64
	v1     []byte // view1 file mmapped
	v2     []byte // view2 file mmapped
	v3     []byte // view3 file mmapped
}

type Dsfs struct {
	srv.Fsrv
}

type View1 struct {
	srv.File
}
type View2 struct {
	srv.File
}
type View3 struct {
	srv.File
}

var ds Dataset
var addr = flag.String("addr", ":5640", "network address")
var debug = flag.Bool("d", false, "print debug messages")
var debugall = flag.Bool("D", false, "print packets as well as debug messages")

var arraylen = flag.Int("len", 0, "array length")
var sync = flag.Bool("sync", true, "sync the other views")
var v1fname = flag.String("v1", "", "file for view1")
var v2fname = flag.String("v2", "", "file for view2")
var v3fname = flag.String("v3", "", "file for view3")
var domsync = flag.Bool("msync", false, "msync after each write")

var Ewrite = &p.Error{"invalid offset", syscall.EIO}

func msync(buf []byte, s, e int) error {
	if !*domsync {
		return nil
	}

	if e > len(buf) {
		e = len(buf)
	}

	if s < 0 {
		s = 0
	}

	if s >= e {
		return nil
	}

	start := uintptr(unsafe.Pointer(&buf[s])) &^ (0xfff) // start address needs to be page-aligned
	end := uintptr(unsafe.Pointer(&buf[e-1]))
	_, _, e1 := syscall.Syscall(syscall.SYS_MSYNC, start, end-start, uintptr(syscall.MS_SYNC))
	errno := int(e1)
	if errno != 0 {
		return syscall.Errno(errno)
	}

	return nil
}

func toError(err error) *p.Error {
	return &p.Error{err.Error(), syscall.EIO}
}

func v1tov1Write(data []byte, offset uint64) (int, error) {
	n := copy(ds.v1[offset:], data)
	err := msync(ds.v1, int(offset), int(offset)+n)
	if err != nil {
		return 0, toError(err)
	}

	return n, nil
}

func v1tov1Read(data []byte, offset uint64) (int, error) {
	if offset > ds.filesz {
		return 0, nil
	}

	n := copy(data, ds.v1[offset:])
	return n, nil
}

func v1tov2Write(data []byte, offset uint64) (int, error) {
	min := int(ds.filesz) / 4
	max := 0
	for start, end := int(offset), int(offset)+len(data); start < end; start += 4 {
		idx := start / 4
		idx2 := 0
		n := uint64(start) - offset
		if idx < ds.acnt {
			// a[idx]
			idx2 = idx * 3
		} else if idx < 2*ds.acnt {
			// b[idx - ds.acnt]
			idx2 = (idx-ds.acnt)*3 + 1
		} else {
			idx2 = (idx-2*ds.acnt)*3 + 2
		}

		copy(ds.v2[idx2*4:], data[n:n+4])
		if idx2 < min {
			min = idx2
		}

		if idx2 > max {
			max = idx2
		}
	}

	if min < max {
		err := msync(ds.v2, min*4, max*4)
		if err != nil {
			return 0, toError(err)
		}
	}

	return len(data), nil
}

// read from ds.v1 and make it look like read from View 2
func v1tov2Read(data []byte, offset uint64) (int, error) {
	for start, end := offset, offset+uint64(len(data)); start < end; start += 12 {
		idx := int(start / 12)
		n := start - offset
		copy(data[n:n+4], ds.v1[idx*4:])
		copy(data[n+4:n+8], ds.v1[(ds.acnt+idx)*4:])
		copy(data[n+8:n+12], ds.v1[(ds.acnt*2+idx)*4:])
	}

	return len(data), nil
}

func v1tov3Write(data []byte, offset uint64) (int, error) {
	asz := uint64(ds.acnt * 4)
	end := offset + uint64(len(data))

	if end < asz {
		// only array a data
		return len(data), nil
	}

	if offset > 2*asz {
		// only array c data
		return len(data), nil
	}

	if offset < asz {
		offset = asz
		data = data[asz-offset:]
	}

	if end > 2*asz {
		end = 2 * asz
		data = data[0 : 2*asz-offset]
	}

	copy(ds.v3[offset-asz:], data)
	err := msync(ds.v3, int(offset), int(end))
	if err != nil {
		return 0, err
	}

	return len(data), nil
}

func v2tov1Write(data []byte, offset uint64) (int, error) {
	min := ds.filesz
	max := uint64(0)
	for start, end := offset, offset+uint64(len(data)); start < end; start += 12 {
		idx := start / 12
		n := start - offset
		copy(ds.v1[idx*4:], data[n:n+4])
		copy(ds.v1[(int(idx)+ds.acnt)*4:], data[n+4:n+8])
		copy(ds.v1[(int(idx)+ds.acnt*2)*4:], data[n+8:n+12])

		if idx < min {
			min = idx
		}

		if idx+uint64(ds.acnt*2) > max {
			max = idx + uint64(ds.acnt*2)
		}
	}

	if min < max {
		err := msync(ds.v1, int(min)*4, int(max)*4)
		if err != nil {
			return 0, err
		}
	}

	return len(data), nil
}

// read from ds.v2 and make it look like read from View 1
func v2tov1Read(data []byte, offset uint64) (int, error) {
	for start, end := offset, offset+uint64(len(data)); start < end; start += 4 {
		idx := int(start / 4)
		idx2 := 0
		if idx < ds.acnt {
			// a[idx]
			idx2 = idx * 3
		} else if idx < 2*ds.acnt {
			// b[idx - ds.acnt]
			idx2 = (idx-ds.acnt)*3 + 1
		} else {
			idx2 = (idx-2*ds.acnt)*3 + 2
		}

		n := start - offset
		copy(data[n:n+4], ds.v2[idx2*4:])
	}

	return len(data), nil
}

func v2tov2Write(data []byte, offset uint64) (int, error) {
	n := copy(ds.v2[offset:], data)
	err := msync(ds.v2, int(offset), int(offset)+n)
	if err != nil {
		return 0, toError(err)
	}

	return n, nil
}

func v2tov2Read(data []byte, offset uint64) (int, error) {
	if offset > ds.filesz {
		return 0, nil
	}

	n := copy(data, ds.v2[offset:])
	return n, nil
}

func v2tov3Write(data []byte, offset uint64) (int, error) {
	min := ds.filesz
	max := uint64(0)
	for start, end := offset, offset+uint64(len(data)); start < end; start += 12 {
		idx := start / 12
		n := start - offset
		copy(ds.v3[idx*4:], data[n+4:n+8])

		if idx < min {
			min = idx
		}

		if idx+4 > max {
			max = idx + uint64(4)
		}
	}

	if min < max {
		err := msync(ds.v3, int(min)*4, int(max)*4)
		if err != nil {
			return 0, err
		}
	}

	return len(data), nil
}

func v3tov1Write(data []byte, offset uint64) (int, error) {
	n := copy(ds.v1[offset+uint64(ds.acnt)*4:], data)
	err := msync(ds.v1, int(offset)+ds.acnt*4, int(offset)+ds.acnt*4+n)
	if err != nil {
		return 0, toError(err)
	}
	return n, nil
}

func v3tov2Write(data []byte, offset uint64) (int, error) {
	for start, end := offset, offset+uint64(len(data)); start < end; start += 4 {
		idx := int(start / 4)
		copy(ds.v2[idx*12+4:], data[start-offset:start-offset+4])
	}

	err := msync(ds.v2, int(offset*3), (int(offset)+len(data))*3)
	if err != nil {
		return 0, toError(err)
	}

	return len(data), nil
}

func v3tov3Write(data []byte, offset uint64) (int, error) {
	n := copy(ds.v3[offset:], data)
	err := msync(ds.v3, int(offset), int(offset)+n)
	if err != nil {
		return 0, toError(err)
	}

	return n, nil
}

// read from ds.v1 and make it look like read from View 3
func v1tov3Read(data []byte, offset uint64) (int, error) {
	end := offset + uint64(len(data))
	if end > uint64(ds.acnt)*4 {
		end = uint64(ds.acnt) * 4
	}

	n := copy(data, ds.v1[offset:end])
	return n, nil
}

// read from ds.v2 and make it look like read from View 3
func v2tov3Read(data []byte, offset uint64) (int, error) {
	for start, end := offset, offset+uint64(len(data)); start < end; start += 4 {
		idx := int(start / 4)
		n := start - offset
		copy(data[n:n+4], ds.v2[idx*12+4:])
	}

	return len(data), nil
}

func v3tov3Read(data []byte, offset uint64) (int, error) {
	if offset > uint64(ds.acnt)*4 {
		return 0, nil
	}

	n := copy(data, ds.v3[offset:])
	return n, nil
}

func (*View1) Read(fid *srv.FFid, buf []byte, offset uint64) (int, error) {
	if ds.v1 != nil {
		return v1tov1Read(buf, offset)
	} else if ds.v2 != nil {
		return v2tov1Read(buf, offset)
	}

	return 0, errors.New("no complete views available")
}

func (*View1) Write(fid *srv.FFid, data []byte, offset uint64) (int, error) {
	var n int

	if offset%4 != 0 || offset > ds.filesz {
		return 0, Ewrite
	}

	if ds.v1 != nil {
		n1, err := v1tov1Write(data, offset)
		if err != nil {
			return 0, err
		}

		n = n1
	} else {
		n = len(data)
	}

	if ds.v2 != nil {
		if n > 0 && !*sync {
			go v1tov2Write(data, offset)
		} else {
			n2, err := v1tov2Write(data, offset)
			if err != nil {
				return 0, err
			}

			if n == 0 {
				n = n2
			}
		}
	}

	if ds.v3 != nil {
		if  n > 0 && !*sync {
			go v1tov3Write(data, offset)
		} else {
			n3, err := v1tov3Write(data, offset)
			if err != nil {
				return 0, err
			}

			if n == 0 {
				n = n3
			}
		}
	}

	return n, nil
}

func (*View2) Read(fid *srv.FFid, buf []byte, offset uint64) (int, error) {
	if ds.v2 != nil {
		return v2tov2Read(buf, offset)
	} else if ds.v1 != nil {
		return v1tov2Read(buf, offset)
	}

	return 0, errors.New("no complete views available")
}

func (*View2) Write(fid *srv.FFid, data []byte, offset uint64) (int, error) {
	var n int

	if offset%12 != 0 || offset > ds.filesz || len(data)%12 != 0 {
		return 0, Ewrite
	}

	if ds.v2 != nil {
		n2, err := v2tov2Write(data, offset)
		if err != nil {
			return 0, err
		}

		n = n2
	} else {
		n = len(data)
	}

	if ds.v1 != nil {
		if n > 0 && !*sync {
			go v2tov1Write(data, offset)
		} else {
			n1, err := v2tov1Write(data, offset)
			if err != nil {
				return 0, err
			}

			if n == 0 {
				n = n1
			}
		}
	}

	if ds.v3 != nil {
		if n > 0 && !*sync {
			go v2tov3Write(data, offset)
		} else {
			n3, err := v2tov3Write(data, offset)
			if err != nil {
				return 0, err
			}

			if n == 0 {
				n = n3
			}
		}
	}

	return n, nil
}

func (*View3) Read(fid *srv.FFid, buf []byte, offset uint64) (int, error) {
	if ds.v3 != nil {
		return v3tov3Read(buf, offset)
	} else if ds.v1 != nil {
		return v1tov3Read(buf, offset)
	} else if ds.v2 != nil {
		return v2tov3Read(buf, offset)
	}

	return 0, errors.New("no complete views available")
}

func (*View3) Write(fid *srv.FFid, data []byte, offset uint64) (int, error) {
	var n int

	if offset%4 != 0 || offset > uint64(ds.acnt)*4 {
		return 0, Ewrite
	}

	if ds.v3 != nil {
		n3, err := v3tov3Write(data, offset)
		if err != nil {
			return 0, err
		}

		n = n3
	} else {
		n = len(data)
	}

	if ds.v1 != nil {
		if n > 0 && !*sync {
			go v3tov1Write(data, offset)
		} else {
			n1, err := v3tov1Write(data, offset)
			if err != nil {
				return 0, err
			}

			if n == 0 {
				n = n1
			}
		}
	}

	if ds.v2 != nil {
		if n > 0 && !*sync {
			go v3tov2Write(data, offset)
		} else {
			n2, err := v3tov2Write(data, offset)
			if err != nil {
				return 0, err
			}

			if n == 0 {
				n = n2
			}
		}
	}

	return n, nil
}

func (*Dsfs) ConnOpened(c *srv.Conn) {
}

func (*Dsfs) ConnClosed(c *srv.Conn) {
	os.Exit(0)
}

func main() {
	var oerr error
	var v1f, v2f, v3f *os.File
	var s *Dsfs
	var v1 *View1
	var v2 *View2
	var v3 *View3
	var root *srv.File

	user := p.OsUsers.Uid2User(os.Geteuid())
	flag.Parse()
	if *arraylen == 0 {
		fmt.Printf("array size not specified\n")
		return
	}

	ds.acnt = *arraylen
	ds.filesz = uint64(ds.acnt) * 3 * 4

	if *v1fname != "" {
		v1f, oerr = os.OpenFile(*v1fname, os.O_RDWR|os.O_CREATE, 0644)
		if oerr != nil {
			goto oerror
		}

		oerr = v1f.Truncate(int64(ds.filesz))
		if oerr != nil {
			goto oerror
		}

		ds.v1, oerr = syscall.Mmap(int(v1f.Fd()), 0, int(ds.filesz), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if oerr != nil {
			goto oerror
		}
	}

	if *v2fname != "" {
		v2f, oerr = os.OpenFile(*v2fname, os.O_RDWR|os.O_CREATE, 0644)
		if oerr != nil {
			goto oerror
		}

		oerr = v2f.Truncate(int64(ds.filesz))
		if oerr != nil {
			goto oerror
		}

		ds.v2, oerr = syscall.Mmap(int(v2f.Fd()), 0, int(ds.filesz), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if oerr != nil {
			goto oerror
		}
	}

	if *v3fname != "" {
		v3f, oerr = os.OpenFile(*v3fname, os.O_RDWR|os.O_CREATE, 0644)
		if oerr != nil {
			goto oerror
		}

		oerr = v3f.Truncate(int64(ds.acnt * 4))
		if oerr != nil {
			goto oerror
		}

		ds.v3, oerr = syscall.Mmap(int(v3f.Fd()), 0, int(ds.acnt*4), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
		if oerr != nil {
			goto oerror
		}
	}

	root = new(srv.File)
	oerr = root.Add(nil, "/", user, nil, p.DMDIR|0555, nil)
	if oerr != nil {
		goto oerror
	}

	v1 = new(View1)
	oerr = v1.Add(root, "legacy", user, nil, 0666, v1)
	if oerr != nil {
		goto oerror
	}
	v1.Length = ds.filesz

	v2 = new(View2)
	oerr = v2.Add(root, "natural", user, nil, 0666, v2)
	if oerr != nil {
		goto oerror
	}
	v2.Length = ds.filesz

	v3 = new(View3)
	oerr = v3.Add(root, "b", user, nil, 0666, v3)
	if oerr != nil {
		goto oerror
	}
	v3.Length = uint64(ds.acnt * 4)

	s = new(Dsfs)
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
