// Package ofx parses OFX 1.x (SGML) investment statement files into a structured
// representation suitable for holdings import. OFX 2.x (XML) is not yet supported.
package ofx

import (
	"fmt"
	"html"
	"io"
	"strconv"
	"strings"
	"time"
)

// ParsedStatement is the result of parsing a single investment statement from an OFX file.
// One OFX file may contain multiple statements (one per account).
type ParsedStatement struct {
	AccountID string
	BrokerID  string
	AsOf      time.Time
	Currency  string
	Positions []ParsedPosition
	SecList   map[string]Security // keyed by UniqueID
}

// ParsedPosition is a holding line from the INVPOSLIST block.
type ParsedPosition struct {
	UniqueID  string  // CUSIP, TICKER, etc.
	IDType    string  // "CUSIP", "TICKER", "ISIN", ...
	PosType   string  // "POSMF", "POSSTOCK", "POSDEBT", "POSOTHER"
	Units     float64 // shares / units
	UnitPrice float64 // price per unit
	MktVal    float64 // total market value
}

// Security is an entry from the SECLIST block — maps UniqueID → ticker + name.
type Security struct {
	UniqueID string
	IDType   string
	Ticker   string
	Name     string
}

// ParseOFX reads an OFX 1.x SGML stream and returns all investment statements found.
func ParseOFX(r io.Reader) ([]ParsedStatement, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read ofx: %w", err)
	}

	body, err := stripHeader(string(raw))
	if err != nil {
		return nil, err
	}

	nodes, err := tokenize(body)
	if err != nil {
		return nil, err
	}

	return extract(nodes)
}

// stripHeader removes the OFX 1.x header lines (key:value pairs before <OFX>)
// and returns just the SGML body starting at <OFX>.
func stripHeader(raw string) (string, error) {
	idx := strings.Index(strings.ToUpper(raw), "<OFX>")
	if idx < 0 {
		return "", fmt.Errorf("no <OFX> element found — is this a valid OFX file?")
	}
	return raw[idx:], nil
}

// node represents a parsed SGML element. Aggregate elements have children;
// data elements have a non-empty Value and no children.
type node struct {
	Tag      string
	Value    string // non-empty only for data elements
	Children []*node
}

// tokenize converts the raw SGML body into a tree of nodes.
func tokenize(body string) ([]*node, error) {
	s := &scanner{input: body}
	root := &node{Tag: "__root__"}
	stack := []*node{root}

	for s.pos < len(s.input) {
		s.skipWhitespace()
		if s.pos >= len(s.input) {
			break
		}
		if s.input[s.pos] != '<' {
			// bare value outside a tag — skip
			s.advanceLine()
			continue
		}

		tag, isClose, err := s.readTag()
		if err != nil {
			return nil, err
		}

		if isClose {
			// pop stack
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
			continue
		}

		n := &node{Tag: tag}

		// Peek: if the next non-whitespace char is '<', this is an aggregate.
		peek := s.peekNonWhitespace()
		if peek == '<' || peek == 0 {
			// aggregate element — push onto stack
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, n)
			stack = append(stack, n)
		} else {
			// data element — read value until newline or next tag
			n.Value = strings.TrimSpace(s.readValue())
			stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, n)
		}
	}

	return root.Children, nil
}

type scanner struct {
	input string
	pos   int
}

func (s *scanner) skipWhitespace() {
	for s.pos < len(s.input) && (s.input[s.pos] == ' ' || s.input[s.pos] == '\t' || s.input[s.pos] == '\n' || s.input[s.pos] == '\r') {
		s.pos++
	}
}

func (s *scanner) advanceLine() {
	for s.pos < len(s.input) && s.input[s.pos] != '\n' {
		s.pos++
	}
	if s.pos < len(s.input) {
		s.pos++ // consume newline
	}
}

func (s *scanner) readTag() (tag string, isClose bool, err error) {
	if s.pos >= len(s.input) || s.input[s.pos] != '<' {
		return "", false, fmt.Errorf("expected '<' at pos %d", s.pos)
	}
	s.pos++ // skip '<'

	if s.pos < len(s.input) && s.input[s.pos] == '/' {
		isClose = true
		s.pos++
	}

	start := s.pos
	for s.pos < len(s.input) && s.input[s.pos] != '>' {
		s.pos++
	}
	tag = strings.TrimSpace(strings.ToUpper(s.input[start:s.pos]))
	if s.pos < len(s.input) {
		s.pos++ // skip '>'
	}
	return tag, isClose, nil
}

func (s *scanner) peekNonWhitespace() byte {
	i := s.pos
	for i < len(s.input) && (s.input[i] == ' ' || s.input[i] == '\t' || s.input[i] == '\r') {
		i++
	}
	if i >= len(s.input) {
		return 0
	}
	// A newline before the next tag means the value is on a new line — treat as aggregate.
	if s.input[i] == '\n' {
		return '<'
	}
	return s.input[i]
}

func (s *scanner) readValue() string {
	start := s.pos
	for s.pos < len(s.input) && s.input[s.pos] != '\n' && s.input[s.pos] != '<' {
		s.pos++
	}
	return html.UnescapeString(s.input[start:s.pos])
}

