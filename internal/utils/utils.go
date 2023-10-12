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

package utils

import (
	"github.com/loopholelabs/cmdutils"
	"github.com/loopholelabs/releaser/analytics"
	"github.com/loopholelabs/releaser/internal/config"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	slash = "/"
)

func JoinStrings(s ...string) string {
	var b strings.Builder
	for _, str := range s {
		b.WriteString(str)
	}
	return b.String()
}

func JoinPaths(s ...string) string {
	ret := filepath.Join(s...)
	if strings.Index(ret, slash) != 0 {
		return slash + ret
	}
	return ret
}

func PostRunAnalytics(_ *cmdutils.Helper[*config.Config]) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		analytics.Cleanup()
		return nil
	}
}

func WaitForSignal(errChan chan error) error {
	sig := make(chan os.Signal, 2)
	defer close(sig)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)
	for {
		select {
		case <-sig:
			return nil
		case err := <-errChan:
			if err == nil {
				continue
			}
			return err
		}
	}
}
