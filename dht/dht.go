package dht

import (
	"fmt"
	"net"
	"sync"
)

const DHTPort = 9001

type DHT struct {
	address  string
	listener net.Listener
	wg       sync.WaitGroup
	done     chan struct{}
	mu       sync.RWMutex
}

func New(address string) *DHT {
	return &DHT{
		address: address,
		done:    make(chan struct{}),
	}
}

func (d *DHT) Start() error {
	listenAddr := fmt.Sprintf("[::]:%d", DHTPort)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}
	d.listener = listener

	fmt.Printf("DHT listening on %s\n", listenAddr)

	d.wg.Add(1)
	go d.acceptLoop()

	return nil
}

func (d *DHT) acceptLoop() {
	defer d.wg.Done()
	fmt.Println("DHT accept loop started")

	for {
		conn, err := d.listener.Accept()
		if err != nil {
			select {
			case <-d.done:
				fmt.Println("DHT accept loop stopping")
				return
			default:
				fmt.Println("DHT accept error:", err)
				continue
			}
		}

		d.wg.Add(1)
		go d.handleConnection(conn)
	}
}

func (d *DHT) handleConnection(conn net.Conn) {
	defer d.wg.Done()
	defer conn.Close()

	fmt.Printf("DHT new connection from %s\n", conn.RemoteAddr())
}

func (d *DHT) Stop() {
	fmt.Println("DHT Stopping")

	close(d.done)

	if d.listener != nil {
		d.listener.Close()
	}

	d.wg.Wait()
	fmt.Println("DHT Stopped")
}
