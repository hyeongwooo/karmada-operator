package controller

import (
	"context"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	operatorv1alpha1 "github.com/wnguddn777.com/autodeploy/api/v1alpha1"
)

// AutoDeployReconciler reconciles a AutoDeploy object
type AutoDeployReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *AutoDeployReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// Fetch the AutoDeploy instance
	autoDeploy := &operatorv1alpha1.AutoDeploy{}
	err := r.Get(ctx, req.NamespacedName, autoDeploy)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("AutoDeploy resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get AutoDeploy")
		return ctrl.Result{}, err
	}

	// Create or update the Deployment
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoDeploy.Name,
			Namespace: autoDeploy.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &autoDeploy.Spec.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": autoDeploy.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": autoDeploy.Name},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  autoDeploy.Name,
							Image: autoDeploy.Spec.Image,
						},
					},
				},
			},
		},
	}

	// Create or update the Deployment
	if err := r.CreateOrUpdate(ctx, deployment); err != nil {
		log.Error(err, "Failed to create or update Deployment")
		return ctrl.Result{}, err
	}

	// Create or update the PropagationPolicy
	pp := &policyv1alpha1.PropagationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoDeploy.Name,
			Namespace: autoDeploy.Namespace,
		},
		Spec: policyv1alpha1.PropagationSpec{
			ResourceSelectors: []policyv1alpha1.ResourceSelector{
				{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       autoDeploy.Name,
				},
			},
			Placement: policyv1alpha1.Placement{
				ClusterAffinity: &policyv1alpha1.ClusterAffinity{
					ClusterNames: []string{autoDeploy.Spec.MemberCluster},
				},
			},
		},
	}

	if err := r.CreateOrUpdate(ctx, pp); err != nil {
		log.Error(err, "Failed to create or update PropagationPolicy")
		return ctrl.Result{}, err
	}

	log.Info("Deployment and PropagationPolicy created/updated successfully")
	return ctrl.Result{}, nil
}

func (r *AutoDeployReconciler) CreateOrUpdate(ctx context.Context, obj client.Object) error {
	if err := r.Create(ctx, obj); err != nil {
		if errors.IsAlreadyExists(err) {
			return r.Update(ctx, obj)
		}
		return err
	}
	return nil
}

func (r *AutoDeployReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.AutoDeploy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&policyv1alpha1.PropagationPolicy{}).
		Complete(r)
}
