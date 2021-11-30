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
	"fmt"
	"github.com/gofiber/fiber/v2"
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
	base string
}

func New(base string) *Client {
	return &Client{
		base: base,
	}
}

func (c *Client) GetVersions() (*server.VersionsResponse, error) {
	a := getAgent()
	defer putAgent(a)
	err := configureAgent(a, fiber.MethodGet, utils.JoinStrings(c.base, server.VersionsPath))
	if err != nil {
		return nil, errors.Wrap(err, GetVersionsError.Error())
	}

	res := new(server.VersionsResponse)
	code, body, errs := a.Struct(res)
	if code != fiber.StatusOK {
		return nil, utils.BodyError(errs, GetVersionsError, body)
	}

	return res, nil
}

func (c *Client) GetLatest() (string, error) {
	a := getAgent()
	defer putAgent(a)
	err := configureAgent(a, fiber.MethodGet, utils.JoinStrings(c.base, server.LatestPath))
	if err != nil {
		return "", errors.Wrap(err, GetVersionsError.Error())
	}

	code, body, errs := a.Bytes()
	if code != fiber.StatusOK {
		return "", utils.BodyError(errs, GetVersionsError, body)
	}

	return string(body), nil
}

func (c *Client) GetChecksum(version string) (string, error) {
	a := getAgent()
	defer putAgent(a)
	err := configureAgent(a, fiber.MethodGet, utils.JoinStrings(c.base, utils.JoinPaths(server.ChecksumPath, version, runtime.GOOS, runtime.GOARCH)))
	if err != nil {
		return "", errors.Wrap(err, DownloadError.Error())
	}

	code, checksumBody, errs := a.Bytes()
	if code != fiber.StatusOK {
		return "", utils.BodyError(errs, DownloadError, checksumBody)
	}

	return string(checksumBody), nil
}

func (c *Client) GetBinary(version string) ([]byte, error) {
	a := getAgent()
	defer putAgent(a)
	err := configureAgent(a, fiber.MethodGet, utils.JoinStrings(c.base, utils.JoinPaths(version, runtime.GOOS, runtime.GOARCH)))
	if err != nil {
		return nil, errors.Wrap(err, DownloadError.Error())
	}

	code, body, errs := a.Bytes()
	if code != fiber.StatusOK {
		return nil, utils.BodyError(errs, DownloadError, body)
	}

	return body, nil
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
