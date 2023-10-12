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

package client

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/loopholelabs/releaser/internal/utils"
	"github.com/loopholelabs/releaser/pkg/server"
	"runtime"
)

var (
	InvalidChecksumError = errors.New("error while verifying checksum")
)

type Client struct {
	base   string
	client *resty.Client
}

func New(base string) *Client {
	return &Client{
		base:   base,
		client: resty.New().SetBaseURL(base),
	}
}

func (c *Client) ListReleaseNames() (*server.ListReleaseNamesResponse, error) {
	req := c.client.NewRequest()
	res, err := req.Get(server.ListReleaseNamesPath)
	if err != nil {
		return nil, fmt.Errorf("error while getting available release names: %w", err)
	}

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	val := new(server.ListReleaseNamesResponse)
	return val, json.Unmarshal(res.Body(), val)
}

func (c *Client) GetLatestReleaseName() (string, error) {
	req := c.client.NewRequest()
	res, err := req.Get(server.LatestReleaseNamePath)
	if err != nil {
		return "", fmt.Errorf("error while getting latest release name: %w", err)
	}

	if res.StatusCode() != 200 {
		return "", fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	return string(res.Body()), nil
}

func (c *Client) GetChecksum(releaseName string) (string, error) {
	req := c.client.NewRequest()
	res, err := req.Get(utils.JoinPaths(server.ChecksumPath, releaseName, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return "", fmt.Errorf("error while getting checksum: %w", err)
	}

	if res.StatusCode() != 200 {
		return "", fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	return string(res.Body()), nil
}

func (c *Client) GetReleaseArtifact(releaseName string) ([]byte, error) {
	req := c.client.NewRequest()
	res, err := req.Get(utils.JoinPaths(releaseName, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return nil, fmt.Errorf("error while getting release artifact: %w", err)
	}

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	return res.Body(), nil
}

func (c *Client) DownloadReleaseArtifactAndVerify(releaseName string) ([]byte, error) {
	body, err := c.GetReleaseArtifact(releaseName)
	if err != nil {
		return nil, err
	}

	checksum, err := c.GetChecksum(releaseName)
	if err != nil {
		return nil, err
	}

	if checksum != fmt.Sprintf("%x", sha256.Sum256(body)) {
		return nil, InvalidChecksumError
	}

	return body, nil
}
