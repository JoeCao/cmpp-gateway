package gateway

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	bolt "go.etcd.io/bbolt"
)

// BoltCache 使用 BoltDB 作为存储后端
type BoltCache struct {
	db *bolt.DB
}

var (
	// Bucket 名称
	waitBucket    = []byte("wait")     // 等待队列
	messageBucket = []byte("messages") // 消息列表
	moBucket      = []byte("mo")       // MO消息列表
)

// StartBoltCache 初始化 BoltDB
func StartBoltCache(dbPath string) (*BoltCache, error) {
	// 打开数据库文件，如果不存在则创建
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		Timeout: 3 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("打开BoltDB失败: %w", err)
	}

	// 创建必要的 Buckets
	err = db.Update(func(tx *bolt.Tx) error {
		for _, bucketName := range [][]byte{waitBucket, messageBucket, moBucket} {
			_, err := tx.CreateBucketIfNotExists(bucketName)
			if err != nil {
				return fmt.Errorf("创建bucket失败: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}

	Infof("[CACHE] BoltDB 初始化成功: %s", dbPath)
	return &BoltCache{db: db}, nil
}

// StopBoltCache 关闭数据库
func (c *BoltCache) StopBoltCache() error {
	if c.db != nil {
		Debugf("[CACHE] 关闭 BoltDB")
		return c.db.Close()
	}
	return nil
}

// SetWaitCache 将发送的记录放到等待缓存中
func (c *BoltCache) SetWaitCache(key uint32, message SmsMes) error {
	if c.db == nil {
		Warnf("[CACHE] BoltDB 未初始化，跳过 SetWaitCache")
		return errors.New("database not initialized")
	}

	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(waitBucket)
		if b == nil {
			return errors.New("wait bucket not found")
		}

		// 序列化消息
		data, err := json.Marshal(message)
		if err != nil {
			return err
		}

		// 使用 SeqId 作为 key
		keyBytes := uint32ToBytes(key)
		return b.Put(keyBytes, data)
	})
}

// GetWaitCache 获取并删除等待缓存
func (c *BoltCache) GetWaitCache(key uint32) (SmsMes, error) {
	if c.db == nil {
		return SmsMes{}, errors.New("database not initialized")
	}

	var mes SmsMes
	err := c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(waitBucket)
		if b == nil {
			return errors.New("wait bucket not found")
		}

		keyBytes := uint32ToBytes(key)
		data := b.Get(keyBytes)
		if data == nil {
			return errors.New("no key in cache")
		}

		// 反序列化
		if err := json.Unmarshal(data, &mes); err != nil {
			return err
		}

		// 删除该键
		return b.Delete(keyBytes)
	})

	return mes, err
}

// GetWaitList 获取所有等待响应的消息
func (c *BoltCache) GetWaitList() []SmsMes {
	if c.db == nil {
		return []SmsMes{}
	}

	result := make([]SmsMes, 0)

	c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(waitBucket)
		if b == nil {
			return nil
		}

		// 遍历所有等待的消息
		cursor := b.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			mes := SmsMes{}
			if err := json.Unmarshal(v, &mes); err == nil {
				result = append(result, mes)
			}
		}

		return nil
	})

	return result
}

// AddSubmits 添加提交消息到列表
func (c *BoltCache) AddSubmits(mes *SmsMes) error {
	if c.db == nil {
		Warnf("[CACHE] BoltDB 未初始化，跳过 AddSubmits")
		return errors.New("database not initialized")
	}

	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(messageBucket)
		if b == nil {
			return errors.New("messages bucket not found")
		}

		// 使用时间戳+序列号作为key，保证倒序
		key := generateTimeKey(tx, b)

		// 序列化消息
		data, err := json.Marshal(mes)
		if err != nil {
			return err
		}

		return b.Put(key, data)
	})
}

