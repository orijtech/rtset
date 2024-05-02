## rtset

rtset aka "Roughtime Set" is a package whose purpose is to provide time synchronization utilities for the new generation
of consensus algorithm that allows proposers to send along their time in a vote and if the other proposers find that the
time is within their bounds, they'll accept the vote otherwise reject it. Previously the consensus algorithm relied on taking
the median of all the times proposed, but that's susceptible to a Byzantine attack or control whereby a malicious
1/3 of the network could skew time.

It uses Google's originated RoughTime NTP secured service, but run by CloudFlare. timeset gives proposers
the option of having their time periodically set which means that there is a source of truth for everyone who opts in.

## Integrating into your code
Inside your main.go file, please add the following code

```go
package main

import (
	"context"
	"os"
	"os/signal"
	"time"

	"github.com/orijtech/rtset"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const PERIOD_TO_SYNC = 10*time.Minute
	tsc, err := rtset.NewClient(ctx, rtset.WithSyncPeriod(PERIOD_TO_SYNC))
	if err != nil {
		panic(err)
	}
	defer tsc.Stop()

	go tsc.Run(ctx)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	<-ch
}
```
