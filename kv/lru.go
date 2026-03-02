package kv

import (
	"time"
)

//基础LRU算法实现层（由力扣146题演变而来）

type Node struct {
	key        string
	value      ByteView
	expire     int64
	prev, next *Node
}

type LRUCache struct {
	maxBytes   int64
	nowBytes   int64
	cache      map[string]*Node
	head, tail *Node
}

func NewCache(maxBytes int64) LRUCache {
	l := LRUCache{
		maxBytes: maxBytes,
		nowBytes: 0,
		cache:    make(map[string]*Node),
		head:     &Node{},
		tail:     &Node{},
	}
	l.head.next = l.tail
	l.tail.prev = l.head
	return l
}

func (this *LRUCache) Get(key string) []byte {
	this.tryEvictExpired(5)
	if node, ok := this.cache[key]; ok {
		if node.expire > 0 && node.expire < time.Now().UnixNano() {
			this.Delete(node.key)
			return nil
		}
		this.moveToHead(node)
		return node.value.ByteSlice()
	}
	return nil
}

func (this *LRUCache) Put(key string, value []byte, ttl time.Duration) error {
	this.tryEvictExpired(5)
	var expire int64
	if ttl > 0 {
		expire = time.Now().Add(ttl).UnixNano()
	}

	view := ByteView{b: cloneBytes(value)}
	itemBytes := int64(len(key)) + view.Len()
	if itemBytes > this.maxBytes {
		return ErrTooLarge
	}

	if node, ok := this.cache[key]; ok {
		this.nowBytes += view.Len() - node.value.Len()
		node.value = view
		node.expire = expire
		this.moveToHead(node)
	} else {
		newNode := &Node{
			key:    key,
			value:  view,
			expire: expire,
		}
		this.cache[key] = newNode
		this.addToHead(newNode)
		this.nowBytes += int64(len(key)) + view.Len()
	}
	if this.nowBytes > this.maxBytes {
		this.tryEvictExpired(20)
		if this.nowBytes > this.maxBytes {
			this.removeTail()
		}
	}
	return nil
}

func (this *LRUCache) Delete(key string) bool {
	if node, ok := this.cache[key]; ok {
		this.removeNode(node)
		delete(this.cache, key)
		this.nowBytes -= int64(len(key)) + node.value.Len()
		return true
	}
	return false
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

func (this *LRUCache) removeTail() {
	for this.nowBytes > this.maxBytes {
		tail := this.tail.prev
		if tail == this.head {
			break
		}
		this.removeNode(tail)
		delete(this.cache, tail.key)
		this.nowBytes -= int64(len(tail.key)) + tail.value.Len()
	}
}

func (this *LRUCache) tryEvictExpired(limit int) {
	count := 0
	now := time.Now().UnixNano()
	for _, node := range this.cache {
		count++
		if node.expire > 0 && node.expire < now {
			this.Delete(node.key)
		}
		if count >= limit {
			break
		}
	}
}

// 这个方法只应由后台任务在低峰期调用
func (this *LRUCache) CleanAllExpired() {
	now := time.Now().UnixNano()
	for _, node := range this.cache {
		if node.expire > 0 && node.expire < now {
			this.Delete(node.key)
		}
	}
}
