package kv

//实现分片map

import (
	"hash/fnv"
	"sync"
	"time"
)

type Shard struct {
	mu    sync.RWMutex
	cache LRUCache
}

type ShardedCache struct {
	shardCount int
	shards     []*Shard
	stopCh     chan struct{}
}

func NewShardedCache(opts ...Option) *ShardedCache {
	//设定默认值
	conf := &Config{
		maxBytes:   128 * 1024 * 1024, // 默认 128MB
		shardCount: 256,               // 默认 256 分片
	}
	//覆盖默认值
	for _, opt := range opts {
		opt(conf)
	}
	//防止创建分片数过小
	if conf.maxBytes/int64(conf.shardCount) < 512 {
		conf.shardCount = 1
	}

	sc := &ShardedCache{
		shardCount: conf.shardCount,
		shards:     make([]*Shard, conf.shardCount),
		stopCh:     make(chan struct{}),
	}
	maxBytesPerShard := conf.maxBytes / int64(conf.shardCount)
	for i := 0; i < conf.shardCount; i++ {
		sc.shards[i] = &Shard{
			cache: NewCache(maxBytesPerShard),
		}
	}
	go sc.dailyCleanupLoop()
	return sc
}

func (sc *ShardedCache) getShard(key string) *Shard {
	h := fnv.New32a()
	h.Write([]byte(key))
	index := h.Sum32() % uint32(sc.shardCount)
	return sc.shards[index]
}

func (sc *ShardedCache) Get(key string) ([]byte, error) {
	shard := sc.getShard(key)
	shard.mu.RLock() // 读锁
	defer shard.mu.RUnlock()

	res := shard.cache.Get(key)
	if len(res) == 0 {
		return nil, ErrNotFound
	}
	return res, nil
}

func (sc *ShardedCache) Put(key string, value []byte) error {
	shard := sc.getShard(key)
	shard.mu.Lock() // 写锁
	defer shard.mu.Unlock()

	return shard.cache.Put(key, value, time.Duration(0))
}

func (sc *ShardedCache) PutWithTTL(key string, value []byte, ttl time.Duration) error {
	shard := sc.getShard(key)
	shard.mu.Lock() // 写锁
	defer shard.mu.Unlock()

	return shard.cache.Put(key, value, ttl)
}

func (sc *ShardedCache) Delete(key string) bool {
	shard := sc.getShard(key)
	shard.mu.Lock() // 写锁
	defer shard.mu.Unlock()

	return shard.cache.Delete(key)
}

// 汇总所有分片占用的内存
func (sc *ShardedCache) TotalBytes() int64 {
	var total int64
	for _, shard := range sc.shards {
		shard.mu.RLock()
		total += shard.cache.nowBytes
		shard.mu.RUnlock()
	}
	return total
}

// CleanExpired 每天凌晨两点进行全体过期扫描
// 但分片进行平滑扫描
func (sc *ShardedCache) CleanExpired() {
	for _, shard := range sc.shards {
		shard.mu.Lock()
		shard.cache.CleanAllExpired() // 调用 LRU 层写的全量清理
		shard.mu.Unlock()
		time.Sleep(10 * time.Millisecond) // 平滑休眠
	}
}

func nextCleanupDuration() time.Duration {
	now := time.Now()
	// 构造今天的凌晨两点
	next := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	// 如果现在已经过了两点，就加 24 小时指向明天的两点
	if now.After(next) {
		next = next.Add(24 * time.Hour)
	}
	return next.Sub(now)
}

func (sc *ShardedCache) dailyCleanupLoop() {
	for {
		timer := time.NewTimer(nextCleanupDuration())
		select {
		case <-timer.C:
			sc.CleanExpired() // 执行具体的分片扫描
		case <-sc.stopCh:
			timer.Stop()
			return
		}
	}
}

// 优雅关机，配合上面的dailyCleanupLoop
func (sc *ShardedCache) Close() {
	close(sc.stopCh)
}
