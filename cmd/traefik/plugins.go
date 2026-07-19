package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/rs/zerolog/log"
	"github.com/traefik/traefik/v3/pkg/config/static"
	"github.com/traefik/traefik/v3/pkg/observability/logs"
	"github.com/traefik/traefik/v3/pkg/plugins"
)

const outputDir = "./plugins-storage/"

// 创建插件管理器
func createPluginBuilder(staticConfiguration *static.Configuration) (*plugins.Builder, error) {
	// 插件管理器，插件描述符，本地插件描述符，错误
	manager, plgs, localPlgs, err := initPlugins(staticConfiguration)
	if err != nil {
		return nil, err
	}

	return plugins.NewBuilder(manager, plgs, localPlgs)
}

func initPlugins(staticCfg *static.Configuration) (*plugins.Manager, map[string]plugins.Descriptor, map[string]plugins.LocalDescriptor, error) {

	// 保证插件唯一
	err := checkUniquePluginNames(staticCfg.Experimental)
	if err != nil {
		return nil, nil, nil, err
	}

	var manager *plugins.Manager
	plgs := map[string]plugins.Descriptor{}

	// 检查用户是否启用了插件
	if hasPlugins(staticCfg) {
		// 创建一个会自动进行失败重试的http client
		httpClient := retryablehttp.NewClient()
		httpClient.Logger = logs.NewRetryableHTTPLogger(log.Logger)
		httpClient.HTTPClient = &http.Client{Timeout: 10 * time.Second}
		// 修改最大重试的次数
		httpClient.RetryMax = 3

		// Create separate downloader for HTTP operations
		archivesPath := filepath.Join(outputDir, "archives")
		downloader, err := plugins.NewRegistryDownloader(plugins.RegistryDownloaderOptions{
			HTTPClient:   httpClient.HTTPClient,
			ArchivesPath: archivesPath,
		})
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to create plugin downloader: %w", err)
		}

		// 创建插件管理器
		opts := plugins.ManagerOptions{
			Output: outputDir,
		}
		manager, err = plugins.NewManager(downloader, opts)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to create plugins manager: %w", err)
		}

		// 将remoter插件下载到本地并将对应的插件信息落盘的json文件
		err = plugins.SetupRemotePlugins(manager, staticCfg.Experimental.Plugins)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("unable to set up plugins environment: %w", err)
		}

		plgs = staticCfg.Experimental.Plugins
	}

	localPlgs := map[string]plugins.LocalDescriptor{}

	if hasLocalPlugins(staticCfg) {
		//
		err := plugins.SetupLocalPlugins(staticCfg.Experimental.LocalPlugins)
		if err != nil {
			return nil, nil, nil, err
		}

		localPlgs = staticCfg.Experimental.LocalPlugins
	}

	return manager, plgs, localPlgs, nil
}

// 确保插件名字唯一
func checkUniquePluginNames(e *static.Experimental) error {
	if e == nil {
		return nil
	}

	// 检查插件名字是否唯一
	// 插件名字和本地插件名字不能重复
	for s := range e.LocalPlugins {
		if _, ok := e.Plugins[s]; ok {
			return fmt.Errorf("the plugin's name %q must be unique", s)
		}
	}

	return nil
}

// 检查是否有插件
func hasPlugins(staticCfg *static.Configuration) bool {
	return staticCfg.Experimental != nil && len(staticCfg.Experimental.Plugins) > 0
}

// 检查是否有本地插件
func hasLocalPlugins(staticCfg *static.Configuration) bool {
	return staticCfg.Experimental != nil && len(staticCfg.Experimental.LocalPlugins) > 0
}
