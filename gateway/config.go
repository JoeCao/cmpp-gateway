package gateway

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
)

type Config struct {
	User string `json:"user"`

	Password string `json:"password"`
	//短信接入码
	SmsAccessNo string `json:"sms_accessno"`
	//业务代码
	ServiceId string `json:"service_id"`

	HttpHost string `json:"http_host"`
	HttpPort string `json:"http_port"`

	CMPPHost  string `json:"cmpp_host"`
	CMPPPort  string `json:"cmpp_port"`
	Debug     bool   `json:"debug"`
	RedisHost string `json:"redis_host"`
	RedisPort string `json:"redis_port"`
}

func (c *Config) LoadFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		log.Fatalf("读取配置文件[%s]出错[%v]", path, err)
	} else {
		fileData, err := ioutil.ReadAll(file)
		if err != nil {
			log.Fatalf("读取配置文件内容[%s]出错[%v]", path, err)
		} else {
			if err := json.Unmarshal(fileData, c); err != nil {
				log.Fatal("读取失败 => ", err)
			} else {
				log.Println("读取成功 => ", c)
			}
		}
	}
}

func (s *Config) Log(arg ...interface{}) {
	if s.Debug {
		log.Println(arg...)
	}
}
