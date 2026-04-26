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

package matcher_test

import (
	"testing"

	"github.com/knabben/istio-qos/internal/matcher"
)

func TestCompile_EmptyPattern(t *testing.T) {
	_, err := matcher.Compile("")
	if err == nil {
		t.Fatal("expected error for empty pattern, got nil")
	}
}

func TestCompile_InvalidPattern(t *testing.T) {
	_, err := matcher.Compile("[invalid")
	if err == nil {
		t.Fatal("expected error for invalid glob, got nil")
	}
}

func TestMatchesAnyImage(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		images  []string
		want    bool
	}{
		{
			name:    "exact match",
			pattern: "nginx:latest",
			images:  []string{"nginx:latest"},
			want:    true,
		},
		{
			name:    "wildcard tag",
			pattern: "nginx:*",
			images:  []string{"nginx:1.25"},
			want:    true,
		},
		{
			name:    "registry prefix wildcard",
			pattern: "*/myapp:*",
			images:  []string{"registry.example.com/myapp:v1.2"},
			want:    true,
		},
		{
			name:    "no match",
			pattern: "nginx:*",
			images:  []string{"redis:7", "postgres:15"},
			want:    false,
		},
		{
			name:    "second image matches",
			pattern: "nginx:*",
			images:  []string{"redis:7", "nginx:1.25"},
			want:    true,
		},
		{
			name:    "empty image list",
			pattern: "nginx:*",
			images:  []string{},
			want:    false,
		},
		{
			name:    "tier-app-high pattern",
			pattern: "*/tier-app-high:*",
			images:  []string{"localhost:5000/tier-app-high:latest"},
			want:    true,
		},
		{
			name:    "tier-app-high pattern no match standard",
			pattern: "*/tier-app-high:*",
			images:  []string{"localhost:5000/tier-app-standard:latest"},
			want:    false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m, err := matcher.Compile(tc.pattern)
			if err != nil {
				t.Fatalf("Compile(%q) unexpected error: %v", tc.pattern, err)
			}
			if got := m.MatchesAnyImage(tc.images); got != tc.want {
				t.Errorf("MatchesAnyImage(%v) = %v, want %v", tc.images, got, tc.want)
			}
		})
	}
}
