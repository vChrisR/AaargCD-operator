/*
Copyright 2023.

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

package controllers

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/vchrisr/aaargcd-operator/api/v1alpha1"
	aaargcdiov1alpha1 "github.com/vchrisr/aaargcd-operator/api/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

// AappReconciler reconciles a Aapp object
type AappReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

//+kubebuilder:rbac:groups=*,resources=*,verbs=*

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Aapp object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *AappReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	aapp := &v1alpha1.Aapp{}
	err := r.Get(ctx, req.NamespacedName, aapp)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		log.Error(err, "Failed to get Aapp")
		return ctrl.Result{}, err
	}

	cronjob := r.cronjobForAapp(aapp)
	found := &batchv1.CronJob{}

	err = r.Client.Get(ctx, req.NamespacedName, found)
	if err != nil && apierrors.IsNotFound(err) {
		sa := r.SaForAapp(aapp)
		rb := r.RoleBindingForAapp(aapp)

		err = r.Client.Create(ctx, &cronjob)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.Client.Create(ctx, &sa)
		if err != nil {
			return ctrl.Result{}, err
		}

		err = r.Client.Create(ctx, &rb)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	err = r.Client.Update(ctx, &cronjob)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *AappReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&aaargcdiov1alpha1.Aapp{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}

func (r *AappReconciler) cronjobForAapp(a *v1alpha1.Aapp) batchv1.CronJob {
	four := int32(4)
	repo := a.Spec.Repo
	folder := a.Spec.Folder

	cronjob := batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Name,
			Namespace: a.Namespace,
		},
		Spec: batchv1.CronJobSpec{
			Schedule: "*/5 * * * *",
			JobTemplate: batchv1.JobTemplateSpec{
				Spec: batchv1.JobSpec{
					Template: corev1.PodTemplateSpec{
						Spec: corev1.PodSpec{
							ServiceAccountName: a.Name + "-sa",
							Containers: []corev1.Container{
								{
									Name:    "cd",
									Image:   "bitnami/kubectl",
									Command: []string{"/bin/sh", "-c", "cd /tmp;git clone $REPO repo; cd repo/$FOLDER; kubectl apply -f ."},
									Env:     []corev1.EnvVar{{Name: "REPO", Value: repo}, {Name: "FOLDER", Value: folder}},
								},
							},
							RestartPolicy: corev1.RestartPolicyNever,
						},
					},
				},
			},
			SuccessfulJobsHistoryLimit: &four,
			FailedJobsHistoryLimit:     &four,
		},
	}
	ctrl.SetControllerReference(a, &cronjob, r.Scheme)

	return cronjob
}

func (r *AappReconciler) SaForAapp(a *v1alpha1.Aapp) corev1.ServiceAccount {
	sa := corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Name + "-sa",
			Namespace: a.Namespace,
		},
	}
	ctrl.SetControllerReference(a, &sa, r.Scheme)

	return sa
}

func (r *AappReconciler) RoleBindingForAapp(a *v1alpha1.Aapp) rbacv1.RoleBinding {
	rb := rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      a.Name + "-rolebinding",
			Namespace: a.Namespace,
		},
		Subjects: []rbacv1.Subject{{
			Kind: "ServiceAccount",
			Name: a.Name + "-sa",
		}},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "admin",
		},
	}

	ctrl.SetControllerReference(a, &rb, r.Scheme)

	return rb
}
