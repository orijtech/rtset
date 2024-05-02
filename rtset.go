// Copyright 2024 Orijtech Inc. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

/*
rtset aids in periodically comparing your system time against that of a common
source of truth, in this case "Roughtime" server implementation of Cloudflare.
Running it requires a set period, and a definition of maximum clock drift.
It firstly obtains Cloudflare's Roughtime Ed25519 public key and then at every period pings
the Roughtime server to get its reported time. If the absolute difference between your
system time (Tst) and Roughtime's server time (Trt) exceeds the defined maximum clock drift,
then your system time shall be set to Roughtime's server time (Trt)
For automatic time synchronization to work, you shall need to run the program with superuser
permissions as it'll require system time changes.
*/
package rtset

import (
	"context"
	"encoding/base64"
	"errors"
	"log"
	"net"
	"sync"
	"syscall"
	"time"

	rtclient "github.com/cloudflare/roughtime/client"
	rtconfig "github.com/cloudflare/roughtime/config"
)

type Client struct {
	syncEvery        time.Duration
	pingVersion      string
	cloudflarePubKey []byte
	maxClockDrift    time.Duration

	rts *rtconfig.Server

	runOnce sync.Once
}

// NewClient creates the client whose .Run method trigger periodic checks
// of clock drift between Cloudflare's Roughtime server (with public key verification)
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	// 1. Firstly get the public key for CloudFlare's RoughTime server.
	cloudflarePubKeyHex, err := lookupCloudFlarePublicKey(ctx)
	if err != nil {
		return nil, err
	}

	cloudflarePubKey, err := base64.StdEncoding.DecodeString(cloudflarePubKeyHex)
	if err != nil {
		return nil, err
	}

	client := &Client{
		cloudflarePubKey: cloudflarePubKey,
		pingVersion:      PING_VERSION_GOOGLE,
	}

	for _, opt := range opts {
		opt(client)
	}

	if client.maxClockDrift < 0 {
		return nil, errors.New("maxClockDrift cannot be a negative value")
	}

	client.rts = &rtconfig.Server{
		Name:          "",
		Version:       client.getPingVersion(),
		PublicKeyType: "ed25519",
		PublicKey:     cloudflarePubKey,
		Addresses: []rtconfig.ServerAddress{
			{
				Protocol: "udp",
				Address:  "roughtime.cloudflare.com:2003",
			},
		},
	}

	return client, nil
}

const PING_VERSION_GOOGLE = "Google-Roughtime"
const PING_VERSION_IETF = "IETF-Roughtime"

type Option func(*Client)

// WithSyncPeriod allows setting the period within which rtset will check for
// maximum clock drift between your system's time and the Roughtime server.
func WithSyncPeriod(syncPeriod time.Duration) Option {
	return func(c *Client) {
		c.syncEvery = syncPeriod
	}
}

// WithMaxClockDrift initializes the client with the defined maxClockDrift.
// maxClockDrift must be a positive value.
func WithMaxClockDrift(maxClockDrift time.Duration) Option {
	return func(c *Client) {
		c.maxClockDrift = maxClockDrift
	}
}

func (c *Client) getPingVersion() string {
	if c.pingVersion != "" {
		return c.pingVersion
	}
	return PING_VERSION_IETF
}

func lookupCloudFlarePublicKey(ctx context.Context) (string, error) {
	resolver := &net.Resolver{} // Use a net.Resolver so that we pass in context.
	records, err := resolver.LookupTXT(ctx, "roughtime.cloudflare.com")
	if err != nil {
		return "", err
	}
	if len(records) == 0 {
		return "", errors.New("no public key result could be found")
	}
	return records[0], nil
}

func (c *Client) Stop() error {
	return nil
}

const DEFAULT_SYNC_TIME_PERIOD = 30 * time.Minute

func (c *Client) syncTimePeriod() time.Duration {
	if c.syncEvery > 0 {
		return c.syncEvery
	}
	return DEFAULT_SYNC_TIME_PERIOD
}

// Run periodically compares your system time (Tst) against that
// of the Roughtime server time (Trt) and if the difference exceeds
// the defined maximum clock drift, will attempt to updated your
// system time to Trt, the Roughtime server time. Please note that the
// process should be run as a privileged user for automatic time updates.
func (c *Client) Run(ctx context.Context) error {
	var err = errors.New("already ran")
	c.runOnce.Do(func() {
		err = c.run(ctx)
	})
	return err
}

func (c *Client) run(ctx context.Context) error {
	ticker := time.NewTicker(c.syncTimePeriod())
	defer ticker.Stop()

	var prevRoughtime *rtclient.Roughtime

	for {
		select {
		case <-ctx.Done():
			// Cancelled so we can exit.
			// TODO: Log this exit.
			return nil

		case <-ticker.C:
			rtTimeFull, err := rtclient.Get(c.rts, 10, 2*time.Second, prevRoughtime)
			if err != nil {
				log.Printf("Failed to get latest Roughtime: %v", err)
				continue
			}

			prevRoughtime = rtTimeFull
			rtTime := rtTimeFull.Midpoint

			now := time.Now()
			absDiff, ok := c.needsSystemTimeUpdate(now, rtTime)
			if !ok { // No need to update the system time.
				log.Printf("No need to change the system since the time difference is only: %s", absDiff)
				continue
			}

			log.Printf("Attempting to change system time due to abs time difference of: %s", absDiff)

			if err := c.updateSystemTimeToLatest(rtTime); err == nil {
				log.Printf("Successfully changed system time from %q to %q", now, rtTime)
			} else {
				log.Printf("Failed to change system now: %v", err)
			}
		}
	}
}

const DEFAULT_MAX_CLOCK_DRIFT = 5 * time.Minute

func (c *Client) getMaxClockDrift() time.Duration {
	if c.maxClockDrift > 0 {
		return c.maxClockDrift
	}
	return DEFAULT_MAX_CLOCK_DRIFT
}

// needsSystemTimeUpdate returns true if the absolute difference
// between attemptedTime and the current system time differs by
// the defined maximum clock drift.
func (c *Client) needsSystemTimeUpdate(now, attemptedTime time.Time) (time.Duration, bool) {
	absDiff := attemptedTime.Sub(now).Abs()
	return absDiff, absDiff > c.getMaxClockDrift()
}

func (c *Client) updateSystemTimeToLatest(t time.Time) error {
	ts := syscall.NsecToTimeval(t.UnixNano())
	return syscall.Settimeofday(&ts)
}
