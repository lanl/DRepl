package parser

import (
	"fmt"
	"os"
	"strconv"
//	"unicode"
//	"utf8"
)

type Parser struct {
	scanner *Scanner
	err	ErrorHandler
	dr	*DRepl
	cview	*View		// the view currently being parsed (nil if not)

	// next token
	pos	Pos
	tok	Token
	lit	[]byte
}

func NewParser(dr *DRepl, sc *Scanner, err ErrorHandler) *Parser {
	ps := new(Parser)
	ps.dr = dr
	ps.scanner = sc
	ps.err = err
	ps.next()

	return ps
}

func (p *Parser) Parse() {
	for p.tok != EOF {
		tok := p.tok
		pos := p.pos

		switch tok {
		case DATASET:
			p.next()
			p.parseDataset()
		case VIEW:
			p.next()
			p.parseView()
		case COMPLETE:
		case REPLICA:
			p.parseReplica()
		case SEMICOLON:
			p.next()
		default:
			p.error(&pos, fmt.Sprintf("invalid token: %d: %v", tok, string(p.lit)))
			return
		}

		p.next()
	}
}

func (p *Parser) next() {
	var pos *Pos

	pos, p.tok, p.lit = p.scanner.Scan()
	p.pos = *pos
}

func (p *Parser) includeFile(fname string) bool {
	var f *os.File

	fi, err := os.Stat(fname)
	if err!=nil {
		p.error(&p.pos, fmt.Sprintf("can't read file '%s': %v", fname, err))
		return false
	}

	buf := make([]byte, fi.Size())
	f, err = os.Open(fname)
	if err!=nil {
		p.error(&p.pos, fmt.Sprintf("can't read file '%s': %v", fname, err))
		return false
	}

	for b:=buf; len(b)>0; {
		var n int

		n, err = f.Read(b)
		if err!=nil {
			p.error(&p.pos, fmt.Sprintf("can't read file '%s': %v", fname, err))
			return false
		}

		b = b[n:]
	}
	f.Close()

	p.scanner.PushFile(fname, buf)
	return true
}

/* dataset related methods */
func (p *Parser) parseDataset() {
	inline := false

	switch p.tok {
	default:
		p.error(&p.pos, fmt.Sprintf("invalid token: %d: %v", p.tok, string(p.lit)))
		return

	case STRING:
		if !p.includeFile(string(p.lit[1:len(p.lit)-1])) {
			return
		}

	case LBRACE:
		inline = true
	}

	p.next()
	for p.tok != EOF && p.tok != RBRACE {
		tok := p.tok
		pos := p.pos

		switch tok {
		case TYPE:
			p.parseDatasetTypeDecl()
		case VAR:
			p.parseDatasetVarDecl()
		case CONST:
			p.parseDatasetConstDecl()
		default:
			p.error(&pos, fmt.Sprintf("invalid token: %d: %v", tok, string(p.lit)))
			return
		}
	}

	if inline && p.tok != RBRACE {
		p.error(&p.pos, "expecting }")
	}
}

func (p *Parser) parseDatasetTypeDecl() {
	p.next()
	if p.tok != IDENT {
		p.error(&p.pos, "identifier expected")
		return
	}

	name := string(p.lit)
	if td, created := p.dr.Dataset.createType(name, &p.pos); created {
		p.next()
		p.parseDatasetType(td)
	} else {
		p.error(&p.pos, fmt.Sprintf("type '%s' already defined at %v", name, &td.pos))
	}
}

func (p *Parser) parseDatasetType(t *Type) bool {
	ds := p.dr.Dataset

	switch p.tok {
	case IDENT:
		// alias
		t1 := ds.getType(string(p.lit))
		if t1 == nil {
			p.error(&p.pos, fmt.Sprintf("type '%v' not defined", string(p.lit)))
			return false
		}

		t.etype = t1
		t.dimnum = 0
		p.next()

	case LBRACK:
		// matrix
		if !p.parseArrayLengths(t) {
			return false
		}

		if p.tok != RBRACK {
			p.error(&p.pos, "expecting ]")
			return false
		}

		p.next()
		t.etype = new(Type)
		return p.parseDatasetType(t.etype)

	case STRUCT:
		// struct
		p.next()
		return p.parseStruct(t)

	default:
		p.error(&p.pos, "expecting type")
		return false
	}

	return true
}

