package drepl

//import "fmt"

type ElementOrder interface {
	FromIdx(idx, dim []int64) int64
	ToIdx(n int64, idx, dim []int64)
	Id() int32
}

type RowMajor int
type RowMinor int

var RowMajorOrder RowMajor
var RowMinorOrder RowMinor

func (RowMajor) FromIdx(idx, dim []int64) int64 {
	n := int64(idx[0])
//	fmt.Printf("RowMajor.FromIdx: i %d n %d dim[i] %d idx[i] %d\n", 0, n, dim[0], idx[0])
	for i := 1; i < len(idx); i++ {
		n = n*dim[i] + idx[i]
//		fmt.Printf("RowMajor.FromIdx: i %d n %d dim[i] %d idx[i] %d\n", i, n, dim[i], idx[i])
	}

//	fmt.Printf("RowMajor.FromIdx: return %d\n", n)
	return n
}

func (RowMajor) ToIdx(n int64, idx, dim []int64) {
	for i := len(idx) - 1; i >= 0; i-- {
		idx[i] = n % dim[i]
//		fmt.Printf("RowMajor.ToIdx: i %d n %d dim[i] %d idx[i] %d\n", i, n, dim[i], idx[i])
		n /= dim[i]
	}
}

func (RowMajor) Id() int32 {
	return 1
}

func (RowMinor) FromIdx(idx, dim []int64) int64 {
	n := int64(idx[len(idx) - 1])
	for i := len(idx) - 2; i >= 0; i-- {
		n = n*dim[i] + idx[i]
	}

	return n
}

func (RowMinor) ToIdx(n int64, idx, dim []int64) {
	for i := 0; i < len(idx); i++ {
		idx[i] = n % dim[i]
//		fmt.Printf("RowMinor.ToIdx: i %d n %d dim[i] %d idx[i] %d\n", i, n, dim[i], idx[i])
		n /= dim[i]
	}
}

func (RowMinor) Id() int32 {
	return 2
}

