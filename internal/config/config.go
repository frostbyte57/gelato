package config

import "runtime"

type Limits struct {
	DefaultBindHost      string
	DefaultPort          int
	MaxLineBytes         int
	MaxSources           int
	PerSourceBufferLines int
	GlobalBufferLines    int
	ShardCount           int
	QueueSizePerShard    int
	MaxConnsGlobal       int
	MaxConnsPerListener  int
	DrainBatchSize       int
	AsyncIngest          bool
	IngestQueueSize      int
}

func DefaultLimits() Limits {
	shards := defaultShardCount()
	return Limits{
		DefaultBindHost:      "127.0.0.1",
		DefaultPort:          9000,
		MaxLineBytes:         65536,
		MaxSources:           500,
		PerSourceBufferLines: 5000,
		GlobalBufferLines:    20000,
		ShardCount:           shards,
		QueueSizePerShard:    10000,
		MaxConnsGlobal:       2000,
		MaxConnsPerListener:  500,
		DrainBatchSize:       512,
		AsyncIngest:          true,
		IngestQueueSize:      shards * 1024,
	}
}

func defaultShardCount() int {
	count := runtime.GOMAXPROCS(0)
	if count < 2 {
		return 2
	}
	if count > 16 {
		return 16
	}
	return count
}
