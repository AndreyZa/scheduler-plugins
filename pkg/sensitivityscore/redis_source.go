package sensitivityscore

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

// redisNodeMetricsKeyPrefix matches the agent's node:metrics:<node> hash
// family (metrics-agent/pkg/redisclient, docs §3.2) — the per-node aggregate
// meant for the scheduler's hot-path read, as opposed to job:metrics:* which
// is analysis-only.
const redisNodeMetricsKeyPrefix = "node:metrics:"

// redisMetricsSource reads the current PressureVector for every node from
// Redis, replacing the MVP's single node-metrics.json file. The agent writes
// each dimension already normalized to [0,1] where that's honest (docs §3.1:
// "нормализовать в единый PressureVector перед записью в Redis"): LLC as
// miss ratio, IO as the PSI io.pressure stall share (field io_pressure — NOT
// raw io_iops, which has no honest [0,1] scale without a per-device
// max-IOPS calibration). Net has the identical problem one level up: raw
// net_bw is bytes/sec with no per-NIC bandwidth calibration, and cgroup v2
// has no network-PSI equivalent to borrow (PSI only covers cpu/io/memory) —
// so it stays out of the score entirely (nodePressure.Net is always 0) until
// a stand-specific bandwidth reference is defined; net_bw is still written
// to Redis and kept for analysis. loadAll converts what IS scored to this
// package's existing 0-100 pressure scale, defensively clamping to [0,1]
// first so one bad upstream value can't blow up the score's dot product.
type redisMetricsSource struct {
	rdb *redis.Client
}

func newRedisMetricsSource(addr string) *redisMetricsSource {
	return &redisMetricsSource{rdb: redis.NewClient(&redis.Options{Addr: addr})}
}

// loadAll scans all node:metrics:<node> keys and returns the parsed
// nodeMetrics map. Uses SCAN (not KEYS) so a large node count doesn't block
// Redis's single-threaded command loop.
func (r *redisMetricsSource) loadAll(ctx context.Context) (nodeMetrics, error) {
	out := make(nodeMetrics)

	iter := r.rdb.Scan(ctx, 0, redisNodeMetricsKeyPrefix+"*", 0).Iterator()
	for iter.Next(ctx) {
		key := iter.Val()
		nodeName := strings.TrimPrefix(key, redisNodeMetricsKeyPrefix)
		if nodeName == key || nodeName == "" {
			continue // shouldn't happen given the SCAN pattern, but don't let a stray key panic on empty node name
		}

		fields, err := r.rdb.HGetAll(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("HGETALL %s: %w", key, err)
		}
		out[nodeName] = nodePressure{
			LLC:  parsePressureField(fields["llc_miss_rate"]) * 100,
			NUMA: parsePressureField(fields["numa_remote_ratio"]) * 100,
			// Net: intentionally not read — see this file's package-level
			// doc comment (raw net_bw has no honest [0,1] scale yet).
			IO: parsePressureField(fields["io_pressure"]) * 100,
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan %s*: %w", redisNodeMetricsKeyPrefix, err)
	}

	return out, nil
}

// parsePressureField parses one Redis hash field as a float and clamps it to
// [0,1] — see redisMetricsSource's doc comment for why the clamp is needed.
func parsePressureField(raw string) float64 {
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	switch {
	case v < 0:
		return 0
	case v > 1:
		return 1
	default:
		return v
	}
}

func (r *redisMetricsSource) Close() error { return r.rdb.Close() }
