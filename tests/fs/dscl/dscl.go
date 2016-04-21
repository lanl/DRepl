package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

var mntpt = flag.String("m", ".", "mountpoint")
var check = flag.Bool("c", false, "check file content")
var lpath = flag.String("l", "", "legacy file path")
var npath = flag.String("n", "", "natural file path")
var bpath = flag.String("b", "", "b file path")

var ops string
var arraycount int
var f1, f2, f3 *os.File
var a []byte
var b []byte
var c []byte
var ds []byte

func usage() {
	fmt.Printf("Usage: dscl -f fname ops\n")
}

func aval(idx int, a []byte) {
	a[0] = byte(idx/1000) + 3
	a[1] = byte(idx/56) - 34
	a[2] = byte(idx%584) + 1
	a[3] = byte(idx%23) + 45
}

func bval(idx int, b []byte) {
	b[0] = byte(idx/1000) + 7
	b[1] = byte(idx/11) - 99
	b[2] = byte(idx%603) + 23
	b[3] = byte(idx%22) + 80
}

func cval(idx int, c []byte) {
	c[0] = byte(idx/591) + 9
	c[1] = byte(idx/23) - 22
	c[2] = byte(idx%609) + 5
	c[3] = byte(idx%67) + 93
}

func readView1() error {
	n, err := f1.ReadAt(a, 0)
	if err != nil || n != len(a) {
		goto error
	}

	n, err = f1.ReadAt(b, int64(arraycount)*4)
	if err != nil || n != len(b) {
		goto error
	}

	n, err = f1.ReadAt(c, int64(arraycount)*4*2)
	if err != nil || n != len(c) {
		goto error
	}

	if *check {
		val := make([]byte, 4)
		for i := 0; i < arraycount; i++ {
			aval(i, val)
			for j := 0; j < len(val); j++ {
				if val[j] != a[i*4+j] {
					return errors.New(fmt.Sprintf("a[%d] incorrect: %v %v\n", i, val, a[i*4:i*4+4]))
				}
			}

			bval(i, val)
			for j := 0; j < len(val); j++ {
				if val[j] != b[i*4+j] {
					return errors.New(fmt.Sprintf("b[%d] incorrect: %v %v\n", i, val, b[i*4:i*4+4]))
				}
			}

			cval(i, val)
			for j := 0; j < len(val); j++ {
				if val[j] != c[i*4+j] {
					return errors.New(fmt.Sprintf("c[%d] incorrect: %v %v\n", i, val, c[i*4:i*4+4]))
				}
			}
		}
	}

	return nil

error:
	if err == nil {
		err = errors.New("short read")
	}

	return err
}

func readView2() error {
	n, err := f2.ReadAt(ds, 0)
	if err != nil || n != len(ds) {
		goto error
	}

	if *check {
		val := make([]byte, 12)
		for i := 0; i < arraycount; i++ {
			aval(i, val[0:4])
			bval(i, val[4:8])
			cval(i, val[8:])

			for j := 0; j < len(val); j++ {
				if val[j] != ds[i*12+j] {
					return errors.New(fmt.Sprintf("ds[%d] incorrect: %v %v\n", i, val, ds[i*12:i*12+12]))
				}
			}
		}
	}

	return nil

error:
	if err == nil {
		err = errors.New("short read")
	}

	return err
}

func readView3() error {
	n, err := f3.ReadAt(b, 0)
	if err != nil || n != len(b) {
		goto error
	}

	if *check {
		val := make([]byte, 4)
		for i := 0; i < arraycount; i++ {
			bval(i, val)
			for j := 0; j < len(val); j++ {
				if val[j] != b[i*4+j] {
					return errors.New(fmt.Sprintf("b[%d] incorrect: %v %v\n", i, val, b[i*4:i*4+4]))
				}
			}
		}
	}

	return nil

error:
	if err == nil {
		err = errors.New("short read")
	}

	return err
}

func writeView1() error {
	if *check {
		val := make([]byte, 4)
		for i := 0; i < arraycount; i++ {
			aval(i, val)
			copy(a[i*4:], val)
			bval(i, val)
			copy(b[i*4:], val)
			cval(i, val)
			copy(c[i*4:], val)
		}
	}

	n, err := f1.WriteAt(a, 0)
	if err != nil || n != len(a) {
		goto error
	}

	n, err = f1.WriteAt(b, int64(arraycount)*4)
	if err != nil || n != len(b) {
		goto error
	}

	n, err = f1.WriteAt(c, int64(arraycount)*4*2)
	if err != nil || n != len(c) {
		goto error
	}

	return nil

error:
	if err == nil {
		err = errors.New("short write")
	}

	return err
}

func writeView2() error {
	if *check {
		val := make([]byte, 4)
		for i := 0; i < arraycount; i++ {
			aval(i, val)
			copy(ds[i*12:], val)
			bval(i, val)
			copy(ds[i*12+4:], val)
			cval(i, val)
			copy(ds[i*12+8:], val)
		}
	}

	n, err := f2.WriteAt(ds, 0)
	if err != nil || n != len(ds) {
		goto error
	}

	return nil

error:
	if err == nil {
		err = errors.New("short write")
	}

	return err
}

func writeView3() error {
	if *check {
		val := make([]byte, 4)
		for i := 0; i < arraycount; i++ {
			bval(i, val)
			copy(b[i*4:], val)
		}
	}

	n, err := f3.WriteAt(b, 0)
	if err != nil || n != len(a) {
		goto error
	}

	return nil

error:
	if err == nil {
		err = errors.New("short write")
	}

	return err
}

func main() {
	var fi os.FileInfo
	var err error

	flag.Parse()
	if flag.NArg() != 1 {
		usage()
		return
	}

	ops = flag.Arg(0)
//	fmt.Printf("trying to open %s\n", *mntpt+"/legacy")
	lfile := *mntpt + "/legacy"
	nfile := *mntpt + "/natural"
	bfile := *mntpt + "/b"
	if *lpath != "" {
		lfile = *lpath
	}

	f1, err = os.OpenFile(lfile, os.O_RDWR/*|os.O_CREATE*/, 0644)
	if err != nil {
		goto error
	}

	fi, err = f1.Stat()
	if err != nil {
		goto error
	}

	arraycount = int(fi.Size() / (3 * 4))
	if int64(arraycount)*3*4 != fi.Size() {
		fmt.Printf("invalid file size\n")
		return
	}

	if *npath != "" {
		nfile = *npath
	}

	f2, err = os.OpenFile(nfile, os.O_RDWR/*|os.O_CREATE*/, 0644)
	if err != nil {
		goto error
	}

	if *bpath != "" {
		bfile = *bpath
	}

	f3, err = os.OpenFile(bfile, os.O_RDWR/*|os.O_CREATE*/, 0644)
	if err != nil {
		goto error
	}

	a = make([]byte, arraycount*4)
	b = make([]byte, arraycount*4)
	c = make([]byte, arraycount*4)
	ds = make([]byte, arraycount*4*3)

	for _, c := range ops {
		st := time.Now().UnixNano() / 1000
		switch c {
		case 'r':
			err = readView1()

		case 'w':
			err = writeView1()

		case 'R':
			err = readView2()

		case 'W':
			err = writeView2()

		case 'b':
			err = readView3()

		case 'B':
			err = writeView3()

		default:
			fmt.Printf("invalid op: %c\n", c)
			break
		}
		et := time.Now().UnixNano() / 1000

		if err != nil {
			goto error
		}

		fmt.Printf("%c %v %v\n", c, len(ds), et-st)
	}

	return

error:
	fmt.Printf("Error: %v\n", err)
	return
}
