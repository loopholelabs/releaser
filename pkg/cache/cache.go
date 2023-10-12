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

package cache

import (
	"bufio"
	"context"
	"errors"
	"github.com/google/go-github/v55/github"
	"github.com/loopholelabs/cmdutils"
	"github.com/loopholelabs/releaser/internal/config"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Cache struct {
	mu sync.RWMutex

	// releases stores whether a release exists, given its name
	releaseNames map[string]struct{}

	// checksums stores the checksum of a given artifact across
	// all releases
	checksums map[artifactKey]string

	// releaseArtifactNames stores the artifact names across all releases
	releaseArtifactNames map[artifactKey]string

	// latestRelease is the name of the latest release
	latestReleaseName string

	// latestReleaseArtifacts stores the artifacts for the latest release
	latestReleaseArtifacts map[artifactKey][]byte

	stop chan struct{}
	wg   sync.WaitGroup

	helper *cmdutils.Helper[*config.Config]
	client *github.Client
}

func New(client *github.Client, helper *cmdutils.Helper[*config.Config]) (*Cache, error) {
	c := &Cache{
		releaseNames:         make(map[string]struct{}),
		checksums:            make(map[artifactKey]string),
		releaseArtifactNames: make(map[artifactKey]string),

		latestReleaseArtifacts: make(map[artifactKey][]byte),

		stop:   make(chan struct{}, 1),
		helper: helper,
		client: client,
	}

	return c, c.init()
}

// GetLatestReleaseName returns the name of the latest release
func (c *Cache) GetLatestReleaseName() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latestReleaseName
}

// GetAllReleaseNames returns an array of all the release names
func (c *Cache) GetAllReleaseNames() []string {
	c.mu.RLock()
	releaseNames := make([]string, 0, len(c.releaseNames))
	for releaseName := range c.releaseNames {
		releaseNames = append(releaseNames, releaseName)
	}
	c.mu.RUnlock()
	return releaseNames
}

// ReleaseNameExists returns true if the given release name exists
func (c *Cache) ReleaseNameExists(releaseName string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.releaseNames[releaseName]
	return ok
}

// GetChecksum returns the checksum for the given version, os, and arch
//
// It will return an empty string if the checksum does not exist
func (c *Cache) GetChecksum(releaseName string, os string, arch string) string {
	if !c.ReleaseNameExists(releaseName) {
		return ""
	}

	key := toArtifactKey(releaseName, os, arch)

	c.mu.RLock()
	defer c.mu.RUnlock()
	if checksum, ok := c.checksums[key]; !ok {
		return ""
	} else {
		return checksum
	}
}

func (c *Cache) GetLatestReleaseArtifact(os string, arch string) []byte {
	key := toArtifactKey(c.latestReleaseName, os, arch)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if artifactBytes, ok := c.latestReleaseArtifacts[key]; !ok {
		return nil
	} else {
		return artifactBytes
	}
}

func (c *Cache) GetReleaseArtifactName(releaseName string, os string, arch string) string {
	if !c.ReleaseNameExists(releaseName) {
		return ""
	}
	key := toArtifactKey(releaseName, os, arch)
	c.mu.RLock()
	defer c.mu.RUnlock()
	if artifactName, ok := c.releaseArtifactNames[key]; !ok {
		return ""
	} else {
		return artifactName
	}
}

func (c *Cache) init() error {
	c.wg.Add(1)
	go c.updateLoop()
	return nil
}

