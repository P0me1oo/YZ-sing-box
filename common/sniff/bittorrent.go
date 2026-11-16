package sniff

import (
	"context"
	"encoding/binary"
	"io"
	"os"

	"github.com/sagernet/sing-box/adapter"
	"github.com/sagernet/sing-box/common/dht"
	C "github.com/sagernet/sing-box/constant"
	E "github.com/sagernet/sing/common/exceptions"
)

const (
	trackerConnectFlag    = 0
	trackerProtocolID     = 0x41727101980
	trackerConnectMinSize = 16
)

// DHT recognizes complete KRPC datagrams as part of the existing BT protocol.
func DHT(_ context.Context, metadata *adapter.InboundContext, packet []byte) error {
	if !dht.IsMessage(packet) {
		return os.ErrInvalid
	}
	metadata.Protocol = C.ProtocolBitTorrent
	return nil
}

// BitTorrent detects if the stream is a BitTorrent connection.
// For the BitTorrent protocol specification, see https://www.bittorrent.org/beps/bep_0003.html
func BitTorrent(_ context.Context, metadata *adapter.InboundContext, reader io.Reader) error {
	var first byte
	err := binary.Read(reader, binary.BigEndian, &first)
	if err != nil {
		return E.Cause1(ErrNeedMoreData, err)
	}

	if first != 19 {
		return os.ErrInvalid
	}

	const header = "BitTorrent protocol"
	var protocol [19]byte
	var n int
	n, err = reader.Read(protocol[:])
	if string(protocol[:n]) != header[:n] {
		return os.ErrInvalid
	}
	if err != nil {
		return E.Cause1(ErrNeedMoreData, err)
	}
	if n < 19 {
		return ErrNeedMoreData
	}

	metadata.Protocol = C.ProtocolBitTorrent
	return nil
}

// UTP detects if the packet is a uTP connection packet.
// For the uTP protocol specification, see
//  1. https://www.bittorrent.org/beps/bep_0029.html
//  2. https://github.com/bittorrent/libutp/blob/2b364cbb0650bdab64a5de2abb4518f9f228ec44/utp_internal.cpp#L112
func UTP(_ context.Context, metadata *adapter.InboundContext, b []byte) error {
	if len(b) < 20 {
		return os.ErrInvalid
	}

	// type 4 (ST_SYN), version 1
	if b[0] != 0x41 {
		return os.ErrInvalid
	}

	// timestamp_difference is always 0 in new connections
	if binary.BigEndian.Uint32(b[8:12]) != 0 {
		return os.ErrInvalid
	}

	// Walk the extension chain. Selective ack (1) and extension bits (2)
	extension, offset := b[1], 20
	for extension != 0 {
		if len(b) < offset+2 {
			return os.ErrInvalid
		}
		length := int(b[offset+1])
		switch extension {
		case 1: // selective ack
			if length < 4 || length%4 != 0 {
				return os.ErrInvalid
			}
		case 2: // extension bits: fixed 8 bytes, sent in ST_SYN by µTorrent
			if length != 8 {
				return os.ErrInvalid
			}
		default:
			return os.ErrInvalid
		}
		if len(b) < offset+2+length {
			return os.ErrInvalid
		}
		extension = b[offset]
		offset += 2 + length
	}

	// extensions should consume all ST_SYN payload
	if len(b) != offset {
		return os.ErrInvalid
	}

	metadata.Protocol = C.ProtocolBitTorrent
	return nil
}

// UDPTracker detects if the packet is a UDP Tracker Protocol packet.
// For the UDP Tracker Protocol specification, see https://www.bittorrent.org/beps/bep_0015.html
func UDPTracker(_ context.Context, metadata *adapter.InboundContext, packet []byte) error {
	if len(packet) < trackerConnectMinSize {
		return os.ErrInvalid
	}
	if binary.BigEndian.Uint64(packet[:8]) != trackerProtocolID {
		return os.ErrInvalid
	}
	if binary.BigEndian.Uint32(packet[8:12]) != trackerConnectFlag {
		return os.ErrInvalid
	}
	metadata.Protocol = C.ProtocolBitTorrent
	return nil
}
