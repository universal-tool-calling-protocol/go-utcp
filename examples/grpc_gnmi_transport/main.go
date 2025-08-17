package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"time"

	authpkg "github.com/universal-tool-calling-protocol/go-utcp/src/auth"
	. "github.com/universal-tool-calling-protocol/go-utcp/src/providers/base"
	providers "github.com/universal-tool-calling-protocol/go-utcp/src/providers/grpc"
	transports "github.com/universal-tool-calling-protocol/go-utcp/src/transports/grpc"
)

func main() {
	// Transport with a simple logger
	tr := transports.NewGRPCClientTransport(func(f string, a ...interface{}) { log.Printf(f, a...) })

	// Build a concrete BasicAuth value, store it in an interface variable,
	// then pass a *pointer to that interface variable* (as required by GRPCProvider.Auth).
	var creds authpkg.Auth = &authpkg.BasicAuth{
		AuthType: authpkg.BasicType,
		Username: "lab",
		Password: "xxxxxx",
	}

	prov := &providers.GRPCProvider{
		BaseProvider: BaseProvider{
			Name:         "g",
			ProviderType: ProviderGRPC,
		},
		Host:        "172.41.151.7",
		Port:        6030,
		ServiceName: "gnmi.gNMI",
		MethodName:  "Subscribe",
		Target:      "",     // set your gNMI target here if the device needs it
		UseSSL:      false,  // insecure transport (TLS off)
		Auth:        &creds, // NOTE: pointer to the interface variable
	}

	// Test three per-subscription modes on the same leaf
	tests := []map[string]any{
		{
			// SAMPLE once per second
			"path":               "/interfaces/interface[name=Ethernet2]/state/counters/out-multicast-pkts",
			"mode":               "STREAM",      // list mode
			"sub_mode":           "SAMPLE",      // per-subscription mode
			"sample_interval_ns": 1_000_000_000, // 1s in nanoseconds
		},
		{
			// ON_CHANGE with heartbeat every 10s; suppress_redundant=false so you’ll see heartbeats too
			"path":                  "/interfaces/interface[name=Ethernet2]/state/counters/out-multicast-pkts",
			"mode":                  "STREAM",
			"sub_mode":              "ON_CHANGE",
			"heartbeat_interval_ns": 10_000_000_000, // 10s
			"suppress_redundant":    false,
		},
		{
			// TARGET_DEFINED — device decides cadence
			"path":     "/interfaces/interface[name=Ethernet2]/state/counters/out-multicast-pkts",
			"mode":     "STREAM",
			"sub_mode": "TARGET_DEFINED",
		},
	}

	for _, args := range tests {
		runOnce(tr, prov, args, 12*time.Second, 3) // 12s timeout, print up to 3 messages
	}
}

func runOnce(
	tr *transports.GRPCClientTransport,
	prov Provider,
	args map[string]any,
	timeout time.Duration,
	maxToShow int,
) {
	log.Printf("=== Testing sub_mode=%v (path=%v) ===", args["sub_mode"], args["path"])

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	sr, err := tr.CallToolStream(ctx, "gnmi_subscribe", args, prov)
	if err != nil {
		log.Printf("sub_mode=%v -> CallToolStream error: %v", args["sub_mode"], err)
		return
	}
	defer sr.Close()

	received := 0
	for {
		item, err := sr.Next()
		if err != nil {
			if err == io.EOF || ctx.Err() != nil {
				break
			}
			log.Printf("sub_mode=%v next error: %v", args["sub_mode"], err)
			break
		}
		received++

		// Pretty-print a couple of messages to confirm content
		if received <= maxToShow {
			switch v := item.(type) {
			case []byte:
				var m map[string]any
				if err := json.Unmarshal(v, &m); err == nil {
					b, _ := json.MarshalIndent(m, "", "  ")
					log.Printf("sub_mode=%v update #%d:\n%s", args["sub_mode"], received, b)
				} else {
					log.Printf("sub_mode=%v update #%d (bytes): %q", args["sub_mode"], received, string(v))
				}
			case map[string]any:
				b, _ := json.MarshalIndent(v, "", "  ")
				log.Printf("sub_mode=%v update #%d:\n%s", args["sub_mode"], received, b)
			default:
				log.Printf("sub_mode=%v update #%d (type %T): %v", args["sub_mode"], received, v, v)
			}
		}
	}

	log.Printf("sub_mode=%v done (received %d messages)\n", args["sub_mode"], received)
}
