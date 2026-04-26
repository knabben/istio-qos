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

package controller

import (
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
)

// ValidateManagerOptions enforces non-negotiable startup invariants.
// Leader election is mandatory and non-configurable; this check prevents
// accidental disabling via programmatic overrides.
func ValidateManagerOptions(opts ctrl.Options) error {
	if !opts.LeaderElection {
		return fmt.Errorf("leader election is mandatory: set LeaderElection=true")
	}
	return nil
}
