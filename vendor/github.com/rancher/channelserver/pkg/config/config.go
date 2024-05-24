package config

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/google/go-github/v29/github"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/sirupsen/logrus"
)

type Config struct {
	sync.Mutex

	url               string
	redirect          *url.URL
	gh                *github.Client
	channelsConfig    *model.ChannelsConfig
	releasesConfig    *model.ReleasesConfig
	appDefaultsConfig *model.AppDefaultsConfig
}

type Wait interface {
	Wait(ctx context.Context) bool
}

type Source interface {
	URL() string
}

type DurationWait struct {
	Duration time.Duration
}

func (d *DurationWait) Wait(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case <-time.After(d.Duration):
		return true
	}
}

type StringSource string

func (s StringSource) URL() string {
	return string(s)
}

func NewConfig(ctx context.Context, subKey string, wait Wait, channelServerVersion string, appName string, urls []Source) *Config {
	c := &Config{
		channelsConfig:    &model.ChannelsConfig{},
		releasesConfig:    &model.ReleasesConfig{},
		appDefaultsConfig: &model.AppDefaultsConfig{},
	}

	_, _ = c.loadConfig(ctx, subKey, channelServerVersion, appName, urls...)

	go func() {
		for wait.Wait(ctx) {
			if index, err := c.loadConfig(ctx, subKey, channelServerVersion, appName, urls...); err != nil {
				logrus.Errorf("failed to reload configuration from %s: %v", urls[index].URL(), err)
			} else {
				urls = urls[:index+1]
				logrus.Infof("Loaded configuration from %s in %v", urls[index].URL(), urls)
			}
		}
	}()

	return c
}

func (c *Config) loadConfig(ctx context.Context, subKey string, channelServerVersion string, appName string, urls ...Source) (int, error) {
	content, index, err := getURLs(ctx, urls...)
	if err != nil {
		return index, fmt.Errorf("failed to get content from url %s: %v", urls[index].URL(), err)
	}

	config, err := GetChannelsConfig(ctx, content, subKey)
	if err != nil {
		return index, fmt.Errorf("failed to get channel config: %v", err)
	}

	releases, err := GetReleasesConfig(content, channelServerVersion, subKey)
	if err != nil {
		return index, fmt.Errorf("failed to get release config: %v", err)
	}

	appDefaultsConfig, err := GetAppDefaultsConfig(content, subKey, appName)
	if err != nil {
		return index, fmt.Errorf("failed to get app default config: %v", err)
	}

	return index, c.setConfig(ctx, channelServerVersion, config, releases, appDefaultsConfig)
}

func (c *Config) ghClient(config *model.ChannelsConfig) (*github.Client, error) {
	if config.GitHub == nil {
		return nil, nil
	}

	if c.gh == nil || c.url != config.GitHub.APIURL {
		if config.GitHub.APIURL == "" {
			return github.NewClient(nil), nil
		}
		return github.NewEnterpriseClient(config.GitHub.APIURL, config.GitHub.APIURL, nil)
	}
	return c.gh, nil
}

func (c *Config) setConfig(ctx context.Context, channelServerVersion string, config *model.ChannelsConfig, releases *model.ReleasesConfig, appDefaultsConfig *model.AppDefaultsConfig) error {
	gh, err := c.ghClient(config)
	if err != nil {
		return err
	}

	redirect, err := url.Parse(config.RedirectBase)
	if err != nil {
		return err
	}

	var ghReleases []string
	if gh != nil {
		ghReleases, err = GetGHReleases(ctx, gh, config.GitHub.Owner, config.GitHub.Repo)
		if err != nil {
			return err
		}
	}

	if err := resolveChannels(ghReleases, config); err != nil {
		return err
	}

	c.Lock()
	defer c.Unlock()
	c.gh = gh
	c.channelsConfig = config
	c.redirect = redirect
	c.releasesConfig = releases
	c.appDefaultsConfig = appDefaultsConfig
	if config.GitHub != nil {
		c.url = config.GitHub.APIURL
	}

	return nil
}

func resolveChannels(releases []string, config *model.ChannelsConfig) error {
	for i, channel := range config.Channels {
		if channel.Latest != "" {
			continue
		}
		if channel.LatestRegexp == "" {
			continue
		}

		release, err := Latest(releases, channel.LatestRegexp, channel.ExcludeRegexp)
		if err != nil {
			return err
		}
		config.Channels[i].Latest = release
	}

	return nil
}

func (c *Config) ChannelsConfig() *model.ChannelsConfig {
	c.Lock()
	defer c.Unlock()
	return c.channelsConfig
}

func (c *Config) ReleasesConfig() *model.ReleasesConfig {
	c.Lock()
	defer c.Unlock()
	return c.releasesConfig
}

func (c *Config) AppDefaultsConfig() *model.AppDefaultsConfig {
	c.Lock()
	defer c.Unlock()
	return c.appDefaultsConfig
}

func (c *Config) Redirect(id string) (string, error) {
	for _, channel := range c.channelsConfig.Channels {
		if channel.Name == id && channel.Latest != "" {
			return c.redirect.ResolveReference(&url.URL{
				Path: channel.Latest,
			}).String(), nil
		}
	}

	return "", nil
}
