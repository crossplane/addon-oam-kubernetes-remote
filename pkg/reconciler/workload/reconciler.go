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
	"time"

	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/crossplane/crossplane-runtime/apis/core/v1alpha1"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/logging"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
)

const (
	reconcileTimeout = 1 * time.Minute
	shortWait        = 30 * time.Second
	longWait         = 1 * time.Minute
)

// Reconcile error strings.
const (
	errGetWorkload              = "cannot get workload"
	errUpdateWorkloadStatus     = "cannot update workload status"
	errTranslateWorkload        = "cannot translate workload"
	errApplyWorkloadTranslation = "cannot apply workload translation"
)

// Reconcile event reasons.
const (
	reasonTranslateWorkload = "WorkloadTranslated"

	reasonCannotTranslateWorkload        = "CannotTranslateWorkload"
	reasonCannotApplyWorkloadTranslation = "CannotApplyWorkloadTranslation"
)

const labelKey = "workload.oam.crossplane.io"

// A ReconcilerOption configures a Reconciler.
type ReconcilerOption func(*Reconciler)

// WithLogger specifies how the Reconciler should log messages.
func WithLogger(l logging.Logger) ReconcilerOption {
	return func(r *Reconciler) {
		r.log = l
	}
}

// WithRecorder specifies how the Reconciler should record events.
func WithRecorder(er event.Recorder) ReconcilerOption {
	return func(r *Reconciler) {
		r.record = er
	}
}

// WithTranslator specifies how the Reconciler should translate the workload.
func WithTranslator(t Translator) ReconcilerOption {
	return func(r *Reconciler) {
		r.workload = t
	}
}

// WithApplicator specifies how the Reconciler should apply the workload
// translation.
func WithApplicator(a resource.Applicator) ReconcilerOption {
	return func(r *Reconciler) {
		r.applicator = a
	}
}

// A Reconciler reconciles an OAM workload type by packaging it into a
// KubernetesApplication.
type Reconciler struct {
	client      client.Client
	newWorkload func() Workload
	workload    Translator
	applicator  resource.Applicator

	log    logging.Logger
	record event.Recorder
}

// Kind is a kind of OAM workload.
type Kind schema.GroupVersionKind

// A Workload is a type of OAM workload.
type Workload interface {
	resource.Conditioned
	metav1.Object
	runtime.Object
}

// NewReconciler returns a Reconciler that reconciles an OAM workload type by
// packaging it into a KubernetesApplication.
func NewReconciler(m ctrl.Manager, workload Kind, o ...ReconcilerOption) *Reconciler {
	nw := func() Workload {
		return resource.MustCreateObject(schema.GroupVersionKind(workload), m.GetScheme()).(Workload)
	}

	r := &Reconciler{
		client:      m.GetClient(),
		newWorkload: nw,
		workload:    TranslateFn(NoopTranslate),
		applicator:  resource.ApplyFn(resource.Apply),
		log:         logging.NewNopLogger(),
		record:      event.NewNopRecorder(),
	}

	for _, ro := range o {
		ro(r)
	}

	return r
}

// Reconcile an OAM workload type by packaging it into a KubernetesApplication.
func (r *Reconciler) Reconcile(req reconcile.Request) (reconcile.Result, error) {
	log := r.log.WithValues("request", req)
	log.Debug("Reconciling")

	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	workload := r.newWorkload()
	if err := r.client.Get(ctx, req.NamespacedName, workload); err != nil {
		return reconcile.Result{}, errors.Wrap(resource.IgnoreNotFound(err), errGetWorkload)
	}

	log = log.WithValues("uid", workload.GetUID(), "version", workload.GetResourceVersion())

	objs, err := r.workload.Translate(ctx, workload)
	if err != nil {
		log.Debug("Cannot translate workload", "error", err, "requeue-after", time.Now().Add(shortWait))
		r.record.Event(workload, event.Warning(reasonCannotTranslateWorkload, err))
		workload.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errTranslateWorkload)))
		return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, workload), errUpdateWorkloadStatus)
	}

	for _, o := range objs {
		if err := r.applicator.Apply(ctx, r.client, o, resource.ControllersMustMatch()); err != nil {
			log.Debug("Cannot apply workload translate", "error", err, "requeue-after", time.Now().Add(shortWait))
			r.record.Event(workload, event.Warning(reasonCannotApplyWorkloadTranslation, err))
			workload.SetConditions(v1alpha1.ReconcileError(errors.Wrap(err, errApplyWorkloadTranslation)))
			return reconcile.Result{RequeueAfter: shortWait}, errors.Wrap(r.client.Status().Update(ctx, workload), errUpdateWorkloadStatus)
		}
	}

	r.record.Event(workload, event.Normal(reasonTranslateWorkload, "Successfully translated workload"))
	log.Debug("Successfully translated workload as KubernetesApplication", "kind", workload.GetObjectKind().GroupVersionKind().String())

	workload.SetConditions(v1alpha1.ReconcileSuccess())
	return reconcile.Result{RequeueAfter: longWait}, errors.Wrap(r.client.Status().Update(ctx, workload), errUpdateWorkloadStatus)
}
