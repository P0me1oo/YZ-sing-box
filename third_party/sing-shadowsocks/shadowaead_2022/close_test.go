package shadowaead_2022

import (
	"crypto/rand"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/sagernet/sing/common/buf"
)

type closeTestConn struct {
	net.Conn
	closes atomic.Int32
	err    error
}

func (c *closeTestConn) Write(payload []byte) (int, error) {
	return len(payload), nil
}

func (c *closeTestConn) Close() error {
	c.closes.Add(1)
	return c.err
}

func TestServerConnCloseDuringFirstResponse(t *testing.T) {
	for _, vectorised := range []bool{false, true} {
		name := "write"
		if vectorised {
			name = "vectorised"
		}
		t.Run(name, func(t *testing.T) {
			for iteration := 0; iteration < 64; iteration++ {
				key, salt := make([]byte, 16), make([]byte, 16)
				if _, err := rand.Read(key); err != nil {
					t.Fatal(err)
				}
				if _, err := rand.Read(salt); err != nil {
					t.Fatal(err)
				}
				service, err := NewService("2022-blake3-aes-128-gcm", key, 60, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				upstream := &closeTestConn{}
				conn := &serverConn{Service: service.(*Service), Conn: upstream, uPSK: key, requestSalt: salt}
				start := make(chan struct{})
				var workers sync.WaitGroup
				workers.Add(2)
				go func() {
					defer workers.Done()
					<-start
					if vectorised {
						first, second := buf.New(), buf.New()
						_, _ = first.Write([]byte("first"))
						_, _ = second.Write([]byte("second"))
						if err := conn.WriteVectorised([]*buf.Buffer{first, second}); err != nil {
							t.Errorf("response: %v", err)
						}
					} else if n, err := conn.Write([]byte("response")); n != len("response") || err != nil {
						t.Errorf("response: n=%d, err=%v", n, err)
					}
				}()
				go func() {
					defer workers.Done()
					<-start
					if err := conn.Close(); err != nil {
						t.Errorf("close: %v", err)
					}
				}()
				close(start)
				workers.Wait()
				if upstream.closes.Load() != 1 {
					t.Fatalf("底层连接关闭次数=%d", upstream.closes.Load())
				}
			}
		})
	}
}

func TestServerConnCloseReturnsUnderlyingError(t *testing.T) {
	want := errors.New("close failed")
	upstream := &closeTestConn{err: want}
	conn := &serverConn{Conn: upstream}
	for iteration := 0; iteration < 2; iteration++ {
		if err := conn.Close(); !errors.Is(err, want) {
			t.Fatalf("close: %v", err)
		}
	}
	if upstream.closes.Load() != 2 {
		t.Fatal("重复关闭未保留底层连接的行为")
	}
}
