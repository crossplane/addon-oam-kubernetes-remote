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

package workload

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/crossplane/crossplane-runtime/pkg/test"
	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"

	workloadfake "github.com/crossplane/addon-oam-kubernetes-remote/pkg/reconciler/workload/fake"
)

var (
	workloadName      = "test-workload"
	workloadNamespace = "test-namespace"
	workloadUID       = "a-very-unique-identifier"

	trueVal = true
)

func TestKubeAppWrapper(t *testing.T) {
	type args struct {
		w Workload
		o runtime.Object
	}

	type want struct {
		result runtime.Object
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NilObject": {
			reason: "Nil object should immediately return an error.",
			args: args{
				w: &workloadfake.Workload{},
			},
			want: want{err: errors.New(errWrapInKubeApp)},
		},
		"SuccessfulWrapDeployment": {
			reason: "A Deployment should be able to be wrapped in a KubernetesApplication.",
			args: args{
				w: &workloadfake.Workload{
					ObjectMeta: metav1.ObjectMeta{
						Name:      workloadName,
						Namespace: workloadNamespace,
						UID:       types.UID(workloadUID),
					},
				},
				o: &appsv1.Deployment{},
			},
			want: want{result: &workloadv1alpha1.KubernetesApplication{
				ObjectMeta: metav1.ObjectMeta{
					Name:      workloadName,
					Namespace: workloadNamespace,
					OwnerReferences: []metav1.OwnerReference{
						{
							Name:               workloadName,
							UID:                types.UID(workloadUID),
							Controller:         &trueVal,
							BlockOwnerDeletion: &trueVal,
						},
					},
				},
				Spec: workloadv1alpha1.KubernetesApplicationSpec{
					ResourceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{
							labelKey: workloadUID,
						},
					},
					ResourceTemplates: []workloadv1alpha1.KubernetesApplicationResourceTemplate{
						{
							ObjectMeta: metav1.ObjectMeta{
								Name:   workloadName,
								Labels: map[string]string{labelKey: workloadUID},
							},
							Spec: workloadv1alpha1.KubernetesApplicationResourceSpec{
								Template: &unstructured.Unstructured{
									Object: map[string]interface{}{
										"metadata": map[string]interface{}{"creationTimestamp": nil},
										"spec": map[string]interface{}{
											"selector": nil,
											"strategy": map[string]interface{}{},
											"template": map[string]interface{}{
												"metadata": map[string]interface{}{"creationTimestamp": nil},
												"spec":     map[string]interface{}{"containers": nil},
											},
										},
										"status": map[string]interface{}{},
									},
								},
							},
						},
					},
				},
			}},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			r, err := KubeAppWrapper(context.TODO(), tc.args.w, tc.args.o)

			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\nReason: %s\nKubeAppWrapper(...): -want error, +got error:\n%s", tc.reason, diff)
			}

			if diff := cmp.Diff(tc.want.result, r); diff != "" {
				t.Errorf("\nReason: %s\nKubeAppWrapper(...): -want, +got:\n%s", tc.reason, diff)
			}
		})
	}
}
