// Package dht recognizes complete BitTorrent KRPC datagrams (BEP 5, 32, 33, 43, 44 and 51).
package dht

import "bytes"

// Values borrow their bytes from the datagram. Unknown containers are validated
// in place; they never allocate a tree of maps and lists.
type value struct {
	kind byte
	text []byte
	size int
}

type field uint8

const (
	fieldA field = iota
	fieldR
	fieldE
	fieldQ
	fieldT
	fieldY
	fieldRo
	fieldV
	fieldIP
	fieldID
	fieldTarget
	fieldInfoHash
	fieldPort
	fieldImpliedPort
	fieldToken
	fieldSeq
	fieldK
	fieldSig
	fieldSalt
	fieldCas
	fieldNodes
	fieldNodes6
	fieldSamples
	fieldValues
	fieldBfpe
	fieldBfsd
	fieldInterval
	fieldNum
	fieldScrape
	fieldNoseed
	fieldWant
	fieldCount
)

type fields [fieldCount]value

type decoder struct {
	data   []byte
	offset int
	budget int
}

// IsMessage validates one complete canonical KRPC datagram without allocating.
// The byte, depth and value limits apply even to unknown extension fields.
func IsMessage(packet []byte) bool {
	if len(packet) < 2 || len(packet) > 65507 || packet[0] != 'd' {
		return false
	}
	d := decoder{data: packet, budget: 4096}
	var m fields
	_, ok := d.read(0, &m)
	if !ok || d.offset != len(packet) {
		return false
	}
	if !text(&m, fieldT, -1) || !text(&m, fieldY, 1) || !optionalFlag(&m, fieldRo) {
		return false
	}
	if v := m[fieldV]; v.kind != 0 && v.kind != 's' {
		return false
	}
	if v := m[fieldIP]; v.kind != 0 && !compact(v) {
		return false
	}
	switch string(m[fieldY].text) {
	case "q":
		if m[fieldR].kind != 0 || m[fieldE].kind != 0 {
			return false
		}
		a := m[fieldA]
		if a.kind != 'd' || !text(&m, fieldQ, -1) {
			return false
		}
		args := readFields(a)
		return query(string(m[fieldQ].text), &args)
	case "r":
		conflicting := m[fieldA].kind != 0 || m[fieldQ].kind != 0 || m[fieldE].kind != 0
		if conflicting || m[fieldR].kind != 'd' {
			return false
		}
		reply := readFields(m[fieldR])
		return response(&reply)
	case "e":
		conflicting := m[fieldA].kind != 0 || m[fieldQ].kind != 0 || m[fieldR].kind != 0
		if conflicting || m[fieldE].kind != 'l' {
			return false
		}
		items := decoder{data: m[fieldE].text, budget: 4096}
		code, ok := items.read(0, nil)
		if !ok {
			return false
		}
		message, ok := items.read(0, nil)
		if !ok || items.offset != len(items.data) {
			return false
		}
		n, ok := number(code)
		standard := n >= 201 && n <= 207
		mutable := n == 301 || n == 302
		return ok && (standard || mutable) && message.kind == 's' && len(message.text) > 0
	default:
		return false
	}
}

// The envelope scan has already validated this dictionary, including all nested
// values. A second bounded scan captures only the fields used by its validator.
func readFields(v value) fields {
	var result fields
	d := decoder{data: v.text, budget: 4096}
	for d.offset < len(d.data) {
		key, _ := d.read(0, nil)
		item, _ := d.read(0, nil)
		if index := fieldIndex(key.text); index < fieldCount {
			result[index] = item
		}
	}
	return result
}

func query(method string, a *fields) bool {
	if !text(a, fieldID, 20) || !extensions(a) {
		return false
	}
	switch method {
	case "ping":
		return true
	case "find_node", "sample_infohashes":
		return text(a, fieldTarget, 20)
	case "get_peers":
		return text(a, fieldInfoHash, 20)
	case "announce_peer":
		validPort := integer(a, fieldPort, 1, 65535) ||
			(integer(a, fieldPort, 0, 0) && integer(a, fieldImpliedPort, 1, 1))
		return text(a, fieldInfoHash, 20) && text(a, fieldToken, -1) && validPort && optionalFlag(a, fieldImpliedPort)
	case "get":
		return text(a, fieldTarget, 20) && optionalInteger(a, fieldSeq, 0)
	case "put":
		v := a[fieldV]
		return v.kind != 0 && v.size <= 1000 && text(a, fieldToken, -1) && mutable(a, true)
	default:
		return false
	}
}

