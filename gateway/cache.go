package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gomodule/redigo/redis"
)

// CacheInterface 定义缓存接口，支持多种实现
type CacheInterface interface {
	SetWaitCache(key uint32, message SmsMes) error
	GetWaitCache(key uint32) (SmsMes, error)
	GetWaitList() []SmsMes // 获取所有等待响应的消息
	AddSubmits(mes *SmsMes) error
	AddMoList(mes *SmsMes) error
	Length(listName string) int
	GetStats() map[string]int
	GetList(listName string, start, end int) *[]SmsMes
	SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes
	GetSearchCount(listName string, filters map[string]string) int
}

type Cache struct {
	pool *redis.Pool
}

var SCache CacheInterface

// InitCache 根据配置初始化缓存（Redis 或 BoltDB）
func InitCache(config *Config) {
	// 如果未指定缓存类型，默认使用 boltdb
	cacheType := config.CacheType
	if cacheType == "" {
		cacheType = "boltdb"
	}

	if cacheType == "redis" {
		// 使用 Redis
		Infof("[CACHE] 使用 Redis 作为缓存后端")
		StartCache(config)
	} else {
		// 使用 BoltDB
		Infof("[CACHE] 使用 BoltDB 作为缓存后端")
		dbPath := config.DBPath
		if dbPath == "" {
			dbPath = "./data/cmpp.db"
		}

		// 创建数据目录
		if err := os.MkdirAll("./data", 0755); err != nil {
			log.Fatalf("创建数据目录失败: %v", err)
		}

		boltCache, err := StartBoltCache(dbPath)
		if err != nil {
			log.Fatalf("启动 BoltDB 失败: %v", err)
		}
		SCache = boltCache
	}
}

