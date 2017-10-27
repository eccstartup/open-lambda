package config

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"path"
	"path/filepath"
)

var Timing bool

// Config represents the configuration for a worker server.
type Config struct {
	// base path for path parameters in this config; must be non-empty if any
	// path (e.g., Worker_dir) is relative
	path string
	// registry type: "local" or "olregistry"
	Registry string `json:"registry"`
	// sandbox type: "docker" or "olcontainer"
	// currently ignored as cgroup sandbox is not fully integrated
	Sandbox string `json:"sandbox"`
	// registry directory for storing local copies of handler code
	Reg_dir string `json:"reg_dir"`
	// name of the cluster
	Cluster_name string `json:"cluster_name"`
	// pip mirror address for installing python packages
	Index_host string `json:"index_host"`
	Index_port string `json:"index_port"`
	// packages directory for unpack-only installations
	Pkgs_dir string `json:"pkgs_dir"`
	// max number of concurrent runners per sandbox
	Max_runners int `json:"max_runners"`

	// cache options
	Handler_cache_size          int    `json:"handler_cache_size"` //kb
	Import_cache_size           int    `json:"import_cache_size"`  //kb
	Import_cache_buffer         int    `json:"import_cache_buffer"`
	Import_cache_buffer_threads int    `json:"import_cache_buffer_threads"`
	Import_cache_dir            string `json:"import_cache_dir"`

	// olregistry options
	// addresses of olregistry cluster
	Reg_cluster []string `json:"reg_cluster"`

	// sandbox options
	// worker directory, which contains handler code, pid file, logs, etc.
	Worker_dir string `json:"worker_dir"`
	// initialization path for olcontainer; currently ignored
	OLContainer_init_path string `json: "olcontainer_init_path"`
	// base path for olcontainer handlers
	OLContainer_handler_base string `json: "olcontainer_handler_base"`
	// base path for olcontainer import cache
	OLContainer_cache_base string `json: "olcontainer_cache_base"`
	// port the worker server listens to
	Worker_port string `json:"worker_port"`

	// sandbox factory options
	// number of sandbox buffers; if zero, no buffer will be used
	Sandbox_buffer         int `json:"sandbox_buffer"`
	Sandbox_buffer_threads int `json:"sandbox_buffer_threads"`
	// if olcontainer -> number of cgroup to init
	Cg_pool_size int `json:"cg_pool_size"`

	// for unit testing to skip pull path
	Skip_pull_existing bool `json:"Skip_pull_existing"`

	// pass through to sandbox envirenment variable
	Sandbox_config interface{} `json:"sandbox_config"`

	// write benchmark times to separate log file
	Benchmark_file string `json:"benchmark_log"`

	Timing bool `json:"timing"`
}

// SandboxConfJson marshals the Sandbox_config of the Config into a JSON string.
func (c *Config) SandboxConfJson() string {
	s, err := json.Marshal(c.Sandbox_config)
	if err != nil {
		panic(err)
	}
	return string(s)
}

// Dump prints the Config as a JSON string.
func (c *Config) Dump() {
	s, err := json.Marshal(c)
	if err != nil {
		panic(err)
	}
	log.Printf("CONFIG = %v\n", string(s))
}

// DumpStr returns the Config as an indented JSON string.
func (c *Config) DumpStr() string {
	s, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		panic(err)
	}
	return string(s)
}

// Save writes the Config as an indented JSON to path with 644 mode.
func (c *Config) Save(path string) error {
	s, err := json.MarshalIndent(c, "", "\t")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, s, 0644)
}

