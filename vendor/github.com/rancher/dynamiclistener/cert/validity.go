/*
Copyright 2014 The Kubernetes Authors.

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

package cert

import (
	"crypto/x509"
	"time"

	clockutil "k8s.io/utils/clock"
)

var clock clockutil.PassiveClock = &clockutil.RealClock{}

// CalculateNotBefore calculates a NotBefore time of 1 hour in the past, or the
// NotBefore time of the optionally provided *x509.Certificate, whichever is greater.
func CalculateNotBefore(ca *x509.Certificate) time.Time {
	// Subtract 1 hour for clock skew
	now := clock.Now().UTC().Add(-time.Hour)

	// It makes no sense to return a time before the CA itself is valid.
	if ca != nil && now.Before(ca.NotBefore) {
		return ca.NotBefore
	}
	return now
}
