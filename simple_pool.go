package simpleConnPool

import (
	"sync/atomic"
	"time"
)

/*
====== 连接池 connection pool 的实现 =======
*/

// Config 连接池相关配置
type Config struct {
	InitialCap  int32                       //连接池中拥有的最小连接数
	MaxCap      int32                       //最大并发存活连接数
	MaxIdle     int32                       //最大空闲连接
	Factory     func() (interface{}, error) //生成连接的方法
	Close       func(interface{}) error     //关闭连接的方法
	IdleTimeout time.Duration               //连接最大空闲时间，超过该事件则将失效
	WaitTimeout time.Duration               //获取链接最大可用时间
	WaitQueue   int32                       //最大等待请求获取链接数量
}

//channelPool 连接池 存放连接信息
type connectionPool struct {
	idleQueue   chan *idleConn      //空闲连接队列
	factory     func() (any, error) //连接创建函数
	close       func(any) error     //链接对应的关闭函数
	reqQueue    chan connReq        //请求等待队列
	idleTimeOut time.Duration       //空闲连接超时时间
	waitTimeOut time.Duration       //请求等待连接时间

	maxActiveConn int32 //允许的最大运行的连接数
	openingConn   int32 //当前正在运行的连接数
}

type idleConn struct {
	connection     any
	lastActiveTime time.Time
}

type connReq struct {
	abandon  bool     //此请求是否被抛弃
	idleConn chan any //一个空闲连接
}

//NewPool 构造函数 返回一个pool
func NewPool(poolConfig *Config) (Pool, error) {
	if !(poolConfig.InitialCap <= poolConfig.MaxIdle && poolConfig.MaxCap >= poolConfig.MaxIdle && poolConfig.InitialCap >= 0) {
		return nil, InvalidCapSet
	}
	if poolConfig.Factory == nil {
		return nil, InvalidFactorySet
	}
	if poolConfig.Close == nil {
		return nil, InvalidCloseSet
	}

	c := &connectionPool{
		idleQueue:     make(chan *idleConn, poolConfig.MaxIdle),
		factory:       poolConfig.Factory,
		close:         poolConfig.Close,
		reqQueue:      make(chan connReq, poolConfig.WaitQueue),
		idleTimeOut:   poolConfig.IdleTimeout,
		waitTimeOut:   poolConfig.WaitTimeout,
		maxActiveConn: poolConfig.MaxCap,
		openingConn:   poolConfig.InitialCap,
	}
	//初始化空闲连接
	for i := int32(0); i < poolConfig.InitialCap; i++ {
		conn, err := c.factory()
		if err != nil {
			return nil, InitPoolErr
		}
		c.idleQueue <- &idleConn{
			connection:     conn,
			lastActiveTime: time.Now(),
		}
	}
	return c, nil
}

//Get 向连接池中获取一个连接
func (c *connectionPool) Get() (any, error) {
	for {
		select {
		//获取空闲队列里面的链接
		case idleC, ok := <-c.idleQueue:
			if ok {
				//检测连接是否超时
				if c.idleTimeOut > 0 {
					if time.Now().Sub(idleC.lastActiveTime) > c.idleTimeOut {
						//关闭连接
						_ = c.Close(idleC)
						continue
					}
				}
				return idleC, nil
			}
			return nil, PoolClosed
		default:
			//未获取到链接 且 还可以创建 则创建一个连接
			if atomic.AddInt32(&c.openingConn, 1) < c.maxActiveConn {
				//创建连接
				return c.factory()
			}
			//无法创建 则放入请求队列
			atomic.AddInt32(&c.openingConn, -1)

			req := connReq{
				//unbuffered channel
				idleConn: make(chan any),
			}
			ticker := time.NewTicker(c.waitTimeOut)
			select {
			//放入等待的channel中
			case c.reqQueue <- req:
				select {
				case conn := <-req.idleConn:
					return conn, nil
				case <-ticker.C:
					//从等待队列中 抛弃这个请求
					req.abandon = true
					return nil, GetConnectionTimeout
				}
			}
		}
	}

}

//Put 向连接池中放入一个连接
func (c *connectionPool) Put(conn any) error {
	if conn == nil {
		return ConnectionIsNull
	}
Try:
	select {
	case req, ok := <-c.reqQueue:
		if !ok {
			return PoolClosed
		}
		if req.abandon {
			//此获取链接请求被抛弃
			goto Try
		}
		req.idleConn <- conn
	default:
		//无等待连接的请求 则放入空闲队列中
		select {
		case c.idleQueue <- &idleConn{
			connection:     conn,
			lastActiveTime: time.Now(),
		}:
			return nil
		default:
			//空闲队列已经满了 则关闭连接
			atomic.AddInt32(&c.openingConn, -1)
			return c.Close(conn)
		}
	}
	return nil
}

//Close 关闭连接
func (c *connectionPool) Close(conn any) error {
	if c.close == nil {
		return nil
	}

	atomic.AddInt32(&c.openingConn, -1)
	return c.close(conn)
}
