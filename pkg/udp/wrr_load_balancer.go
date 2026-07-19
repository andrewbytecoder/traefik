package udp

import (
	"errors"
	"sync"

	"github.com/rs/zerolog/log"
)

type server struct {
	Handler

	weight int
}

// WRRLoadBalancer is a naive RoundRobin load balancer for UDP services.
// 加权轮询 加权轮询实现规则
type WRRLoadBalancer struct {
	servers       []server
	lock          sync.Mutex
	currentWeight int
	index         int
}

// NewWRRLoadBalancer creates a new WRRLoadBalancer.
func NewWRRLoadBalancer() *WRRLoadBalancer {
	return &WRRLoadBalancer{
		index: -1,
	}
}

// ServeUDP forwards the connection to the right service.
func (b *WRRLoadBalancer) ServeUDP(conn *Conn) {
	// 加锁，避免并发问题
	b.lock.Lock()
	// 获取下一个服务器 获取算法是进行加权轮询
	next, err := b.next()
	b.lock.Unlock()

	if err != nil {
		log.Error().Err(err).Msg("Error during load balancing")
		conn.Close()
		return
	}

	next.ServeUDP(conn)
}

// AddServer appends a handler to the existing list.
func (b *WRRLoadBalancer) AddServer(serverHandler Handler) {
	w := 1
	b.AddWeightedServer(serverHandler, &w)
}

// AddWeightedServer appends a handler to the existing list with a weight.
func (b *WRRLoadBalancer) AddWeightedServer(serverHandler Handler, weight *int) {
	b.lock.Lock()
	defer b.lock.Unlock()

	w := 1
	if weight != nil {
		w = *weight
	}
	b.servers = append(b.servers, server{Handler: serverHandler, weight: w})
}

func (b *WRRLoadBalancer) maxWeight() int {
	maximum := -1
	for _, s := range b.servers {
		if s.weight > maximum {
			maximum = s.weight
		}
	}
	return maximum
}

// weightGcd 计算所有权重的 GCD 最大公约数
func (b *WRRLoadBalancer) weightGcd() int {
	divisor := -1
	for _, s := range b.servers {
		if divisor == -1 {
			divisor = s.weight
		} else {
			divisor = gcd(divisor, s.weight)
		}
	}
	return divisor
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// 实现了 平滑加权轮询（Smooth Weighted Round-Robin） 算法，
// 与 Nginx 的 upstream 模块使用同一算法。
// 每次调用选出一个后端服务器，权重高的被选中的频率更高，
// 且分布均匀（不会连续选同一个）。
func (b *WRRLoadBalancer) next() (Handler, error) {
	// 如果没有服务器，则返回错误
	if len(b.servers) == 0 {
		return nil, errors.New("no servers in the pool")
	}

	// The algorithm below may look messy,
	// but is actually very simple it calculates the GCD  and subtracts it on every iteration,
	// what interleaves servers and allows us not to build an iterator every time we readjust weights.

	// Maximum weight across all enabled servers
	// 注释解释核心思想：
	//   虽然代码看起来复杂，但算法很简单：
	//   计算所有权重的 GCD，每一轮递减一次，
	//   这样能让不同权重的服务器均匀穿插，而不需要每次都重建迭代器。

	maximum := b.maxWeight()
	if maximum == 0 {
		return nil, errors.New("all servers have 0 weight")
	}

	// GCD across all enabled servers
	gcd := b.weightGcd()

	for {
		b.index = (b.index + 1) % len(b.servers)
		if b.index == 0 {
			b.currentWeight -= gcd
			if b.currentWeight <= 0 {
				b.currentWeight = maximum
			}
		}
		srv := b.servers[b.index]
		if srv.weight >= b.currentWeight {
			return srv, nil
		}
	}
}
