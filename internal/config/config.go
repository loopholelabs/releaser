/*
	Copyright 2023 Loophole Labs

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

		   http://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package config

import (
	"errors"
	"fmt"
	"github.com/loopholelabs/cmdutils/pkg/config"
	"github.com/mitchellh/go-homedir"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"os"
	"path"
)

var _ config.Config = (*Config)(nil)

var (
	ErrRepositoryRequired      = errors.New("repository is required")
	ErrRepositoryOwnerRequired = errors.New("repository owner is required")
	ErrHostnameRequired        = errors.New("hostname is required")
	ErrListenAddressRequired   = errors.New("listen address is required")
	ErrDomainRequired          = errors.New("domain is required")
	ErrBinaryRequired          = errors.New("binary is required")
)

var (
	configFile string
	logFile    string
)

const (
	defaultConfigPath = "~/.config/releaser"
	configName        = "releaser.yml"
	logName           = "releaser.log"

	DefaultListenAddress = "0.0.0.0:8080"
	DefaultTLS           = false
	DefaultDomain        = "localhost"
	DefaultBinary        = "bin"
)

// Config is dynamically sourced from various files and environment variables.
type Config struct {
	GithubToken     string `mapstructure:"github_token"`
	Repository      string `mapstructure:"repository"`
	RepositoryOwner string `mapstructure:"repository_owner"`
	Hostname        string `mapstructure:"hostname"`
	ListenAddress   string `mapstructure:"listen_address"`
	TLS             bool   `mapstructure:"tls"`
	Domain          string `mapstructure:"domain"`
	Binary          string `mapstructure:"binary"`
}

func New() *Config {
	return &Config{
		ListenAddress: DefaultListenAddress,
		TLS:           DefaultTLS,
		Domain:        DefaultDomain,
		Binary:        DefaultBinary,
	}
}

func (c *Config) RootPersistentFlags(flags *pflag.FlagSet) {
	defaultHostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	flags.StringVar(&c.GithubToken, "github-token", "", "Github Token")
	flags.StringVar(&c.Repository, "repository", "", "Github Repository")
	flags.StringVar(&c.RepositoryOwner, "repository-owner", "", "Github Repository Owner")
	flags.StringVar(&c.Hostname, "hostname", defaultHostname, "Hostname")
	flags.StringVar(&c.ListenAddress, "listen-address", DefaultListenAddress, "Listen Address")
	flags.BoolVar(&c.TLS, "TLS", DefaultTLS, "TLS")
	flags.StringVar(&c.Domain, "domain", DefaultDomain, "Domain Name")
	flags.StringVar(&c.Binary, "binary", DefaultBinary, "Binary Name")
}

func (c *Config) GlobalRequiredFlags(_ *cobra.Command) error {
	return nil
}

func (c *Config) Validate() error {
	err := viper.Unmarshal(c)
	if err != nil {
		return fmt.Errorf("unable to unmarshal config: %w", err)
	}

	if c.Repository == "" {
		return ErrRepositoryRequired
	}

	if c.RepositoryOwner == "" {
		return ErrRepositoryOwnerRequired
	}

	if c.Hostname == "" {
		return ErrHostnameRequired
	}

	if c.ListenAddress == "" {
		return ErrListenAddressRequired
	}

	if c.Domain == "" {
		return ErrDomainRequired
	}

	if c.Binary == "" {
		return ErrBinaryRequired
	}

	return nil
}

func (c *Config) DefaultConfigDir() (string, error) {
	dir, err := homedir.Expand(defaultConfigPath)
	if err != nil {
		return "", fmt.Errorf("can't expand path %q: %s", defaultConfigPath, err)
	}

	return dir, nil
}

func (c *Config) DefaultConfigFile() string {
	return configName
}

func (c *Config) DefaultLogFile() string {
	return logName
}

func (c *Config) DefaultConfigPath() (string, error) {
	configDir, err := c.DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return path.Join(configDir, c.DefaultConfigFile()), nil
}

func (c *Config) DefaultLogPath() (string, error) {
	configDir, err := c.DefaultConfigDir()
	if err != nil {
		return "", err
	}
	return path.Join(configDir, c.DefaultLogFile()), nil
}

func (c *Config) GetConfigFile() string {
	return configFile
}

func (c *Config) GetLogFile() string {
	return logFile
}

func (c *Config) SetLogFile(file string) {
	logFile = file
}

func (c *Config) SetConfigFile(file string) {
	configFile = file
}
