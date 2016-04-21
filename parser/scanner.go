package parser

import (
	"fmt"
//	"log"
//	"strconv"
	"unicode"
	"unicode/utf8"
)

type Token int

const (
	// special
	ILLEGAL Token = iota
	EOF
	COMMENT

	// literals
	litstart
	IDENT
	INT
	FLOAT
	STRING
	litend

	// operators
	opstart
	ADD		// +
	SUB		// -
	MUL		// *
	QUO		// /
	REM		// %

	LPAREN		// (
	LBRACE		// {
	LBRACK		// [
	RPAREN		// )
	RBRACE		// }
	RBRACK		// ]
	COMMA		// ,
	SEMICOLON	// ;
	COLON		// :
	ASSIGN		// =

	NOT		// !
	NEQ		// !=
	EQL		// ==
	AND		// &&
	OR		// ||
	opend

	// keywords
	keystart
	COMPLETE
	CONST
	DATASET
	DEFAULT
	REPLICA
	STRUCT
	TYPE
	VAR
	VIEW
	READONLY
	ROWMAJOR
	COLMAJOR
	keyend
)

const ( // scanner modes
	ScanComments	= 1<< iota
	InsertSemis
)

var keywords = map[string] Token {
	"columnmajor":	COLMAJOR,
	"complete":	COMPLETE,
	"const":	CONST,
	"dataset":	DATASET,
	"default":	DEFAULT,
	"readonly":	READONLY,
	"replica":	REPLICA,
	"rowmajor":	ROWMAJOR,
	"struct":	STRUCT,
	"type":		TYPE,
	"var":		VAR,
	"view":		VIEW,
}

type Pos struct {
	fname		string		// name of the file
	data		[]byte		// content of the file
	offset		int
	line		int
}

type Scanner struct {
	err		ErrorHandler	// error handler or nil
	mode		uint		// scanning mode

	c		rune		// current character
	pos		Pos		// current offset
	rdpos		Pos		// offset of the next character
	insertSemi	bool		// insert a semicolon before next newline

	fnum		int		// number of files in the file stack
	fstack		[2]Pos
}

type ErrorHandler interface {
	Error(pos *Pos, msg string)
}

func (s *Scanner) error(pos *Pos, msg string) {
	if s.err != nil {
		s.err.Error(pos, msg)
	}
}

func (s *Scanner) nextChar() {
	p := s.rdpos
//	log.Println("nextChar start offset", p.offset, "len", len(p.data))
	if p.offset >= len(p.data) {
		p.offset = len(p.data)
		s.c = -1
		s.fnum--
//		log.Println("EOF offset", p.offset, "len", len(p.data), "fnum", s.fnum)
		if s.fnum > 0 {
			s.rdpos = s.fstack[s.fnum]
		}

		return
	}

	s.pos = p
	if s.c == '\n' {
		s.pos.line++
		s.rdpos.line++
	}

	r, w := rune(p.data[p.offset]), 1
	switch {
	case r == 0:
		s.error(&s.pos, "illegal character NULL")
	case r >= 0x80:
		r, w = utf8.DecodeRune(p.data[p.offset:])
		if r==utf8.RuneError && w == 1 {
			s.error(&s.pos, "illegal UTF-8 encoding")
		}
	}

	s.rdpos.offset += w
	s.c = r
//	log.Println("nextChar", s.c)
}

func (s *Scanner) PushFile(fname string, data []byte) {
	if s.fnum == len(s.fstack) {
		s.error(&s.pos, "too many nested files")
		return
	}

	p := &s.fstack[s.fnum]
	p.fname = fname
	p.data = data
	p.offset = 0
	p.line = 1

	s.fnum++
	s.rdpos = *p
}

func NewScanner(fname string, data []byte, err ErrorHandler, mode uint) *Scanner {
	s := new(Scanner)
	s.err = err
	s.mode = mode
	s.c = ' '
	s.insertSemi = false
	s.PushFile(fname, data)
	s.nextChar()

	return s
}

func isLetter(c rune) bool {
	return 'a'<=c && c<='z' || 'A'<=c && c<='Z' || c=='_' || c>=0x80 && unicode.IsLetter(c)
}

func isDigit(c rune) bool {
	return '0'<=c && c<='9' || c>=0x80 && unicode.IsDigit(c)
}

func (s *Scanner) scanIdentifier() Token {
	offset := s.pos.offset
	for isLetter(s.c) || isDigit(s.c) {
		s.nextChar()
	}

	tok, ok := keywords[string(s.pos.data[offset:s.pos.offset])]
	if !ok {
		tok = IDENT
	}

	return tok
}

