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
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
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
	checksums map[releaseKey]string
	latest    string

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
		checksums: make(map[releaseKey]string),
		close:     make(chan struct{}, 1),
		owner:     owner,
		repo:      repo,
		client:    client,
	}

	return c, c.init()
}

func (c *Cache) GetLatest() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.latest
}

func (c *Cache) GetVersions() (versions []string) {
	c.mu.RLock()
	for version := range c.versions {
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

func (c *Cache) GetChecksum(version string, os string, arch string) (string, bool) {
	if !c.GetVersion(version) {
		return "", false
	}

	key := releaseKey{
		Version: version,
		OS:      os,
		Arch:    arch,
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if checksum, ok := c.checksums[key]; !ok {
		return "", false
	} else {
		return checksum, true
	}
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
	var latest string

	c.mu.RLock()
	if len(releases) > 0 {
		latest = strings.ToLower(*releases[0].Name)
	}
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
	log.Printf("Found %d new releases, keeping %d existing releases (latest %s)", len(releasesToUpdate), len(releasesToKeep), latest)

	updateMu := new(sync.Mutex)
	updatedReleases := make(map[releaseKey][]byte)
	updatedChecksums := make(map[releaseKey]string)

	if len(releasesToUpdate) > 0 {
		var wg sync.WaitGroup
		for _, release := range releasesToUpdate {
			releaseName := strings.ToLower(release.GetName())
			for _, asset := range release.Assets {
				assetID := asset.GetID()
				assetName := strings.ToLower(asset.GetName())

				if assetName == "checksums.txt" {
					deadline, cancel = context.WithDeadline(context.Background(), time.Now().Add(time.Second*30))
					assetReader, _, err := c.client.Repositories.DownloadReleaseAsset(deadline, c.owner, c.repo, assetID, http.DefaultClient)
					if err != nil {
						cancel()
						return err
					}

					assetBytes, err := io.ReadAll(assetReader)
					if err != nil {
						cancel()
						return err
					}
					cancel()

					bytesReader := bytes.NewReader(assetBytes)
					bufioReader := bufio.NewReader(bytesReader)
					for {
						line, err := bufioReader.ReadString(byte('\n'))
						if err != nil {
							break
						}
						checksumLine := strings.Split(strings.TrimSpace(line), "  ")
						if len(checksumLine) > 1 && strings.HasSuffix(checksumLine[1], ".tar.gz") {
							trimmed := strings.TrimSuffix(checksumLine[1], ".tar.gz")
							split := strings.Split(trimmed, "_")
							if len(split) > 2 {
								key := releaseKey{
									Version: releaseName,
									OS:      split[2],
									Arch:    strings.Join(split[3:], "_"),
								}
								updateMu.Lock()
								updatedChecksums[key] = checksumLine[0]
								updateMu.Unlock()
								log.Printf("Added checksum for asset with version: %s, os: %s, arch: %s (checksum %s)", key.Version, key.OS, key.Arch, checksumLine[0])
							} else {
								log.Printf("Ignoring checksum for asset %s for version %s", checksumLine[1], releaseName)
							}
						} else {
							log.Printf("Ignoring checksum line %s for version %s", checksumLine, releaseName)
						}
					}
				} else if strings.HasSuffix(assetName, ".tar.gz") {
					log.Printf("Starting Goroutine to download %s for version %s", assetName, releaseName)
					wg.Add(1)
					go func() {
						defer wg.Done()
						deadline, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second*30))
						assetReader, _, err := c.client.Repositories.DownloadReleaseAsset(deadline, c.owner, c.repo, assetID, http.DefaultClient)
						if err != nil {
							cancel()
							log.Printf("Unable to download release asset %s for version %s due to error %s", assetName, releaseName, err)
							return
						}

						assetBytes, err := io.ReadAll(assetReader)
						if err != nil {
							cancel()
							log.Printf("Unable to download release asset %s for version %s due to error %s", assetName, releaseName, err)
							return
						}
						cancel()

						trimmed := strings.TrimSuffix(assetName, ".tar.gz")
						split := strings.Split(trimmed, "_")
						if len(split) > 2 {
							key := releaseKey{
								Version: releaseName,
								OS:      split[2],
								Arch:    strings.Join(split[3:], "_"),
							}

							updateMu.Lock()
							if checksum, ok := updatedChecksums[key]; ok {
								computedChecksum := sha256.Sum256(assetBytes)
								if stringComputedChecksum := fmt.Sprintf("%x", computedChecksum); stringComputedChecksum != checksum {
									log.Printf("Invalid checksum '%s' for asset with version: %s, os: %s, arch: %s (stored checksum %s)", stringComputedChecksum, key.Version, key.OS, key.Arch, checksum)
									updateMu.Unlock()
									return
								} else {
									log.Printf("Valid checksum '%s' for asset with version: %s, os: %s, arch: %s", stringComputedChecksum, key.Version, key.OS, key.Arch)
								}
							} else {
								log.Printf("Unable to find stored checksum for asset with version: %s, os: %s, arch: %s, will ignore", key.Version, key.OS, key.Arch)
							}
							updatedReleases[key] = assetBytes
							updateMu.Unlock()
							log.Printf("Downloaded asset with version: %s, os: %s, arch: %s (%d bytes)", key.Version, key.OS, key.Arch, len(assetBytes))
						} else {
							log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
						}
					}()
				} else {
					log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
				}
			}
		}
		wg.Wait()
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
						if checksum, ok := c.checksums[key]; ok {
							updatedChecksums[key] = checksum
							log.Printf("Keeping checksum for asset with version: %s, os: %s, arch: %s (checksum %s)", key.Version, key.OS, key.Arch, checksum)
						} else {
							log.Printf("Lost checksum for asset with version: %s, os: %s, arch: %s", key.Version, key.OS, key.Arch)
						}
					} else {
						log.Printf("Lost asset and checksum %s for version %s", assetName, releaseName)
					}
				} else {
					log.Printf("Ignoring asset %s for version %s", assetName, releaseName)
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
	c.latest = latest
	c.mu.Unlock()

	log.Printf("Done updating cache in %s", time.Since(start))

	return nil
}

func (c *Cache) updater() {
	defer c.wg.Done()

	log.Printf("Doing initial update of cache")
	err := c.update()
	if err != nil {
		log.Printf("Error during initial update of cache: %s", err)
		panic(err)
	}

	timer := time.NewTimer(time.Minute)
	defer timer.Stop()

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
