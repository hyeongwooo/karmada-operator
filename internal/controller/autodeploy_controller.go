package controller

import (
	"context"
	"fmt"

	"github.com/wnguddn777.com/autodeploy/internal/utils"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	policyv1alpha1 "github.com/karmada-io/karmada/pkg/apis/policy/v1alpha1"
	operatorv1alpha1 "github.com/wnguddn777.com/autodeploy/api/v1alpha1"
)

const autoDeployFinalizer = "operator.wnguddn777.com/autodeploy-finalizer"

type AutoDeployReconciler struct {
	client.Client
	KarmadaClient client.Client
	Scheme        *runtime.Scheme
}

func (r *AutoDeployReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	autoDeploy := &operatorv1alpha1.AutoDeploy{}
	err := r.Get(ctx, req.NamespacedName, autoDeploy)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// 파이널라이저 처리
	if autoDeploy.ObjectMeta.DeletionTimestamp.IsZero() {
		if !utils.ContainsString(autoDeploy.GetFinalizers(), autoDeployFinalizer) {
			controllerutil.AddFinalizer(autoDeploy, autoDeployFinalizer)
			if err := r.Update(ctx, autoDeploy); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		if utils.ContainsString(autoDeploy.GetFinalizers(), autoDeployFinalizer) {
			if err := r.deleteExternalResources(ctx, autoDeploy); err != nil {
				return ctrl.Result{}, err
			}
			controllerutil.RemoveFinalizer(autoDeploy, autoDeployFinalizer)
			if err := r.Update(ctx, autoDeploy); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Deployment 생성 또는 업데이트
	deployment := r.deploymentForAutoDeploy(autoDeploy)
	if err := controllerutil.SetControllerReference(autoDeploy, deployment, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdateDeployment(ctx, deployment); err != nil {
		log.Error(err, "Failed to create or update Deployment")
		return ctrl.Result{}, err
	}

	// PropagationPolicy 생성 또는 업데이트
	pp := r.propagationPolicyForAutoDeploy(autoDeploy)
	if err := controllerutil.SetControllerReference(autoDeploy, pp, r.Scheme); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.createOrUpdatePropagationPolicy(ctx, pp); err != nil {
		log.Error(err, "Failed to create or update PropagationPolicy")
		return ctrl.Result{}, err
	}

	log.Info("Deployment and PropagationPolicy created/updated successfully")
	return ctrl.Result{}, nil
}

func (r *AutoDeployReconciler) deleteExternalResources(ctx context.Context, autoDeploy *operatorv1alpha1.AutoDeploy) error {
	// Deployment 삭제
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoDeploy.Name,
			Namespace: autoDeploy.Namespace,
		},
	}
	if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		return err
	}

	// PropagationPolicy 삭제
	pp := &policyv1alpha1.PropagationPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      autoDeploy.Name,
			Namespace: autoDeploy.Namespace,
		},
	}
	if err := r.Delete(ctx, pp); err != nil && !errors.IsNotFound(err) {
		return err
	}

	return nil
}

func (r *AutoDeployReconciler) deploymentForAutoDeploy(autoDeploy *operatorv1alpha1.AutoDeploy) *appsv1.Deployment {
	return &appsv1.Deployment{
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
}

func (r *AutoDeployReconciler) propagationPolicyForAutoDeploy(autoDeploy *operatorv1alpha1.AutoDeploy) *policyv1alpha1.PropagationPolicy {
	return &policyv1alpha1.PropagationPolicy{
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
}

func (r *AutoDeployReconciler) createOrUpdateDeployment(ctx context.Context, deployment *appsv1.Deployment) error {
	if r.KarmadaClient == nil {
		return fmt.Errorf("KarmadaClient is not initialized")
	}

	err := r.KarmadaClient.Create(ctx, deployment)
	if err != nil {
		if errors.IsAlreadyExists(err) {
			// 업데이트 로직
			existingDeployment := &appsv1.Deployment{}
			err = r.KarmadaClient.Get(ctx, client.ObjectKey{Namespace: deployment.Namespace, Name: deployment.Name}, existingDeployment)
			if err != nil {
				return fmt.Errorf("failed to get existing deployment: %w", err)
			}
			existingDeployment.Spec = deployment.Spec
			return r.KarmadaClient.Update(ctx, existingDeployment)
		}
		return fmt.Errorf("failed to create deployment: %w", err)
	}
	return nil
}

func (r *AutoDeployReconciler) createOrUpdatePropagationPolicy(ctx context.Context, pp *policyv1alpha1.PropagationPolicy) error {
	err := r.KarmadaClient.Create(ctx, pp)
	if err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	if errors.IsAlreadyExists(err) {
		existingPP := &policyv1alpha1.PropagationPolicy{}
		err = r.KarmadaClient.Get(ctx, client.ObjectKey{Namespace: pp.Namespace, Name: pp.Name}, existingPP)
		if err != nil {
			return err
		}
		existingPP.Spec = pp.Spec
		return r.KarmadaClient.Update(ctx, existingPP)
	}
	return nil
}

func ContainsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func (r *AutoDeployReconciler) SetupWithManager(mgr ctrl.Manager, karmadaClient client.Client) error {
	r.KarmadaClient = karmadaClient // Initialize KarmadaClient
	return ctrl.NewControllerManagedBy(mgr).
		For(&operatorv1alpha1.AutoDeploy{}).
		Owns(&appsv1.Deployment{}).
		Owns(&policyv1alpha1.PropagationPolicy{}).
		Complete(r)
}
