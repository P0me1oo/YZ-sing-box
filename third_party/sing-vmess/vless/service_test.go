package vless

import (
	"bytes"
	"sync"
	"testing"

	"github.com/sagernet/sing/common/buf"
	N "github.com/sagernet/sing/common/network"
)

type responseTestConn struct {
	N.ExtendedConn
	output bytes.Buffer
}

func (c *responseTestConn) Write(p []byte) (int, error) { return c.output.Write(p) }
func (c *responseTestConn) WriteBuffer(buffer *buf.Buffer) error {
	_, err := c.output.Write(buffer.Bytes())
	buffer.Release()
	return err
}

func TestResponseStateConcurrentHeadroom(t *testing.T) {
	for _, mode := range []string{"write", "write_buffer"} {
		t.Run(mode, func(t *testing.T) {
			transport := &responseTestConn{}
			conn := &serverConn{ExtendedConn: transport}
			var group sync.WaitGroup
			group.Add(2)
			start := make(chan struct{})
			go func() {
				defer group.Done()
				<-start
				for range 1000 {
					conn.FrontHeadroom()
					conn.WriterReplaceable()
				}
			}()
			go func() {
				defer group.Done()
				<-start
				if mode == "write" {
					if _, err := conn.Write([]byte("x")); err != nil {
						t.Error(err)
					}
				} else {
					buffer := buf.NewSize(3)
					buffer.Resize(2, 0)
					buffer.WriteByte('x')
					if err := conn.WriteBuffer(buffer); err != nil {
						t.Error(err)
					}
				}
			}()
			close(start)
			group.Wait()
			if conn.FrontHeadroom() != 0 || !conn.WriterReplaceable() {
				t.Fatal("response state not published")
			}
			if _, err := conn.Write([]byte("y")); err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(transport.output.Bytes(), []byte{Version, 0, 'x', 'y'}) {
				t.Fatalf("response header changed: %x", transport.output.Bytes())
			}
		})
	}
}
