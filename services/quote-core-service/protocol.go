package main

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	packetMagic = "2003"

	rtLogin = uint16(0x0102)
)

type loginRequestVariant struct {
	Name     string `json:"name"`
	Size     int16  `json:"size"`
	BodySize int    `json:"body_size"`
	LoginTyp byte   `json:"login_type,omitempty"`
}

type parsedDataHead struct {
	Type       string `json:"type"`
	Index      uint8  `json:"index"`
	ServerByte uint8  `json:"server_byte"`
	Key        int32  `json:"key"`
	PrivateKey string `json:"private_key_hex"`
}

type parsedLoginResponse struct {
	DataHead  *parsedDataHead `json:"data_head,omitempty"`
	ErrorCode int16           `json:"error_code,omitempty"`
	Size      int32           `json:"size,omitempty"`
	Text      string          `json:"text,omitempty"`
}

func defaultLoginVariants() []loginRequestVariant {
	return []loginRequestVariant{
		{Name: "padded-128", Size: 16, BodySize: 128},
		{Name: "negative-128", Size: -128, BodySize: 128},
		{Name: "padded-129-login-type", Size: 17, BodySize: 129, LoginTyp: 1},
		{Name: "negative-129-login-type", Size: -129, BodySize: 129, LoginTyp: 1},
	}
}

func selectLoginVariant(name string) (loginRequestVariant, bool) {
	if strings.TrimSpace(name) == "" || name == "auto" {
		return loginRequestVariant{}, false
	}
	for _, variant := range defaultLoginVariants() {
		if variant.Name == name {
			return variant, true
		}
	}
	return loginRequestVariant{}, false
}

func buildWirePacket(body []byte) []byte {
	packet := bytes.NewBuffer(make([]byte, 0, 8+len(body)))
	packet.WriteString(packetMagic)
	_ = binary.Write(packet, binary.LittleEndian, uint32(len(body)))
	packet.Write(body)
	return packet.Bytes()
}

func buildLoginBody(username, password string, variant loginRequestVariant) []byte {
	body := bytes.NewBuffer(make([]byte, 0, 20+variant.BodySize))

	_ = binary.Write(body, binary.LittleEndian, rtLogin)
	body.WriteByte(0) // m_nIndex
	body.WriteByte(0) // m_cSrv / operator flags
	_ = binary.Write(body, binary.LittleEndian, int32(0))
	body.Write(make([]byte, 8)) // HSPrivateKey
	_ = binary.Write(body, binary.LittleEndian, variant.Size)
	_ = binary.Write(body, binary.LittleEndian, int16(0))

	login := make([]byte, variant.BodySize)
	copyCString(login[0:64], username)
	copyCString(login[64:128], password)
	if variant.BodySize > 128 {
		login[128] = variant.LoginTyp
	}
	body.Write(login)

	return body.Bytes()
}

func copyCString(dst []byte, value string) {
	copy(dst, []byte(value))
}

func parseWirePacket(packet []byte) (string, uint32, []byte, error) {
	if len(packet) < 8 {
		return "", 0, nil, errors.New("packet shorter than 8-byte wire header")
	}
	magic := string(packet[:4])
	size := binary.LittleEndian.Uint32(packet[4:8])
	if int(size) != len(packet)-8 {
		return magic, size, packet[8:], fmt.Errorf("wire body length mismatch: header=%d actual=%d", size, len(packet)-8)
	}
	return magic, size, packet[8:], nil
}

func parseDataHead(body []byte) (*parsedDataHead, error) {
	if len(body) < 12 {
		return nil, errors.New("body shorter than DataHead")
	}
	return &parsedDataHead{
		Type:       fmt.Sprintf("0x%04X", binary.LittleEndian.Uint16(body[0:2])),
		Index:      body[2],
		ServerByte: body[3],
		Key:        int32(binary.LittleEndian.Uint32(body[4:8])),
		PrivateKey: hex.EncodeToString(body[8:16]),
	}, nil
}

func parseLoginResponse(body []byte) (*parsedLoginResponse, error) {
	head, err := parseDataHead(body)
	if err != nil {
		return nil, err
	}
	resp := &parsedLoginResponse{DataHead: head}
	if len(body) >= 24 {
		resp.ErrorCode = int16(binary.LittleEndian.Uint16(body[16:18]))
		resp.Size = int32(binary.LittleEndian.Uint32(body[20:24]))
	}
	if len(body) > 24 {
		resp.Text = string(bytes.TrimRight(body[24:], "\x00"))
	}
	return resp, nil
}
