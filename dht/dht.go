package dht

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

const DHTPort = 9001

type DHT struct {
	address  string
	table    *RoutingTable
	store    *Store
	listener net.Listener
	wg       sync.WaitGroup
	done     chan struct{}
	mu       sync.RWMutex
}

func New(address string, selfID NodeID) *DHT {
	return &DHT{
		address: address,
		table:   NewRoutingTable(selfID),
		store:   NewStore(),
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

	d.store.Start()
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

	msg, err := readMessage(conn)
	if err != nil {
		return
	}

	switch msg.Type {

	case MsgPing:
		d.handlePing(conn, msg)

	case MsgFindNode:
		d.handleFindNode(conn, msg)

	case MsgStore:
		d.handleStore(conn, msg)

	case MsgFindValue:
		d.handleFindValue(conn, msg)

	default:
		// unknown message type — ignore silently
	}
}

func (d *DHT) handlePing(conn net.Conn, msg Message) {
	var ping PingBody
	if err := json.Unmarshal(msg.Body, &ping); err != nil {
		return
	}

	senderID, err := NodeIDFromHex(ping.SenderID)
	if err == nil {
		d.table.Add(Contact{
			ID:      senderID,
			Address: net.ParseIP(ping.SenderAddr),
			Port:    ping.SenderPort,
		})
	}

	pongBody, _ := json.Marshal(PongBody{
		SenderID:   d.table.self.String(),
		SenderAddr: d.address,
		SenderPort: DHTPort,
	})

	writeMessage(conn, Message{
		Type: MsgPong,
		Body: pongBody,
	})
}

func (d *DHT) handleFindNode(conn net.Conn, msg Message) {
	var req FindNodeBody
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return
	}

	senderID, err := NodeIDFromHex(req.SenderID)
	if err == nil {
		_ = senderID
	}

	targetID, err := NodeIDFromHex(req.TargetID)
	if err != nil {
		return
	}

	closest := d.table.Closest(targetID, K)

	var contacts []ContactInfo
	for _, c := range closest {
		contacts = append(contacts, ContactInfo{
			ID:   c.ID.String(),
			Addr: c.Address.String(),
			Port: c.Port,
		})
	}

	body, _ := json.Marshal(FoundNodesBody{Nodes: contacts})
	writeMessage(conn, Message{Type: MsgFoundNodes, Body: body})
}

func (d *DHT) handleStore(_ net.Conn, msg Message) {
	var req StoreBody
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return
	}

	d.store.Put(req.Record)
}

func (d *DHT) handleFindValue(conn net.Conn, msg Message) {
	var req FindValueBody
	if err := json.Unmarshal(msg.Body, &req); err != nil {
		return
	}

	var record Record
	var found bool

	if req.GroupKey == "" {
		record, found = d.store.GetPublic(req.Name)
	} else {
		record, found = d.store.GetForGroup(req.Name, req.GroupKey)
	}

	if found {
		body, _ := json.Marshal(FoundValueBody{Record: record})
		writeMessage(conn, Message{Type: MsgFoundValue, Body: body})
		return
	}

	targetID := RecordID(req.Name)
	closest := d.table.Closest(targetID, K)

	var contacts []ContactInfo
	for _, c := range closest {
		contacts = append(contacts, ContactInfo{
			ID:   c.ID.String(),
			Addr: c.Address.String(),
			Port: c.Port,
		})
	}

	if len(contacts) > 0 {
		body, _ := json.Marshal(FoundNodesBody{Nodes: contacts})
		writeMessage(conn, Message{Type: MsgFoundNodes, Body: body})
	} else {
		writeMessage(conn, Message{
			Type: MsgNotFound,
			Body: json.RawMessage("{}"),
		})
	}
}

func (d *DHT) Stop() {
	fmt.Println("DHT Stopping")

	close(d.done)

	if d.listener != nil {
		d.listener.Close()
	}

	d.store.Stop()
	d.wg.Wait()
	fmt.Println("DHT Stopped")
}
