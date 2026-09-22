package quoteprotocol

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"
)

func (c *Client) GetServerTime(ctx context.Context) (time.Time, *Response, error) {
	resp, err := c.Request(ctx, Request{Type: RT_SERVERTIME})
	if err != nil {
		return time.Time{}, nil, err
	}
	serverTime, err := ParseServerTime(resp.Body)
	if err != nil {
		return time.Time{}, resp, err
	}
	return serverTime, resp, nil
}

func ParseServerTime(body []byte) (time.Time, error) {
	if len(body) < 20 {
		return time.Time{}, fmt.Errorf("RT_SERVERTIME body too short: %d", len(body))
	}
	if binary.LittleEndian.Uint16(body[0:2]) != RT_SERVERTIME {
		return time.Time{}, fmt.Errorf("unexpected server time response type 0x%04X", binary.LittleEndian.Uint16(body[0:2]))
	}
	seconds := int64(binary.LittleEndian.Uint32(body[16:20]))
	return time.Unix(seconds, 0), nil
}
