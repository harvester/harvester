package ippoolusage

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	harvesterv1beta1 "github.com/harvester/harvester/pkg/apis/harvesterhci.io/v1beta1"
)

func TestCreateValidatesCIDR(t *testing.T) {
	validator := NewValidator()

	err := validator.Create(nil, &harvesterv1beta1.IPPoolUsage{
		ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
		Spec: harvesterv1beta1.IPPoolUsageSpec{
			CIDR: "invalid-cidr",
		},
	})

	assert.Error(t, err)
}

func TestUpdateRejectsCIDRMutation(t *testing.T) {
	validator := NewValidator()

	err := validator.Update(
		nil,
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.0.1/28",
				ReservedIPCount: 4,
			},
		},
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.1.1/28",
				ReservedIPCount: 4,
			},
		},
	)

	assert.Error(t, err)
}

func TestUpdateRejectsIncreasingReservedIPCount(t *testing.T) {
	validator := NewValidator()

	err := validator.Update(
		nil,
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.0.1/28",
				ReservedIPCount: 4,
			},
		},
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.0.1/28",
				ReservedIPCount: 8,
			},
		},
	)

	assert.Error(t, err)
}

func TestUpdateAllowsDecreasingReservedIPCount(t *testing.T) {
	validator := NewValidator()

	err := validator.Update(
		nil,
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.0.1/28",
				ReservedIPCount: 8,
			},
		},
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.0.1/28",
				ReservedIPCount: 4,
			},
		},
	)

	assert.NoError(t, err)
}

func TestUpdateTreatsZeroAsDefaultReservedIPCount(t *testing.T) {
	validator := NewValidator()

	err := validator.Update(
		nil,
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR: "172.16.0.1/28",
			},
		},
		&harvesterv1beta1.IPPoolUsage{
			ObjectMeta: metav1.ObjectMeta{Name: "pool1"},
			Spec: harvesterv1beta1.IPPoolUsageSpec{
				CIDR:            "172.16.0.1/28",
				ReservedIPCount: 9,
			},
		},
	)

	assert.Error(t, err)
}