func (p *Parser) parseArrayLengths(t *Type) bool {
	dimnum := 0
	dim := make([]*Expr, 8)
	for p.tok != EOF {
		if dimnum >= len(dim) {
			d := make([]*Expr, len(dim) + 8)
			copy(d, dim)
			dim = d
		}

		p.next()
		if p.tok != COMMA && p.tok != RBRACK {
			dim[dimnum] = p.parseExpr(true)
			if dim[dimnum]==nil {
				return false
			}

		}

		dimnum++
		if p.tok == RBRACK {
			break
		}
	}

	t.dimnum = dimnum
	t.dimexpr = dim
	return true
}

func (p *Parser) parseExpr(constexpr bool) *Expr {
	var err error

	e := new(Expr)
	e.op = ILLEGAL
	ds := p.dr.Dataset
	vw := p.cview

	// left operand
	switch p.tok {
	case LPAREN:
		p.next()
		e = p.parseExpr(constexpr)
		if e == nil {
			return nil
		}

		if p.tok != RPAREN {
			p.error(&p.pos, "expecting )")
			return nil
		}

	case IDENT:
		e.op = p.tok
		name := string(p.lit)
		if constexpr {
			c, _ := ds.createConstDecl(name, nil)
			e.val = c
		} else {
			if c := ds.getConstDecl(name); c!=nil {
				e.val = c
			} else {
				e.val, _ = vw.createVarDecl(name, nil)
			}
		}

		p.next()

	case INT:
		s := string(p.lit)
		e.val, err = strconv.ParseInt(s, 0, 64)
		if err == strconv.ErrRange {
			e.val, err = strconv.ParseUint(s, 0, 64)
		}
		p.next()

	case FLOAT:
		e.val, err = strconv.ParseFloat(string(p.lit), 64)
		p.next()

	case STRING:
		e.val, err = strconv.Unquote(string(p.lit))
		p.next()

	default:
		p.error(&p.pos, "invalid expression")
		return nil
	}

	if err!=nil {
		p.error(&p.pos, err.Error())
		return nil
	}

	// operator
	switch p.tok {
	case ADD, SUB, MUL, QUO, REM:
		e1 := new(Expr)
		e1.left = e
		e1.op = p.tok
		p.next()
		e1.right = p.parseExpr(constexpr)
		e = e1
	}

	return e
}

func (p *Parser) parseStruct(t *Type) bool {
	if p.tok != LBRACE {
		p.error(&p.pos, "expecting {")
		return false
	}

	ds := p.dr.Dataset
	p.next()
	fnames := make([]string, 16)
	for p.tok == IDENT {
		ftype := (*Type)(nil)
		n := 0
		pos := p.pos

		// read the fields
		for p.tok == IDENT {
//			log.Println("field", n, "name", string(p.lit), "...", p.lit)
			if n>len(fnames) {
				f := make([]string, len(fnames) + 16)
				copy(f, fnames)
				fnames = f
			}

			fnames[n] = string(p.lit)
			n++
			p.next()
			if p.tok != COMMA {
				break
			}

			p.next()
		}

		// read the type
		switch p.tok {
		case IDENT:
			ftype, _ = ds.createType(string(p.lit), nil)
			p.next()

		default:
			ftype, _ = ds.createType("", &pos)
			if !p.parseDatasetType(ftype) {
				return false
			}
		}

		if !t.addFields(fnames[0:n], ftype, &pos) {
			return false
		}

		if p.tok == SEMICOLON {
			p.next()
		}
	}

	if p.tok != RBRACE {
		p.error(&p.pos, "expecting }")
		return false
	}

	p.next()
	return true
}