// StartCache 启动Redis缓存（保留用于兼容）
func StartCache(config *Config) {
	// 创建Redis连接池
	pool := &redis.Pool{
		MaxIdle:     10,                // 最大空闲连接数
		MaxActive:   30,                // 最大活跃连接数，0表示无限制
		IdleTimeout: 240 * time.Second, // 空闲连接超时时间
		Wait:        true,              // 连接池满时等待可用连接
		Dial: func() (redis.Conn, error) {
			conn, err := redis.Dial("tcp", config.RedisHost+":"+config.RedisPort)
			if err != nil {
				return nil, err
			}
			// 如果配置了密码，进行认证
			if config.RedisPassword != "" {
				if _, err := conn.Do("AUTH", config.RedisPassword); err != nil {
					conn.Close()
					return nil, err
				}
			}
			return conn, nil
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			// 测试连接是否可用
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}

	// 测试连接
	conn := pool.Get()
	defer conn.Close()
	if _, err := conn.Do("PING"); err != nil {
		log.Fatalf("连接Redis出错[%v]", err)
	}

	cache := &Cache{pool: pool}
	SCache = cache
	Infof("[CACHE] 连接 Redis 成功: %s", config.RedisHost+":"+config.RedisPort)
}

func StopCache() {
	// 如果是Redis实现
	if cache, ok := SCache.(*Cache); ok && cache.pool != nil {
		cache.pool.Close()
	}
	// 如果是BoltDB实现
	if boltCache, ok := SCache.(*BoltCache); ok {
		boltCache.StopBoltCache()
	}
}

// 将发送的记录转为json放到redis中保存下来,为异步返回的submit reponse做准备
func (c *Cache) SetWaitCache(key uint32, message SmsMes) error {
	if c.pool == nil {
		Warnf("[CACHE] Redis 连接池未初始化，跳过 SetWaitCache")
		return errors.New("cache pool not initialized")
	}
	conn := c.pool.Get()
	defer conn.Close()

	data, _ := json.Marshal(message)
	_, err := conn.Do("HSET", "waitseqcache", strconv.FormatUint(uint64(key), 10), data)
	return err
}

func (c *Cache) GetWaitCache(key uint32) (SmsMes, error) {
	if c.pool == nil {
		return SmsMes{}, errors.New("cache pool not initialized")
	}
	conn := c.pool.Get()
	defer conn.Close()

	seq_id := strconv.FormatUint(uint64(key), 10)
	ret, _ := redis.String(conn.Do("HGET", "waitseqcache", seq_id))
	mes := SmsMes{}
	if ret != "" {
		//从json还原为对象
		json.Unmarshal([]byte(ret), &mes)
		conn.Do("HDEL", "waitseqcache", seq_id)
		return mes, nil
	} else {
		return mes, errors.New("no key in cache")
	}

}

// GetWaitList 获取所有等待响应的消息
func (c *Cache) GetWaitList() []SmsMes {
	if c.pool == nil {
		return []SmsMes{}
	}
	conn := c.pool.Get()
	defer conn.Close()

	// 获取 hash 中所有的值
	values, err := redis.Strings(conn.Do("HVALS", "waitseqcache"))
	if err != nil {
		return []SmsMes{}
	}

	result := make([]SmsMes, 0, len(values))
	for _, value := range values {
		mes := SmsMes{}
		if json.Unmarshal([]byte(value), &mes) == nil {
			result = append(result, mes)
		}
	}

	return result
}

func (c *Cache) AddSubmits(mes *SmsMes) error {
	if c.pool == nil {
		Warnf("[CACHE] Redis 连接池未初始化，跳过 AddSubmits")
		return errors.New("cache pool not initialized")
	}
	conn := c.pool.Get()
	defer conn.Close()

	//将submit结果提交到redis的队列存放
	data, _ := json.Marshal(mes)
	//新的记录加在头部,自然就倒序排列了
	_, err := conn.Do("LPUSH", "list_message", data)
	//只保留最近五十条
	//conn.Do("LTRIM", "submitlist", "0", "49")
	return err
}

func (c *Cache) AddMoList(mes *SmsMes) error {
	if c.pool == nil {
		Warnf("[CACHE] Redis 连接池未初始化，跳过 AddMoList")
		return errors.New("cache pool not initialized")
	}
	conn := c.pool.Get()
	defer conn.Close()

	//将submit结果提交到redis的队列存放
	data, _ := json.Marshal(mes)
	//新的记录加在头部,自然就倒序排列了
	_, err := conn.Do("LPUSH", "list_mo", data)
	//只保留最近五十条
	//conn.Do("LTRIM", "molist", "0", "49")
	return err
}

func (c *Cache) Length(listName string) int {
	if listName == "" || c.pool == nil {
		return 0
	}
	conn := c.pool.Get()
	defer conn.Close()

	size, _ := redis.Int(conn.Do("LLEN", listName))
	return size
}

// GetStats 获取短信统计信息
func (c *Cache) GetStats() map[string]int {
	if c.pool == nil {
		return map[string]int{
			"total":   0,
			"success": 0,
			"failed":  0,
		}
	}

	conn := c.pool.Get()
	defer conn.Close()

	stats := map[string]int{
		"total":   0,
		"success": 0,
		"failed":  0,
	}

	// 获取总记录数
	total, _ := redis.Int(conn.Do("LLEN", "list_message"))
	stats["total"] = total

	// 如果记录数太多，只统计最近1000条以避免性能问题
	sampleSize := total
	if sampleSize > 1000 {
		sampleSize = 1000
	}

	// 获取样本数据
	values, err := redis.Strings(conn.Do("LRANGE", "list_message", 0, sampleSize-1))
	if err != nil {
		return stats
	}

	// 统计成功和失败数量
	successCount := 0
	failedCount := 0
	waitingCount := 0

	for _, value := range values {
		mes := SmsMes{}
		if err := json.Unmarshal([]byte(value), &mes); err == nil {
			if mes.SubmitResult == 0 {
				successCount++
			} else if mes.SubmitResult == 65535 {
				waitingCount++ // 等待响应，不算失败
			} else {
				failedCount++ // 其他状态都算失败
			}
		}
	}

	// 基于样本估算总体统计
	if total > sampleSize {
		ratio := float64(total) / float64(sampleSize)
		stats["success"] = int(float64(successCount) * ratio)
		stats["failed"] = int(float64(failedCount) * ratio)
	} else {
		stats["success"] = successCount
		stats["failed"] = failedCount
	}

	return stats
}

func (c *Cache) GetList(listName string, start, end int) *[]SmsMes {
	if c.pool == nil {
		return &[]SmsMes{}
	}
	conn := c.pool.Get()
	defer conn.Close()

	values, err := redis.Strings(conn.Do("LRANGE", listName, start, end))
	if err != nil {
		fmt.Println(err)
		//返回空对象
		return &[]SmsMes{}
	}
	v := make([]SmsMes, 0, len(values))
	for _, s := range values {
		mes := SmsMes{}
		json.Unmarshal([]byte(s), &mes)
		v = append(v, mes)
	}
	return &v
}

// SearchList 在Redis中搜索消息列表
func (c *Cache) SearchList(listName string, filters map[string]string, start, end int) *[]SmsMes {
	if c.pool == nil {
		return &[]SmsMes{}
	}
	conn := c.pool.Get()
	defer conn.Close()

	// 获取所有数据（Redis不支持复杂的搜索，需要内存过滤）
	totalCount, _ := redis.Int(conn.Do("LLEN", listName))
	if totalCount == 0 {
		return &[]SmsMes{}
	}

	// 获取所有记录进行过滤
	values, err := redis.Strings(conn.Do("LRANGE", listName, 0, totalCount-1))
	if err != nil {
		return &[]SmsMes{}
	}

	// 过滤消息
	var filteredMessages []SmsMes
	for _, s := range values {
		mes := SmsMes{}
		if json.Unmarshal([]byte(s), &mes) != nil {
			continue
		}

		if c.matchFilters(&mes, filters, listName) {
			filteredMessages = append(filteredMessages, mes)
		}
	}

	// 分页处理
	result := make([]SmsMes, 0)
	if start < len(filteredMessages) {
		end := end + 1
		if end > len(filteredMessages) {
			end = len(filteredMessages)
		}
		result = filteredMessages[start:end]
	}

	return &result
}

// GetSearchCount 获取搜索结果的数量
func (c *Cache) GetSearchCount(listName string, filters map[string]string) int {
	if c.pool == nil {
		return 0
	}
	conn := c.pool.Get()
	defer conn.Close()

	// 获取所有数据
	totalCount, _ := redis.Int(conn.Do("LLEN", listName))
	if totalCount == 0 {
		return 0
	}

	// 获取所有记录进行过滤
	values, err := redis.Strings(conn.Do("LRANGE", listName, 0, totalCount-1))
	if err != nil {
		return 0
	}

	// 统计匹配的消息数量
	count := 0
	for _, s := range values {
		mes := SmsMes{}
		if json.Unmarshal([]byte(s), &mes) != nil {
			continue
		}

		if c.matchFilters(&mes, filters, listName) {
			count++
		}
	}

	return count
}

// matchFilters 检查消息是否匹配过滤条件
func (c *Cache) matchFilters(mes *SmsMes, filters map[string]string, listName string) bool {
	// 如果没有过滤条件，都匹配
	if len(filters) == 0 {
		return true
	}

	// 通用过滤条件
	if content, ok := filters["content"]; ok && content != "" {
		if !contains(mes.Content, content) {
			return false
		}
	}

	if listName == "list_message" {
		// 下发消息特定过滤
		if dest, ok := filters["dest"]; ok && dest != "" {
			if !contains(mes.Dest, dest) {
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
			if !contains(mes.Src, src) {
				return false
			}
		}

		if dest, ok := filters["dest"]; ok && dest != "" {
			if !contains(mes.Dest, dest) {
				return false
			}
		}
	}

	return true
}

// contains 检查字符串是否包含子字符串（不区分大小写）
func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}

	// 简单的包含检查，可以根据需要扩展为更复杂的匹配
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