// AddMoList 添加MO消息到列表
func (c *BoltCache) AddMoList(mes *SmsMes) error {
	if c.db == nil {
		Warnf("[CACHE] BoltDB 未初始化，跳过 AddMoList")
		return errors.New("database not initialized")
	}

	return c.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(moBucket)
		if b == nil {
			return errors.New("mo bucket not found")
		}

		// 使用时间戳+序列号作为key
		key := generateTimeKey(tx, b)

		// 序列化消息
		data, err := json.Marshal(mes)
		if err != nil {
			return err
		}

		return b.Put(key, data)
	})
}

// Length 获取列表长度
func (c *BoltCache) Length(listName string) int {
	if c.db == nil || listName == "" {
		return 0
	}

	var count int
	c.db.View(func(tx *bolt.Tx) error {
		var b *bolt.Bucket
		switch listName {
		case "list_message":
			b = tx.Bucket(messageBucket)
		case "list_mo":
			b = tx.Bucket(moBucket)
		default:
			return nil
		}

		if b == nil {
			return nil
		}

		// 统计键数量
		b.ForEach(func(k, v []byte) error {
			count++
			return nil
		})
		return nil
	})

	return count
}

// GetStats 获取短信统计信息
func (c *BoltCache) GetStats() map[string]int {
	stats := map[string]int{
		"total":   0,
		"success": 0,
		"failed":  0,
	}

	if c.db == nil {
		return stats
	}

	c.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(messageBucket)
		if b == nil {
			return nil
		}

		successCount := 0
		failedCount := 0
		totalCount := 0
		sampleSize := 0
		maxSample := 1000

		// 倒序遍历（从最新到最旧）
		cursor := b.Cursor()
		for k, v := cursor.Last(); k != nil && sampleSize < maxSample; k, v = cursor.Prev() {
			totalCount++
			sampleSize++

			mes := SmsMes{}
			if err := json.Unmarshal(v, &mes); err == nil {
				if mes.SubmitResult == 0 {
					successCount++
				} else if mes.SubmitResult != 65535 {
					// 65535 是等待状态，不算失败
					failedCount++
				}
			}
		}

		// 继续计数总数（不解析JSON）
		if sampleSize >= maxSample {
			for k, _ := cursor.Prev(); k != nil; k, _ = cursor.Prev() {
				totalCount++
			}
		}

		stats["total"] = totalCount

		// 基于样本估算
		if totalCount > sampleSize && sampleSize > 0 {
			ratio := float64(totalCount) / float64(sampleSize)
			stats["success"] = int(float64(successCount) * ratio)
			stats["failed"] = int(float64(failedCount) * ratio)
		} else {
			stats["success"] = successCount
			stats["failed"] = failedCount
		}

		return nil
	})

	return stats
}

// GetList 获取消息列表
func (c *BoltCache) GetList(listName string, start, end int) *[]SmsMes {
	result := make([]SmsMes, 0)

	if c.db == nil {
		return &result
	}

	c.db.View(func(tx *bolt.Tx) error {
		var b *bolt.Bucket
		switch listName {
		case "list_message":
			b = tx.Bucket(messageBucket)
		case "list_mo":
			b = tx.Bucket(moBucket)
		default:
			return nil
		}

		if b == nil {
			return nil
		}

		// 正序遍历（由于使用了反转时间戳作为key，正序即为最新→最旧）
		cursor := b.Cursor()
		idx := 0

		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			// 跳过 start 之前的记录
			if idx < start {
				idx++
				continue
			}

			// 超过 end 则停止
			if idx > end {
				break
			}

			mes := SmsMes{}
			if err := json.Unmarshal(v, &mes); err == nil {
				result = append(result, mes)
			}
			idx++
		}

		return nil
	})

	return &result
}

