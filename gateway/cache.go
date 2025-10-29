package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/gomodule/redigo/redis"
)

type Cache struct {
	pool *redis.Pool
}

var SCache Cache = Cache{}

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

	SCache.pool = pool
	log.Printf("连接Redis %s 成功", config.RedisHost+":"+config.RedisPort)
}

func StopCache() {
	if SCache.pool != nil {
		SCache.pool.Close()
	}
}

// 将发送的记录转为json放到redis中保存下来,为异步返回的submit reponse做准备
func (c *Cache) SetWaitCache(key uint32, message SmsMes) {
	if c.pool == nil {
		log.Printf("Cache pool not initialized, skipping SetWaitCache")
		return
	}
	conn := c.pool.Get()
	defer conn.Close()

	data, _ := json.Marshal(message)
	conn.Do("HSET", "waitseqcache", strconv.FormatUint(uint64(key), 10), data)
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

func (c *Cache) AddSubmits(mes *SmsMes) {
	if c.pool == nil {
		log.Printf("Cache pool not initialized, skipping AddSubmits")
		return
	}
	conn := c.pool.Get()
	defer conn.Close()

	//将submit结果提交到redis的队列存放
	data, _ := json.Marshal(mes)
	//新的记录加在头部,自然就倒序排列了
	conn.Do("LPUSH", "list_message", data)
	//只保留最近五十条
	//conn.Do("LTRIM", "submitlist", "0", "49")
}

func (c *Cache) AddMoList(mes *SmsMes) {
	if c.pool == nil {
		log.Printf("Cache pool not initialized, skipping AddMoList")
		return
	}
	conn := c.pool.Get()
	defer conn.Close()

	//将submit结果提交到redis的队列存放
	data, _ := json.Marshal(mes)
	//新的记录加在头部,自然就倒序排列了
	conn.Do("LPUSH", "list_mo", data)
	//只保留最近五十条
	//conn.Do("LTRIM", "molist", "0", "49")
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
