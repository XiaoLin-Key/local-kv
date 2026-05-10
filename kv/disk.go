package kv

import (
	"fmt"
	"os"
)

func diskHandler(msg *Message) error {
	// 落盘逻辑（AOF）
	f, err := os.OpenFile("./data/cache.aof", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if msg.ExpireAt >= 0 {
		_, err = f.WriteString(fmt.Sprintf("SET %s %s %d\n", msg.Key, msg.Value, msg.ExpireAt))
	} else {
		_, err = f.WriteString(fmt.Sprintf("DEL %s\n", msg.Key))
	}
	fmt.Printf("[磁盘] 成功 → %s\n", msg.Key)
	return nil
}

func StartDiskConsumer() {
	mq.Subscribe("disk", func(msg *Message) error {
		return diskHandler(msg)
	})
}