// SearchList 在BoltDB中搜索消息列表
func (c *BoltCache) SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes {
	result := make([]SmsMes, 0)

	if c.db == nil {
		return &result
	}

	// 先收集所有匹配的消息
	var allMatches []SmsMes

	c.db.View(func(tx *bolt.Tx) error {
		var b *bolt.Bucket
		switch listName {
		case "list_message":
			b = tx.Bucket(messageBucket)
		case "list_mo":
			b = tx.Bucket(moBucket)
		default:
			return nil
		}

		if b == nil {
			return nil
		}

		// 遍历所有记录
		cursor := b.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			mes := SmsMes{}
			if err := json.Unmarshal(v, &mes); err != nil {
				continue
			}

			if c.matchFilters(&mes, filters, listName) {
				allMatches = append(allMatches, mes)
			}
		}

		return nil
	})

	// 分页处理
	if start < len(allMatches) {
		end := end + 1
		if end > len(allMatches) {
			end = len(allMatches)
		}
		result = allMatches[start:end]
	}

	return &result
}

// GetSearchCount 获取搜索结果的数量
func (c *BoltCache) GetSearchCount(listName string, filters map[string]string) int {
	if c.db == nil {
		return 0
	}

	count := 0

	c.db.View(func(tx *bolt.Tx) error {
		var b *bolt.Bucket
		switch listName {
		case "list_message":
			b = tx.Bucket(messageBucket)
		case "list_mo":
			b = tx.Bucket(moBucket)
		default:
			return nil
		}

		if b == nil {
			return nil
		}

		// 遍历所有记录并统计匹配的数量
		cursor := b.Cursor()
		for k, v := cursor.First(); k != nil; k, v = cursor.Next() {
			mes := SmsMes{}
			if err := json.Unmarshal(v, &mes); err != nil {
				continue
			}

			if c.matchFilters(&mes, filters, listName) {
				count++
			}
		}

		return nil
	})

	return count
}

// matchFilters 检查消息是否匹配过滤条件
func (c *BoltCache) matchFilters(mes *SmsMes, filters map[string]string, listName string) bool {
	// 如果没有过滤条件，都匹配
	if len(filters) == 0 {
		return true
	}

	// 通用过滤条件
	if content, ok := filters["content"]; ok && content != "" {
		if !strings.Contains(strings.ToLower(mes.Content), strings.ToLower(content)) {
			return false
		}
	}

	if listName == "list_message" {
		// 下发消息特定过滤
		if dest, ok := filters["dest"]; ok && dest != "" {
			if !strings.Contains(strings.ToLower(mes.Dest), strings.ToLower(dest)) {
				return false
			}
		}

		if status, ok := filters["status"]; ok && status != "" {
			statusInt, err := strconv.Atoi(status)
			if err != nil {
				return false
			}
			if statusInt == 0 && mes.SubmitResult != 0 {
				return false
			}
			if statusInt == 1 && mes.SubmitResult == 0 {
				return false
			}
		}
	} else if listName == "list_mo" {
		// 上行消息特定过滤
		if src, ok := filters["src"]; ok && src != "" {
			if !strings.Contains(strings.ToLower(mes.Src), strings.ToLower(src)) {
				return false
			}
		}

		if dest, ok := filters["dest"]; ok && dest != "" {
			if !strings.Contains(strings.ToLower(mes.Dest), strings.ToLower(dest)) {
				return false
			}
		}
	}

	return true
}

// 工具函数：uint32 转 []byte
func uint32ToBytes(n uint32) []byte {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, n)
	return b
}

// 生成基于时间的key（倒序）
// 使用 (MaxUint64 - timestamp) 确保最新的记录排在前面
func generateTimeKey(tx *bolt.Tx, b *bolt.Bucket) []byte {
	// 获取当前时间的纳秒时间戳
	now := time.Now().UnixNano()

	// 使用反转的时间戳（最大值减去当前时间）来实现倒序
	// 这样最新的记录会有最小的key值，在BoltDB中排在最前面
	invertedTime := uint64(1<<63 - 1 - now) // 使用63位最大值

	// 获取序列号（BoltDB的NextSequence）
	seq, _ := b.NextSequence()

	// 组合成16字节的key: 8字节倒序时间戳 + 8字节序列号
	key := make([]byte, 16)
	binary.BigEndian.PutUint64(key[:8], invertedTime)
	binary.BigEndian.PutUint64(key[8:], seq)

	return key
}
