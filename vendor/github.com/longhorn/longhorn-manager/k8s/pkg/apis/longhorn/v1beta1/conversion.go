package v1beta1

import (
	"github.com/jinzhu/copier"

	"github.com/longhorn/longhorn-manager/k8s/pkg/apis/longhorn/v1beta2"
)

func copyConditionsFromMapToSlice(srcConditions map[string]Condition) ([]v1beta2.Condition, error) {
	dstConditions := []v1beta2.Condition{}
	for _, src := range srcConditions {
		dst := v1beta2.Condition{}
		if err := copier.Copy(&dst, &src); err != nil {
			return nil, err
		}
		dstConditions = append(dstConditions, dst)
	}
	return dstConditions, nil
}

func copyConditionFromSliceToMap(srcConditions []v1beta2.Condition) (map[string]Condition, error) {
	dstConditions := make(map[string]Condition, 0)
	for _, src := range srcConditions {
		dst := Condition{}
		if err := copier.Copy(&dst, &src); err != nil {
			return nil, err
		}
		dstConditions[dst.Type] = dst
	}
	return dstConditions, nil
}