func (s *Scanner) scanComment() Token {
	pos := s.pos
	pos.offset--				// offset of the initial slash
	if s.c == '/' {
		if s.insertSemi {
			goto semi
		}

		// line comment
		for s.c != '\n' && s.c >= 0 {
			s.nextChar()
		}

		return COMMENT
	} else {
		s.nextChar()
		for s.c >= 0 {
			c := s.c
			if s.insertSemi && c == '\n' {
				goto semi
			}

			s.nextChar()
			if c == '*' && s.c == '/' {
				s.nextChar()
				return COMMENT
			}
		}
	}

	s.error(&pos, "comment not terminated")
	return ILLEGAL

semi:
	s.pos = pos
	s.rdpos = pos
	s.rdpos.offset++
	s.c = '/'
	s.insertSemi = false
	return SEMICOLON
}

func (s *Scanner) scanNumber(seenDecimalPoint bool) Token {
	tok := INT

	if seenDecimalPoint {
		// if the number is ".65"
		tok = FLOAT
		for isDigit(s.c) {
			s.nextChar()
		}
	}

	for isDigit(s.c) {
		s.nextChar()
	}

	if s.c == '.' {
		s.nextChar()
		tok = FLOAT
		for isDigit(s.c) {
			s.nextChar()
		}
	}

	if s.c == 'e' || s.c == 'E' {
		tok = FLOAT
		s.nextChar()
		if s.c == '-' || s.c == '+' {
			s.nextChar()
		}

		for isDigit(s.c) {
			s.nextChar()
		}
	}

	return tok
}

func (s *Scanner) scanEscape() {
	
}

func (s *Scanner) scanString() {
	pos := s.pos
	pos.offset--	// opening quote already scanned

	for s.c != '"' {
		c := s.c
		s.nextChar()
		if c == '\n' || c < 0 {
			s.error(&pos, "string not terminated")
			break
		}

		if c == '\\' && s.c == '"' {
			s.nextChar()
		}
	}

	s.nextChar()
}

func (s *Scanner) skipWhitespace() {
	for s.c==' ' || s.c=='\t' || (s.c=='\n' && !s.insertSemi) || s.c=='\r' {
		s.nextChar()
	}
}

var newline = []byte{'\n'}

func (s *Scanner) Scan() (*Pos, Token, []byte) {
again:
	s.skipWhitespace()
	insertSemi := false
	pos := s.pos
	tok := ILLEGAL

	switch c:=s.c; {
	case isLetter(c):
		tok = s.scanIdentifier()
		if tok == IDENT {
			insertSemi = true
		}

	case isDigit(c):
		insertSemi = true
		tok = s.scanNumber(false)

	default:
		s.nextChar()
		switch c {
		case -1:
			if s.insertSemi {
				s.insertSemi = false
				return &pos, SEMICOLON, newline
			}

			tok = EOF

		case '\n':
			s.insertSemi = false
			return &pos, SEMICOLON, newline

		case '"':
			insertSemi = true
			tok = STRING
			s.scanString()

		case '.', '-', '+':
			if isDigit(s.c) {
				insertSemi = true
				tok = s.scanNumber(c == '.')
			} else {
				switch c {
				default:
					s.error(&pos, fmt.Sprintf("illegal character '%v'", s.c))
				case '-':
					tok = SUB
				case '+':
					tok = ADD
				}
			}

		case '/':
			if s.c=='/' || s.c=='*' {
				tok = s.scanComment()
				if tok==SEMICOLON {
					return &pos, SEMICOLON, newline
				}

				if s.mode&ScanComments==0 {
					s.insertSemi = false
					goto again
				}
			} else {
				tok = QUO
			}

		case '*':
			tok = MUL
		case '%':
			tok = REM
		case '(':
			tok = LPAREN
		case '{':
			tok = LBRACE
		case '[':
			tok = LBRACK
		case ')':
			tok = RPAREN
		case '}':
			tok = RBRACE
		case ']':
			tok = RBRACK
		case ';':
			tok = SEMICOLON
		case ':':
			tok = COLON
		case ',':
			tok = COMMA
		case '&':
			s.nextChar()
			if s.c=='&' {
				tok = AND
				s.nextChar()
			} else {
				s.error(&pos, fmt.Sprintf("illegal character '%v'", s.c))
			}
		case '|':
			if s.c=='|' {
				tok = OR
				s.nextChar()
			} else {
				s.error(&pos, fmt.Sprintf("illegal character '%v'", s.c))
			}
		case '!':
			if s.c=='=' {
				tok = NEQ
				s.nextChar()
			} else {
				tok = NOT
			}
		case '=':
			if s.c != '=' {
				tok = ASSIGN
			} else {
				tok = EQL
				s.nextChar()
			}
		default:
			s.error(&pos, fmt.Sprintf("illegal character '%v'", c))
		}
	}

	if s.mode&InsertSemis != 0 {
		s.insertSemi = insertSemi
	}

//	log.Println("len", len(s.pos.data), "offset", pos.offset, s.pos.offset)
//	log.Println("Token", tok, "literal '", string(s.pos.data[pos.offset:s.pos.offset]), "'")
	return &pos, tok, s.pos.data[pos.offset:s.pos.offset]
}

func (p *Pos) String() string {
	return fmt.Sprintf("%s:%d:", p.fname, p.line)
}
