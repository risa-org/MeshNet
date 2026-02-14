package dht

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	MsgPing byte = iota
	MsgPong
	MsgFindNode
	MsgFoundNodes
	MsgStore
	MsgFindValue
	MsgFoundValue
	MsgNotFound
)

const readTimeout = 10 * time.Second

type Message struct {
	Type byte
	Body json.RawMessage
}

type PingBody struct {
	SenderID   string `json:"sender_id"`
	SenderAddr string `json:"sender_addr"`
	SenderPort int    `json:"sender_port"`
}

type PongBody struct {
	SenderID   string `json:"sender_id"`
	SenderAddr string `json:"sender_addr"`
	SenderPort int    `json:"sender_port"`
}

type FindNodeBody struct {
	SenderID string `json:"sender_id"`
	TargetID string `json:"target_id"`
}

type FoundNodesBody struct {
	Nodes []ContactInfo `json:"nodes"`
}

type ContactInfo struct {
	ID   string `json:"id"`
	Addr string `json:"addr"`
	Port int    `json:"port"`
}

type StoreBody struct {
	Record Record `json:"record"`
}

type FindValueBody struct {
	SenderID string `json:"sender_id"`
	Name     string `json:"name"`
	GroupKey string `json:"group_key"`
}

type FoundValueBody struct {
	Record Record `json:"record"`
}

func writeMessage(conn net.Conn, msg Message) error {
	body, err := json.Marshal(msg.Body)
	if err != nil {
		return fmt.Errorf("failed to encode message body: %w", err)
	}

	w := bufio.NewWriter(conn)
	if err := w.WriteByte(msg.Type); err != nil {
		return fmt.Errorf("failed to write message type: %w", err)
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(body)))
	if _, err := w.Write(lenBuf); err != nil {
		return fmt.Errorf("failed to write message length: %w", err)
	}

	if _, err := w.Write(body); err != nil {
		return fmt.Errorf("failed to write message body: %w", err)
	}

	return w.Flush()
}

func readMessage(conn net.Conn) (Message, error) {
	conn.SetReadDeadline(time.Now().Add(readTimeout))

	typeBuf := make([]byte, 1)
	if _, err := io.ReadFull(conn, typeBuf); err != nil {
		return Message{}, fmt.Errorf("failed to read message type: %w", err)
	}

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(conn, lenBuf); err != nil {
		return Message{}, fmt.Errorf("failed to read message length: %w", err)
	}
	bodyLen := binary.BigEndian.Uint32(lenBuf)

	if bodyLen > 1024*1024 {
		return Message{}, fmt.Errorf("message too large: %d bytes", bodyLen)
	}

	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(conn, body); err != nil {
		return Message{}, fmt.Errorf("failed to read message body: %w", err)
	}

	return Message{
		Type: typeBuf[0],
		Body: json.RawMessage(body),
	}, nil
}

func SendPing(addr string, self Contact) (*PongBody, error) {
	conn, err := net.DialTimeout("tcp", addr, readTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer conn.Close()

	pingBody, _ := json.Marshal(PingBody{
		SenderID:   self.ID.String(),
		SenderAddr: self.Address.String(),
		SenderPort: self.Port,
	})

	err = writeMessage(conn, Message{
		Type: MsgPing,
		Body: pingBody,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to send ping: %w", err)
	}

	response, err := readMessage(conn)
	if err != nil {
		return nil, fmt.Errorf("failed to read pong: %w", err)
	}
	if response.Type != MsgPong {
		return nil, fmt.Errorf("expected pong,got type: %d", response.Type)
	}

	var pong PongBody
	if err := json.Unmarshal(response.Body, &pong); err != nil {
		return nil, fmt.Errorf("failed to decode pong: %w", err)
	}
	return &pong, nil
}

func SendFindNode(addr string, senderID NodeID, targetID NodeID) ([]ContactInfo, error) {
	conn, err := net.DialTimeout("tcp", addr, readTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	body, _ := json.Marshal(FindNodeBody{
		SenderID: senderID.String(),
		TargetID: targetID.String(),
	})

	err = writeMessage(conn, Message{Type: MsgFindNode, Body: body})
	if err != nil {
		return nil, err
	}

	response, err := readMessage(conn)
	if err != nil {
		return nil, err
	}

	if response.Type != MsgFoundNodes {
		return nil, fmt.Errorf("expected found_nodes, got %d", response.Type)
	}

	var found FoundNodesBody
	if err := json.Unmarshal(response.Body, &found); err != nil {
		return nil, err
	}

	return found.Nodes, nil
}

func SendStore(addr string, record Record) error {
	conn, err := net.DialTimeout("tcp", addr, readTimeout)
	if err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	body, _ := json.Marshal(StoreBody{Record: record})
	return writeMessage(conn, Message{Type: MsgStore, Body: body})
}

func SendFindValue(addr string, senderID NodeID, name string, groupKey string) (*Record, []ContactInfo, error) {
	conn, err := net.DialTimeout("tcp", addr, readTimeout)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect: %w", err)
	}
	defer conn.Close()

	body, _ := json.Marshal(FindValueBody{
		SenderID: senderID.String(),
		Name:     name,
		GroupKey: groupKey,
	})

	err = writeMessage(conn, Message{Type: MsgFindValue, Body: body})
	if err != nil {
		return nil, nil, err
	}

	response, err := readMessage(conn)
	if err != nil {
		return nil, nil, err
	}

	switch response.Type {
	case MsgFoundValue:
		var found FoundValueBody
		if err := json.Unmarshal(response.Body, &found); err != nil {
			return nil, nil, err
		}
		return &found.Record, nil, nil

	case MsgFoundNodes:
		var nodes FoundNodesBody
		if err := json.Unmarshal(response.Body, &nodes); err != nil {
			return nil, nil, err
		}
		return nil, nodes.Nodes, nil

	case MsgNotFound:
		return nil, nil, nil

	default:
		return nil, nil, fmt.Errorf("unexpected response type %d", response.Type)
	}
}