func response(r *fields) bool {
	if !text(r, fieldID, 20) || !extensions(r) || !mutable(r, false) {
		return false
	}
	for _, check := range [...]struct {
		key   field
		width int
	}{
		{key: fieldNodes, width: 26}, {key: fieldNodes6, width: 38}, {key: fieldSamples, width: 20},
	} {
		if v := r[check.key]; v.kind != 0 && (v.kind != 's' || len(v.text)%check.width != 0) {
			return false
		}
	}
	if v := r[fieldValues]; v.kind != 0 {
		if v.kind != 'l' {
			return false
		}
		items := decoder{data: v.text, budget: 4096}
		for items.offset < len(items.data) {
			peer, ok := items.read(0, nil)
			if !ok || !compact(peer) {
				return false
			}
		}
	}
	if r[fieldToken].kind != 0 && !text(r, fieldToken, -1) {
		return false
	}
	for _, key := range []field{fieldBfpe, fieldBfsd} {
		if r[key].kind != 0 && !text(r, key, 256) {
			return false
		}
	}
	if r[fieldSamples].kind != 0 {
		if !integer(r, fieldInterval, 0, 21600) || !integer(r, fieldNum, 0, 1<<63-1) {
			return false
		}
		sampleCount, _ := number(r[fieldNum])
		if sampleCount < int64(len(r[fieldSamples].text)/20) {
			return false
		}
	}
	if v := r[fieldV]; v.kind != 0 && v.size > 1000 {
		return false
	}
	return true
}

func extensions(m *fields) bool {
	if !optionalFlag(m, fieldScrape) || !optionalFlag(m, fieldNoseed) {
		return false
	}
	if v := m[fieldWant]; v.kind != 0 {
		if v.kind != 'l' || len(v.text) == 0 {
			return false
		}
		items := decoder{data: v.text, budget: 4096}
		for items.offset < len(items.data) {
			family, ok := items.read(0, nil)
			// BEP 32 requires unknown address-family strings to be ignored.
			if !ok || family.kind != 's' {
				return false
			}
		}
	}
	return true
}

func mutable(m *fields, put bool) bool {
	key := m[fieldK].kind != 0
	signature := m[fieldSig].kind != 0
	sequence := m[fieldSeq].kind != 0
	if key || signature {
		if !text(m, fieldK, 32) || !text(m, fieldSig, 64) || !integer(m, fieldSeq, 0, 1<<63-1) {
			return false
		}
		if m[fieldV].kind == 0 {
			return false
		}
	} else if sequence && (put || !optionalInteger(m, fieldSeq, 0)) {
		return false
	}
	if salt := m[fieldSalt]; salt.kind != 0 && (!key || salt.kind != 's' || len(salt.text) > 64) {
		return false
	}
	if m[fieldCas].kind != 0 && (!put || !key || !optionalInteger(m, fieldCas, 0)) {
		return false
	}
	return true
}

func text(m *fields, key field, length int) bool {
	v := m[key]
	return v.kind == 's' && ((length < 0 && len(v.text) > 0) || len(v.text) == length)
}

func integer(m *fields, key field, low, high int64) bool {
	v := m[key]
	n, ok := number(v)
	return ok && n >= low && n <= high
}

func optionalInteger(m *fields, key field, low int64) bool {
	return m[key].kind == 0 || integer(m, key, low, 1<<63-1)
}

func optionalFlag(m *fields, key field) bool {
	return m[key].kind == 0 || integer(m, key, 0, 1)
}

func compact(v value) bool {
	return v.kind == 's' && (len(v.text) == 6 || len(v.text) == 18)
}