// Defaults verifies the fields of Config are correct, and initializes some
// if they are empty.
func (c *Config) Defaults() error {
	if c.Cluster_name == "" {
		c.Cluster_name = "default"
	}

	if c.Worker_port == "" {
		c.Worker_port = "8080"
	}

	if c.Registry == "olregistry" && len(c.Reg_cluster) == 0 {
		return fmt.Errorf("must specify reg_cluster")
	}

	if c.Reg_dir == "" {
		return fmt.Errorf("must specify local registry directory")
	}

	if !path.IsAbs(c.Reg_dir) {
		if c.path == "" {
			return fmt.Errorf("Reg_dir cannot be relative, unless config is loaded from file")
		}
		path, err := filepath.Abs(path.Join(path.Dir(c.path), c.Reg_dir))
		if err != nil {
			return err
		}
		c.Reg_dir = path
	}

	// worker dir
	if c.Worker_dir == "" {
		return fmt.Errorf("must specify local worker directory")
	}

	if !path.IsAbs(c.Worker_dir) {
		if c.path == "" {
			return fmt.Errorf("Worker_dir cannot be relative, unless config is loaded from file")
		}
		path, err := filepath.Abs(path.Join(path.Dir(c.path), c.Worker_dir))
		if err != nil {
			return err
		}
		c.Worker_dir = path
	}

	if c.Pkgs_dir == "" {
		return fmt.Errorf("must specify packages directory")
	}

	if !path.IsAbs(c.Pkgs_dir) {
		if c.path == "" {
			return fmt.Errorf("Pkgs_dir cannot be relative, unless config is loaded from file")
		}
		path, err := filepath.Abs(path.Join(path.Dir(c.path), c.Pkgs_dir))
		if err != nil {
			return err
		}
		c.Pkgs_dir = path
	}

	// olcontainer sandboxes require some extra settings
	if c.Sandbox == "olcontainer" {
		// olcontainer_init path
		if c.OLContainer_init_path == "" {
			return fmt.Errorf("must specify OLContainer_init_path")
		}

		if !path.IsAbs(c.OLContainer_init_path) {
			if c.path == "" {
				return fmt.Errorf("OLContainer_init_path cannot be relative, unless config is loaded from file")
			}
			path, err := filepath.Abs(path.Join(path.Dir(c.path), c.OLContainer_init_path))
			if err != nil {
				return err
			}
			c.OLContainer_init_path = path
		}

		// olcontainer base paths
		if c.OLContainer_handler_base == "" {
			return fmt.Errorf("must specify olcontainer_handler_base")
		}

		if !path.IsAbs(c.OLContainer_handler_base) {
			if c.path == "" {
				return fmt.Errorf("olcontainer_handler_base cannot be relative, unless config is loaded from file")
			}
			path, err := filepath.Abs(path.Join(path.Dir(c.path), c.OLContainer_handler_base))
			if err != nil {
				return err
			}
			c.OLContainer_handler_base = path
		}

		if c.Import_cache_size != 0 {
			if c.OLContainer_cache_base == "" {
				return fmt.Errorf("must specify olcontainer_cache_base if using import cache")
			}

			if !path.IsAbs(c.OLContainer_cache_base) {
				if c.path == "" {
					return fmt.Errorf("olcontainer_cache_base cannot be relative, unless config is loaded from file")
				}
				path, err := filepath.Abs(path.Join(path.Dir(c.path), c.OLContainer_cache_base))
				if err != nil {
					return err
				}
				c.OLContainer_cache_base = path
			}
		}
	}

	// import cache
	if c.Import_cache_size != 0 {
		if c.Import_cache_dir == "" {
			return fmt.Errorf("must specify import_cache_dir if using import cache")
		}

		if !path.IsAbs(c.Import_cache_dir) {
			if c.path == "" {
				return fmt.Errorf("Import_cache_dir cannot be relative, unless config is loaded from file")
			}
			path, err := filepath.Abs(path.Join(path.Dir(c.path), c.Import_cache_dir))
			if err != nil {
				return err
			}
			c.Import_cache_dir = path
		}
	}

	Timing = c.Timing

	return nil
}

// ParseConfig reads a file and tries to parse it as a JSON string to a Config
// instance.
func ParseConfig(path string) (*Config, error) {
	config_raw, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not open config (%v): %v\n", path, err.Error())
	}
	var config Config

	if err := json.Unmarshal(config_raw, &config); err != nil {
		log.Printf("FILE: %v\n", config_raw)
		return nil, fmt.Errorf("could not parse config (%v): %v\n", path, err.Error())
	}

	config.path = path
	if err := config.Defaults(); err != nil {
		return nil, err
	}

	return &config, nil
}
