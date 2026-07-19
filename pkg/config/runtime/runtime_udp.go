package runtime

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/config/dynamic"
	"github.com/traefik/traefik/v3/pkg/observability/logs"
)

// GetUDPRoutersByEntryPoints returns all the UDP routers by entry points name and routers name.
// 获取所有entrypoint对应的udp router
func (c *Configuration) GetUDPRoutersByEntryPoints(ctx context.Context, entryPoints []string) map[string]map[string]*UDPRouterInfo {
	// 创建一个map，key为entrypoint名称，value为map[string]*UDPRouterInfo，用于存储entrypoint对应的udp router
	entryPointsRouters := make(map[string]map[string]*UDPRouterInfo)

	for rtName, rt := range c.UDPRouters {
		logger := log.Ctx(ctx).With().Str(logs.RouterName, rtName).Logger()

		// 如果配置信息中没有entrypoint，则使用默认的entrypoint
		eps := rt.EntryPoints
		if len(eps) == 0 {
			logger.Debug().Msgf("No entryPoint defined for this router, using the default one(s) instead: %+v", entryPoints)
			eps = entryPoints
		}

		entryPointsCount := 0
		for _, entryPointName := range eps {
			// 如果entrypoint不存在，则记录错误信息，并跳过该entrypoint
			if !slices.Contains(entryPoints, entryPointName) {
				rt.AddError(fmt.Errorf("entryPoint %q doesn't exist", entryPointName), false)
				logger.Error().Str(logs.EntryPointName, entryPointName).
					Msg("EntryPoint doesn't exist")
				continue
			}

			// 如果entrypoint对应的udp router不存在，则创建一个空的map[string]*UDPRouterInfo
			if _, ok := entryPointsRouters[entryPointName]; !ok {
				entryPointsRouters[entryPointName] = make(map[string]*UDPRouterInfo)
			}

			entryPointsCount++
			rt.Using = append(rt.Using, entryPointName)

			// rtName 运行时的名字
			entryPointsRouters[entryPointName][rtName] = rt
		}

		if entryPointsCount == 0 {
			rt.AddError(errors.New("no valid entryPoint for this router"), true)
			logger.Error().Msg("No valid entryPoint for this router")
		}
	}

	return entryPointsRouters
}

// UDPRouterInfo holds information about a currently running UDP router.
type UDPRouterInfo struct {
	*dynamic.UDPRouter // dynamic configuration

	Err []string `json:"error,omitempty"` // initialization error
	// Status reports whether the router is disabled, in a warning state, or all good (enabled).
	// If not in "enabled" state, the reason for it should be in the list of Err.
	// It is the caller's responsibility to set the initial status.
	Status string   `json:"status,omitempty"`
	Using  []string `json:"using,omitempty"` // Effective entry points used by that router.
}

// AddError adds err to r.Err, if it does not already exist.
// If critical is set, r is marked as disabled.
func (r *UDPRouterInfo) AddError(err error, critical bool) {
	if slices.Contains(r.Err, err.Error()) {
		return
	}

	r.Err = append(r.Err, err.Error())
	if critical {
		r.Status = StatusDisabled
		return
	}

	// only set it to "warning" if not already in a worse state
	if r.Status != StatusDisabled {
		r.Status = StatusWarning
	}
}

// UDPServiceInfo holds information about a currently running UDP service.
// udp 服务器，当routers多的时候，分别分配给不同的udp服务器
type UDPServiceInfo struct {
	*dynamic.UDPService // dynamic configuration

	Err []string `json:"error,omitempty"` // initialization error
	// Status reports whether the service is disabled, in a warning state, or all good (enabled).
	// If not in "enabled" state, the reason for it should be in the list of Err.
	// It is the caller's responsibility to set the initial status.
	Status string   `json:"status,omitempty"`
	UsedBy []string `json:"usedBy,omitempty"` // list of routers using that service
}

// AddError adds err to s.Err, if it does not already exist.
// If critical is set, s is marked as disabled.
func (s *UDPServiceInfo) AddError(err error, critical bool) {
	if slices.Contains(s.Err, err.Error()) {
		return
	}

	s.Err = append(s.Err, err.Error())
	if critical {
		s.Status = StatusDisabled
		return
	}

	// only set it to "warning" if not already in a worse state
	if s.Status != StatusDisabled {
		s.Status = StatusWarning
	}
}