// Only numeric protocol fields need a signed 64-bit interpretation. BEP 44
// values may contain arbitrarily large canonical bencoded integers.
func number(v value) (int64, bool) {
	if v.kind != 'i' || len(v.text) == 0 || v.text[0] == '-' {
		return 0, false
	}
	var n uint64
	for _, digit := range v.text {
		value := uint64(digit - '0')
		if n > ((1<<63-1)-value)/10 {
			return 0, false
		}
		n = n*10 + value
	}
	return int64(n), true
}

func (d *decoder) read(depth int, capture *fields) (value, bool) {
	if depth > 16 || d.budget == 0 || d.offset >= len(d.data) {
		return value{}, false
	}
	d.budget--
	start := d.offset
	v := value{kind: d.data[d.offset]}
	d.offset++
	switch v.kind {
	case 'd', 'l':
		previous := []byte{}
		first := true
		for d.offset < len(d.data) && d.data[d.offset] != 'e' {
			var key value
			if v.kind == 'd' {
				var ok bool
				key, ok = d.read(depth+1, nil)
				if !ok || key.kind != 's' {
					return value{}, false
				}
				if !first && bytes.Compare(key.text, previous) <= 0 {
					return value{}, false
				}
				previous = key.text
				first = false
			}
			item, ok := d.read(depth+1, nil)
			if !ok {
				return value{}, false
			}
			if capture != nil {
				if index := fieldIndex(key.text); index < fieldCount {
					capture[index] = item
				}
			}
		}
		if d.offset == len(d.data) {
			return value{}, false
		}
		v.text = d.data[start+1 : d.offset]
		d.offset++
	case 'i':
		begin := d.offset
		if d.offset < len(d.data) && d.data[d.offset] == '-' {
			d.offset++
		}
		digits := d.offset
		for d.offset < len(d.data) && d.data[d.offset] >= '0' && d.data[d.offset] <= '9' {
			d.offset++
		}
		if digits == d.offset || d.offset == len(d.data) || d.data[d.offset] != 'e' {
			return value{}, false
		}
		if d.data[digits] == '0' && (d.offset-digits > 1 || digits != begin) {
			return value{}, false
		}
		v.text = d.data[begin:d.offset]
		d.offset++
	default:
		d.offset = start
		length := 0
		for d.offset < len(d.data) && d.data[d.offset] >= '0' && d.data[d.offset] <= '9' {
			length = length*10 + int(d.data[d.offset]-'0')
			// Bound on every digit prevents integer overflow as well as over-reading.
			if length > len(d.data) {
				return value{}, false
			}
			d.offset++
		}
		if d.offset == start || d.offset == len(d.data) || d.data[d.offset] != ':' {
			return value{}, false
		}
		if d.offset-start > 1 && d.data[start] == '0' {
			return value{}, false
		}
		d.offset++
		if length > len(d.data)-d.offset {
			return value{}, false
		}
		v.kind = 's'
		v.text = d.data[d.offset : d.offset+length]
		d.offset += length
	}
	v.size = d.offset - start
	return v, true
}

func fieldIndex(key []byte) field {
	switch string(key) {
	case "a":
		return fieldA
	case "r":
		return fieldR
	case "e":
		return fieldE
	case "q":
		return fieldQ
	case "t":
		return fieldT
	case "y":
		return fieldY
	case "ro":
		return fieldRo
	case "v":
		return fieldV
	case "ip":
		return fieldIP
	case "id":
		return fieldID
	case "target":
		return fieldTarget
	case "info_hash":
		return fieldInfoHash
	case "port":
		return fieldPort
	case "implied_port":
		return fieldImpliedPort
	case "token":
		return fieldToken
	case "seq":
		return fieldSeq
	case "k":
		return fieldK
	case "sig":
		return fieldSig
	case "salt":
		return fieldSalt
	case "cas":
		return fieldCas
	case "nodes":
		return fieldNodes
	case "nodes6":
		return fieldNodes6
	case "samples":
		return fieldSamples
	case "values":
		return fieldValues
	case "BFpe":
		return fieldBfpe
	case "BFsd":
		return fieldBfsd
	case "interval":
		return fieldInterval
	case "num":
		return fieldNum
	case "scrape":
		return fieldScrape
	case "noseed":
		return fieldNoseed
	case "want":
		return fieldWant
	default:
		return fieldCount
	}
}
