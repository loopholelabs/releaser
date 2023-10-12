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

package analytics

import (
	"github.com/loopholelabs/releaser/analytics/posthog"
)

var _ Handler = (*posthog.PostHog)(nil)

var (
	handler Handler
)

func init() {
	p := posthog.Init()
	if p != nil {
		handler = p
	}
}

type Handler interface {
	Event(name string, properties map[string]string)
	Cleanup()
}

func Event(name string, properties ...map[string]string) {
	if handler != nil {
		if len(properties) > 0 {
			handler.Event(name, properties[0])
		}
		handler.Event(name, nil)
	}
}

func Cleanup() {
	if handler != nil {
		handler.Cleanup()
		handler = nil
	}
}