package server

import (
	"net"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kiosk404/echoryn/pkg/logger"
	"github.com/kiosk404/echoryn/pkg/utils/homedir"
	"github.com/spf13/viper"
)

const (
	// RecommendedHomeDir defines the default directory used to place all generic service configurations.
	RecommendedHomeDir = ".echoryn"

	// RecommendedEnvPrefix defines the ENV prefix used by all generic service.
	RecommendedEnvPrefix = "echoryn"
)

// Config is a structure used to configure a GenericAPIServer.
// Its members are sorted roughly in order of importance for composers.
type Config struct {
	Serving         *ServingInfo
	Mode            string
	Middlewares     []string
	Healthz         bool
	EnableProfiling bool
	EnableMetrics   bool
}

// ServingInfo holds configuration
type ServingInfo struct {
	BindAddress string
	BindPort    int
}

// Address join host IP address and host port number into an address string, like: 0.0.0.0:11789.
func (s *ServingInfo) Address() string {
	return net.JoinHostPort(s.BindAddress, strconv.Itoa(s.BindPort))
}

func NewConfig() *Config {
	return &Config{
		Serving: &ServingInfo{
			BindAddress: "0.0.0.0",
			BindPort:    11789,
		},
		Healthz:         true,
		Mode:            gin.DebugMode,
		Middlewares:     []string{},
		EnableProfiling: true,
		EnableMetrics:   true,
	}
}

// CompletedConfig is the completed configuration for GenericAPIServer.
type CompletedConfig struct {
	*Config
}

// Complete fills in any fields not set that are required to have valid data and can be derived
// from other fields. If you're going to `ApplyOptions`, do that first. It's mutating the receiver.
func (c *Config) Complete() CompletedConfig {
	return CompletedConfig{c}
}

// New returns a new instance of GenericAPIServer from the given config.
func (c CompletedConfig) New() (*GenericAPIServer, error) {
	// setMode before gin.New()
	gin.SetMode(c.Mode)

	engine := gin.New()

	s := &GenericAPIServer{
		ServingInfo:     c.Serving,
		healthz:         c.Healthz,
		enableMetrics:   c.EnableMetrics,
		enableProfiling: c.EnableProfiling,
		middlewares:     c.Middlewares,
		Engine:          engine,
		Server: &http.Server{
			Addr:    c.Serving.Address(),
			Handler: engine,
		},
	}

	initGenericAPIServer(s)

	return s, nil
}

// LoadConfig reads in config file and ENV variables if set.
func LoadConfig(cfg string, defaultName string) {
	if cfg != "" {
		viper.SetConfigFile(cfg)
	} else {
		viper.AddConfigPath(".")
		viper.AddConfigPath(filepath.Join(homedir.HomeDir(), RecommendedHomeDir))
		viper.AddConfigPath("/etc/echoryn")
		viper.SetConfigName(defaultName + ".json")
	}

	// Use config file from the flag.
	viper.SetConfigType("json")              // set the type of the configuration to yaml.
	viper.AutomaticEnv()                     // read in environment variables that match.
	viper.SetEnvPrefix(RecommendedEnvPrefix) // set ENVIRONMENT variables prefix to echoryn.
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// If a config file is found, read it in.
	if err := viper.ReadInConfig(); err != nil {
		logger.Warn("WARNING: viper failed to discover and load the configuration file: %s", err.Error())
	}
}
