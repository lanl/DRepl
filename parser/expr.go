package parser

import (
	"fmt"
	"strconv"
	"drepl/drepl"
)

type Expr struct {
	op		Token
	val		interface{}
	left, right	*Expr
}

type EVar struct {
	name		string
	eval		bool			// true if the value is being evaluated
	expr		*Expr
	val		interface{}

	aux		interface{}
}

type Range struct {
	b		int64
	t		int64
}

func evalInt64(l, r interface{}, op Token) (val interface{}, err string) {
	n, nok := l.(int64)
	m, mok := r.(int64)

	if !nok || !mok {
		return nil, ""
	}

	//	fmt.Printf("evalInt: op %d\n", op)
	switch op {
	case ADD:
		val = n + m
	case SUB:
		val = n - m
	case MUL:
		val = n * m
	case QUO:
		if m == 0 {
			err = "division by zero"
		} else {
			val = n / m
		}

	case REM:
		if m == 0 {
			err = "division by zero"
		} else {
			val = n % m
		}
	}

	return
}

func evalUint64(l, r interface{}, op Token) (val interface{}, err string) {
	var n, m uint64
	var ok bool

	if n, ok = l.(uint64); !ok {
		if i, iok := l.(int64); iok {
			// check if we can convert it to uint64
			if i < 0 {
				return nil, "can't convert to uint"
			}

			n = uint64(i)
		} else {
			return nil, ""
		}
	}

	if m, ok = r.(uint64); !ok {
		if i, iok := r.(int64); iok {
			// check if we can convert it to uint64
			if i < 0 {
				return nil, "can't convert to uint"
			}

			m = uint64(i)
		} else {
			return nil, ""
		}
	}

	//	fmt.Printf("evalUint: op %d\n", op)
	switch op {
	case ADD:
		val = n + m
	case SUB:
		val = n - m
	case MUL:
		val = n * m
	case QUO:
		if m == 0 {
			err = "division by zero"
		} else {
			val = n / m
		}

	case REM:
		if m == 0 {
			err = "division by zero"
		} else {
			val = n % m
		}
	}

	return
}

func evalFloat(l, r interface{}, op Token) (val interface{}, err string) {
	var a, b float64
	var ok bool

	if a, ok = l.(float64); !ok {
		if i, iok := l.(int64); iok {
			a = float64(i)
		} else if u, uok := l.(uint64); uok {
			a = float64(u)
		} else {
			return nil, ""
		}
	}

	if b, ok = r.(float64); !ok {
		if i, iok := r.(int64); iok {
			b = float64(i)
		} else if u, uok := r.(uint64); uok {
			b = float64(u)
		} else {
			return nil, ""
		}
	}

	//	fmt.Printf("evalFloat: op %d\n", op)
	switch op {
	case ADD:
		val = a + b
	case SUB:
		val = a - b
	case MUL:
		val = a * b
	case QUO:
		if b == 0 {
			err = "division by zero"
		} else {
			val = a / b
		}

	case REM:
		err = "invalid operator"
	}

	return
}

func (e *Expr) eval() (val interface{}, err string) {
	if e == nil {
		return nil, ""
	}

//	fmt.Printf("\neval: %v\n", e)
	val = e.val
	err = ""
	if val != nil {
		if v, ok := val.(*EVar); ok {
			val, err = evalVar(v)
			if err!="" {
				return nil, err
			}
		} else if b, ok := val.([]byte); ok {
			var oe error

			s := string(b)
			//			fmt.Printf("evalExpr: %p %d\n", e, e.op)
			switch e.op {
			case INT:
				val, oe = strconv.ParseInt(s, 0, 64)
				if oe == strconv.ErrRange {
					val, oe = strconv.ParseUint(s, 0, 64)
				}

				if oe != nil {
					return nil, oe.Error()
				}
				break

			case FLOAT:
				val, oe = strconv.ParseFloat(s, 64)
				if oe != nil {
					return nil, oe.Error()
				}

			case STRING:
				val, oe = strconv.Unquote(s)
				if oe != nil {
					return nil, oe.Error()
				}
			}
		}

//		// store the value so we don't evaluate it next time
//		e.val = val
		return
	}

	switch e.op {
	case ADD, SUB, MUL, QUO, REM:
		lv, le := e.left.eval()
		rv, re := e.right.eval()
		if le != "" {
			return nil, le
		}

		if re != "" {
			return nil, re
		}

		// first try int64 result
		if val, err = evalInt64(lv, rv, e.op); val != nil || err != "" {
//			e.val = val
			return
		}

		// then uint64
		if val, err = evalUint64(lv, rv, e.op); val != nil || err != "" {
//			e.val = val
			return
		}

		if val, err = evalFloat(lv, rv, e.op); val != nil || err != "" {
//			e.val = val
			return
		}

		err = fmt.Sprintf("invalid operand type(s): %v %v", lv, rv)

	default:
		err = fmt.Sprintf("invalid operator: %d", e.op)
	}

	return
}

