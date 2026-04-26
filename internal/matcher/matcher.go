/*
Copyright 2026.

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

package matcher

import (
	"fmt"

	"github.com/gobwas/glob"
)

// Matcher wraps a compiled glob pattern for image matching.
type Matcher struct {
	g       glob.Glob
	pattern string
}

// Compile parses a glob pattern and returns a Matcher.
// Returns an error if the pattern is empty or invalid.
func Compile(pattern string) (Matcher, error) {
	if pattern == "" {
		return Matcher{}, fmt.Errorf("imagePattern must not be empty")
	}
	g, err := glob.Compile(pattern)
	if err != nil {
		return Matcher{}, fmt.Errorf("invalid imagePattern %q: %w", pattern, err)
	}
	return Matcher{g: g, pattern: pattern}, nil
}

// MatchesAnyImage returns true if at least one image in the slice matches the pattern.
func (m Matcher) MatchesAnyImage(images []string) bool {
	for _, img := range images {
		if m.g.Match(img) {
			return true
		}
	}
	return false
}

// Pattern returns the original glob pattern string.
func (m Matcher) Pattern() string {
	return m.pattern
}
