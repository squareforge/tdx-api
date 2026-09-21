package main

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

type ProbeRequest struct {
	Addr     string
	Username string
	Password string
	Timeout  time.Duration
	Mode     string
}

type ProbeAttempt struct {
	Variant       loginRequestVariant  `json:"variant"`
	Connected     bool                 `json:"connected"`
	ElapsedMS     float64              `json:"elapsed_ms"`
	RequestBytes  int                  `json:"request_bytes"`
	RequestBody   int                  `json:"request_body"`
	ResponseBytes int                  `json:"response_bytes,omitempty"`
	Magic         string               `json:"magic,omitempty"`
	BodyLength    uint32               `json:"body_length,omitempty"`
	BodyHexPrefix string               `json:"body_hex_prefix,omitempty"`
	ParsedLogin   *parsedLoginResponse `json:"parsed_login,omitempty"`
	Error         string               `json:"error,omitempty"`
}

type ProbeResult struct {
	Addr     string         `json:"addr"`
	Username string         `json:"username"`
	Mode     string         `json:"mode"`
	Success  bool           `json:"success"`
	Attempts []ProbeAttempt `json:"attempts"`
}

func ProbeLogin(ctx context.Context, req ProbeRequest) ProbeResult {
	result := ProbeResult{
		Addr:     req.Addr,
		Username: req.Username,
		Mode:     req.Mode,
	}
	var variants []loginRequestVariant
	if variant, ok := selectLoginVariant(req.Mode); ok {
		variants = []loginRequestVariant{variant}
	} else {
		result.Mode = "auto"
		variants = defaultLoginVariants()
	}

	for _, variant := range variants {
		attempt := probeLoginOnce(ctx, req, variant)
		result.Attempts = append(result.Attempts, attempt)
		if attempt.Error == "" && attempt.ResponseBytes > 0 {
			result.Success = true
			break
		}
	}
	return result
}

func probeLoginOnce(ctx context.Context, req ProbeRequest, variant loginRequestVariant) ProbeAttempt {
	start := time.Now()
	body := buildLoginBody(req.Username, req.Password, variant)
	packet := buildWirePacket(body)
	attempt := ProbeAttempt{
		Variant:      variant,
		RequestBytes: len(packet),
		RequestBody:  len(body),
	}

	var dialer net.Dialer
	conn, err := dialer.DialContext(ctx, "tcp", req.Addr)
	attempt.ElapsedMS = float64(time.Since(start).Microseconds()) / 1000
	if err != nil {
		attempt.Error = err.Error()
		return attempt
	}
	attempt.Connected = true
	defer conn.Close()

	deadline := time.Now().Add(req.Timeout)
	_ = conn.SetDeadline(deadline)
	if _, err := conn.Write(packet); err != nil {
		attempt.Error = err.Error()
		return attempt
	}

	header := make([]byte, 8)
	if _, err := io.ReadFull(conn, header); err != nil {
		attempt.Error = normalizeNetError(err)
		return attempt
	}
	if string(header[:4]) != packetMagic {
		attempt.Magic = string(header[:4])
		attempt.BodyHexPrefix = hex.EncodeToString(header)
		attempt.Error = fmt.Sprintf("unexpected response magic %q", attempt.Magic)
		return attempt
	}
	bodyLen := uint32(header[4]) | uint32(header[5])<<8 | uint32(header[6])<<16 | uint32(header[7])<<24
	respBody := make([]byte, bodyLen)
	if _, err := io.ReadFull(conn, respBody); err != nil {
		attempt.Error = normalizeNetError(err)
		return attempt
	}
	respPacket := append(header, respBody...)
	magic, parsedBodyLen, parsedBody, err := parseWirePacket(respPacket)
	attempt.ResponseBytes = len(respPacket)
	attempt.Magic = magic
	attempt.BodyLength = parsedBodyLen
	if len(parsedBody) > 0 {
		prefix := parsedBody
		if len(prefix) > 96 {
			prefix = prefix[:96]
		}
		attempt.BodyHexPrefix = hex.EncodeToString(prefix)
		if loginResp, parseErr := parseLoginResponse(parsedBody); parseErr == nil {
			attempt.ParsedLogin = loginResp
		}
	}
	if err != nil {
		attempt.Error = err.Error()
	}
	return attempt
}

func normalizeNetError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, io.EOF) {
		return "connection closed by peer"
	}
	if ne, ok := err.(net.Error); ok && ne.Timeout() {
		return "timeout waiting for response"
	}
	return err.Error()
}
