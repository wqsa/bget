package bencode

//Valid reports whether data is a valid Bencode encoding.
func Valid(data []byte) bool {
	return checkValid(data, &scanner{}) == nil
}

func checkValid(data []byte, scan *scanner) error {
	scan.reset()
	var i int64
	for i != int64(len(data)) {
		if i > int64(len(data)) {
			return &SyntaxError{"out of index", i}
		}
		scan.bytes += scan.stepSize
		if scan.step(scan, data[i]) == scanError {
			return scan.err
		}
		i += scan.stepSize
	}
	if scan.eof() == scanError {
		return scan.err
	}
	return nil
}

// A SyntaxError is a description of a Bencode syntax error.
type SyntaxError struct {
	msg    string
	Offset int64
}

func (e *SyntaxError) Error() string { return e.msg }

type scanner struct {
	step func(*scanner, byte) int

	endTop bool

	parseState []int

	err error

	//for string
	stepSize  int64
	stringLen int

	bytes int64
}

const (
	scanContinue = iota
	scanBeginString
	scanInString
	scanBeginInt
	scanEndInt
	scanBeginDict
	scanEndDict
	scanBeginList
	scanEndList

	scanEnd
	scanError
)

const (
	parseDictKey = iota
	parseDictValue
	parseListValue
)

func (s *scanner) reset() {
	s.step = stateBeginValue
	s.parseState = s.parseState[0:0]
	s.stepSize = 1
	s.err = nil
	s.endTop = false
}

func (s *scanner) eof() int {
	if s.err != nil {
		return scanError
	}
	if s.endTop {
		return scanEnd
	}
	s.step(s, ' ')
	if s.endTop {
		return scanEnd
	}
	if s.err == nil {
		s.err = &SyntaxError{"unexpected end of Bcode input", s.bytes}
	}
	return scanError
}

func (s *scanner) pushParseState(p int) {
	s.parseState = append(s.parseState, p)
}

func (s *scanner) popParseState() {
	n := len(s.parseState) - 1
	s.parseState = s.parseState[0:n]
	if n == 0 {
		s.step = stateEndTop
		s.endTop = true
	} else {
		s.step = stateEndValue
	}
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == 0
}

func stateBeginValue(s *scanner, c byte) int {
	switch c {
	case 'd':
		s.step = stateBeginString
		s.pushParseState(parseDictKey)
		return scanBeginDict
	case 'l':
		s.step = stateBeginValue
		s.pushParseState(parseListValue)
		return scanBeginList
	case 'i':
		s.step = stateBeginInt
		return scanBeginInt
	case 'e':
		return stateEndValue(s, c)
	}
	if c >= '0' && c <= '9' {
		s.step = stateBeginString
		s.stringLen = int(c - '0')
		return scanBeginString
	}
	return s.error(c, "looking for beginning of value")
}

func stateEndValue(s *scanner, c byte) int {
	n := len(s.parseState)
	if n == 0 {
		s.step = stateEndTop
		s.endTop = true
		return stateEndTop(s, c)
	}
	s.stepSize = 1
	ps := s.parseState[n-1]
	switch ps {
	case parseDictKey:
		s.parseState[n-1] = parseDictValue
		return stateBeginValue(s, c)
	case parseDictValue:
		if c > '0' && c <= '9' {
			s.parseState[n-1] = parseDictKey
			s.step = stateBeginString
			s.stringLen = int(c - '0')
			return scanBeginString
		} else if c == 'e' {
			s.popParseState()
			s.step = stateEndValue
			return scanEndDict
		}
		return s.error(c, "key-value pair")
	case parseListValue:
		if c != 'e' {
			return stateBeginValue(s, c)
		}
		s.popParseState()
		s.step = stateEndValue
		return scanEndList
	}
	return s.error(c, "")
}

func stateBeginString(s *scanner, c byte) int {
	if c >= '0' && c <= '9' {
		if s.stringLen == 0 {
			s.stringLen = int(c - '0')
			return scanBeginString
		}
		s.stringLen = s.stringLen*10 + int(c-'0')
		return scanContinue
	} else if c == ':' {
		s.step = stateInString
		return scanInString
	} else if c == 'e' && s.stringLen == 0 {
		return stateEndValue(s, c)
	}
	return s.error(c, "handle string length")
}

func stateInString(s *scanner, c byte) int {
	s.stepSize = int64(s.stringLen)
	s.stringLen = 0
	s.step = stateEndValue
	return scanContinue
}

func stateBeginInt(s *scanner, c byte) int {
	switch {
	case c == '-':
		s.step = stateNeg
		return scanContinue
	case c == '0':
		s.step = state0
		return scanContinue
	case c > '0' && c <= '9':
		s.step = stateNum
		return scanContinue
	}
	return s.error(c, "begin int")
}

func stateNeg(s *scanner, c byte) int {
	if c > '0' && c <= '9' {
		s.step = stateNum
		return scanContinue
	}
	return s.error(c, "in numeric literal")
}

func state0(s *scanner, c byte) int {
	if c == 'e' {
		s.step = stateEndValue
		return scanEndInt
	}
	return s.error(c, "in leading zero")
}

func stateNum(s *scanner, c byte) int {
	if c >= '0' && c <= '9' {
		return scanContinue
	} else if c == 'e' {
		s.step = stateEndValue
		return scanEndInt
	}
	return s.error(c, "in numeric literal")
}

func stateEndTop(s *scanner, c byte) int {
	if !isSpace(c) {
		s.error(c, "after top-level value")
	}
	return scanEnd
}

func stateError(s *scanner, c byte) int {
	return scanError
}

func (s *scanner) error(c byte, context string) int {
	s.step = stateError
	s.err = &SyntaxError{"invalid character " + string(c) + " " + context, s.bytes}
	return scanError
}
