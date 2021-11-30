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

package server

import (
	"crypto/tls"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/helmet/v2"
	"github.com/google/go-github/v40/github"
	"github.com/loopholelabs/releaser/embed"
	"github.com/loopholelabs/releaser/pkg/cache"
	"github.com/valyala/fasttemplate"
	"log"
	"net"
	"time"
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

	s.app.Get("/", s.GetRoot)
	s.app.Get("/ping", s.GetPing)
	s.app.Get("/versions", s.GetVersions)
	s.app.Get("/:version", s.GetVersion)
	s.app.Get("/checksum/:version/:os/:arch", s.GetChecksum)
	s.app.Get("/:version/:os/:arch", s.GetBinary)
}

func (s *Server) GetRoot(ctx *fiber.Ctx) error {
	version := s.cache.GetLatest()
	if len(version) == 0 {
		return ctx.Status(fiber.StatusInternalServerError).SendString("no releases available")
	}

	ctx.Response().Header.SetContentType(fiber.MIMEOctetStream)
	return ctx.SendString(s.template.ExecuteString(map[string]interface{}{
		"domain":  s.domain,
		"version": version,
		"prefix":  s.prefix,
		"binary":  s.binary,
	}))
}

func (s *Server) GetPing(ctx *fiber.Ctx) error {
	return ctx.SendStatus(fiber.StatusOK)
}

func (s *Server) GetVersions(ctx *fiber.Ctx) error {
	res := getVersionsResponse()
	defer putVersionsResponse(res)
	res.Versions = s.cache.GetVersions()
	return ctx.JSON(res)
}

func (s *Server) GetVersion(ctx *fiber.Ctx) error {
	version := ctx.Params("version")

	if !s.cache.GetVersion(version) {
		return ctx.Status(fiber.StatusNotFound).SendString("version not available")
	}

	ctx.Response().Header.SetContentType(fiber.MIMEOctetStream)
	return ctx.SendString(s.template.ExecuteString(map[string]interface{}{
		"domain":  s.domain,
		"version": version,
		"prefix":  s.prefix,
		"binary":  s.binary,
	}))
}

func (s *Server) GetChecksum(ctx *fiber.Ctx) error {
	version := ctx.Params("version")
	os := ctx.Params("os")
	arch := ctx.Params("arch")

	checksum, ok := s.cache.GetChecksum(version, os, arch)
	if !ok {
		return ctx.Status(fiber.StatusNotFound).SendString("checksum does not exist")
	}

	ctx.Response().Header.SetContentType(fiber.MIMETextPlain)
	return ctx.SendString(checksum)
}

func (s *Server) GetBinary(ctx *fiber.Ctx) error {
	version := ctx.Params("version")
	os := ctx.Params("os")
	arch := ctx.Params("arch")

	asset, ok := s.cache.GetRelease(version, os, arch)
	if !ok {
		return ctx.Status(fiber.StatusNotFound).SendString("release does not exist")
	}

	ctx.Response().Header.SetContentType(fiber.MIMEOctetStream)
	ctx.Response().SetBody(asset)
	return nil
}
