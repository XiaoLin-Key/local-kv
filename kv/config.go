package kv

type Config struct {
	maxBytes   int64
	shardCount int
}

type Option func(*Config)

// WithMaxBytes 设置内存大小
func WithMaxBytes(m int64) Option {
	return func(c *Config) {
		c.maxBytes = m
	}
}

// WithShards 设置分片数
func WithShards(s int) Option {
	return func(c *Config) {
		c.shardCount = s
	}
}