// ---- extraction ----

func extract(nodes []*node) ([]ParsedStatement, error) {
	// Build a lookup of nodes by tag at the top level.
	ofxNode := findFirst(nodes, "OFX")
	if ofxNode == nil {
		// root IS the ofx content (root.Children already unwrapped)
		ofxNode = &node{Tag: "OFX", Children: nodes}
	}

	// Extract the security list (shared across all statements in the file).
	secList := extractSecList(ofxNode)

	// Each INVSTMTTRNRS contains one statement.
	var stmts []ParsedStatement
	for _, trnrs := range findAll(ofxNode, "INVSTMTTRNRS") {
		stmtrs := findFirst(trnrs.Children, "INVSTMTRS")
		if stmtrs == nil {
			continue
		}
		stmt, err := extractStatement(stmtrs, secList)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}
	return stmts, nil
}

func extractStatement(stmtrs *node, secList map[string]Security) (ParsedStatement, error) {
	stmt := ParsedStatement{
		SecList:  secList,
		Currency: childValue(stmtrs, "CURDEF"),
	}

	acctFrom := findFirst(stmtrs.Children, "INVACCTFROM")
	if acctFrom != nil {
		stmt.AccountID = childValue(acctFrom, "ACCTID")
		stmt.BrokerID = childValue(acctFrom, "BROKERID")
	}

	if raw := childValue(stmtrs, "DTASOF"); raw != "" {
		stmt.AsOf = parseOFXDate(raw)
	}

	poslist := findFirst(stmtrs.Children, "INVPOSLIST")
	if poslist != nil {
		for _, child := range poslist.Children {
			posType := child.Tag // POSMF, POSSTOCK, POSDEBT, POSOTHER, POSOPT
			invpos := findFirst(child.Children, "INVPOS")
			if invpos == nil {
				continue
			}
			pos, err := extractPosition(invpos, posType)
			if err != nil {
				continue // skip malformed positions
			}
			stmt.Positions = append(stmt.Positions, pos)
		}
	}

	return stmt, nil
}

func extractPosition(invpos *node, posType string) (ParsedPosition, error) {
	secid := findFirst(invpos.Children, "SECID")
	if secid == nil {
		return ParsedPosition{}, fmt.Errorf("missing SECID")
	}

	units, err := strconv.ParseFloat(childValue(invpos, "UNITS"), 64)
	if err != nil {
		return ParsedPosition{}, fmt.Errorf("invalid UNITS: %w", err)
	}

	unitPrice, _ := strconv.ParseFloat(childValue(invpos, "UNITPRICE"), 64)
	mktVal, _ := strconv.ParseFloat(childValue(invpos, "MKTVAL"), 64)

	return ParsedPosition{
		UniqueID:  childValue(secid, "UNIQUEID"),
		IDType:    childValue(secid, "UNIQUEIDTYPE"),
		PosType:   posType,
		Units:     units,
		UnitPrice: unitPrice,
		MktVal:    mktVal,
	}, nil
}

func extractSecList(ofxNode *node) map[string]Security {
	secList := make(map[string]Security)
	seclistMsg := findFirst(ofxNode.Children, "SECLISTMSGSRSV1")
	if seclistMsg == nil {
		return secList
	}
	seclistNode := findFirst(seclistMsg.Children, "SECLIST")
	if seclistNode == nil {
		return secList
	}

	for _, child := range seclistNode.Children {
		// Each child is MFINFO, STOCKINFO, DEBTINFO, OTHERINFO, OPTINFO
		secinfo := findFirst(child.Children, "SECINFO")
		if secinfo == nil {
			continue
		}
		secid := findFirst(secinfo.Children, "SECID")
		if secid == nil {
			continue
		}
		uid := childValue(secid, "UNIQUEID")
		if uid == "" {
			continue
		}
		secList[uid] = Security{
			UniqueID: uid,
			IDType:   childValue(secid, "UNIQUEIDTYPE"),
			Ticker:   strings.ToUpper(childValue(secinfo, "TICKER")),
			Name:     childValue(secinfo, "SECNAME"),
		}
	}
	return secList
}

// ---- tree helpers ----

func findFirst(nodes []*node, tag string) *node {
	for _, n := range nodes {
		if n.Tag == tag {
			return n
		}
	}
	return nil
}

func findAll(parent *node, tag string) []*node {
	var result []*node
	for _, n := range parent.Children {
		if n.Tag == tag {
			result = append(result, n)
		}
		// recurse one level — handles INVSTMTMSGSRSV1 > INVSTMTTRNRS
		for _, c := range n.Children {
			if c.Tag == tag {
				result = append(result, c)
			}
		}
	}
	return result
}

func childValue(n *node, tag string) string {
	for _, c := range n.Children {
		if c.Tag == tag {
			return c.Value
		}
	}
	return ""
}

// parseOFXDate parses OFX date formats: YYYYMMDD, YYYYMMDDHHmmss, YYYYMMDDHHmmss.mmm[tz].
func parseOFXDate(s string) time.Time {
	// Strip timezone suffix like [-5:EST]
	if i := strings.Index(s, "["); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)

	formats := []string{"20060102150405.000", "20060102150405", "20060102"}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
