package kv

import (
	"fmt"
)

// 消息结构
type Message struct {
	Topic    string      // 动态 topic
	Key      string      // 缓存 key
	Value    interface{} // 缓存 value
	ExpireAt int64       // 0=永久
	RetryCnt int         // 重试次数
}

// 动态 Topic 消息队列
type SimpleMQ struct {
	queues   map[string]chan *Message // 动态 topic -> 队列
	dlq      chan *Message            // 死信队列
	maxRetry int                      // 最大重试次数
}

// 全局单例
var mq = &SimpleMQ{
	queues:   make(map[string]chan *Message),
	dlq:      make(chan *Message, 512),
	maxRetry: 3,
}

// 初始化/获取 Topic（动态创建）
func (m *SimpleMQ) topic(topic string) chan *Message {
	if ch, ok := m.queues[topic]; ok {
		return ch
	}

	ch := make(chan *Message, 1024)
	m.queues[topic] = ch
	return ch
}

// 发送消息
func (m *SimpleMQ) Send(topic string, key string, value interface{}, expireAt int64) {
	msg := &Message{
		Topic:    topic,
		Key:      key,
		Value:    value,
		ExpireAt: expireAt,
	}

	select {
	case m.topic(topic) <- msg:
	default:
		fmt.Printf("[WARN] 队列已满 topic=%s key=%s\n", topic, key)
	}
}

// 订阅消费
func (m *SimpleMQ) Subscribe(topic string, handler func(msg *Message) error) {
	ch := m.topic(topic)

	go func() {
		for msg := range ch {
			err := handler(msg)

			if err != nil {
				msg.RetryCnt++
				if msg.RetryCnt > m.maxRetry {
					m.dlq <- msg
					fmt.Printf("[DLQ] 重试耗尽: %s %s\n", topic, msg.Key)
				} else {
					fmt.Printf("[RETRY] %s %s (%d)\n", topic, msg.Key, msg.RetryCnt)
					ch <- msg
				}
			}
		}
	}()
}

// 死信队列
func (m *SimpleMQ) StartDLQ() {
	go func() {
		for msg := range m.dlq {
			fmt.Printf("[死信] %s %s\n", msg.Topic, msg.Key)
		}
	}()
}
