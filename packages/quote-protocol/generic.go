package quoteprotocol

import "context"

func (c *Client) RequestFunction(ctx context.Context, function string, size int16, option int16, payload []byte) (*Response, error) {
	requestType, err := ParseFunction(function)
	if err != nil {
		return nil, err
	}
	return c.Request(ctx, Request{
		Type:    requestType,
		Size:    size,
		Option:  option,
		Payload: payload,
	})
}

func (c *Client) RequestFunctionWithExpect(ctx context.Context, function string, expect string, size int16, option int16, payload []byte) (*Response, error) {
	requestType, err := ParseFunction(function)
	if err != nil {
		return nil, err
	}
	expectType := requestType
	if expect != "" {
		expectType, err = ParseFunction(expect)
		if err != nil {
			return nil, err
		}
	}
	return c.Request(ctx, Request{
		Type:       requestType,
		ExpectType: expectType,
		Size:       size,
		Option:     option,
		Payload:    payload,
	})
}
