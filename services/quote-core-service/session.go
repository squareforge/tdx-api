package main

import (
	"context"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

type RequestOptions struct {
	Addr         string
	Username     string
	Password     string
	Timeout      time.Duration
	RequestType  uint16
	Size         int16
	Option       int16
	Payload      []byte
	ExpectType   uint16
	MaxSkipCount int
}

type ProtocolResponse struct {
	Type          string           `json:"type"`
	FunctionName  string           `json:"function_name,omitempty"`
	Meta          parsedPacketMeta `json:"meta"`
	BodyBytes     int              `json:"body_bytes"`
	BodyHexPrefix string           `json:"body_hex_prefix,omitempty"`
	DataHead      *parsedDataHead  `json:"data_head,omitempty"`
	RawBody       []byte           `json:"-"`
}

type GenericRequestResult struct {
	Addr     string             `json:"addr"`
	Request  string             `json:"request"`
	Expected string             `json:"expected"`
	Skipped  []ProtocolResponse `json:"skipped,omitempty"`
	Response *ProtocolResponse  `json:"response,omitempty"`
	Error    string             `json:"error,omitempty"`
}

func DoLoggedInRequest(ctx context.Context, opts RequestOptions) GenericRequestResult {
	if opts.ExpectType == 0 {
		opts.ExpectType = opts.RequestType
	}
	if opts.MaxSkipCount == 0 {
		opts.MaxSkipCount = 8
	}
	result := GenericRequestResult{
		Addr:     opts.Addr,
		Request:  formatFunction(opts.RequestType),
		Expected: formatFunction(opts.ExpectType),
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", opts.Addr)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(opts.Timeout))

	loginBody := buildLoginBody(opts.Username, opts.Password, defaultLoginVariants()[0])
	if _, err := conn.Write(buildWirePacket(loginBody)); err != nil {
		result.Error = err.Error()
		return result
	}
	if _, err := readProtocolResponse(conn); err != nil {
		result.Error = "login failed: " + normalizeNetError(err)
		return result
	}

	requestBody := buildAskDataBody(opts.RequestType, opts.Size, opts.Option, opts.Payload)
	if _, err := conn.Write(buildWirePacket(requestBody)); err != nil {
		result.Error = err.Error()
		return result
	}

	for i := 0; i <= opts.MaxSkipCount; i++ {
		resp, err := readProtocolResponse(conn)
		if err != nil {
			result.Error = normalizeNetError(err)
			return result
		}
		if bodyType, ok := packetType(resp.RawBody); ok && bodyType == opts.ExpectType {
			result.Response = resp
			return result
		}
		result.Skipped = append(result.Skipped, *resp)
	}
	result.Error = fmt.Sprintf("expected %s not received after %d skipped packet(s)", result.Expected, opts.MaxSkipCount)
	return result
}

func readProtocolResponse(conn net.Conn) (*ProtocolResponse, error) {
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
	resp := &ProtocolResponse{
		Type:      "unknown",
		Meta:      meta,
		BodyBytes: len(unwrapped),
		RawBody:   unwrapped,
	}
	if len(unwrapped) >= 2 {
		t := binary.LittleEndian.Uint16(unwrapped[0:2])
		resp.Type = fmt.Sprintf("0x%04X", t)
		resp.FunctionName = functionName(t)
	}
	if len(unwrapped) > 0 {
		prefix := unwrapped
		if len(prefix) > 96 {
			prefix = prefix[:96]
		}
		resp.BodyHexPrefix = hex.EncodeToString(prefix)
	}
	if head, err := parseDataHead(unwrapped); err == nil {
		resp.DataHead = head
	}
	return resp, nil
}

func parseFunction(value string) (uint16, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty function type")
	}
	for _, def := range functionDefinitions {
		if strings.EqualFold(def.Name, value) {
			return def.Value, nil
		}
	}
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	n, err := strconv.ParseUint(value, 16, 16)
	if err == nil {
		return uint16(n), nil
	}
	n, err = strconv.ParseUint(value, 10, 16)
	return uint16(n), err
}

func functionName(value uint16) string {
	for _, def := range functionDefinitions {
		if def.Value == value {
			return def.Name
		}
	}
	return ""
}

func formatFunction(value uint16) string {
	name := functionName(value)
	if name == "" {
		return fmt.Sprintf("0x%04X", value)
	}
	return fmt.Sprintf("%s(0x%04X)", name, value)
}
