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
	corev1 "k8s.io/api/core/v1"
	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/crossplane-runtime/pkg/test"
	oamv1alpha2 "github.com/crossplane/crossplane/apis/oam/v1alpha2"
	workloadv1alpha1 "github.com/crossplane/crossplane/apis/workload/v1alpha1"
)

var replicas = int32(3)

type kubeAppModifier func(*workloadv1alpha1.KubernetesApplication)

func kaWithController(o metav1.Object, s schema.GroupVersionKind) kubeAppModifier {
	return func(a *workloadv1alpha1.KubernetesApplication) {
		ref := metav1.NewControllerRef(o, s)
		meta.AddControllerReference(a, *ref)
	}
}

func kaWithTemplate(name string, o runtime.Object) kubeAppModifier {
	return func(a *workloadv1alpha1.KubernetesApplication) {
		u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
		a.Spec.ResourceTemplates = append(a.Spec.ResourceTemplates, workloadv1alpha1.KubernetesApplicationResourceTemplate{
			ObjectMeta: metav1.ObjectMeta{
				Name: name,
			},
			Spec: workloadv1alpha1.KubernetesApplicationResourceSpec{
				Template: &unstructured.Unstructured{Object: u},
			},
		})
	}
}

func kubeApp(mod ...kubeAppModifier) *workloadv1alpha1.KubernetesApplication {
	a := &workloadv1alpha1.KubernetesApplication{
		ObjectMeta: metav1.ObjectMeta{
			Name: "cool-kapp",
		},
	}

	for _, m := range mod {
		m(a)
	}

	return a
}

var _ resource.Applicator = resource.ApplyFn(KubeAppApply)

