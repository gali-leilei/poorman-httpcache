package backup

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"
)

type ScanMetricsKeyArgs struct {
	Prefix string `json:"metrics"`
	Parent Parent `json:"parent"`
}

func (ScanMetricsKeyArgs) Kind() string {
	return "scan_metrics_key"
}

var scanMetricKey = `
local prefix = ARGV[1]
local cursor = ARGV[2]
local count = ARGV[3]
local bucket_count = ARGV[4]
local limit = tonumber(count) or 10
local bucket_count = tonumber(bucket_count) or 10

local keys = redis.call("SCAN", cursor, "MATCH", prefix .. "*", "COUNT", limit)
local new_cursor = keys[1]
local matched_keys = keys[2]

-- iterate over "prefix:*", get at most $count
local result = {}
for i = 1, #matched_keys do
	local single_result = {}
    local key = matched_keys[i]
	-- iterate over hash of "prefix:*", get at most $bucket_count
    local value = redis.call("HSCAN", key, 0, "MATCH", "bucket:*", "COUNT", bucket_count)
	local buckets = value[2]
	for i = 1, #buckets do
		local bucket_name = buckets[i]
		local bucket_val = redis.call("HGET", key, bucket_name)
		single_result[bucket_name] = bucket_val
	end
    result[key] = single_result
end

-- pack the result into a json
local final = cjson.encode(result)
return {new_cursor, final}
`

type ScanMetricsKeyResult struct {
	Cursor string                       `json:"cursor"`
	Result map[string]map[string]string `json:"result"`
}

type ScanMetricsKeyWorker struct {
	river.WorkerDefaults[ScanMetricsKeyArgs]
	dbPool    *pgxpool.Pool
	redis     *redis.Client
	luaScript *redis.Script
}

func NewScanMetricsKeyWorker(redisC *redis.Client) *ScanMetricsKeyWorker {
	return &ScanMetricsKeyWorker{
		redis:     redisC,
		luaScript: redis.NewScript(scanMetricKey),
	}
}

func parseRedisScriptResult(result any) (*ScanMetricsKeyResult, error) {
	values, ok := result.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid script result format")
	}

	if len(values) != 2 {
		return nil, fmt.Errorf("invalid script result format")
	}

	cursor, ok := values[0].(string)
	if !ok {
		return nil, fmt.Errorf("invalid script result format")
	}

	keys, ok := values[1].(string)
	if !ok {
		return nil, fmt.Errorf("invalid script result format")
	}

	// json unmarshal
	var twoLevelJson map[string]map[string]string
	err := json.Unmarshal([]byte(keys), &twoLevelJson)
	if err != nil {
		return nil, fmt.Errorf("invalid script result format")
	}

	finalResult := &ScanMetricsKeyResult{
		Cursor: cursor,
		Result: twoLevelJson,
	}

	return finalResult, nil
}

func (w *ScanMetricsKeyWorker) Run(ctx context.Context, job *river.Job[ScanMetricsKeyArgs]) error {
	client := river.ClientFromContext[pgx.Tx](ctx)

	tx, err := w.dbPool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	keys := []string{}
	cursor := "0"
	for {
		argv := []string{job.Args.Prefix, cursor, "100"}
		result, err := w.luaScript.Run(ctx, w.redis, keys, argv).Result()
		if err != nil {
			return err
		}

		cursor, metricKeys, err = parseRedisScriptResult(result)
		if err != nil {
			return err
		}

		_, err := client.InsertTx(ctx, tx, ArchiveMetricKeyJob, job.Args)
		if err != nil {
			return err
		}
	}
	return nil
}
