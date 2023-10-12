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

package server

import (
	"crypto/tls"
	"fmt"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/helmet/v2"
	"github.com/google/go-github/v55/github"
	"github.com/loopholelabs/releaser/analytics"
	"github.com/loopholelabs/releaser/embed"
	"github.com/loopholelabs/releaser/internal/utils"
	"github.com/loopholelabs/releaser/pkg/cache"
	"github.com/valyala/fasttemplate"
	"log"
	"net"
	"strings"
	"time"
)

const (
	LatestReleasePath     = "/"
	PingPath              = "/ping"
	LatestReleaseNamePath = "/latest"
	ListReleaseNamesPath  = "/releases"
	ChecksumPath          = "/checksum"

	ReleaseNameArgPath = "/:release_name"
	OSArgPath          = "/:os"
	ArchArgPath        = "/:arch"
)

type Server struct {
	app      *fiber.App
	cache    *cache.Cache
	github   *github.Client
	owner    string
	repo     string
	domain   string
	prefix   string
	binary   string
	template *fasttemplate.Template
}

func New(github *github.Client, hostname string, owner string, repo string, domain string, binary string) *Server {
	s := &Server{
		app: fiber.New(fiber.Config{
			ServerHeader:                 hostname,
			BodyLimit:                    -1,
			ReadTimeout:                  time.Minute * 3,
			WriteTimeout:                 time.Second * 30,
			IdleTimeout:                  time.Second * 30,
			GETOnly:                      true,
			DisableKeepalive:             true,
			DisableStartupMessage:        true,
			DisablePreParseMultipartForm: true,
		}),
		github: github,
		owner:  owner,
		repo:   repo,
		domain: domain,
		binary: binary,
	}

	s.init()

	return s
}

func (s *Server) Start(address string, config *tls.Config, tlsOverride bool) (err error) {
	s.template = fasttemplate.New(embed.Shell, embed.StartTag, embed.EndTag)
	s.cache, err = cache.New(s.github, s.owner, s.repo)
	if err != nil {
		return err
	}

	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}

	s.prefix = "http"
	if config != nil {
		listener = tls.NewListener(listener, config)
	}

	if config != nil || tlsOverride {
		s.prefix = "https"
	}

	log.Printf("Starting server on %s://%s (domain %s)", s.prefix, address, s.domain)
	return s.app.Listener(listener)
}

func (s *Server) Stop() error {
	return s.app.Shutdown()
}

func (s *Server) init() {
	s.app.Use(helmet.New())

	s.app.Get(PingPath, s.GetPing)
	s.app.Get(LatestReleasePath, s.GetLatestReleaseShellScript)
	s.app.Get(LatestReleaseNamePath, s.GetLatestReleaseName)
	s.app.Get(ListReleaseNamesPath, s.ListReleaseNames)
	s.app.Get(ReleaseNameArgPath, s.GetReleaseShellScript)

	s.app.Get(utils.JoinStrings(ChecksumPath, ReleaseNameArgPath, OSArgPath, ArchArgPath), s.GetChecksum)
	s.app.Get(utils.JoinStrings(ReleaseNameArgPath, OSArgPath, ArchArgPath), s.GetReleaseArtifact)
}

// GetPing is a simple health check endpoint that always returns 200
func (s *Server) GetPing(ctx *fiber.Ctx) error {
	return ctx.SendStatus(fiber.StatusOK)
}

// GetLatestReleaseShellScript returns a shell script which will download the latest release of the binary
// and install it on the system
func (s *Server) GetLatestReleaseShellScript(ctx *fiber.Ctx) error {
	latestReleaseName := s.cache.GetLatestReleaseName()
	if len(latestReleaseName) == 0 {
		return ctx.Status(fiber.StatusInternalServerError).SendString("no releases available")
	}

	return ctx.Redirect("/"+latestReleaseName, fiber.StatusFound)
}

// GetReleaseShellScript returns a shell script which will download the given release of the binary
// and install it on the system
func (s *Server) GetReleaseShellScript(ctx *fiber.Ctx) error {
	releaseName := ctx.Params("release_name")

	analytics.Event("release_shell", map[string]string{"release_name": releaseName})

	if !s.cache.ReleaseNameExists(releaseName) {
		return ctx.Status(fiber.StatusNotFound).SendString("release not found")
	}

	ctx.Response().Header.SetContentType(fiber.MIMETextPlainCharsetUTF8)
	return ctx.SendString(s.template.ExecuteString(map[string]interface{}{
		"domain":       s.domain,
		"release_name": releaseName,
		"prefix":       s.prefix,
		"binary":       s.binary,
	}))
}

// GetLatestReleaseName returns the name of the latest release
func (s *Server) GetLatestReleaseName(ctx *fiber.Ctx) error {
	latestReleaseName := s.cache.GetLatestReleaseName()
	if len(latestReleaseName) == 0 {
		return ctx.Status(fiber.StatusInternalServerError).SendString("no releases available")
	}
	ctx.Response().Header.SetContentType(fiber.MIMETextPlainCharsetUTF8)
	return ctx.SendString(latestReleaseName)
}

// ListReleaseNames returns a list of all available release names
func (s *Server) ListReleaseNames(ctx *fiber.Ctx) error {
	res := getListReleaseNamesResponse()
	defer putListReleaseNamesResponse(res)
	res.ReleaseNames = s.cache.GetAllReleaseNames()
	ctx.Response().Header.SetContentType(fiber.MIMEApplicationJSONCharsetUTF8)
	return ctx.JSON(res)
}

// GetChecksum returns the checksum for the given release name, os, and arch
func (s *Server) GetChecksum(ctx *fiber.Ctx) error {
	releaseName := ctx.Params("release_name")
	os := ctx.Params("os")
	arch := ctx.Params("arch")

	checksum := s.cache.GetChecksum(releaseName, os, arch)
	if len(checksum) == 0 {
		return ctx.Status(fiber.StatusNotFound).SendString("checksum not found")
	}

	ctx.Response().Header.SetContentType(fiber.MIMETextPlainCharsetUTF8)
	return ctx.SendString(checksum)
}

// GetReleaseArtifact returns the artifact for the given release name, os, and arch
func (s *Server) GetReleaseArtifact(ctx *fiber.Ctx) error {
	releaseName := strings.ToLower(ctx.Params("release_name"))
	os := ctx.Params("os")
	arch := ctx.Params("arch")

	analytics.Event("release_artifact", map[string]string{
		"release_name": releaseName,
		"os":           os,
		"arch":         arch,
	})

	if s.cache.GetLatestReleaseName() == releaseName {
		artifactBytes := s.cache.GetLatestReleaseArtifact(os, arch)
		if artifactBytes == nil {
			return ctx.Status(fiber.StatusNotFound).SendString("release not found")
		}

		ctx.Response().Header.SetContentType(fiber.MIMEOctetStream)
		ctx.Response().SetBody(artifactBytes)
		return nil
	}

	artifactName := s.cache.GetReleaseArtifactName(releaseName, os, arch)
	if artifactName == "" {
		return ctx.Status(fiber.StatusNotFound).SendString("release not found")
	}

	return ctx.Redirect(fmt.Sprintf("https://github.com/%s/%s/releases/download/%s/%s", s.owner, s.repo, releaseName, artifactName))
}
