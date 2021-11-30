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
	"github.com/pkg/errors"
	"path/filepath"
	"strings"
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

func BodyError(errs []error, def error, body []byte) error {
	if len(errs) > 0 {
		if body != nil {
			return errors.Wrap(errors.New(string(body)), def.Error())
		}
		retError := errs[0]
		for i := 1; i < len(errs); i++ {
			retError = errors.Wrap(retError, errs[i].Error())
		}
		return errors.Wrap(retError, def.Error())
	}
	return def
}
