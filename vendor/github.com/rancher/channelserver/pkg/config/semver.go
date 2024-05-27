package config

import (
	"regexp"

	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
)

func Latest(releases []string, pattern, exclude string) (string, error) {
	regex, err := regexp.Compile(pattern)
	if err != nil {
		return "", err
	}

	var excludeRegexp *regexp.Regexp
	if exclude != "" {
		excludeRegexp, err = regexp.Compile(exclude)
		if err != nil {
			return "", err
		}
	}

	var (
		latest        semver.Version
		latestRelease string
	)
	for _, release := range releases {
		if !regex.MatchString(release) {
			continue
		}

		if excludeRegexp != nil && excludeRegexp.MatchString(release) {
			continue
		}

		current, err := semver.ParseTolerant(release)
		if err != nil {
			logrus.Infof("failed to parse tag %s: %v", release, err)
			continue
		}

		if current.GT(latest) {
			latest = current
			latestRelease = release
		}
	}

	return latestRelease, nil
}
