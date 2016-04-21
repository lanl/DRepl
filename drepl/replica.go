package drepl

import (
	"fmt"
_	"io"
	"os"
	"syscall"
	"unsafe"
)

type Replica struct {
	Name		string
	FileName	string
	views		[]*View
	f		*os.File

	data		[]byte
	s, e		int64		// start and end of the dirty region in data
}

func NewReplica(name, fname string) *Replica {
	r := new(Replica)
	r.Name = name
	r.FileName = fname

	return r
}

/*
func (r *Replica) NewView(name string, elo ElementOrder) *View {
	v := new(View)

	v.repl = r
	v.offset = r.Size()
	v.Name = name
	v.elo = elo
	v.bs.v = v
	r.Views = append(r.Views, v)
	return v
}
*/

func (r *Replica) AddView(v *View) {
	v.repl = r
	v.offset = r.Size()
	r.views = append(r.views, v)
//	fmt.Printf("Replica.AddView %p view %p\n", r, v)
}

func (r *Replica) Size() int64 {
	if r.views == nil || len(r.views) == 0 {
		return 0
	}

	v := r.views[len(r.views) - 1]
//	fmt.Printf("Replica.Size view %p %d\n", v, v.offset + v.Size())
	return v.offset + v.Size()
}

func (r *Replica) SetFile(f *os.File) error {
	r.f = f

	sz := r.Size()
//	fmt.Printf("Replica.SetFile: size %d\n", sz)
	err := f.Truncate(sz)
	if err != nil {
		return err
	}

	r.data, err = syscall.Mmap(int(f.Fd()), 0, int(sz), syscall.PROT_READ|syscall.PROT_WRITE, syscall.MAP_SHARED)
	if err != nil {
		return err
	}

	return nil
}

func (r *Replica) Sync() error {
	if r==nil {
		return nil
	}

	if r.s >= r.e {
		return nil
	}

	start := uintptr(unsafe.Pointer(&r.data[r.s])) &^ (0xfff) // start address needs to be page-aligned
	end := uintptr(unsafe.Pointer(&r.data[r.e-1]))
	_, _, e1 := syscall.Syscall(syscall.SYS_MSYNC, start, end-start, uintptr(syscall.MS_SYNC))
	errno := int(e1)
	if errno != 0 {
		return syscall.Errno(errno)
	}

	r.s = 0
	r.e = 0
	return nil
}


func (r *Replica) Read(buf []byte, offset int64) (int64, error) {
//	fmt.Printf("Replica.Read offset %d count %d\n", offset, len(buf))
	if offset > int64(len(r.data)) {
		return 0, nil
	}

	return int64(copy(buf, r.data[offset:offset + int64(len(buf))])), nil
}

func (r *Replica) Write(data []byte, offset int64) (int64, error) {
	if r.s > offset {
		r.s = offset
	}

	if r.e < offset + int64(len(data)) {
		r.e = offset + int64(len(data))
	}

	return int64(copy(r.data[offset:], data)), nil
}

func (r *Replica) String() string {
	s := fmt.Sprintf("Replica '%s':\n", r.Name)
	for _, v := range r.views {
		s = fmt.Sprintf("%s\t%v\n", s, v)
	}

	return s
}
