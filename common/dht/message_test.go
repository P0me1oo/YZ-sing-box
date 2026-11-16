package dht_test

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/sagernet/sing-box/common/dht"
)

func encode(v any) string {
	switch v := v.(type) {
	case string:
		return fmt.Sprintf("%d:%s", len(v), v)
	case int:
		return fmt.Sprintf("i%de", v)
	case []any:
		out := "l"
		for _, item := range v {
			out += encode(item)
		}
		return out + "e"
	case map[string]any:
		keys := make([]string, 0, len(v))
		for key := range v {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		out := "d"
		for _, key := range keys {
			out += encode(key) + encode(v[key])
		}
		return out + "e"
	default:
		panic("unsupported fixture")
	}
}

func query(method string, fields map[string]any) map[string]any {
	a := map[string]any{"id": strings.Repeat("n", 20)}
	for key, v := range fields {
		a[key] = v
	}
	return map[string]any{"t": "aa", "y": "q", "q": method, "a": a}
}

func response(fields map[string]any) map[string]any {
	r := map[string]any{"id": strings.Repeat("n", 20)}
	for key, v := range fields {
		r[key] = v
	}
	return map[string]any{"t": "aa", "y": "r", "r": r}
}

func TestIsMessage(t *testing.T) {
	t.Parallel()
	hash := strings.Repeat("h", 20)
	mutable := map[string]any{
		"token": "secret", "v": []any{"hello", 3}, "k": strings.Repeat("k", 32),
		"sig": strings.Repeat("s", 64), "seq": 1, "salt": "salt", "cas": 0,
	}
	cases := []struct {
		name    string
		message map[string]any
		valid   bool
	}{
		{"ping", query("ping", nil), true},
		{"unknown address family", query("get_peers", map[string]any{"info_hash": hash, "want": []any{"n4", "future"}}), true},
		{"long value dictionary key", query("put", map[string]any{"token": "token", "v": map[string]any{strings.Repeat("k", 129): "value"}}), true},
		{"find node", query("find_node", map[string]any{"target": hash}), true},
		{"get peers", query("get_peers", map[string]any{"info_hash": hash}), true},
		{"announce", query("announce_peer", map[string]any{"info_hash": hash, "port": 6881, "token": "token"}), true},
		{"implied port", query("announce_peer", map[string]any{"info_hash": hash, "port": 0, "token": "token", "implied_port": 1}), true},
		{"IPv6 want scrape", query("get_peers", map[string]any{"info_hash": hash, "want": []any{"n4", "n6"}, "scrape": 1, "noseed": 1}), true},
		{"get mutable", query("get", map[string]any{"target": hash, "seq": 3}), true},
		{"put immutable", query("put", map[string]any{"token": "token", "v": map[string]any{"a": 1}}), true},
		{"put mutable", query("put", mutable), true},
		{"sample query", query("sample_infohashes", map[string]any{"target": hash}), true},
		{"ping response", response(nil), true},
		{"nodes response", response(map[string]any{"nodes": strings.Repeat("4", 26), "nodes6": strings.Repeat("6", 38)}), true},
		{"peer response", response(map[string]any{"values": []any{strings.Repeat("4", 6), strings.Repeat("6", 18)}, "token": "token"}), true},
		{"scrape response", response(map[string]any{"BFpe": strings.Repeat("p", 256), "BFsd": strings.Repeat("s", 256)}), true},
		{"sample response", response(map[string]any{"samples": hash + hash, "interval": 21600, "num": 2}), true},
		{"empty samples", response(map[string]any{"samples": "", "interval": 0, "num": 0}), true},
		{"immutable response", response(map[string]any{"v": "value", "token": "token"}), true},
		{"sequence only response", response(map[string]any{"seq": 3, "token": "token"}), true},
		{"error", map[string]any{"t": "aa", "y": "e", "e": []any{203, "Protocol Error"}}, true},
		{"mutable error", map[string]any{"t": "aa", "y": "e", "e": []any{302, "Sequence number less than current"}}, true},
		{"unknown query", query("ordinary_rpc", nil), false},
		{"missing target", query("find_node", nil), false},
		{"short hash", query("get_peers", map[string]any{"info_hash": "short"}), false},
		{"invalid want type", query("ping", map[string]any{"want": []any{5}}), false},
		{"invalid flag", query("get_peers", map[string]any{"info_hash": hash, "scrape": "1"}), false},
		{"missing port", query("announce_peer", map[string]any{"info_hash": hash, "token": "token", "implied_port": 1}), false},
		{"invalid port", query("announce_peer", map[string]any{"info_hash": hash, "token": "token", "port": 65536}), false},
		{"missing value", query("put", map[string]any{"token": "token"}), false},
		{"missing signature", query("put", map[string]any{"token": "token", "v": "hello", "k": strings.Repeat("k", 32), "seq": 1}), false},
		{"oversized value", query("put", map[string]any{"token": "token", "v": strings.Repeat("v", 1000)}), false},
		{"partial nodes", response(map[string]any{"nodes": "invalid"}), false},
		{"partial nodes6", response(map[string]any{"nodes6": strings.Repeat("6", 37)}), false},
		{"invalid peer list", response(map[string]any{"values": []any{"invalid"}}), false},
		{"invalid bloom filter", response(map[string]any{"BFpe": "invalid"}), false},
		{"missing sampling interval", response(map[string]any{"samples": hash, "num": 1}), false},
		{"invalid samples", response(map[string]any{"samples": "short", "num": 1, "interval": 1}), false},
		{"negative sequence", query("get", map[string]any{"target": hash, "seq": -1}), false},
		{"invalid error", map[string]any{"t": "aa", "y": "e", "e": []any{"203", "Error"}}, false},
		{"other RPC error", map[string]any{"t": "aa", "y": "e", "e": []any{1, "Error"}}, false},
		{"zero port without implied flag", query("announce_peer", map[string]any{"info_hash": hash, "token": "token", "port": 0}), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			packet := []byte(encode(tc.message))
			original := bytes.Clone(packet)
			if got := dht.IsMessage(packet); got != tc.valid {
				t.Fatalf("got %v, want %v: %q", got, tc.valid, packet)
			}
			if !bytes.Equal(packet, original) {
				t.Fatal("input mutated")
			}
		})
	}
}

