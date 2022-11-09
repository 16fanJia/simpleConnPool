package simpleConnPool

type Pool interface {
	Get() (any, error)
	Put(any) error
	Close(any) error
}
