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

package main

import (
	"context"
	"flag"
	"os"
	"os/signal"
	"time"

	"github.com/orijtech/rtset"
)

func main() {
	syncPeriodStr := flag.String("sync-period", "30s", "The synchronization period")
	maxClockDriftStr := flag.String("max-clock-drift", "8m", "The maximum clock drift between which to compare and set times")
	flag.Parse()

	syncPeriod := must(time.ParseDuration(*syncPeriodStr))
	maxClockDrift := must(time.ParseDuration(*maxClockDriftStr))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tsc, err := rtset.NewClient(ctx,
		rtset.WithSyncPeriod(syncPeriod),
		rtset.WithMaxClockDrift(maxClockDrift))
	if err != nil {
		panic(err)
	}
	defer tsc.Stop()

	go tsc.Run(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	<-ch
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
