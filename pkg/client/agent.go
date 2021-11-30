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

import "github.com/gofiber/fiber/v2"

const (
	agentName = "Releaser Agent"
)

func getAgent() *fiber.Agent {
	a := fiber.AcquireAgent()
	a.Name = agentName
	a.NoDefaultUserAgentHeader = true
	return a
}

func putAgent(a *fiber.Agent) {
	fiber.ReleaseAgent(a)
}

func configureAgent(a *fiber.Agent, method string, path string) error {
	a.Request().Header.SetMethod(method)
	a.Request().SetRequestURI(path)
	return a.Parse()
}
