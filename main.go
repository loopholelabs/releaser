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

package main

import (
	"context"
	"errors"
	"github.com/google/go-github/v40/github"
	"github.com/loopholelabs/releaser/pkg/server"
	"github.com/namsral/flag"
	"golang.org/x/oauth2"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

var token string
var owner string
var repo string
var hostname string
var address string
var domain string
var binary string
var tls bool

func main() {
	defaultHostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}

	flag.StringVar(&token, "token", "", "Github Token")
	flag.StringVar(&owner, "owner", "", "Github Repository Owner")
	flag.StringVar(&repo, "repo", "", "Github Repository")
	flag.StringVar(&hostname, "hostname", defaultHostname, "Hostname for this Server")
	flag.StringVar(&address, "address", "0.0.0.0:8080", "Listen Address for this Server")
	flag.StringVar(&domain, "domain", "localhost", "Domain Name for this Server")
	flag.StringVar(&binary, "binary", "bin", "Binary name to install")
	flag.BoolVar(&tls, "TLS", false, "Override whether TLS is enabled")

	flag.Parse()

	if len(token) == 0 {
		panic(errors.New("invalid github token"))
	}

	if len(owner) == 0 {
		panic(errors.New("invalid github repository owner"))
	}

	if len(repo) == 0 {
		panic(errors.New("invalid github repository"))
	}

	if len(hostname) == 0 {
		panic(errors.New("invalid hostname"))
	}

	if len(address) == 0 {
		panic(errors.New("invalid address"))
	}

	if len(domain) == 0 {
		panic(errors.New("invalid domain"))
	}

	if len(binary) == 0 {
		panic(errors.New("invalid binary"))
	}

	ctx := context.Background()
	tokenSource := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: token},
	)
	oauthClient := oauth2.NewClient(ctx, tokenSource)
	githubClient := github.NewClient(oauthClient)

	log.Printf("Releaser starting for Github Repository %s/%s, binaries will be created as %s", owner, repo, binary)
	s := server.New(githubClient, hostname, owner, repo, domain, binary)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		err = s.Start(address, nil, tls)
		if err != nil {
			panic(err)
		}
	}()

	<-stop
	err = s.Stop()
	if err != nil {
		panic(err)
	}
}