func evalVar(v *EVar) (val interface{}, err string) {
	if v.val != nil {
		return v.val, ""
	}

	if v.eval {
		return nil, fmt.Sprintf("using variable %s while evaluating it", v.name)
	}

	v.eval = true
	v.val, err = v.expr.eval()
	return v.val, err
}

func (e *Expr) clone(ep **Expr) (*Expr, **Expr) {
	var lep, rep, eep **Expr

	if e==nil {
		return nil, nil
	}

	ne := new(Expr)
	if ep!=nil {
		if ep == &e.left {
			eep = &ne.left
		} else if ep == &e.right {
			eep = &ne.right
		}
	}

	ne.op = e.op
	ne.val = e.val
	ne.left, lep = e.left.clone(ep)
	ne.right, rep = e.right.clone(ep)
	if lep!=nil {
		eep = lep
	}

	if rep!=nil {
		eep = rep
	}

	return ne, eep
}

func (e *Expr) transform() (v *EVar, ne *Expr, ep **Expr, err string) {
	var ve, nve *Expr

	if e==nil {
		return
	}

	if e.val != nil {
		var ok bool

		val := e.val
		if v, ok = e.val.(*EVar); ok {
			if v.val == nil {
				return
			} else {
				val = v.val
				v = nil
			}
		}

		ne = new(Expr)
		ne.val = val
		return
	}

	v, ve, ep, err = e.left.transform()
	if err!="" {
		return
	}

	if v==nil {
		nve = ve
		v, ve, ep, err = e.right.transform()
		if v!=nil && e.op==REM {
			err = "no references on the right side of % supported"
			return
		}
	} else {
		var vv *EVar

		vv, nve, _, err = e.right.transform()
		if vv!=nil {
			err = "only one reference allowed"
			return
		}

	}
	if err!="" {
		return
	}

	ne = new(Expr)
	if v==nil {
		// const op const
		ne.op = e.op
		ne.left = ve
		ne.right = nve
		return
	}

	op := Token(0)
	switch e.op {
	case ADD:
		op = SUB
	case SUB:
		op = ADD
	case MUL:
		op = QUO
	case QUO:
		op = MUL
	default:
		err = "invalid operation"
		return
	}

	ne.op = op
	ne.right = nve
	if ep != nil {
		*ep = ne
		ep = &ne.left
		ne = ve
	} else {
		ep = &ne.left
	}

	return
}

func (e *Expr) replace(v *EVar, ee *Expr) (*Expr, bool) {
	if e==nil {
		return nil, false
	}

	if e.val == v {
		return ee, true
	}

	if le, found := e.left.replace(v, ee); found {
		e.left = le
		return e, true
	}

	if re, found := e.right.replace(v, ee); found {
		e.right = re
		return e, true
	}

	return e, false
}

func (e *Expr) solve(right *Expr) (*EVar, *Expr, string) {
	v, le, ep, err := e.transform()
	if err != "" {
		return nil, nil, err
	}

	if v==nil {
		return nil, nil, "nothing to solve"
	}

	if ep!=nil {
		*ep = right
	} else {
		le = right
	}

	return v, le, err
}
func (e *Expr) findVar() (*EVar, string) {
	if e==nil {
		return nil, ""
	}

	if e.val != nil {
		if v, ok := e.val.(*EVar); ok && v.val == nil {
			return v, ""
		}

		return nil, ""
	}

	v1, err1 := e.left.findVar()
	if err1!="" {
		return nil, err1
	}

	v2, err2 := e.right.findVar()
	if err2!="" {
		return nil, err2
	}

	if v1!=nil && v2!=nil && v1!=v2 {
		return nil, "multiple variables not allowed"
	}

	if v1==nil {
		v1 = v2
	}

	return v1, ""
}

func VarExpr(v *EVar) *Expr {
	e := new(Expr)
	e.val = v
	return e
}

