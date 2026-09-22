package main

import (
	"fmt"
	"time"

	"github.com/injoyai/tdx"
)

const tdxProbeCode = "sh600000"

// tdxRuntime owns the only standard TDX connection used by market-service.
//
// DialDefault keeps the upstream failover dialer: when the connection drops,
// the client retries the configured Hosts list. The one-slot pool lets the
// existing tdx.Manage APIs reuse this same client instead of opening their own
// connections.
type tdxRuntime struct {
	client *tdx.Client
	pool   tdx.IPool
}

func newTDXRuntime() (*tdxRuntime, error) {
	client, err := tdx.DialDefault(
		tdx.WithDebug(false),
		tdx.WithRedial(true),
	)
	if err != nil {
		return nil, err
	}

	// DialDefault establishes TCP and sends the protocol handshake, but it can
	// return before the server has accepted an actual request. Probe a normal
	// quote so startup never publishes a half-open client as ready. Retries give
	// the built-in failover dialer time to move to the next host after EOF.
	client.SetTimeout(8 * time.Second)
	var probeErr error
	for attempt := 0; attempt < 3; attempt++ {
		if _, probeErr = client.GetQuote(tdxProbeCode); probeErr == nil {
			break
		}
		if attempt < 2 {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if probeErr != nil {
		client.Close()
		return nil, fmt.Errorf("TDX 行情探测失败: %w", probeErr)
	}

	// NewPool does not dial here: its dial function returns the already
	// verified client exactly once. All manager operations therefore share the
	// same connection and the same automatic host failover path.
	pool, err := tdx.NewPool(func() (*tdx.Client, error) {
		return client, nil
	}, 1)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("初始化 TDX 共享连接池失败: %w", err)
	}

	return &tdxRuntime{client: client, pool: pool}, nil
}

func (r *tdxRuntime) Client() *tdx.Client {
	if r == nil {
		return nil
	}
	return r.client
}

func (r *tdxRuntime) Pool() tdx.IPool {
	if r == nil {
		return nil
	}
	return r.pool
}
