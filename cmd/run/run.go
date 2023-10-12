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

package run

import (
	"context"
	"fmt"
	"github.com/google/go-github/v55/github"
	"github.com/loopholelabs/cmdutils"
	"github.com/loopholelabs/cmdutils/pkg/command"
	"github.com/loopholelabs/releaser/internal/config"
	"github.com/loopholelabs/releaser/internal/log"
	"github.com/loopholelabs/releaser/internal/utils"
	"github.com/loopholelabs/releaser/pkg/server"
	"github.com/spf13/cobra"
	"golang.org/x/oauth2"
	"net/http"
)

// Cmd encapsulates the commands for running the CLI.
func Cmd() command.SetupCommand[*config.Config] {
	return func(cmd *cobra.Command, ch *cmdutils.Helper[*config.Config]) {
		runCmd := &cobra.Command{
			Use:  "run",
			Long: "Run the releaser",
			PreRunE: func(cmd *cobra.Command, args []string) error {
				log.Init(ch.Config.GetLogFile(), ch.Debug())
				err := ch.Config.GlobalRequiredFlags(cmd)
				if err != nil {
					return err
				}

				err = ch.Config.Validate()
				if err != nil {
					return err
				}

				return nil
			},
			PostRunE: utils.PostRunAnalytics(ch),
			RunE: func(cmd *cobra.Command, args []string) error {
				ctx := context.Background()
				httpClient := http.DefaultClient
				if ch.Config.GithubToken != "" {
					tokenSource := oauth2.StaticTokenSource(
						&oauth2.Token{AccessToken: ch.Config.GithubToken},
					)
					httpClient = oauth2.NewClient(ctx, tokenSource)
				}

				githubClient := github.NewClient(httpClient)

				ch.Printer.Printf("Releaser starting for Github Repository %s/%s, binaries will be created as %s", ch.Config.RepositoryOwner, ch.Config.Repository, ch.Config.Binary)

				errCh := make(chan error, 1)
				s := server.New(githubClient, ch)
				go func() {
					errCh <- s.Start(ch.Config.ListenAddress, nil, ch.Config.TLS)
				}()

				err := utils.WaitForSignal(errCh)
				if err != nil {
					_ = s.Stop()
					return fmt.Errorf("error while starting Releaser API: %w", err)
				}

				err = s.Stop()
				if err != nil {
					return fmt.Errorf("failed to stop Releaser API: %w", err)
				}
				return nil
			},
		}

		cmd.AddCommand(runCmd)
	}
}
