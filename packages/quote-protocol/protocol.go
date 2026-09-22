package quoteprotocol

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const packetMagic = "2003"

type DataHead struct {
	Type       uint16 `json:"type"`
	TypeHex    string `json:"type_hex"`
	Index      byte   `json:"index"`
	ServerByte byte   `json:"server_byte"`
	Key        int32  `json:"key"`
	PrivateKey string `json:"private_key_hex"`
}

type PacketMeta struct {
	Compressed        bool   `json:"compressed"`
	WireType          string `json:"wire_type"`
	OriginalType      string `json:"original_type,omitempty"`
	CompressedBytes   int32  `json:"compressed_bytes,omitempty"`
	ExpandedBytes     int32  `json:"expanded_bytes,omitempty"`
	ResponseBodyBytes int    `json:"response_body_bytes"`
}

type Response struct {
	Type          uint16     `json:"type"`
	TypeHex       string     `json:"type_hex"`
	FunctionName  string     `json:"function_name,omitempty"`
	Meta          PacketMeta `json:"meta"`
	BodyBytes     int        `json:"body_bytes"`
	BodyHexPrefix string     `json:"body_hex_prefix,omitempty"`
	DataHead      *DataHead  `json:"data_head,omitempty"`
	Body          []byte     `json:"-"`
}

type LoginResponse struct {
	DataHead        *DataHead `json:"data_head,omitempty"`
	ErrorCode       int32     `json:"error_code,omitempty"`
	Size            int32     `json:"size,omitempty"`
	TextPreview     string    `json:"text_preview,omitempty"`
	RawTextHexStart string    `json:"raw_text_hex_start,omitempty"`
}

func BuildWirePacket(body []byte) []byte {
	packet := bytes.NewBuffer(make([]byte, 0, 8+len(body)))
	packet.WriteString(packetMagic)
	_ = binary.Write(packet, binary.LittleEndian, uint32(len(body)))
	packet.Write(body)
	return packet.Bytes()
}

func BuildAskData(requestType uint16, index byte, size int16, option int16, payload []byte) []byte {
	body := bytes.NewBuffer(make([]byte, 0, 20+len(payload)))
	_ = binary.Write(body, binary.LittleEndian, requestType)
	body.WriteByte(index)
	body.WriteByte(0)
	_ = binary.Write(body, binary.LittleEndian, int32(0))
	body.Write(make([]byte, 8))
	_ = binary.Write(body, binary.LittleEndian, size)
	_ = binary.Write(body, binary.LittleEndian, option)
	body.Write(payload)
	return body.Bytes()
}

func BuildLogin(username, password string, index byte) []byte {
	login := make([]byte, 128)
	copy(login[0:64], []byte(username))
	copy(login[64:128], []byte(password))
	return BuildAskData(RT_LOGIN, index, 16, 0, login)
}

func BuildKeepAlive(index byte) []byte {
	body := make([]byte, 4)
	binary.LittleEndian.PutUint16(body[0:2], RT_KEEPACTIVE)
	body[2] = index
	body[3] = 1
	return body
}

func ReadResponse(conn net.Conn) (*Response, error) {
	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	if string(header[:4]) != packetMagic {
		return nil, fmt.Errorf("unexpected response magic %q", string(header[:4]))
	}
	bodyLen := binary.LittleEndian.Uint32(header[4:8])
	body := make([]byte, bodyLen)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, err
	}
	unwrapped, meta, err := unwrapResponseBody(body)
	if err != nil {
		return nil, err
	}
	resp := &Response{
		Meta:      meta,
		BodyBytes: len(unwrapped),
		Body:      unwrapped,
	}
	if len(unwrapped) >= 2 {
		resp.Type = binary.LittleEndian.Uint16(unwrapped[0:2])
		resp.TypeHex = HexType(resp.Type)
		resp.FunctionName = FunctionName(resp.Type)
	}
	if len(unwrapped) > 0 {
		prefix := unwrapped
		if len(prefix) > 96 {
			prefix = prefix[:96]
		}
		resp.BodyHexPrefix = hex.EncodeToString(prefix)
	}
	if head, err := ParseDataHead(unwrapped); err == nil {
		resp.DataHead = head
	}
	return resp, nil
}

