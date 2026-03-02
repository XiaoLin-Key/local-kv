package main

import (
	"fmt"
	"kv/kv"
	"sync"
	"time"
)

func main() {
	// 1. 初始化缓存：1MB 内存，2 分片，每秒清理一次（方便测试，生产建议 5 分钟）
	cache := kv.NewShardedCache(
		kv.WithMaxBytes(1024*1024),
		kv.WithShards(2),
	)
	defer cache.Close()

	fmt.Println("=== 场景 1：TTL 过期测试 ===")
	// 存入一个 2 秒过期的 key
	_ = cache.PutWithTTL("short_live", []byte("hello"), 2*time.Second)
	// 存入一个永久 key
	_ = cache.Put("permanent", []byte("world"))

	val, _ := cache.Get("short_live")
	fmt.Printf("立即获取 short_live: %s\n", string(val))

	fmt.Println("等待 3 秒后...")
	time.Sleep(3 * time.Second)

	_, err := cache.Get("short_live")
	if err == kv.ErrNotFound {
		fmt.Println("验证成功：short_live 已过期并被惰性删除")
	}

	valP, _ := cache.Get("permanent")
	fmt.Printf("永久 key 依然存在: %s\n", string(valP))

	fmt.Println("\n=== 场景 2：内存淘汰策略测试 (先过期后 LRU) ===")
	// 创建一个极小缓存：30 字节
	tinyCache := kv.NewShardedCache(kv.WithMaxBytes(30), kv.WithShards(1))

	// 存入 key1 (10B), key2 (10B)，都设为快过期
	_ = tinyCache.PutWithTTL("k1", []byte("val1"), 1*time.Second) // 约 10B
	_ = tinyCache.PutWithTTL("k2", []byte("val2"), 1*time.Second) // 约 10B

	time.Sleep(2 * time.Second) // 让它们过期

	// 存入 key3 (15B)，此时内存本该不够，但由于 k1, k2 已过期，Put 会先清理它们
	fmt.Println("存入 k3，触发采样清理已过期的 k1, k2...")
	_ = tinyCache.Put("k3", []byte("value3"))

	fmt.Printf("当前总内存占用: %d 字节\n", tinyCache.TotalBytes())

	fmt.Println("\n=== 场景 3：并发安全压测 ===")
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			k := fmt.Sprintf("key_%d", id)
			_ = cache.PutWithTTL(k, []byte("data"), time.Duration(id)*time.Millisecond)
			_, _ = cache.Get(k)
		}(i)
	}
	wg.Wait()
	fmt.Println("验证成功：1000 协程并发读写无 panic")

	fmt.Println("\n=== 场景 4：凌晨两点平滑扫描模拟 ===")
	fmt.Println("手动触发一次全量平滑扫描...")
	cache.CleanExpired() // 模拟后台任务调用的逻辑
	fmt.Println("扫描完成，系统运行平稳")

	fmt.Println("\n=== 场景 5：全量永久数据 LRU 淘汰测试 ===")
	// 创建 20 字节缓存，1 个分片
	permCache := kv.NewShardedCache(kv.WithMaxBytes(20), kv.WithShards(1))
	// k1-k4 每个占用 5 字节 (key:2B + val:3B)，总共 20 字节
	_ = permCache.Put("k1", []byte("v11"))
	_ = permCache.Put("k2", []byte("v22"))
	_ = permCache.Put("k3", []byte("v33"))
	_ = permCache.Put("k4", []byte("v44"))

	fmt.Printf("存入 4 个永久 key 后，当前内存: %d 字节\n", permCache.TotalBytes())

	fmt.Println("访问 k1，使其成为最近使用的节点...")
	_, _ = permCache.Get("k1")

	// 此时内存已满 (20/20)。存入 k5 预期会根据 LRU 算法淘汰 k2（因为 k1 刚被访问过）
	fmt.Println("存入 k5 (5字节)，预期根据 LRU 算法淘汰目前最旧的 k2 (k1 刚被访问)...")
	_ = permCache.Put("k5", []byte("v55"))

	_, errK1 := permCache.Get("k1")
	if errK1 == nil {
		fmt.Println("验证成功：k1 依然存在，没有被淘汰")
	} else {
		fmt.Println("验证失败：k1 被误淘汰了")
	}

	_, errK2 := permCache.Get("k2")
	if errK2 == kv.ErrNotFound {
		fmt.Println("验证成功：k2 已被正确淘汰")
	} else {
		fmt.Println("验证失败：k2 依然存在")
	}

	valK5, _ := permCache.Get("k5")
	fmt.Printf("最新存入的 k5 依然存在: %s\n", string(valK5))
}
