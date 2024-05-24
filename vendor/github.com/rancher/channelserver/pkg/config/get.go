package config

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/blang/semver"
	"github.com/google/go-github/v29/github"
	"github.com/rancher/channelserver/pkg/model"
	"github.com/rancher/wrangler/pkg/data/convert"
	"sigs.k8s.io/yaml"
)

var (
	httpClient = &http.Client{
		Timeout: time.Second * 5,
	}
)

func getURLs(ctx context.Context, urls ...Source) ([]byte, int, error) {
	var (
		bytes []byte
		err   error
		index int
	)
	for i, url := range urls {
		index = i
		bytes, err = get(ctx, url)
		if err == nil {
			break
		}
	}

	return bytes, index, err
}

func get(ctx context.Context, url Source) ([]byte, error) {
	content, err := os.ReadFile(url.URL())
	if err == nil {
		return content, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url.URL(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %v", resp.Status)
	}

	return io.ReadAll(resp.Body)
}

func GetChannelsConfig(ctx context.Context, content []byte, subKey string) (*model.ChannelsConfig, error) {
	var (
		data   = map[string]interface{}{}
		config = &model.ChannelsConfig{}
	)

	if subKey == "" {
		return config, yaml.Unmarshal(content, config)
	}

	if err := yaml.Unmarshal(content, &data); err != nil {
		return nil, err
	}
	data, _ = data[subKey].(map[string]interface{})
	if data == nil {
		return nil, fmt.Errorf("failed to find key %s in config", subKey)
	}
	return config, convert.ToObj(data, config)
}

func GetReleasesConfig(content []byte, channelServerVersion, subKey string) (*model.ReleasesConfig, error) {
	var (
		allReleases       model.ReleasesConfig
		availableReleases model.ReleasesConfig
		data              map[string]interface{}
	)

	if subKey == "" {
		if err := yaml.Unmarshal(content, &allReleases); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}
		data, _ = data[subKey].(map[string]interface{})
		if err := convert.ToObj(data, &allReleases); err != nil {
			return nil, err
		}
	}

	// no server version specified, show all releases
	if channelServerVersion == "" {
		return &allReleases, nil
	}

	availableReleases = model.ReleasesConfig{}

	serverVersion, err := semver.ParseTolerant(channelServerVersion)
	if err != nil {
		return nil, err
	}

	for _, release := range allReleases.Releases {
		minServerVer, err := semver.ParseTolerant(release.ChannelServerMinVersion)
		if err != nil {
			continue
		}

		maxServerVer, err := semver.ParseTolerant(release.ChannelServerMaxVersion)
		if err != nil {
			continue
		}

		if serverVersion.GE(minServerVer) && serverVersion.LE(maxServerVer) {
			availableReleases.Releases = append(availableReleases.Releases, release)
		}
	}

	return &availableReleases, nil
}

func GetGHReleases(ctx context.Context, client *github.Client, owner, repo string) ([]string, error) {
	var (
		opt         = &github.ListOptions{}
		allReleases []string
	)

	for {
		releases, resp, err := client.Repositories.ListReleases(ctx, owner, repo, opt)
		if err != nil {
			return nil, err
		}
		for _, release := range releases {
			if release.GetTagName() != "" && !release.GetPrerelease() {
				allReleases = append(allReleases, *release.TagName)
			}
		}
		if resp.NextPage == 0 {
			break
		}
		opt.Page = resp.NextPage
	}

	return allReleases, nil
}

func GetAppDefaultsConfig(content []byte, subKey, appName string) (*model.AppDefaultsConfig, error) {
	var (
		data             map[string]interface{}
		allConfigs       model.AppDefaultsConfig
		availableConfigs model.AppDefaultsConfig
	)

	if subKey == "" {
		if err := yaml.Unmarshal(content, &allConfigs); err != nil {
			return nil, err
		}
	} else {
		if err := yaml.Unmarshal(content, &data); err != nil {
			return nil, err
		}
		subData, ok := data[subKey].(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("content at key %s expected to be map[string]interface{}, found %T instead", subKey, data[subKey])
		}
		if err := convert.ToObj(subData, &allConfigs); err != nil {
			return nil, err
		}
	}

	// no app name is specified, return all AppDefaultsConfigs
	if appName == "" {
		return &allConfigs, nil
	}
	for _, appDefault := range allConfigs.AppDefaults {
		if appDefault.AppName == appName {
			availableConfigs.AppDefaults = append(availableConfigs.AppDefaults, appDefault)
			break
		}
	}
	return &availableConfigs, nil
}
