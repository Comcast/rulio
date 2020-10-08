package configuration

import "fmt"
import "github.com/kelseyhightower/envconfig"

type AppEnvConfigPoc struct {
	EngineEnvConfig
	GenericEnvConfig
}

type EngineEnvConfig struct {
	// "linear (or indexed) state?
	LinearState bool `envconfig:"LinearState" default:"false" `
	// max pending requests; 0 means no max
	MaxPending int `envconfig:"max-pending" default:"0"  required:"true"`
	// Max locations
	MaxLocations int `envconfig:"max-locations" default:"1000"  required:"true"`
	// Max facts per location
	MaxFacts int `envconfig:"max-facts" default:"1000"  required:"true"`
	// Log accumulator verbosity
	AccVerbosity string `envconfig:"acc-verbosity" default:"EVERYTHING"  required:"true"`
	// storage type
	StorageType string `envconfig:"storage" default:"mem"  required:"true"`
	// type-specific storage config
	StorageConfig string `envconfig:"storage-config" default:""  required:"false"`
	// port engine will serve
	// enginePort string   `envconfig:"engine-port" default:"8001"  required:"true"`
	// Location TTL, a duration, 'forever', or 'never'
	LocationTTL string `envconfig:"ttl" default:"forever"  required:"true"`
	// Optional URL for external cron service"
	CronURL string `envconfig:"cron-url" default:""  required:"false"`
	// Optional URL for external cron service to reach rules engine
	RulesURL string `envconfig:"rules-url" default:"http://localhost:8001/"  required:"true"`
	// Whether to check for state consistency
	CheckState bool `envconfig:"check-state" default:"false"  required:"true"`
	// enable Bash script actions
	BashActions bool `envconfig:"bash-actions" default:"false"  required:"true"`
}

type GenericEnvConfig struct {
	// write cpu profile to this file
	CpuProfile string `envconfig:"cpuprofile" default:"1000"  required:"true"`
	// Run an HTTP server that serves profile data; 'none' to turn off
	HttpProfilePort string `envconfig:"httpProfilePort" default:"localhost:6060"  required:"true"`
	// Logging verbosity.
	Verbosity string `envconfig:"verbosity" default:"NOTHING"  required:"true"`
	// runtime.MemProfileRate
	MemProfileRate int `envconfig:"memProfileRate" default:"524288"  required:"true"`
	// runtime.SetBlockProfileRate
}

func ParseEnvConfiguration() (*AppEnvConfigPoc, error) {
	conf := &AppEnvConfigPoc{}
	if err := envconfig.Process("", conf); err != nil {
		return nil, fmt.Errorf("Environment variables are not provided: %v ", err)
	}
	return conf, nil
}