func TestKubeAppApply(t *testing.T) {
	errBoom := errors.New("boom")

	type args struct {
		ctx context.Context
		c   client.Client
		o   runtime.Object
		ao  []resource.ApplyOption
	}

	type want struct {
		o   runtime.Object
		err error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"NotAKubernetesApplication": {
			reason: "An error should be returned if the object is not a KubernetesApplication",
			args: args{
				c: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
				o: &corev1.Namespace{},
			},
			want: want{
				o:   &corev1.Namespace{},
				err: errors.New("object is not a KubernetesApplication"),
			},
		},
		"GetError": {
			reason: "An error should be returned if we can't get the KubernetesApplication",
			args: args{
				c: &test.MockClient{MockGet: test.NewMockGetFn(errBoom)},
				o: kubeApp(),
			},
			want: want{
				o:   kubeApp(),
				err: errors.Wrap(errBoom, errGetKubeApp),
			},
		},
		"CreateError": {
			reason: "No error should be returned if we successfully create a new KubernetesApplication",
			args: args{
				c: &test.MockClient{
					MockGet:    test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, "")),
					MockCreate: test.NewMockCreateFn(errBoom),
				},
				o: kubeApp(),
			},
			want: want{
				o:   kubeApp(),
				err: errors.Wrap(errBoom, errCreateKubeApp),
			},
		},
		"ControllerMismatch": {
			reason: "An error should be returned if controllers must match, but don't",
			args: args{
				c: &test.MockClient{MockGet: test.NewMockGetFn(nil, func(o runtime.Object) error {
					*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp(kaWithController(&oamv1alpha2.ContainerizedWorkload{
						ObjectMeta: metav1.ObjectMeta{
							Name: "controller",
						},
					}, oamv1alpha2.ContainerizedWorkloadGroupVersionKind))
					return nil
				})},
				o:  kubeApp(),
				ao: []resource.ApplyOption{resource.ControllersMustMatch()},
			},
			want: want{
				o: kubeApp(kaWithController(&oamv1alpha2.ContainerizedWorkload{
					ObjectMeta: metav1.ObjectMeta{
						Name: "controller",
					},
				}, oamv1alpha2.ContainerizedWorkloadGroupVersionKind)),
				err: errors.New(errDiffController),
			},
		},
		"PatchError": {
			reason: "An error should be returned if we can't patch the KubernetesApplication",
			args: args{
				c: &test.MockClient{
					MockGet:   test.NewMockGetFn(nil),
					MockPatch: test.NewMockPatchFn(errBoom),
				},
				o: kubeApp(),
			},
			want: want{
				o:   kubeApp(),
				err: errors.Wrap(errBoom, errPatchKubeApp),
			},
		},
		"Created": {
			reason: "No error should be returned if we successfully create a new object",
			args: args{
				c: &test.MockClient{
					MockGet: test.NewMockGetFn(kerrors.NewNotFound(schema.GroupResource{}, "")),
					MockCreate: test.NewMockCreateFn(nil, func(o runtime.Object) error {
						*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp()
						return nil
					}),
				},
				o: kubeApp(),
			},
			want: want{
				o: kubeApp(),
			},
		},
		"PatchedNoOverwrite": {
			reason: "If existing and desired have the same name and kind of a template, non-array fields in templates should not be overwritten in patch",
			args: args{
				c: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(o runtime.Object) error {
						*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp(kaWithTemplate("cool-temp", deployment(dmWithReplicas(&replicas))))
						return nil
					}),
					MockPatch: test.NewMockPatchFn(nil, func(o runtime.Object) error {
						a, ok := o.(*workloadv1alpha1.KubernetesApplication)
						if !ok {
							return errors.Errorf("Not KubernetesApplication: %+v\n", o)
						}
						d := &appsv1.Deployment{}
						_ = runtime.DefaultUnstructuredConverter.FromUnstructured(a.Spec.ResourceTemplates[0].Spec.Template.UnstructuredContent(), d)
						if *d.Spec.Replicas != replicas {
							return errors.Errorf("Deployment missing replicas: %+v\n", d)
						}
						return nil
					}),
				},
				o: kubeApp(kaWithTemplate("cool-temp", deployment())),
			},
			want: want{
				o: kubeApp(kaWithTemplate("cool-temp", deployment(dmWithReplicas(&replicas)))),
			},
		},
		"PatchedOverwrite": {
			reason: "If existing and desired have different template names, the existing template should be overwritten by the desired",
			args: args{
				c: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(o runtime.Object) error {
						*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp(kaWithTemplate("nice-temp", deployment()))
						return nil
					}),
					MockPatch: test.NewMockPatchFn(nil, func(o runtime.Object) error {
						*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp(kaWithTemplate("cool-temp", deployment()))
						return nil
					}),
				},
				o: kubeApp(kaWithTemplate("cool-temp", deployment())),
			},
			want: want{
				o: kubeApp(kaWithTemplate("cool-temp", deployment())),
			},
		},
		"PatchedPartialOverwrite": {
			reason: "If existing and desired have the same name and kind of a template, array fields in templates should be overwritten in patch",
			args: args{
				c: &test.MockClient{
					MockGet: test.NewMockGetFn(nil, func(o runtime.Object) error {
						*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp(kaWithTemplate("cool-temp", deployment(dmWithReplicas(&replicas), dmWithContainerPorts(replicas))))
						return nil
					}),
					MockPatch: test.NewMockPatchFn(nil, func(o runtime.Object) error {
						*(o.(*workloadv1alpha1.KubernetesApplication)) = *kubeApp(kaWithTemplate("cool-temp", deployment(dmWithReplicas(&replicas))))
						return nil
					}),
				},
				o: kubeApp(kaWithTemplate("cool-temp", deployment(dmWithReplicas(&replicas)))),
			},
			want: want{
				o: kubeApp(kaWithTemplate("cool-temp", deployment(dmWithReplicas(&replicas)))),
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			err := KubeAppApply(tc.args.ctx, tc.args.c, tc.args.o, tc.args.ao...)
			if diff := cmp.Diff(tc.want.err, err, test.EquateErrors()); diff != "" {
				t.Errorf("\n%s\nKubeAppApply(...): -want error, +got error\n%s\n", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.o, tc.args.o); diff != "" {
				t.Errorf("\n%s\nKubeAppApply(...): -want, +got\n%s\n", tc.reason, diff)
			}
		})
	}
}

var dWithReplicas = &appsv1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       deploymentKind,
		APIVersion: deploymentAPIVersion,
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "cool-name",
	},
	Spec: appsv1.DeploymentSpec{
		Replicas: &replicas,
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				labelKey: "cool-val",
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					labelKey: "cool-val",
				},
			},
			// Spec: corev1.PodSpec{
			// 	Containers: []corev1.Container{
			// 		{
			// 			Name:  "cool-container",
			// 			Image: "mycool/image",
			// 		},
			// 	},
			// },
		},
	},
}

var dNoReplicas = &appsv1.Deployment{
	TypeMeta: metav1.TypeMeta{
		Kind:       deploymentKind,
		APIVersion: deploymentAPIVersion,
	},
	ObjectMeta: metav1.ObjectMeta{
		Name: "cool-name",
	},
	Spec: appsv1.DeploymentSpec{
		Selector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				labelKey: "cool-val",
			},
		},
		Template: corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					labelKey: "cool-val",
				},
			},
			// Spec: corev1.PodSpec{
			// 	Containers: []corev1.Container{
			// 		{
			// 			Name: "cool-container",
			// 		},
			// 	},
			// },
		},
	},
}

func toUn(o runtime.Object) *unstructured.Unstructured {
	u, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(o)
	return &unstructured.Unstructured{Object: u}
}
