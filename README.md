# High-Performance Sharded LRU Cache

这是一个基于 Go 语言实现的工业级本地缓存库。本项目从 LeetCode 146 题（LRU Cache）原型出发，针对生产环境进行了深度优化和功能扩展。(后期将添加一致性哈希逻辑实现分布式缓存)

## 🚀 核心扩展功能

### 1. 数据结构工业化
- **类型演进**：将原有的 `{int: int}` 映射升级为 `{string: []byte}`，适配通用的 KV 存储需求。
- **内存安全 (ByteView)**：
    - 引入 `ByteView` 结构体封装 `[]byte`。
    - **只读设计**：仅提供 `ByteSlice()` 方法返回数据副本，严防外部代码通过引用类型篡改缓存内部数据。

### 2. 精准内存控制
- **维度转换**：舍弃简单的“个数”统计，改用**字节数**统计。
- **计算逻辑**：`TotalSize = len(key) + len(value)`。通过精准计算每一条 Entry 占用的内存，有效防止 OOM（内存溢出）。

### 3. TTL 时效管理
- **过期支持**：新增 `expire` 纳秒级时间戳字段。
- **灵活策略**：支持设置任意时长 TTL，若 `expire` 为 0 则视为永久存储。

### 4. 四级复合清理体系 (Expiration Strategy)
为了平衡 CPU 损耗与内存回收率，系统构建了四层清理防线：

| 策略 | 触发时机 | 逻辑说明 |
| :--- | :--- | :--- |
| **惰性删除** | `Get` 操作 | 访问时检查，若过期则立即物理删除并返回空。 |
| **顺带扫描** | `Get/Put` 操作 | 每次操作随机抽样 5 个数据，发现过期则清理。 |
| **紧急避险** | `Put` 内存不足时 | 优先随机扫描 20 个 Key 清理过期数据，保护热数据不被 LRU 误踢。 |
| **全量扫描** | 每日 02:00 | 后台协程定时启动，对 256 个分片进行平滑轮询清理。 |

>

### 5. 高并发分片架构 (Sharding)
- **并发优化**：默认开启 **256 分片**。
- **设计原理**：通过 `Hash(key) % 256` 将全局大锁拆分为 256 把独立读写锁。
- **性能提升**：极大降低了锁竞争（Lock Contention），在多核环境下支持真正的并行读写。

---

## 🛠️ 技术实现细节

### 凌晨清理的平滑设计
为防止凌晨全量扫描时产生长时间的 STW（Stop The World），我们采用了**分片间歇休眠**技术：
1. 逐个获取分片锁。
2. 清理当前分片过期数据。
3. 释放锁并主动 `time.Sleep`，给业务请求留出处理空隙。
```
// 核心逻辑片段
func (sc *ShardedCache) CleanExpired() {
for _, shard := range sc.shards {
shard.mu.Lock()
shard.cache.CleanAllExpired() // 调用 LRU 层写的全量清理
shard.mu.Unlock()
time.Sleep(10 * time.Millisecond) // 平滑休眠
}
```






## 原算法具体实现
```
type Node struct{
    key,value int
    prev,next *Node
}

type LRUCache struct {
    capacity int
    cache map[int]*Node
    head,tail *Node
}


func Constructor(capacity int) LRUCache {
    l := LRUCache{
        capacity: capacity,
        cache:    make(map[int]*Node),
        head:     &Node{},
        tail:     &Node{},
    }
    l.head.next = l.tail
    l.tail.prev = l.head
    return l
}


func (this *LRUCache) Get(key int) int {
    if node,ok:=this.cache[key];ok{
        this.moveToHead(node)
        return node.value
    }
    return -1
}


func (this *LRUCache) Put(key int, value int)  {
    if node,ok:=this.cache[key];ok{
        node.value=value
        this.moveToHead(node)
    }else{
        newNode:=&Node{key:key,value:value}
        this.cache[key]=newNode
        this.addToHead(newNode)
        if len(this.cache)>this.capacity{
            remove:=this.removeTail()
            delete(this.cache,remove.key)
        }
    }
}

func (this *LRUCache) addToHead(node *Node) {
    node.prev = this.head
    node.next = this.head.next
    this.head.next.prev = node
    this.head.next = node
}

func (this *LRUCache) removeNode(node *Node) {
    node.prev.next = node.next
    node.next.prev = node.prev
}

func (this *LRUCache) moveToHead(node *Node) {
    this.removeNode(node)
    this.addToHead(node)
}

func (this *LRUCache) removeTail() *Node {
    node := this.tail.prev
    this.removeNode(node)
    return node
}

实现了最基本的Get、Put

扩展的功能：
1.改变原有数据类型{key:int,value:int}->{key:string,value:[]byte}
符合常规缓存的基本数据结构
另外由于value为[]byte类型，为了防止被外部修改该引用类型，建立ByteView结构体，对value进行封装，提供只读方法
2.支持统计内存大小
原LRU算法中，大小按照数据个数统计，现扩展为按照数据占用内存大小统计->size(key)+size(value)
3.支持数据定时删除功能（增加expire字段）
原LRU算法中，数据是永久存储的，现扩展为支持数据定时删除功能（expire为0即永久字段）
4.支持批量删除过期数据
为支持3，增加过期数据删除功能，具体逻辑实现
1）顺带扫描：在每次Get、Put操作时，随机检查是否有过期数据（顺带扫描5个随机数据），若有则删除
2）惰性删除：Get操作时，若数据过期，则删除
3）紧急避险：在Put操作时，若出现内存不足情况，先随机检查缓存中是否有过期数据（为防止长时间STW，随机扫描20个则结束），若有则删除；若检查后仍内存不足，则调用LRU算法淘汰最近最少使用的数据
4）全量扫描：后台启动定期扫描进程，每天凌晨两点执行，扫描所有数据，删除过期数据（为防止STW时间过长，采用分片扫描，平滑休眠）
5.增加256分片（默认值）
为提高并发性能防止Get、Put、Delete操作时对所有数据上锁，并通过分片将map拆分为256个分片，对每个分片单独加读写锁，大大提高并发性能
```
