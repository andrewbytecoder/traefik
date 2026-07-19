package udp

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"net"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/config/runtime"
	"github.com/traefik/traefik/v3/pkg/observability/logs"
	"github.com/traefik/traefik/v3/pkg/server/provider"
	"github.com/traefik/traefik/v3/pkg/udp"
)

// Manager handles UDP services creation.
type Manager struct {
	configs map[string]*runtime.UDPServiceInfo
	rand    *rand.Rand // For the initial shuffling of load-balancers.
}

// NewManager creates a new manager.
func NewManager(conf *runtime.Configuration) *Manager {
	return &Manager{
		configs: conf.UDPServices,
		rand:    rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// BuildUDP creates the UDP handler for the given service name.
func (m *Manager) BuildUDP(rootCtx context.Context, serviceName string) (udp.Handler, error) {
	// 获取服务的全称，包括provider名称
	serviceQualifiedName := provider.GetQualifiedName(rootCtx, serviceName)

	logger := log.Ctx(rootCtx).With().Str(logs.ServiceName, serviceQualifiedName).Logger()

	// 如果没有 @ 则添加 @providerName， 对应的name存储在ctx的value中实现
	ctx := provider.AddInContext(rootCtx, serviceQualifiedName)

	// 获取指定服务的配置信息
	conf, ok := m.configs[serviceQualifiedName]
	if !ok {
		return nil, fmt.Errorf("the UDP service %q does not exist", serviceQualifiedName)
	}

	// 如果配置信息中同时存在 LoadBalancer 和 Weighted 则返回错误
	// 两种类型的service不支持同时存在，应该分别进行配置
	if conf.LoadBalancer != nil && conf.Weighted != nil {
		err := errors.New("cannot create service: multi-types service not supported, consider declaring two different pieces of service instead")
		conf.AddError(err, true)
		return nil, err
	}

	switch {
	case conf.LoadBalancer != nil:
		// 创建一个WRR负载均衡器
		loadBalancer := udp.NewWRRLoadBalancer()
		// 将原有的配置信息进行打乱，保证不同Traefik实例的轮询起始点不同
		for index, server := range shuffle(conf.LoadBalancer.Servers, m.rand) {

			srvLogger := logger.With().
				Int(logs.ServerIndex, index).
				Str("serverAddress", server.Address).Logger()

			// 这里的address 移动是  xx.x.xx.xx:80 的形式，因为port字段并不参与序列化
			if _, _, err := net.SplitHostPort(server.Address); err != nil {
				srvLogger.Error().Err(err).Msg("Failed to split host port")
				continue
			}
			// 创建服务代理，真正需要代理数据的时候再进行拨号
			handler, err := udp.NewProxy(server.Address)
			if err != nil {
				srvLogger.Error().Err(err).Msg("Failed to create server")
				continue
			}

			// 将服务代理添加到负载均衡器中
			loadBalancer.AddServer(handler)
			srvLogger.Debug().Msg("Creating UDP server")
		}

		return loadBalancer, nil

	case conf.Weighted != nil:
		// 创建一个WRR负载均衡器，并将服务代理添加到负载均衡器中
		// 这里是按照权重进行数据的转发
		loadBalancer := udp.NewWRRLoadBalancer()

		for _, service := range shuffle(conf.Weighted.Services, m.rand) {
			handler, err := m.BuildUDP(ctx, service.Name)
			if err != nil {
				logger.Error().Err(err).Msg("Failed to build UDP handler")
				return nil, err
			}

			loadBalancer.AddWeightedServer(handler, service.Weight)
		}

		return loadBalancer, nil

	default:
		err := fmt.Errorf("the UDP service %q does not have any type defined", serviceQualifiedName)
		conf.AddError(err, true)
		return nil, err
	}
}

// shuffle 使用泛型对任意类型的切片进行随机打乱，
// 返回一个新的打乱后的切片，原切片保持不变。
//
// 在 WRR 负载均衡初始化时调用，
// 目的是让不同 Traefik 实例的轮询起始点不同，
// 避免所有实例第一次都选同一台服务器造成瞬时压力（"惊群效应"）。
func shuffle[T any](values []T, r *rand.Rand) []T {
	// 创建与原切片等长的新切片，避免修改原数据
	shuffled := make([]T, len(values))

	// 将原切片数据复制到新切片
	copy(shuffled, values)

	// Fisher–Yates 洗牌算法：遍历每个位置 i，
	// 随机选择 j ∈ [0, len)，交换 i 和 j 位置的元素
	r.Shuffle(len(shuffled), func(i, j int) { shuffled[i], shuffled[j] = shuffled[j], shuffled[i] })

	return shuffled
}