func ConstInt64Expr(n int64) *Expr {
	e := new(Expr)
	e.val = n
	return e
}

func AddExpr(e1, e2 *Expr) *Expr {
	e := new(Expr)
	e.op = ADD
	e.left = e1
	e.right = e2
	return e
}

func SubExpr(e1, e2 *Expr) *Expr {
	e := new(Expr)
	e.op = SUB
	e.left = e1
	e.right = e2
	return e
}

func DivExpr(e1, e2 *Expr) *Expr {
	e := new(Expr)
	e.op = QUO
	e.left = e1
	e.right = e2
	return e
}

func MulExpr(e1, e2 *Expr) *Expr {
	e := new(Expr)
	e.op = MUL
	e.left = e1
	e.right = e2
	return e
}

func (e *Expr) String() string {
	if e==nil {
		return "(nil)"
	}

	if e.val != nil {
		if v, ok := e.val.(*EVar); ok {
			return v.name
		} else {
			return fmt.Sprintf("%v", e.val)
		}
	}

	c := " "
	switch e.op {
	case MUL:
		c = "*"
	case ADD:
		c = "+"
	case QUO:
		c = "/"
	case SUB:
		c = "-"
	case REM:
		c = "%"
	default:
		c = "?"
	}

	return fmt.Sprintf("(%s %s %s)", e.left.String(), c, e.right.String())
}

func (e *Expr) printPtr() string {
	if e==nil {
		return "(nil)"
	}

	return fmt.Sprintf("(%p %v %v)", e, e.left.printPtr(), e.right.printPtr())
}

func (e *Expr) toPExpr(pe *drepl.PExpr, vars []EVar) bool {
	var l, r drepl.PExpr

//	fmt.Printf("%stoPExpr %v\n", indent, e)
	if e==nil {
		return false
	}

	if e.val != nil {
		if v, ok := e.val.(*EVar); ok {
			pe.Xidx = -1
			for i := 0; i < len(vars); i++ {
				if &vars[i] == v {
					pe.Xidx = i
					break
				}
			}

			pe.A = 1
			pe.B = 0
			pe.C = 0
			pe.D = 1
//			fmt.Printf("%s(%d, %d, %d, %d)\n", indent, pe.a, pe.b, pe.c, pe.d)
			return true
		}

		if n, ok := e.val.(int64); ok {
			pe.A = 0
			pe.B = n
			pe.C = 0
			pe.D = 1
//			fmt.Printf("%s(%d, %d, %d, %d)\n", indent, pe.a, pe.b, pe.c, pe.d)
			return true
		}

		return false
	}

	if !e.left.toPExpr(&l, vars) {
		return false
	}

	if !e.right.toPExpr(&r, vars) {
		return false
	}

	switch e.op {
	case ADD, SUB:
		if e.op==SUB {
			r.A = -r.A
			r.B = -r.B
		}

		if (l.A*r.C + r.A*l.A) != 0 || l.C*r.C != 0 {
			// x^2 coefficients, can't have them!
			return false
		}

		pe.A = l.A*r.D + l.B*r.C + l.D*r.A + l.C*r.B
		pe.B = l.D*r.B + l.B*r.D
		pe.C = l.C*r.D + l.D*r.C
		pe.D = l.D * r.D
//		fmt.Printf("%s(%d, %d, %d, %d) + (%d, %d, %d, %d) = (%d, %d, %d, %d)\n", indent, l.a, l.b, l.c, l.d, r.a, r.b, r.c, r.d, pe.a, pe.b, pe.c, pe.d)
		
	case MUL, QUO:
		if e.op==QUO {
			t := r.A
			r.A = r.C
			r.C = t
			t = r.B
			r.B = r.D
			r.D = t
		}

		if l.A*r.A != 0 || l.C*r.C != 0 {
			// x^2 coefficients, can't have them
			return false
		}

		pe.A = l.A*r.B + l.B*r.A
		pe.B = l.B * r.B
		pe.D = l.C*r.D + l.D*r.C
		pe.D = l.D * r.D
//		fmt.Printf("%s(%d, %d, %d, %d) * (%d, %d, %d, %d) = (%d, %d, %d, %d)\n", indent, l.a, l.b, l.c, l.d, r.a, r.b, r.c, r.d, pe.a, pe.b, pe.c, pe.d)

	default:
		return false
	}

	return true
}

