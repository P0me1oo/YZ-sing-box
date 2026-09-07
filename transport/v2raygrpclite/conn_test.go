package v2raygrpclite

import (
	"bytes"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type gunTestReader struct {
	io.Reader
	closes atomic.Int32
}

func (r *gunTestReader) Close() error {
	r.closes.Add(1)
	return nil
}

func gunTestFrame(t *testing.T, payload string) []byte {
	t.Helper()
	var encoded bytes.Buffer
	conn := newGunConn(bytes.NewReader(nil), &encoded, nil)
	if _, err := conn.Write([]byte(payload)); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func TestLateGunConnReadDuringSetup(t *testing.T) {
	frame := gunTestFrame(t, "grpc response")
	for range 64 {
		conn := newLateGunConn(io.Discard)
		start := make(chan struct{})
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-start
			conn.setup(bytes.NewReader(frame), nil)
		}()
		go func() {
			defer workers.Done()
			<-start
			payload := make([]byte, len("grpc response"))
			if _, err := io.ReadFull(conn, payload); err != nil || string(payload) != "grpc response" {
				t.Errorf("read: %q, %v", payload, err)
			}
		}()
		close(start)
		workers.Wait()
		_ = conn.Close()
	}
}

func TestLateGunConnSetupError(t *testing.T) {
	want := errors.New("setup failed")
	conn := newLateGunConn(io.Discard)
	go conn.setup(nil, want)
	for range 2 {
		if _, err := conn.Read(make([]byte, 1)); !errors.Is(err, want) {
			t.Fatalf("setup error: %v", err)
		}
	}
}

func TestLateGunConnCloseBeforeSetup(t *testing.T) {
	writerReader, writer := io.Pipe()
	defer writerReader.Close()
	conn := newLateGunConn(writer)
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		done <- err
	}()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, net.ErrClosed) {
			t.Fatalf("未完成初始化时关闭应唤醒读取: %v", err)
		}
	case <-time.After(5 * time.Second):
		conn.setup(bytes.NewReader(nil), io.EOF)
		<-done
		t.Fatal("初始化前关闭没有唤醒读取")
	}
	lateReader := &gunTestReader{Reader: bytes.NewReader(nil)}
	conn.setup(lateReader, nil)
	if lateReader.closes.Load() != 1 {
		t.Fatal("关闭后到达的响应没有关闭")
	}
	if _, err := writer.Write([]byte("closed")); !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("请求写入端仍未关闭: %v", err)
	}
}

func TestLateGunConnConcurrentSetupAndClose(t *testing.T) {
	for range 64 {
		conn := newLateGunConn(io.Discard)
		reader := &gunTestReader{Reader: bytes.NewReader(nil)}
		start := make(chan struct{})
		var workers sync.WaitGroup
		workers.Add(3)
		go func() {
			defer workers.Done()
			<-start
			conn.setup(reader, nil)
		}()
		for range 2 {
			go func() {
				defer workers.Done()
				<-start
				if err := conn.Close(); err != nil {
					t.Errorf("close: %v", err)
				}
			}()
		}
		close(start)
		workers.Wait()
		if reader.closes.Load() != 1 {
			t.Fatalf("响应应只关闭一次: %d", reader.closes.Load())
		}
	}
}

func TestGunConnCloseUnblocksRead(t *testing.T) {
	reader, writer := io.Pipe()
	defer writer.Close()
	conn := newGunConn(reader, io.Discard, nil)
	done := make(chan error, 1)
	go func() {
		_, err := conn.Read(make([]byte, 1))
		done <- err
	}()
	if err := conn.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("关闭后读取未返回错误")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("关闭没有解除底层读取阻塞")
	}
}