func (p *Parser) parseDatasetVarDecl() bool {
	ds := p.dr.Dataset
	vnames := make([]string, 16)
	vtype := (*Type)(nil)
	n := 0
	pos := p.pos
	p.next()
	for p.tok == IDENT {
		if n>len(vnames) {
			v := make([]string, len(vnames) + 8)
			copy(v, vnames)
			vnames = v
		}

		vnames[n] = string(p.lit)
		n++

		p.next()
		if p.tok != COMMA {
			break
		}

		p.next()
	}

	switch p.tok {
	case IDENT:
		vtype, _ = ds.createType(string(p.lit), nil)
		p.next()

	default:
		vtype, _ = ds.createType("", &pos)
		if !p.parseDatasetType(vtype) {
			return false
		}
	}

	for i:=0; i<n; i++ {
		name := vnames[i]
		if v, created := ds.createVarDecl(name, &pos); !created {
			p.error(&p.pos, fmt.Sprintf("variable '%v' already defined at %v", name, &v.pos))
			return false
		} else {
			v.t = vtype
		}
	}

	if p.tok == SEMICOLON {
		p.next()
	}

	return true
}

func (p *Parser) parseDatasetConstDecl() {
	ds := p.dr.Dataset
	p.next()
	if p.tok != IDENT {
		p.error(&p.pos, "expecting identifier")
		return
	}

	name := string(p.lit)
	c, created := ds.createConstDecl(name, &p.pos)
	if !created && c.isDefined() {
		p.error(&p.pos, fmt.Sprintf("constant '%s' already defined at %v", name, &c.pos))
		return
	}

	p.next()
	if p.tok != ASSIGN {
		p.error(&p.pos, "expecting =")
		return
	}

	p.next()
	c.expr = p.parseExpr(true)
	if c.expr==nil {
		return
	}

	if p.tok == SEMICOLON {
		p.next()
	}
}

func (p *Parser) parseView() {
	var vw *View

	inline := false

	if p.tok != IDENT {
		p.error(&p.pos, "expecting view name")
		return
	}

	name := string(p.lit)
	p.next()

	// view flags
	flags := 0
l1:	for {
		switch p.tok {
		case ROWMAJOR:
			flags |= Vrowmajor

		case COLMAJOR:
			flags |= Vrowminor

		case DEFAULT:
			flags |= Vdefault

		case READONLY:
			flags |= Vreadonly

		case IDENT:
			// for backward compatibility
			s := string(p.lit)
			if s=="rowmajor" {
				flags |= Vrowmajor
			} else if s=="rowminor" {
				flags |= Vrowminor
			} else if s=="default" {
				flags |= Vdefault
			} else {
				p.error(&p.pos, fmt.Sprintf("undefined view flag: %s", s))
				goto done
			}

		default:
			break l1
//			p.error(&p.pos, fmt.Sprintf("unexpected literal: %s", p.lit))
//			goto done
		}

		p.next()
	}

//	fmt.Printf("view %s flags %x\n", name, flags)
	vw = p.dr.createView(name, flags)
	if vw==nil {
		p.error(&p.pos, fmt.Sprintf("cannot create view '%s'", name))
		return
	}

	p.cview = vw
	switch p.tok {
	default:
		p.error(&p.pos, fmt.Sprintf("invalid token: %d: %v", p.tok, string(p.lit)))
		goto done

	case STRING:
		if !p.includeFile(string(p.lit[1:len(p.lit)-1])) {
			goto done
		}

	case LBRACE:
		inline = true
	}

	p.next()
	for p.tok != EOF && p.tok != RBRACE {
		tok := p.tok
		pos := p.pos

		switch tok {
		case TYPE:
			p.parseViewTypeDecl()
		case VAR:
			p.parseViewVarDecl()
		case SEMICOLON:
			p.next()
		default:
			p.error(&pos, fmt.Sprintf("invalid token: %d: %v", tok, string(p.lit)))
			goto done
		}
	}

	if inline && p.tok != RBRACE {
		p.error(&p.pos, "expecting }")
	}

done:
	p.cview = nil
}

func (p *Parser) parseViewTypeDecl() {
	vw := p.cview
	ds := p.dr.Dataset
	p.next()
	if p.tok != IDENT {
		p.error(&p.pos, "identifier expected")
		return
	}

	name := string(p.lit)
	if td := ds.getType(name); td != nil {
		p.error(&p.pos, fmt.Sprintf("type '%s' already defined at %v", name, td.pos))
		return
	}

	p.next()
	td, ok := p.parseViewType()
	if !ok {
		return
	}

	if td.dt == nil {
		p.error(&p.pos, fmt.Sprintf("dataset type must be specified when defining view type"))
	}
	
	if !vw.addType(name, td) {
		p.error(&p.pos, fmt.Sprintf("type '%s' already defined at %v", name, td.pos))
	}

	if p.tok == SEMICOLON {
		p.next()
	}
}

