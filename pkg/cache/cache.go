/*
	Copyright 2021 Loophole Labs

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
	"context"
	"fmt"
	"github.com/google/go-github/v40/github"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

type releaseKey struct {
	Version string
	OS      string
	Arch    string
}

type Cache struct {
	mu        sync.RWMutex
	versions  map[string]string
	releases  map[releaseKey][]byte
	checksums map[string][]byte

	close chan struct{}
	wg    sync.WaitGroup

	owner  string
	repo   string
	client *github.Client
}

func New(client *github.Client, owner string, repo string) (*Cache, error) {
	c := &Cache{
		versions:  make(map[string]string),
		releases:  make(map[releaseKey][]byte),
		checksums: make(map[string][]byte),
		close:     make(chan struct{}, 1),
		owner:     owner,
		repo:      repo,
		client:    client,
	}

	return c, c.init()
}

func (c *Cache) GetVersions() (versions []string) {
	c.mu.RLock()
	for version, _ := range c.versions {
		versions = append(versions, version)
	}
	c.mu.RUnlock()
	return versions
}

func (c *Cache) GetVersion(version string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, ok := c.versions[version]
	return ok
}

func (c *Cache) GetRelease(version string, os string, arch string) ([]byte, bool) {
	if !c.GetVersion(version) {
		return nil, false
	}

	key := releaseKey{
		Version: version,
		OS:      os,
		Arch:    arch,
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if assetBytes, ok := c.releases[key]; !ok {
		return nil, false
	} else {
		return assetBytes, true
	}
}

func (c *Cache) init() error {
	log.Printf("Doing initial update of cache")
	err := c.update()
	if err != nil {
		log.Printf("Error during initial update of cache: %s", err)
		return err
	}

	c.wg.Add(1)
	go c.updater()

	return nil
}

func (c *Cache) update() error {
	start := time.Now()
	deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*30))
	releases, _, err := c.client.Repositories.ListReleases(deadline, c.owner, c.repo, nil)
	if err != nil {
		cancel()
		return err
	}
	cancel()

	var releasesToUpdate []*github.RepositoryRelease
	var releasesToKeep []*github.RepositoryRelease
	updatedVersions := make(map[string]string)

	c.mu.RLock()
	for _, release := range releases {
		releaseName := strings.ToLower(*release.Name)
		updatedVersions[releaseName] = release.GetTargetCommitish()
		if commitHash, ok := c.versions[releaseName]; !ok || release.GetTargetCommitish() != commitHash {
			releasesToUpdate = append(releasesToUpdate, release)
		} else {
			releasesToKeep = append(releasesToKeep, release)
		}
	}
	c.mu.RUnlock()
	log.Printf("Found %d new releases, keeping %d existing releases", len(releasesToUpdate), len(releasesToKeep))

	updatedReleases := make(map[releaseKey][]byte)
	updatedChecksums := make(map[string][]byte)

	if len(releasesToUpdate) > 0 {
		for _, release := range releasesToUpdate {
			releaseName := strings.ToLower(*release.Name)
			for _, asset := range release.Assets {
				assetName := strings.ToLower(*asset.Name)
				deadline, cancel = context.WithDeadline(context.Background(), time.Now().Add(time.Second*30))
				assetReader, _, err := c.client.Repositories.DownloadReleaseAsset(deadline, c.owner, c.repo, *asset.ID, http.DefaultClient)
				if err != nil {
					_ = assetReader.Close()
					cancel()
					return err
				}

				assetBytes, err := io.ReadAll(assetReader)
				if err != nil {
					_ = assetReader.Close()
					cancel()
					return err
				}
				cancel()

				if strings.HasSuffix(assetName, ".tar.gz") {
					trimmed := strings.TrimSuffix(assetName, ".tar.gz")
					split := strings.Split(trimmed, "_")
					if len(split) > 2 {
						key := releaseKey{
							Version: releaseName,
							OS:      split[2],
							Arch:    strings.Join(split[3:], "_"),
						}
						updatedReleases[key] = assetBytes
						log.Printf("Downloaded asset with version: %s, os: %s, arch: %s (%d bytes)", key.Version, key.OS, key.Arch, len(assetBytes))
					} else {
						log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
					}
				} else if assetName == "checksums.txt" {
					updatedChecksums[releaseName] = assetBytes
					log.Printf("Downloaded checksum for version: %s (%d bytes)", releaseName, len(assetBytes))
				} else {
					log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
				}
			}
		}
	}

	c.mu.RLock()
	for _, release := range releasesToKeep {
		releaseName := strings.ToLower(*release.Name)
		for _, asset := range release.Assets {
			assetName := strings.ToLower(*asset.Name)
			if strings.HasSuffix(assetName, ".tar.gz") {
				trimmed := strings.TrimSuffix(assetName, ".tar.gz")
				split := strings.Split(trimmed, "_")
				if len(split) > 2 {
					key := releaseKey{
						Version: releaseName,
						OS:      split[2],
						Arch:    strings.Join(split[3:], "_"),
					}
					if assetBytes, ok := c.releases[key]; ok {
						updatedReleases[key] = assetBytes
						log.Printf("Keeping asset with version: %s, os: %s, arch: %s (%d bytes)", key.Version, key.OS, key.Arch, len(assetBytes))
					} else {
						log.Printf("Lost asset %s for version %s", assetName, releaseName)
					}
				} else {
					log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
				}
			} else if assetName == "checksums.txt" {
				if assetBytes, ok := c.checksums[releaseName]; ok {
					updatedChecksums[releaseName] = assetBytes
					log.Printf("Keeping checksum for version: %s (%d bytes)", releaseName, len(assetBytes))
				} else {
					log.Printf("Lost checksum for version %s", releaseName)
				}
			} else {
				log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
			}
		}
	}
	c.mu.RUnlock()

	c.mu.Lock()
	c.versions = updatedVersions
	c.releases = updatedReleases
	c.checksums = updatedChecksums
	c.mu.Unlock()

	log.Printf("Done updating cache in %s", time.Now().Sub(start))

	return nil
}

func (c *Cache) updater() {
	timer := time.NewTimer(time.Minute)
	defer func() { timer.Stop(); c.wg.Done() }()
	for {
		select {
		case <-c.close:
			return
		case <-timer.C:
			log.Printf("Updating Cache")
			err := c.update()
			if err != nil {
				fmt.Printf("Error while updating cache: %s", err)
			}
			timer.Reset(time.Minute)
		}
	}
}
