package setting

import (
	"encoding/json"
	"fmt"
	"reflect"

	kubevirtv1 "kubevirt.io/api/core/v1"

	harvesterv1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
	"github.com/harvester/harvester/pkg/util"
)

func (h *Handler) syncKubeVirtMigration(setting *harvesterv1.Setting) error {
	kubevirt, err := h.kubeVirtConfigCache.Get(util.HarvesterSystemNamespaceName, util.KubeVirtObjectName)
	if err != nil {
		return fmt.Errorf("failed to get kubevirt object %v/%v %w", util.HarvesterSystemNamespaceName, util.KubeVirtObjectName, err)
	}

	// The setting is created with an empty value, both on a fresh install and when an
	// existing cluster is upgraded. Before this setting existed the migration configuration
	// could only be set on the KubeVirt object directly, so adopt whatever is already there
	// instead of overwriting it. Otherwise the setting would report the defaults while the
	// cluster runs a different configuration.
	if setting.Value == "" && setting.Annotations[util.AnnotationHash] == "" {
		return h.adoptKubeVirtMigration(setting, kubevirt)
	}

	var value string
	if setting.Value != "" {
		value = setting.Value
	} else {
		value = setting.Default
	}

	migrationConfiguration := &kubevirtv1.MigrationConfiguration{}
	if value != "" {
		if err := json.Unmarshal([]byte(value), migrationConfiguration); err != nil {
			return fmt.Errorf("invalid value: `%s`: %w", value, err)
		}
	}

	kubevirtCpy := kubevirt.DeepCopy()
	if kubevirtCpy.Spec.Configuration.MigrationConfiguration == nil {
		kubevirtCpy.Spec.Configuration.MigrationConfiguration = &kubevirtv1.MigrationConfiguration{}
	}
	// ignore nodeDrainTaintKey and network field
	// The default nodeDrainTaintKey is "kubevirt.io/drain" and it's used in upgrade script.
	// The network field is handled by "vm-migration-network" setting.
	migrationConfiguration.NodeDrainTaintKey = nil
	migrationConfiguration.Network = kubevirtCpy.Spec.Configuration.MigrationConfiguration.Network

	if !reflect.DeepEqual(kubevirtCpy.Spec.Configuration.MigrationConfiguration, migrationConfiguration) {
		kubevirtCpy.Spec.Configuration.MigrationConfiguration = migrationConfiguration
		if _, err := h.kubeVirtConfig.Update(kubevirtCpy); err != nil {
			return fmt.Errorf("failed to update KubeVirt migration configuration, err: %w", err)
		}
	}
	return nil
}

// adoptKubeVirtMigration copies the migration configuration of the KubeVirt object into the
// setting value, so that the setting reports what the cluster is actually running. It never
// writes to the KubeVirt object.
func (h *Handler) adoptKubeVirtMigration(setting *harvesterv1.Setting, kubevirt *kubevirtv1.KubeVirt) error {
	current := kubevirt.Spec.Configuration.MigrationConfiguration
	if current == nil {
		// KubeVirt falls back to its own defaults, which the setting default already mirrors.
		return nil
	}

	// Start from the setting default and overlay the fields the KubeVirt object sets, so the
	// adopted value is complete and applying it back is a no-op.
	migrationConfiguration := &kubevirtv1.MigrationConfiguration{}
	if setting.Default != "" {
		if err := json.Unmarshal([]byte(setting.Default), migrationConfiguration); err != nil {
			return fmt.Errorf("invalid default: `%s`: %w", setting.Default, err)
		}
	}
	overlay, err := json.Marshal(current)
	if err != nil {
		return fmt.Errorf("failed to marshal KubeVirt migration configuration, err: %w", err)
	}
	if err := json.Unmarshal(overlay, migrationConfiguration); err != nil {
		return fmt.Errorf("invalid KubeVirt migration configuration: `%s`: %w", overlay, err)
	}

	// nodeDrainTaintKey and network are not configurable through this setting, see above.
	migrationConfiguration.NodeDrainTaintKey = nil
	migrationConfiguration.Network = nil

	value, err := json.Marshal(migrationConfiguration)
	if err != nil {
		return fmt.Errorf("failed to marshal migration configuration, err: %w", err)
	}

	toUpdate := setting.DeepCopy()
	toUpdate.Value = string(value)
	if _, err := h.settings.Update(toUpdate); err != nil {
		return fmt.Errorf("failed to adopt KubeVirt migration configuration into setting %s, err: %w", setting.Name, err)
	}
	return nil
}
