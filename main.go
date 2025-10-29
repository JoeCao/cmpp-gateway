package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/JoeCao/cmpp-gateway/gateway"
)

func main() {
	//
	var configPath string
	var config = &gateway.Config{}

	//
	flag.StringVar(&configPath, "c", "", "配置文件路径")
	flag.Parse()
	if configPath == "" {
		configPath = "./config.json"
	}
	//
	err := LoadJsonFile(configPath, config)
	if err == nil {
		log.Println("加载成功 => ", config)
	} else {
		log.Fatal("加载失败 ", configPath, " => ", err)
	}
	gateway.InitCache(config)
	defer gateway.StopCache()

	go gateway.StartClient(config)
	// go gateway.StartCmdLine() // 临时禁用命令行功能以避免空输入循环
	go gateway.Serve(config)

	<-gateway.Abort
}

func LoadJsonFile(filePath string, obj interface{}) error {
	data, err := ReadBytes(filePath)
	if err != nil {
		return err
	}
	err = json.Unmarshal(data, obj)
	if err != nil {
		return err
	}
	return nil
}

func ReadBytes(filePath string) ([]byte, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return data, nil
}
