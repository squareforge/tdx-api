package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/transform"
)

const (
	packetMagic = "2003"

	rtLogin   = RT_LOGIN
	rtZipData = RT_ZIPDATA
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
	DataHead        *parsedDataHead `json:"data_head,omitempty"`
	Zip             *parsedZipData  `json:"zip,omitempty"`
	ErrorCode       int32           `json:"error_code,omitempty"`
	Size            int32           `json:"size,omitempty"`
	TextPreview     string          `json:"text_preview,omitempty"`
	RawTextHexStart string          `json:"raw_text_hex_start,omitempty"`
}

type parsedZipData struct {
	Type              string `json:"type"`
	OriginalType      string `json:"original_type"`
	CompressedBytes   int32  `json:"compressed_bytes"`
	DecompressedBytes int32  `json:"decompressed_bytes"`
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

func buildAskDataBody(requestType uint16, size int16, option int16, payload []byte) []byte {
	body := bytes.NewBuffer(make([]byte, 0, 20+len(payload)))
	_ = binary.Write(body, binary.LittleEndian, requestType)
	body.WriteByte(0) // m_nIndex
	body.WriteByte(0) // m_cSrv / operator flags
	_ = binary.Write(body, binary.LittleEndian, int32(0))
	body.Write(make([]byte, 8)) // HSPrivateKey
	_ = binary.Write(body, binary.LittleEndian, size)
	_ = binary.Write(body, binary.LittleEndian, option)
	body.Write(payload)
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
	unwrapped, _, err := unwrapResponseBody(body)
	if err != nil {
		return nil, err
	}
	resp, err := parsePlainLoginResponse(unwrapped)
	if err != nil {
		return nil, err
	}
	if len(body) >= 12 && binary.LittleEndian.Uint16(body[0:2]) == rtZipData {
		resp.Zip = &parsedZipData{
			Type:              fmt.Sprintf("0x%04X", rtZipData),
			OriginalType:      fmt.Sprintf("0x%04X", binary.LittleEndian.Uint16(body[2:4])),
			CompressedBytes:   int32(binary.LittleEndian.Uint32(body[4:8])),
			DecompressedBytes: int32(binary.LittleEndian.Uint32(body[8:12])),
		}
	}
	return resp, nil
}

type parsedPacketMeta struct {
	Compressed        bool   `json:"compressed"`
	WireType          string `json:"wire_type"`
	OriginalType      string `json:"original_type,omitempty"`
	CompressedBytes   int32  `json:"compressed_bytes,omitempty"`
	ExpandedBytes     int32  `json:"expanded_bytes,omitempty"`
	ResponseBodyBytes int    `json:"response_body_bytes"`
}

func unwrapResponseBody(body []byte) ([]byte, parsedPacketMeta, error) {
	meta := parsedPacketMeta{ResponseBodyBytes: len(body)}
	if len(body) >= 2 {
		meta.WireType = fmt.Sprintf("0x%04X", binary.LittleEndian.Uint16(body[0:2]))
	}
	if len(body) < 12 || binary.LittleEndian.Uint16(body[0:2]) != rtZipData {
		return body, meta, nil
	}
	zipLen := int32(binary.LittleEndian.Uint32(body[4:8]))
	origLen := int32(binary.LittleEndian.Uint32(body[8:12]))
	if zipLen < 0 || int(zipLen) > len(body)-12 {
		return nil, meta, fmt.Errorf("zip length mismatch: header=%d actual=%d", zipLen, len(body)-12)
	}
	decompressed, err := inflateZlib(body[12 : 12+zipLen])
	if err != nil {
		return nil, meta, err
	}
	meta.Compressed = true
	meta.OriginalType = fmt.Sprintf("0x%04X", binary.LittleEndian.Uint16(body[2:4]))
	meta.CompressedBytes = zipLen
	meta.ExpandedBytes = origLen
	return decompressed, meta, nil
}

func parsePlainLoginResponse(body []byte) (*parsedLoginResponse, error) {
	head, err := parseDataHead(body)
	if err != nil {
		return nil, err
	}
	resp := &parsedLoginResponse{DataHead: head}
	if len(body) >= 20 {
		packed := binary.LittleEndian.Uint32(body[16:20])
		resp.ErrorCode = int32(packed & 0x03)
		resp.Size = int32(packed >> 2)
	}
	if len(body) > 20 {
		rawText := bytes.TrimRight(body[20:], "\x00")
		if len(rawText) > 0 {
			resp.TextPreview = trimPreview(decodeMarketText(rawText), 700)
			prefix := rawText
			if len(prefix) > 48 {
				prefix = prefix[:48]
			}
			resp.RawTextHexStart = hex.EncodeToString(prefix)
		}
	}
	return resp, nil
}

func inflateZlib(data []byte) ([]byte, error) {
	reader, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func decodeMarketText(data []byte) string {
	decoded, _, err := transform.Bytes(simplifiedchinese.GB18030.NewDecoder(), data)
	if err == nil {
		return string(decoded)
	}
	return string(data)
}

func packetType(body []byte) (uint16, bool) {
	unwrapped, _, err := unwrapResponseBody(body)
	if err != nil || len(unwrapped) < 2 {
		return 0, false
	}
	return binary.LittleEndian.Uint16(unwrapped[0:2]), true
}

func trimPreview(text string, maxRunes int) string {
	text = strings.TrimSpace(text)
	runes := []rune(text)
	if len(runes) <= maxRunes {
		return text
	}
	return string(runes[:maxRunes]) + "..."
}
