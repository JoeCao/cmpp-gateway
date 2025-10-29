package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/gomodule/redigo/redis"
)

type Cache struct {
	conn redis.Conn
}

var SCache Cache = Cache{}

func StartCache(config *Config) {
	conn, err := redis.Dial("tcp", config.RedisHost+":"+config.RedisPort)
	if err != nil {
		log.Fatalf("连接Redis出错出错[%v]", err)
	} else {
		SCache.conn = conn
		log.Printf("连接Redis %s 成功", config.RedisHost+":"+config.RedisPort)
	}
}

func StopCache() {
	SCache.conn.Close()
}

//将发送的记录转为json放到redis中保存下来,为异步返回的submit reponse做准备
func (c *Cache) SetWaitCache(key uint32, message SmsMes) {
	data, _ := json.Marshal(message)
	c.conn.Do("HSET", "waitseqcache", strconv.FormatUint(uint64(key), 10), data)
}

func (c *Cache) GetWaitCache(key uint32) (SmsMes, error) {
	seq_id := strconv.FormatUint(uint64(key), 10)
	ret, _ := redis.String(c.conn.Do("HGET", "waitseqcache", seq_id))
	mes := SmsMes{}
	if ret != "" {
		//从json还原为对象
		json.Unmarshal([]byte(ret), &mes)
		c.conn.Do("HDEL", "waitseqcache", seq_id)
		return mes, nil
	} else {
		return mes, errors.New("no key in cache")
	}

}

func (c *Cache) AddSubmits(mes *SmsMes) {
	//将submit结果提交到redis的队列存放
	data, _ := json.Marshal(mes)
	//新的记录加在头部,自然就倒序排列了
	c.conn.Do("LPUSH", "list_message", data)
	//只保留最近五十条
	//c.conn.Do("LTRIM", "submitlist", "0", "49")
}

func (c *Cache) AddMoList(mes *SmsMes) {
	//将submit结果提交到redis的队列存放
	data, _ := json.Marshal(mes)
	//新的记录加在头部,自然就倒序排列了
	c.conn.Do("LPUSH", "list_mo", data)
	//只保留最近五十条
	//c.conn.Do("LTRIM", "molist", "0", "49")
}

func (c *Cache) Length(listName string) int {
	if listName == "" {
		return 0
	}
	size, _ := redis.Int(c.conn.Do("LLEN", listName))
	return size
}

func (c *Cache) GetList(listName string, start, end int) *[]SmsMes {
	values, err := redis.Strings(c.conn.Do("LRANGE", listName, start, end))
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
