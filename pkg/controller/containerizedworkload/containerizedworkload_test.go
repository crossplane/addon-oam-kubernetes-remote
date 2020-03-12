/*
Copyright 2020 The Crossplane Authors.

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

package containerizedworkload

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	oamv1alpha2 "github.com/crossplane/crossplane/apis/oam/v1alpha2"

	"github.com/crossplane/addon-oam-kubernetes-remote/pkg/reconciler/workload"
	workloadfake "github.com/crossplane/addon-oam-kubernetes-remote/pkg/reconciler/workload/fake"
)

var (
	cwName      = "test-name"
	cwNamespace = "test-namespace"
	cwUID       = "a-very-unique-identifier"
)

type deploymentModifier func(*appsv1.Deployment)

func dmWithOS(os string) deploymentModifier {
	return func(d *appsv1.Deployment) {
		if d.Spec.Template.Spec.NodeSelector == nil {
			d.Spec.Template.Spec.NodeSelector = map[string]string{}
		}
		d.Spec.Template.Spec.NodeSelector["beta.kubernetes.io/os"] = os
	}
}

func deployment(mod ...deploymentModifier) *appsv1.Deployment {
	d := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{
			Kind:       deploymentKind,
			APIVersion: deploymentAPIVersion,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: cwName,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					labelKey: cwUID,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						labelKey: cwUID,
					},
				},
			},
		},
	}

	for _, m := range mod {
		m(d)
	}

	return d
}

type cwModifier func(*oamv1alpha2.ContainerizedWorkload)

func cwWithOS(os string) cwModifier {
	return func(cw *oamv1alpha2.ContainerizedWorkload) {
		oamOS := oamv1alpha2.OperatingSystem(os)
		cw.Spec.OperatingSystem = &oamOS
	}
}

func containerizedWorkload(mod ...cwModifier) *oamv1alpha2.ContainerizedWorkload {
	cw := &oamv1alpha2.ContainerizedWorkload{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cwName,
			Namespace: cwNamespace,
			UID:       types.UID(cwUID),
		},
	}

	for _, m := range mod {
		m(cw)
	}

	return cw
}

func TestContainerizedWorkloadPackager(t *testing.T) {
	type args struct {
		w workload.Workload
	}

	type want struct {
		result []runtime.Object
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"ErrorWorkloadNotContainerizedWorkload": {
			reason: "Workload passed to packager that is not ContainerizedWorkload should return error.",
			args: args{
				w: &workloadfake.Workload{},
			},
			want: want{err: errors.New(errNotContainerizedWorkload)},
		},
		"SuccessfulEmpty": {
			reason: "A ContainerizedWorkload should be successfully packaged into a deployment.",
			args: args{
				w: containerizedWorkload(),
			},
			want: want{result: []runtime.Object{deployment()}},
		},
		"SuccessfulOS": {
			reason: "A ContainerizedWorkload should be successfully packaged into a deployment.",
			args: args{
				w: containerizedWorkload(cwWithOS("test")),
			},
			want: want{result: []runtime.Object{deployment(dmWithOS("test"))}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := containerizedWorkloadTranslator(context.TODO(), tc.args.w)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, r); diff != "" {
				t.Errorf("\nReason: %s\ncontainerizedWorkloadTranslator(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
