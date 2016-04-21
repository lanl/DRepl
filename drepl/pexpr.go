package drepl

import (
	"fmt"
)

type PExpr struct {
	// (ax + b) / (cx + d), x variable
	A	int64
	B	int64
	C	int64
	D	int64
	Xidx	int
}

func (p *PExpr) calc(xa []int64) (q int64, r int64) {
	x := xa[p.Xidx]

	// TODO: handle overflows
	n := x*p.A + p.B
	m := x*p.C + p.D
	q = n / m
	r = n % m
	return
}

//func (p *PExpr) String() string {
//	return fmt.Sprintf("(%d | %d, %d, %d, %d)", p.Xidx, p.A, p.B, p.C, p.D)
//}

func (p PExpr) String() string {
	return fmt.Sprintf("(%d | %d, %d, %d, %d)", p.Xidx, p.A, p.B, p.C, p.D)
}
