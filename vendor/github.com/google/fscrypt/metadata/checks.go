/*
 * checks.go - Some sanity check methods for our metadata structures
 *
 * Copyright 2017 Google Inc.
 * Author: Joe Richey (joerichey@google.com)
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not
 * use this file except in compliance with the License. You may obtain a copy of
 * the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
 * License for the specific language governing permissions and limitations under
 * the License.
 */

package metadata

import (
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"

	"github.com/google/fscrypt/util"
)

var errNotInitialized = errors.New("not initialized")

// Metadata is the interface to all of the protobuf structures that can be
// checked for validity.
type Metadata interface {
	CheckValidity() error
	proto.Message
}

// CheckValidity ensures the mode has a name and isn't empty.
func (m EncryptionOptions_Mode) CheckValidity() error {
	if m == EncryptionOptions_default {
		return errNotInitialized
	}
	if m.String() == "" {
		return errors.Errorf("unknown %d", m)
	}
	return nil
}

// CheckValidity ensures the source has a name and isn't empty.
func (s SourceType) CheckValidity() error {
	if s == SourceType_default {
		return errNotInitialized
	}
	if s.String() == "" {
		return errors.Errorf("unknown %d", s)
	}
	return nil
}

// CheckValidity ensures the hash costs will be accepted by Argon2.
func (h *HashingCosts) CheckValidity() error {
	if h == nil {
		return errNotInitialized
	}
	if h.Time <= 0 {
		return errors.Errorf("time=%d is not positive", h.Time)
	}
	if h.Parallelism <= 0 {
		return errors.Errorf("parallelism=%d is not positive", h.Parallelism)
	}
	minMemory := 8 * h.Parallelism
	if h.Memory < minMemory {
		return errors.Errorf("memory=%d is less than minimum (%d)", h.Memory, minMemory)
	}
	return nil
}

// CheckValidity ensures our buffers are the correct length.
func (w *WrappedKeyData) CheckValidity() error {
	if w == nil {
		return errNotInitialized
	}
	if len(w.EncryptedKey) == 0 {
		return errors.Wrap(errNotInitialized, "encrypted key")
	}
	if err := util.CheckValidLength(IVLen, len(w.IV)); err != nil {
		return errors.Wrap(err, "IV")
	}
	return errors.Wrap(util.CheckValidLength(HMACLen, len(w.Hmac)), "HMAC")
}

// CheckValidity ensures our ProtectorData has the correct fields for its source.
func (p *ProtectorData) CheckValidity() error {
	if p == nil {
		return errNotInitialized
	}

	if err := p.Source.CheckValidity(); err != nil {
		return errors.Wrap(err, "protector source")
	}

	// Source specific checks
	switch p.Source {
	case SourceType_pam_passphrase:
		if p.Uid < 0 {
			return errors.Errorf("UID=%d is negative", p.Uid)
		}
		fallthrough
	case SourceType_custom_passphrase:
		if err := p.Costs.CheckValidity(); err != nil {
			return errors.Wrap(err, "passphrase hashing costs")
		}
		if err := util.CheckValidLength(SaltLen, len(p.Salt)); err != nil {
			return errors.Wrap(err, "passphrase hashing salt")
		}
	}

	// Generic checks
	if err := p.WrappedKey.CheckValidity(); err != nil {
		return errors.Wrap(err, "wrapped protector key")
	}
	if err := util.CheckValidLength(ProtectorDescriptorLen, len(p.ProtectorDescriptor)); err != nil {
		return errors.Wrap(err, "protector descriptor")

	}
	err := util.CheckValidLength(InternalKeyLen, len(p.WrappedKey.EncryptedKey))
	return errors.Wrap(err, "encrypted protector key")
}

// CheckValidity ensures each of the options is valid.
func (e *EncryptionOptions) CheckValidity() error {
	if e == nil {
		return errNotInitialized
	}
	if _, ok := util.Index(e.Padding, paddingArray); !ok {
		return errors.Errorf("padding of %d is invalid", e.Padding)
	}
	if err := e.Contents.CheckValidity(); err != nil {
		return errors.Wrap(err, "contents encryption mode")
	}
	if err := e.Filenames.CheckValidity(); err != nil {
		return errors.Wrap(err, "filenames encryption mode")
	}
	// If PolicyVersion is unset, treat it as 1.
	if e.PolicyVersion == 0 {
		e.PolicyVersion = 1
	}
	if e.PolicyVersion != 1 && e.PolicyVersion != 2 {
		return errors.Errorf("policy version of %d is invalid", e.PolicyVersion)
	}
	return nil
}

// CheckValidity ensures the fields are valid and have the correct lengths.
func (w *WrappedPolicyKey) CheckValidity() error {
	if w == nil {
		return errNotInitialized
	}
	if err := w.WrappedKey.CheckValidity(); err != nil {
		return errors.Wrap(err, "wrapped key")
	}
	if err := util.CheckValidLength(PolicyKeyLen, len(w.WrappedKey.EncryptedKey)); err != nil {
		return errors.Wrap(err, "encrypted key")
	}
	err := util.CheckValidLength(ProtectorDescriptorLen, len(w.ProtectorDescriptor))
	return errors.Wrap(err, "wrapping protector descriptor")
}

// CheckValidity ensures the fields and each wrapped key are valid.
func (p *PolicyData) CheckValidity() error {
	if p == nil {
		return errNotInitialized
	}
	// Check each wrapped key
	for i, w := range p.WrappedPolicyKeys {
		if err := w.CheckValidity(); err != nil {
			return errors.Wrapf(err, "policy key slot %d", i)
		}
	}

	if err := p.Options.CheckValidity(); err != nil {
		return errors.Wrap(err, "policy options")
	}

	var expectedLen int
	switch p.Options.PolicyVersion {
	case 1:
		expectedLen = PolicyDescriptorLenV1
	case 2:
		expectedLen = PolicyDescriptorLenV2
	default:
		return errors.Errorf("policy version of %d is invalid", p.Options.PolicyVersion)
	}

	if err := util.CheckValidLength(expectedLen, len(p.KeyDescriptor)); err != nil {
		return errors.Wrap(err, "policy key descriptor")
	}

	return nil
}

// CheckValidity ensures the Config has all the necessary info for its Source.
func (c *Config) CheckValidity() error {
	// General checks
	if c == nil {
		return errNotInitialized
	}
	if err := c.Source.CheckValidity(); err != nil {
		return errors.Wrap(err, "default config source")
	}

	// Source specific checks
	switch c.Source {
	case SourceType_pam_passphrase, SourceType_custom_passphrase:
		if err := c.HashCosts.CheckValidity(); err != nil {
			return errors.Wrap(err, "config hashing costs")
		}
	}

	return errors.Wrap(c.Options.CheckValidity(), "config options")
}
