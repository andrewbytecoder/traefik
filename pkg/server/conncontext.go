package server

import (
	"context"
	"net"
)

type connContextFunc func(context.Context, net.Conn) context.Context

type multipleConnContext struct {
	fns []connContextFunc
}

func (m *multipleConnContext) AddConnContextFunc(fn connContextFunc) {
	m.fns = append(m.fns, fn)
}

// Build returns a connContextFunc that runs the list of connContextFunc in order.
// 定义一个变量，然后遍历这个变量
func (m *multipleConnContext) Build() connContextFunc {
	if len(m.fns) == 0 {
		return nil
	}

	return func(ctx context.Context, c net.Conn) context.Context {
		for _, contextFunc := range m.fns {
			ctx = contextFunc(ctx, c)
		}
		return ctx
	}
}