func unwrapResponseBody(body []byte) ([]byte, PacketMeta, error) {
	meta := PacketMeta{ResponseBodyBytes: len(body)}
	if len(body) >= 2 {
		meta.WireType = HexType(binary.LittleEndian.Uint16(body[0:2]))
	}
	if len(body) < 12 || binary.LittleEndian.Uint16(body[0:2]) != RT_ZIPDATA {
		return body, meta, nil
	}
	zipLen := int32(binary.LittleEndian.Uint32(body[4:8]))
	origLen := int32(binary.LittleEndian.Uint32(body[8:12]))
	if zipLen < 0 || int(zipLen) > len(body)-12 {
		return nil, meta, fmt.Errorf("zip length mismatch: header=%d actual=%d", zipLen, len(body)-12)
	}
	reader, err := zlib.NewReader(bytes.NewReader(body[12 : 12+zipLen]))
	if err != nil {
		return nil, meta, err
	}
	defer reader.Close()
	decompressed, err := io.ReadAll(reader)
	if err != nil {
		return nil, meta, err
	}
	meta.Compressed = true
	meta.OriginalType = HexType(binary.LittleEndian.Uint16(body[2:4]))
	meta.CompressedBytes = zipLen
	meta.ExpandedBytes = origLen
	return decompressed, meta, nil
}

func ParseDataHead(body []byte) (*DataHead, error) {
	if len(body) < 16 {
		return nil, errors.New("body shorter than DataHead")
	}
	t := binary.LittleEndian.Uint16(body[0:2])
	return &DataHead{
		Type:       t,
		TypeHex:    HexType(t),
		Index:      body[2],
		ServerByte: body[3],
		Key:        int32(binary.LittleEndian.Uint32(body[4:8])),
		PrivateKey: hex.EncodeToString(body[8:16]),
	}, nil
}

func ParseLoginResponse(resp *Response) (*LoginResponse, error) {
	if resp == nil {
		return nil, errors.New("nil response")
	}
	head, err := ParseDataHead(resp.Body)
	if err != nil {
		return nil, err
	}
	out := &LoginResponse{DataHead: head}
	if len(resp.Body) >= 20 {
		packed := binary.LittleEndian.Uint32(resp.Body[16:20])
		out.ErrorCode = int32(packed & 0x03)
		out.Size = int32(packed >> 2)
	}
	if len(resp.Body) > 20 {
		rawText := bytes.TrimRight(resp.Body[20:], "\x00")
		out.TextPreview = trimPreview(decodeMarketText(rawText), 700)
		if len(rawText) > 0 {
			prefix := rawText
			if len(prefix) > 48 {
				prefix = prefix[:48]
			}
			out.RawTextHexStart = hex.EncodeToString(prefix)
		}
	}
	return out, nil
}

func ParseFunction(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty function type")
	}
	for _, def := range functionDefinitions {
		if strings.EqualFold(def.Name, value) {
			return def.Value, nil
		}
	}
	if strings.HasPrefix(strings.ToLower(value), "0x") {
		n, err := strconv.ParseUint(value[2:], 16, 16)
		return uint16(n), err
	}
	if strings.HasPrefix(value, "0") || strings.ContainsAny(strings.ToLower(value), "abcdef") {
		if n, err := strconv.ParseUint(value, 16, 16); err == nil {
			return uint16(n), nil
		}
	}
	if n, err := strconv.ParseUint(value, 10, 16); err == nil {
		return uint16(n), nil
	}
	n, err := strconv.ParseUint(value, 16, 16)
	return uint16(n), err
}

func FunctionName(value uint16) string {
	for _, def := range functionDefinitions {
		if def.Value == value {
			return def.Name
		}
	}
	return ""
}

func Functions() []FunctionDefinition {
	out := make([]FunctionDefinition, len(functionDefinitions))
	copy(out, functionDefinitions)
	return out
}

func HexType(value uint16) string {
	return fmt.Sprintf("0x%04X", value)
}

func decodeMarketText(data []byte) string {
	decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
	if err == nil {
		return string(decoded)
	}
	return string(data)
}

func trimPreview(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
