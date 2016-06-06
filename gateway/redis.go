package gateway

import (
	"github.com/garyburd/redigo/redis"
	"log"
)

var RedisConn redis.Conn

func StartRedis(config *Config) {
	conn, err := redis.Dial("tcp", config.RedisHost + ":" + config.RedisPort)
	if err != nil {
		log.Fatalf("连接Redis出错出错[%v]", err)
	} else {
		RedisConn = conn
		log.Printf("连接Redis %s 成功", config.RedisHost + ":" + config.RedisPort)
	}
	defer RedisConn.Close()
	<- Abort
}
