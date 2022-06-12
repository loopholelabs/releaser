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

package client

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/go-resty/resty/v2"
	"github.com/loopholelabs/releaser/internal/utils"
	"github.com/loopholelabs/releaser/pkg/server"
	"github.com/pkg/errors"
	"runtime"
)

var (
	GetVersionsError     = errors.New("error while getting available versions")
	DownloadError        = errors.New("error while downloading release")
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

func (c *Client) GetVersions() (*server.VersionsResponse, error) {
	req := c.client.NewRequest()
	res, err := req.Get(server.VersionsPath)
	if err != nil {
		return nil, errors.Wrap(err, GetVersionsError.Error())
	}

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	val := new(server.VersionsResponse)
	return val, json.Unmarshal(res.Body(), val)
}

func (c *Client) GetLatest() (string, error) {
	req := c.client.NewRequest()
	res, err := req.Get(server.LatestPath)
	if err != nil {
		return "", errors.Wrap(err, GetVersionsError.Error())
	}

	if res.StatusCode() != 200 {
		return "", fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	return string(res.Body()), nil
}

func (c *Client) GetChecksum(version string) (string, error) {
	req := c.client.NewRequest()
	res, err := req.Get(utils.JoinPaths(server.ChecksumPath, version, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return "", errors.Wrap(err, DownloadError.Error())
	}

	if res.StatusCode() != 200 {
		return "", fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	return string(res.Body()), nil
}

func (c *Client) GetBinary(version string) ([]byte, error) {
	req := c.client.NewRequest()
	res, err := req.Get(utils.JoinPaths(version, runtime.GOOS, runtime.GOARCH))
	if err != nil {
		return nil, errors.Wrap(err, DownloadError.Error())
	}

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("invalid response status code: %d with body '%s'", res.StatusCode(), string(res.Body()))
	}

	return res.Body(), nil
}

func (c *Client) DownloadVersion(version string) ([]byte, error) {
	body, err := c.GetBinary(version)
	if err != nil {
		return nil, err
	}

	checksum, err := c.GetChecksum(version)
	if err != nil {
		return nil, err
	}

	if checksum != fmt.Sprintf("%x", sha256.Sum256(body)) {
		return nil, InvalidChecksumError
	}

	return body, nil
}
