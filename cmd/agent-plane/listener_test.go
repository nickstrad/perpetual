package main

import (
	"errors"
	"net"
	"sync"
	"testing"
	"testing/synctest"
)

type controlledListener struct {
	entered     chan struct{}
	connections chan net.Conn
	closed      chan struct{}
	once        sync.Once
}

func (l *controlledListener) Accept() (net.Conn, error) {
	l.entered <- struct{}{}
	select {
	case c := <-l.connections:
		if c == nil {
			return nil, errors.New("fixture accept failure")
		}
		return c, nil
	case <-l.closed:
		return nil, net.ErrClosed
	}
}
func (l *controlledListener) Close() error   { l.once.Do(func() { close(l.closed) }); return nil }
func (l *controlledListener) Addr() net.Addr { return &net.TCPAddr{} }
func TestT05ListenerCapacityAndClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		raw := &controlledListener{entered: make(chan struct{}, 3), connections: make(chan net.Conn, 3), closed: make(chan struct{})}
		limited := newLimitedListener(raw, 1)
		defer limited.Close()
		client, server := net.Pipe()
		defer client.Close()
		raw.connections <- server
		first, err := limited.Accept()
		if err != nil {
			t.Fatal(err)
		}
		<-raw.entered
		result := make(chan error, 1)
		go func() { _, err := limited.Accept(); result <- err }()
		synctest.Wait()
		select {
		case <-raw.entered:
			t.Fatal("listener created another accepted connection above capacity")
		default:
		}
		// Closing a connection releases exactly one slot even when called twice.
		_ = first.Close()
		_ = first.Close()
		raw.connections <- nil
		synctest.Wait()
		<-raw.entered
		if err := <-result; err == nil {
			t.Fatal("underlying accept failure hidden")
		}
		go func() { _, err := limited.Accept(); result <- err }()
		<-raw.entered
		_ = limited.Close()
		synctest.Wait()
		if err := <-result; !errors.Is(err, net.ErrClosed) {
			t.Fatalf("listener close did not unblock accept: %v", err)
		}
	})
}
