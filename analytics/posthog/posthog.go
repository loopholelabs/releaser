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

package posthog

import (
	"github.com/posthog/posthog-go"
	"time"
)

var (
	// APIKey is the PostHog API Key
	APIKey = ""

	// APIHost is the PostHog API Host
	APIHost = ""
)

type PostHog struct {
	client posthog.Client
}

func Init() *PostHog {
	if APIKey == "" || APIHost == "" {
		return nil
	}

	client, _ := posthog.NewWithConfig(APIKey, posthog.Config{
		Endpoint:  APIHost,
		BatchSize: 1,
		Logger:    new(noopLogger),
	})
	if client != nil {
		return &PostHog{
			client: client,
		}
	}

	return nil
}

func (p *PostHog) Event(id string, name string, properties map[string]string) {
	c := posthog.Capture{
		DistinctId: id,
		Event:      name,
		Timestamp:  time.Now(),
	}

	if len(properties) > 0 {
		props := posthog.NewProperties()
		for k, v := range properties {
			props.Set(k, v)
		}
		c.Properties = props
	}

	_ = p.client.Enqueue(c)
}

func (p *PostHog) Cleanup() {
	_ = p.client.Close()
}