func (p *Parser) parseViewType() (*VType, bool) {
	vw := p.cview
	ds := p.dr.Dataset

//	fmt.Printf("parseViewType\n")
	switch p.tok {
	case IDENT:
		name := string(p.lit)
		if t1 := ds.getType(name); t1 != nil {
			t,_ := vw.createType("", &p.pos)
			t.dt = t1
			p.next()
			return t, true
		} else if vt := vw.getType(name); vt != nil {
			p.next()
			return vt, true
		} else {
			p.error(&p.pos, fmt.Sprintf("type '%s' not defined", name))
			return nil, false
		}

	case LBRACK:
		// VArrayType
		t, _ := vw.createType("", &p.pos)
		if !p.parseVArrayLengths(t) {
			return nil, false
		}

		if p.tok != RBRACK {
			p.error(&p.pos, "expecting ]")
			return nil, false
		}

		p.next()

		var ok bool
		t.etype, ok = p.parseViewType()
		if !ok {
			return nil, false
		}
		return t, true

	case LBRACE:
		// VStructType
		p.next()
		return p.parseVStruct()

/*
	case SEMICOLON:
		// <nothing>
	default:
		p.error(&p.pos, "expecting type")
		return false
*/
	}

	return nil, true
}

func (p *Parser) parseVArrayLengths(t *VType) bool {
	dimnum := 0
	dim := make([]*Expr, 8)
	for p.tok != EOF {
		if dimnum >= len(dim) {
			d := make([]*Expr, len(dim) + 8)
			copy(d, dim)
			dim = d
		}

		p.next()
		if p.tok != COMMA && p.tok != RBRACK {
			dim[dimnum] = p.parseVExpr(t)
			if dim[dimnum]==nil {
				return false
			}

		}

		dimnum++
		if p.tok == RBRACK {
			break
		}
	}

	t.dim = dim[0:dimnum]
	
	return true
}

func (p *Parser) parseVStruct() (*VType, bool) {
	vw := p.cview
//	if p.tok != LBRACE {
//		p.error(&p.pos, fmt.Sprintf("expecting {, got %d", p.tok))
//		return false
//	}

//	p.next()
	t, _ := vw.createType("", &p.pos)
	fnames := make([]string, 16)
	for p.tok == IDENT {
		ftype := (*VType)(nil)
		n := 0
		pos := p.pos

		// read the fields
		for p.tok == IDENT {
			if n>len(fnames) {
				f := make([]string, len(fnames) + 16)
				copy(f, fnames)
				fnames = f
			}

			fnames[n] = string(p.lit)
			n++
			p.next()
			if p.tok != COMMA {
				break
			}

			p.next()
		}

		// read the type
		var ok bool
		ftype, ok = p.parseViewType()
		if !ok {
			return nil, false
		}

		if !t.addFields(fnames[0:n], ftype, &pos) {
			return nil, false
		}

		if p.tok == SEMICOLON {
			p.next()
		}
	}

	if p.tok != RBRACE {
		p.error(&p.pos, "expecting }")
		return nil, false
	}

	p.next()
	if p.tok == IDENT {
		name := string(p.lit)
		if t1 := p.dr.Dataset.getType(name); t1 != nil {
			t.dt = t1
		} else {
			p.error(&p.pos, fmt.Sprintf("type '%s' not defined", name))
			return nil, false
		}
		p.next()
	}

	return t, true
}

