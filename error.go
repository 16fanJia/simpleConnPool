package simpleConnPool

import "errors"

var (
	PoolClosed           = errors.New("连接池已经关闭！")
	GetConnectionTimeout = errors.New("获取链接超时")
	ConnectionIsNull     = errors.New("连接为空")
	InvalidCapSet        = errors.New("无效容量设置")
	InvalidFactorySet    = errors.New("无效factory函数设置")
	InvalidCloseSet      = errors.New("无效close函数设置")
	InitPoolErr          = errors.New("初始化连接池错误")
)
