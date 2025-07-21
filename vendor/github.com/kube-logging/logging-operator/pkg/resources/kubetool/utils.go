// Copyright © 2019 Banzai Cloud
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kubetool

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	maxNameLength = 50
	hashSuffixLen = 8
)

func FindContainerByName(cnrs []corev1.Container, name string) *corev1.Container {
	for i := range cnrs {
		cnr := &cnrs[i]
		if cnr.Name == name {
			return cnr
		}
	}
	return nil
}

func FindVolumeByName(vols []corev1.Volume, name string) *corev1.Volume {
	for i := range vols {
		vol := &vols[i]
		if vol.Name == name {
			return vol
		}
	}
	return nil
}

func FindVolumeMountByName(mnts []corev1.VolumeMount, name string) *corev1.VolumeMount {
	for i := range mnts {
		mnt := &mnts[i]
		if mnt.Name == name {
			return mnt
		}
	}
	return nil
}

func JobSuccessfullyCompleted(job *batchv1.Job) bool {
	return job.Status.CompletionTime != nil && job.Status.Succeeded > 0
}

// ValidateQualifiedName ensures the given name meets DNS-1123 label requirements
// and handles long names by truncating and adding a unique hash suffix.
func FixQualifiedNameIfInvalid(name string) string {
	// Check if the name is RFC 1123 compliant and within length limits
	errs := validation.IsDNS1123Label(name)
	// Remove max length error if present
	for i := range len(errs) {
		if errs[i] == validation.MaxLenError(validation.DNS1123LabelMaxLength) {
			errs = append(errs[:i], errs[i+1:]...)
		}
	}

	// Sanitize the name if there are remaining validation errors
	if len(errs) > 0 {
		name = sanitizeName(name)
	}

	// If the name is too long, truncate it and add a unique suffix
	if len(name) > maxNameLength {
		name = truncateName(name)
	}

	return name
}

// sanitizeName replaces invalid characters with hyphens and converts to lowercase
func sanitizeName(name string) string {
	// Trim leading and trailing whitespaces and convert to lowercase
	trimmedName := strings.ToLower(strings.TrimSpace(name))

	// Replace non-alphanumeric characters (except existing hyphens) with hyphens
	var sanitized strings.Builder
	for _, r := range trimmedName {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || r == '-' {
			sanitized.WriteRune(r)
		} else {
			sanitized.WriteRune('-')
		}
	}

	// Remove consecutive hyphens and trim leading and trailing ones aswell
	sanitizedName := strings.Trim(regexp.MustCompile(`-+`).ReplaceAllString(sanitized.String(), "-"), "-")

	return sanitizedName
}

func truncateName(name string) string {
	// Leave room for hyphen and hash suffix
	truncatedBase := name[:maxNameLength-hashSuffixLen-1]
	truncatedName := fmt.Sprintf("%s-%s", truncatedBase, generateUniqueSuffix(name))
	return truncatedName
}

// generateUniqueSuffix creates a consistent hash suffix for name uniqueness
func generateUniqueSuffix(name string) string {
	hash := sha256.Sum256([]byte(name))
	return hex.EncodeToString(hash[:])[:hashSuffixLen]
}