func (p *Parser) parseViewVarDecl() {
	ds := p.dr.Dataset
	vw := p.cview
	vt := (*VType)(nil)
	pos := p.pos
	p.next()
	if p.tok != IDENT {
		p.error(&p.pos, "identifier expected")
		return
	}

	vname := string(p.lit)
	p.next()
	if p.tok == IDENT {
		vt = vw.getType(string(p.lit))
	}

	if vt==nil {
		var ok bool
		vt, ok = p.parseViewType()
		if !ok {
			return
		}
	}

	v, created := vw.createVarDecl(vname, &pos)
	if !created {
		p.error(&p.pos, fmt.Sprintf("variable '%v' already defined at %v", vname, &v.pos))
		return
	}

	v.lt = vt
	if p.tok != ASSIGN {
		p.error(&p.pos, fmt.Sprintf("expecting =, got %v", string(p.lit)))
		return
	}

	p.next()
	if p.tok != IDENT {
		p.error(&p.pos, "identifier expected")
		return
	}

	v.v = ds.getVarDecl(string(p.lit))
	if v.v == nil {
		p.error(&p.pos, fmt.Sprintf("variable '%s' not found", string(p.lit)))
		return
	}

	p.next()	
	vt = (*VType)(nil)
	if p.tok == IDENT {
		vt = vw.getType(string(p.lit))
	}

	if vt==nil {
		// are we too flexible here? we should probably only allow array type
		var ok bool
		vt, ok = p.parseViewType()
		if !ok {
			return
		}
	}

	v.rt = vt
	if p.tok == SEMICOLON {
		p.next()
	}

	return

}

func (p *Parser) parseVExpr(t *VType) *Expr {
	var err error

	e := new(Expr)
	ds := p.dr.Dataset

	// left operand
	switch p.tok {
	case LPAREN:
		p.next()
		e = p.parseVExpr(t)
		if e == nil {
			return nil
		}

		if p.tok != RPAREN {
			p.error(&p.pos, "expecting )")
			return nil
		}
		p.next()

	case IDENT:
		e.op = p.tok
		name := string(p.lit)
		if c := ds.getConstDecl(name); c!=nil {
			e.val = &c.EVar
		} else {
			temp, _ := t.createTemp(name, &p.pos)
			e.val = &temp.EVar
		}

		p.next()

	case INT:
		s := string(p.lit)
		e.val, err = strconv.ParseInt(s, 0, 64)
		if err == strconv.ErrRange {
			e.val, err = strconv.ParseUint(s, 0, 64)
		}
		p.next()

	case FLOAT:
		e.val, err = strconv.ParseFloat(string(p.lit), 64)
		p.next()

	case STRING:
		e.val, err = strconv.Unquote(string(p.lit))
		p.next()

	default:
		p.error(&p.pos, "invalid expression")
		return nil
	}

	if err!=nil {
		p.error(&p.pos, err.Error())
		return nil
	}

	// operator
	switch p.tok {
	case ADD, SUB, MUL, QUO, REM:
		e1 := new(Expr)
		e1.left = e
		e1.op = p.tok
		p.next()
		e1.right = p.parseVExpr(t)
		e = e1
	}

	return e
}

func (p *Parser) parseReplica() {
	if p.tok != REPLICA {
		p.error(&p.pos, "'replica' keyword expected")
		return
	}
	p.next()

	flags := 0

L:
	for p.tok != IDENT {
		switch p.tok {
		default:
			break L
		case COMPLETE:
			flags |= Rcomplete
		case READONLY:
			flags |= Rreadonly
		}
		p.next()
	}

	if p.tok != IDENT {
		p.error(&p.pos, "replica name expected")
		return
	}

	name := string(p.lit)
	p.next()

	fname := name
	if p.tok == STRING {
		fname, _ = strconv.Unquote(string(p.lit))
		p.next()
	}

	r := p.dr.createReplica(name, fname, flags)
	if r==nil {
		p.error(&p.pos, fmt.Sprintf("cannot create replica '%s'", name))
		return
	}

	if p.tok != LBRACE {
		p.error(&p.pos, "{ expected")
	}

	p.next()
	for p.tok != RBRACE {
		if p.tok == VIEW {
			p.parseReplicaViewDecl(r)
		} else {
			p.error(&p.pos, fmt.Sprintf("invalid token: %d: %v", p.tok, string(p.lit)))
			return
		}
	}
}

func (p *Parser) parseReplicaViewDecl(r *Repl) {
	p.next()
	if p.tok != IDENT {
		p.error(&p.pos, "view name expected")
		return
	}

	name := string(p.lit)
	v := p.dr.findView(name)
	if v==nil {
		p.error(&p.pos, fmt.Sprintf("can't find view '%s'", name))
		return
	}

	r.addView(v)
	p.next()
	if p.tok == SEMICOLON {
		p.next()
	}
}

func (p *Parser) error(pos *Pos, msg string) {
	if p.err != nil {
		p.err.Error(pos, msg)
	}
}