// doUpdate updates the cache once and returns an error if one occurred
func (c *Cache) doUpdate() error {
	start := time.Now()

	ctx := context.Background()
	deadline, cancel := context.WithDeadline(ctx, time.Now().Add(time.Second*30))
	releases, _, err := c.client.Repositories.ListReleases(deadline, c.helper.Config.RepositoryOwner, c.helper.Config.Repository, nil)
	if err != nil {
		cancel()
		return err
	}
	cancel()

	releaseNames := make(map[string]struct{})
	checksums := make(map[artifactKey]string)
	releaseArtifactNames := make(map[artifactKey]string)

	if len(releases) < 1 {
		c.helper.Printer.Printf("no releases available\n")
		return nil
	}

	for _, release := range releases {
		releaseName := strings.ToLower(release.GetName())
		releaseNames[releaseName] = struct{}{}
		for _, asset := range release.Assets {
			assetID := asset.GetID()
			assetName := strings.ToLower(asset.GetName())
			switch {
			case assetName == "checksums.txt":
				deadline, cancel = context.WithDeadline(ctx, time.Now().Add(time.Second*30))
				assetReader, _, err := c.client.Repositories.DownloadReleaseAsset(deadline, c.helper.Config.RepositoryOwner, c.helper.Config.Repository, assetID, http.DefaultClient)
				if err != nil {
					cancel()
					return err
				}

				reader := bufio.NewReader(assetReader)
				for {
					line, err := reader.ReadString(byte('\n'))
					if err != nil {
						cancel()
						if !errors.Is(err, io.EOF) {
							return err
						}
						break
					}
					checksumLine := strings.Split(strings.TrimSpace(line), "  ")
					if len(checksumLine) > 1 && strings.HasSuffix(checksumLine[1], ".tar.gz") {
						trimmed := strings.TrimSuffix(checksumLine[1], ".tar.gz")
						split := strings.Split(trimmed, "_")
						if len(split) > 2 {
							key := toArtifactKey(releaseName, split[2], strings.Join(split[3:], "_"))
							checksums[key] = checksumLine[0]
							c.helper.Printer.Printf("added checksum for asset with key %s (checksum %s)\n", key, checksumLine[0])
						} else {
							c.helper.Printer.Printf("error: malformed asset name %s for release %s\n", checksumLine[1], releaseName)
						}
					} else {
						c.helper.Printer.Printf("error: invalid checksum %s for release %s\n", checksumLine, releaseName)
					}
				}
			case strings.HasSuffix(assetName, ".tar.gz"):
				trimmed := strings.TrimSuffix(assetName, ".tar.gz")
				split := strings.Split(trimmed, "_")
				if len(split) > 2 {
					key := toArtifactKey(releaseName, split[2], strings.Join(split[3:], "_"))
					releaseArtifactNames[key] = assetName
					c.helper.Printer.Printf("saved release artifact name %s with key %s\n", assetName, key)
				} else {
					c.helper.Printer.Printf("error: malformed artifact name %s for release %s\n", assetName, releaseName)
				}
			}
		}
	}

	c.mu.Lock()
	c.releaseNames = releaseNames
	c.checksums = checksums
	c.releaseArtifactNames = releaseArtifactNames
	c.mu.Unlock()

	latestRelease := releases[0]
	latestReleaseName := strings.ToLower(latestRelease.GetName())
	latestReleaseArtifacts := make(map[artifactKey][]byte)

	if c.latestReleaseName != latestReleaseName {
		c.helper.Printer.Printf("updating cached assets for latest release to %s (was %s)\n", latestReleaseName, c.latestReleaseName)
		for _, asset := range latestRelease.Assets {
			assetID := asset.GetID()
			assetName := strings.ToLower(asset.GetName())
			if strings.HasSuffix(assetName, ".tar.gz") {
				deadline, cancel = context.WithDeadline(ctx, time.Now().Add(time.Second*30))
				assetReader, _, err := c.client.Repositories.DownloadReleaseAsset(deadline, c.helper.Config.RepositoryOwner, c.helper.Config.Repository, assetID, http.DefaultClient)
				if err != nil {
					c.helper.Printer.Printf("error: unable to download release asset %s for latest release %s: %s\n", assetName, latestReleaseName, err)
					cancel()
					return err
				}

				artifactBytes, err := io.ReadAll(assetReader)
				if err != nil {
					cancel()
					c.helper.Printer.Printf("error: unable to download release asset %s for latest release %s: %s\n", assetName, latestReleaseName, err)
					return err
				}

				trimmed := strings.TrimSuffix(assetName, ".tar.gz")
				split := strings.Split(trimmed, "_")
				if len(split) > 2 {
					key := toArtifactKey(latestReleaseName, split[2], strings.Join(split[3:], "_"))
					latestReleaseArtifacts[key] = artifactBytes
					c.helper.Printer.Printf("downloaded release artifact %s with key %s (%d bytes)\n", assetName, key, len(artifactBytes))
				} else {
					c.helper.Printer.Printf("error: malformed artifact name %s for latest release %s\n", assetName, latestReleaseName)
				}
				cancel()
			}
		}
	} else {
		c.helper.Printer.Printf("latest release %s already cached\n", c.latestReleaseName)
	}

	c.mu.Lock()
	c.latestReleaseName = latestReleaseName
	c.latestReleaseArtifacts = latestReleaseArtifacts
	c.mu.Unlock()

	c.helper.Printer.Printf("done updating cache in %s\n", time.Since(start))

	return nil
}

// updateLoop runs the update function every minute and updates the latest cache
func (c *Cache) updateLoop() {
	defer c.wg.Done()

	c.helper.Printer.Printf("Doing initial update of cache\n")
	err := c.doUpdate()
	if err != nil {
		c.helper.Printer.Printf("error: unable to do initial update of cache: %s\n", err)
		panic(err)
	}

	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

	for {
		select {
		case <-c.stop:
			return
		case <-timer.C:
			c.helper.Printer.Printf("updating cache\n")
			err := c.doUpdate()
			if err != nil {
				c.helper.Printer.Printf("error: unable to update cache: %s\n", err)
			}
			timer.Reset(time.Minute)
		}
	}
}