func TestIsMessageEnvelopeAndEncoding(t *testing.T) {
	t.Parallel()
	ping := encode(query("ping", nil))
	cases := []struct{ name, packet string }{
		{"ordinary UDP", "normal UDP payload"},
		{"JSON", `{"t":"aa","y":"q","q":"ping"}`},
		{"generic bencode", "d1:ai1e1:t2:aa1:y1:qe"},
		{"prefix only", "d1:ad2:id20:01234567890123456789"},
		{"trailing bytes", ping + "data"},
		{"two datagrams", ping + ping},
		{"duplicate transaction", strings.Replace(ping, "1:t2:aa", "1:t2:aa1:t2:bb", 1)},
		{"unsorted keys", strings.Replace(ping, "1:q4:ping1:t2:aa", "1:t2:aa1:q4:ping", 1)},
		{"empty transaction", strings.Replace(ping, "1:t2:aa", "1:t0:", 1)},
		{"noncanonical length", strings.Replace(ping, "20:", "020:", 1)},
		{"oversized length", "d999999999999999999999999999:"},
		{"negative zero", strings.Replace(ping, "1:y", "2:roi-0e1:y", 1)},
		{"leading zero integer", strings.Replace(ping, "1:y", "2:roi01e1:y", 1)},
		{"integer overflow", strings.Replace(ping, "1:y", "2:roi9223372036854775808e1:y", 1)},
		{"query and response", strings.Replace(ping, "1:t", "1:rde1:t", 1)},
		{"deep nesting", strings.Replace(ping, "1:t", "1:x"+strings.Repeat("l", 18)+strings.Repeat("e", 18)+"1:t", 1)},
		{"many values", strings.Replace(ping, "1:t", "1:x"+"l"+strings.Repeat("0:", 5000)+"e1:t", 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if dht.IsMessage([]byte(tc.packet)) {
				t.Fatalf("unexpected match: %q", tc.packet)
			}
		})
	}
	for i := range len(ping) {
		if dht.IsMessage([]byte(ping[:i])) {
			t.Fatalf("truncated prefix matched at %d", i)
		}
	}
	message := query("ping", nil)
	message["ro"] = 1
	message["v"] = "LT01"
	message["ip"] = strings.Repeat("4", 6)
	message["extension"] = []any{1, "future"}
	if !dht.IsMessage([]byte(encode(message))) {
		t.Fatal("valid optional extensions rejected")
	}
}

func FuzzIsMessage(f *testing.F) {
	f.Add([]byte(encode(query("ping", nil))))
	f.Add([]byte(encode(response(nil))))
	f.Add([]byte("ordinary UDP"))
	f.Fuzz(func(t *testing.T, packet []byte) {
		original := bytes.Clone(packet)
		if dht.IsMessage(packet) && dht.IsMessage(append(bytes.Clone(packet), 0)) {
			t.Fatal("accepted trailing data")
		}
		if !bytes.Equal(packet, original) {
			t.Fatal("input mutated")
		}
	})
}

func TestIsMessageArbitraryIntegers(t *testing.T) {
	packet := encode(query("put", map[string]any{"token": "token", "v": 3}))
	packet = strings.Replace(packet, "1:vi3e", "1:vi18446744073709551616e", 1)
	if !dht.IsMessage([]byte(packet)) {
		t.Fatal("BEP 44 arbitrary integer value rejected")
	}
}

var benchmarkResult bool

func BenchmarkIsMessage(b *testing.B) {
	cases := []struct {
		name   string
		packet []byte
	}{
		{name: "ordinary", packet: []byte("normal UDP payload")},
		{name: "d_prefix", packet: []byte("data from an ordinary application")},
		{name: "ping", packet: []byte(encode(query("ping", nil)))},
		{name: "get_peers", packet: []byte(encode(query("get_peers", map[string]any{"info_hash": strings.Repeat("h", 20)})))},
		{name: "nodes_reply", packet: []byte(encode(response(map[string]any{"nodes": strings.Repeat("n", 26*8), "token": "token"})))},
		{name: "malformed_many_values", packet: []byte("d1:al" + strings.Repeat("0:", 4096) + "ee")},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				benchmarkResult = dht.IsMessage(tc.packet)
			}
		})
	}
}

func TestIsMessageResourceLimits(t *testing.T) {
	for _, tc := range []struct {
		name         string
		depth, items int
		valid        bool
	}{
		{name: "within depth", depth: 15, valid: true},
		{name: "over depth", depth: 16, valid: false},
		{name: "within work limit", items: 1000, valid: true},
		{name: "over work limit", items: 4096, valid: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			message := query("ping", nil)
			var extension any = "value"
			for range tc.depth {
				extension = []any{extension}
			}
			if tc.items > 0 {
				values := make([]any, tc.items)
				for i := range values {
					values[i] = ""
				}
				extension = values
			}
			message["extension"] = extension
			if got := dht.IsMessage([]byte(encode(message))); got != tc.valid {
				t.Fatalf("got %v, want %v", got, tc.valid)
			}
		})
	}
}
