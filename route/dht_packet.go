package route

import (
	"github.com/sagernet/sing/common/buf"
	"github.com/sagernet/sing/common/bufio"
	M "github.com/sagernet/sing/common/metadata"
	N "github.com/sagernet/sing/common/network"
)

// Only writes may bypass the guard. Read waiters and cached packets must still
// inspect every datagram, including when optimized copying is selected.
func (c *dhtPacketConn) WriterReplaceable() bool { return true }
func (c *dhtPacketConn) UpstreamWriter() any     { return c.PacketConn }

func (c *dhtPacketConn) ReadCachedPacket() *N.PacketBuffer {
	reader := c.PacketConn
	for {
		if cached, ok := reader.(N.CachedPacketReader); ok {
			packet := cached.ReadCachedPacket()
			if packet != nil {
				if packet.Buffer != nil {
					if !c.block(packet.Buffer.Bytes(), packet.Destination) {
						return packet
					}
					packet.Buffer.Release()
				}
				N.PutPacketBuffer(packet)
			}
		}
		// Traverse only the explicit sniff cache, preserving any counters or
		// other transport wrappers below it.
		cached, ok := reader.(*bufio.CachedPacketConn)
		if !ok {
			return nil
		}
		reader = cached.PacketConn
	}
}

func (c *dhtPacketConn) CreateReadWaiter() (N.PacketReadWaiter, bool) {
	reader, ok := bufio.CreatePacketReadWaiter(c.PacketConn)
	if !ok {
		return nil, false
	}
	return &dhtReadWaiter{PacketReadWaiter: reader, guard: c}, true
}

type dhtReadWaiter struct {
	N.PacketReadWaiter
	guard *dhtPacketConn
}

func (w *dhtReadWaiter) WaitReadPacket() (*buf.Buffer, M.Socksaddr, error) {
	for {
		buffer, destination, err := w.PacketReadWaiter.WaitReadPacket()
		if buffer == nil || !w.guard.block(buffer.Bytes(), destination) {
			return buffer, destination, err
		}
		buffer.Release()
		if err != nil {
			return nil, destination, err
		}
	}
}

func (c *dhtPacketConn) CreatePacketBatchReadWaiter() (N.PacketBatchReadWaiter, bool) {
	reader, ok := bufio.CreatePacketBatchReadWaiter(c.PacketConn)
	if !ok {
		return nil, false
	}
	return &dhtBatchReadWaiter{PacketBatchReadWaiter: reader, guard: c}, true
}

type dhtBatchReadWaiter struct {
	N.PacketBatchReadWaiter
	guard *dhtPacketConn
}

func (w *dhtBatchReadWaiter) WaitReadPackets() ([]*buf.Buffer, []M.Socksaddr, error) {
	for {
		buffers, destinations, err := w.PacketBatchReadWaiter.WaitReadPackets()
		kept := 0
		for index, buffer := range buffers {
			if buffer == nil {
				continue
			}
			if w.guard.block(buffer.Bytes(), destinations[index]) {
				buffer.Release()
				continue
			}
			buffers[kept], destinations[kept] = buffer, destinations[index]
			kept++
		}
		clear(buffers[kept:])
		clear(destinations[kept:])
		if kept > 0 || err != nil {
			return buffers[:kept], destinations[:kept], err
		}
	}
}

func (c *dhtPacketConn) CreateConnectedPacketBatchReadWaiter() (N.ConnectedPacketBatchReadWaiter, bool) {
	reader, ok := bufio.CreateConnectedPacketBatchReadWaiter(c.PacketConn)
	if !ok {
		return nil, false
	}
	return &dhtConnectedBatchReadWaiter{ConnectedPacketBatchReadWaiter: reader, guard: c}, true
}

type dhtConnectedBatchReadWaiter struct {
	N.ConnectedPacketBatchReadWaiter
	guard *dhtPacketConn
}

func (w *dhtConnectedBatchReadWaiter) WaitReadConnectedPackets() ([]*buf.Buffer, M.Socksaddr, error) {
	for {
		buffers, destination, err := w.ConnectedPacketBatchReadWaiter.WaitReadConnectedPackets()
		kept := 0
		for _, buffer := range buffers {
			if buffer == nil {
				continue
			}
			if w.guard.block(buffer.Bytes(), destination) {
				buffer.Release()
				continue
			}
			buffers[kept] = buffer
			kept++
		}
		clear(buffers[kept:])
		if kept > 0 || err != nil {
			return buffers[:kept], destination, err
		}
	}
}
