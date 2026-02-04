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

package v1beta1

import (
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
)

var Log = ctrl.Log.WithName("Defaulter:v1beta1")

// Hub marks these types as conversion hub.
func (l *Logging) Hub()       {}
func (o *Output) Hub()        {}
func (c *ClusterOutput) Hub() {}
func (f *Flow) Hub()          {}
func (c *ClusterFlow) Hub()   {}

func SetupWebhookWithManager(mgr ctrl.Manager, apiTypes ...runtime.Object) error {
	for _, apiType := range apiTypes {
		// register webhook using controller-runtime because of interface checks
		if err := ctrl.NewWebhookManagedBy(mgr).
			For(apiType).
			Complete(); err != nil {
			return err
		}
	}
	return nil
}

func APITypes() []runtime.Object {
	return []runtime.Object{&Logging{}, &Output{}, &ClusterOutput{}, &Flow{}, &ClusterFlow{}}
}

func (l *Logging) Default() {
	Log.Info("Defaulter called for", "logging", l)
	if l.Spec.FluentdSpec != nil {
		fluentdSpec := l.Spec.FluentdSpec
		if fluentdSpec.Scaling == nil {
			fluentdSpec.Scaling = new(FluentdScaling)
		}
		if fluentdSpec.Scaling.PodManagementPolicy == "" {
			fluentdSpec.Scaling.PodManagementPolicy = string(appsv1.ParallelPodManagement)
		}
	} else {
		Log.Info("l.Spec.FluentdSpec is missing, skipping Defaulter")
	}
}
