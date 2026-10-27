package controller

import (
	"context"
	"time"

	computev1 "github.com/Mohdcode/ec2Operator/api/v1"
	"github.com/aws/aws-sdk-go-v2/aws"
	"k8s.io/apimachinery/pkg/api/errors"
	apiMeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// Ec2InstanceReconciler is a struct that implements the logic for reconciling Ec2Instance custom resources.
// It embeds the Kubernetes client.Client interface, which provides methods for interacting with the Kubernetes API server,
// and holds a pointer to a runtime.Scheme, which is used for type conversions between Go structs and Kubernetes objects.
// This struct is used to reconcile the Ec2Instance custom resource.

type Ec2InstanceReconciler struct {
	client.Client                 // Used to perform CRUD operations on Kubernetes resources.
	Scheme        *runtime.Scheme // Used to map Go types to Kubernetes GroupVersionKinds and vice versa.
}

/* Following are "Markers": These comments are special markers that the controller-gen tool (part of the Kubebuilder framework) understands.
Used for Code Generation: When you run make manifests in your project, controller-gen reads these markers and automatically generates the ClusterRole YAML manifest file.
This file defines all the permissions your controller needs to interact with Kubernetes API objects.
*/
// +kubebuilder:rbac:groups=compute.cloud.com,resources=ec2instances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=compute.cloud.com,resources=ec2instances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=compute.cloud.com,resources=ec2instances/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// After updating the status of the resource (e.g., with r.Status().Update), the Kubernetes API server
// will emit an update event for the resource. This event will be picked up by the controller-runtime
// and will cause the Reconcile function to be called again for the same resource. This is why, after
// updating the status, the reconciler is called again: it is a result of the Kubernetes watch mechanism
// and ensures that the controller can observe and react to any changes, including those it made itself.
// This pattern is common in Kubernetes controllers to ensure eventual consistency and to handle
// situations where the status update may not have been fully applied or observed yet.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.2/pkg/reconcile
func (r *Ec2InstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	l.Info("=== RECONCILE LOOP STARTED ===", "namespace", req.Namespace, "name", req.Name)

	// Create a new instance of the Ec2Instance struct to hold the data retrieved from the Kubernetes API.
	// This struct will be populated with the current state of the Ec2Instance resource specified by the request.
	ec2Instance := &computev1.Ec2Instance{}
	// Retrieve the Ec2Instance resource from the Kubernetes API server using the provided request's NamespacedName.
	if err := r.Get(ctx, req.NamespacedName, ec2Instance); err != nil {
		if errors.IsNotFound(err) {
			l.Info("Instance Deleted. No need to reconcile")
			// Kubernetes will not retry - done, wait for next event
			return ctrl.Result{}, nil
		}
		// Kubernetes will retry with backoff
		return ctrl.Result{}, err
	}

	//check if deletionTimestamp is not zero
	if !ec2Instance.DeletionTimestamp.IsZero() {
		l.Info("Has deletionTimestamp, Instance is being deleted")
		if ec2Instance.Status.InstanceID == "" {
			controllerutil.RemoveFinalizer(ec2Instance, "ec2instance.compute.cloud.com")
			return ctrl.Result{}, r.Update(ctx, ec2Instance)
		}
		_, err := deleteEc2Instance(ctx, ec2Instance)
		if err != nil {
			l.Error(err, "Failed to delete EC2 instance")
			// Kubernetes will retry with backoff
			return ctrl.Result{Requeue: true}, err
		}

		// Remove the finalizer
		controllerutil.RemoveFinalizer(ec2Instance, "ec2instance.compute.cloud.com")
		if err := r.Update(ctx, ec2Instance); err != nil {
			l.Error(err, "Failed to remove finalizer")
			// Kubernetes will retry with backoff
			return ctrl.Result{Requeue: true}, err
		}
		// at this point, the instance state is terminated and the finalizer is removed
		return ctrl.Result{}, nil
	}

	// if errors.IsNotFound(err) {
	// 	// Object was deleted
	// 	fmt.Println("Ran kubectl delete ec2instance")
	// 	l.Info("Got a delete request for the instance. Will delete the instance from AWS")
	// 	// Any cleanup logic here (though you can't access the object anymore)
	// 	return ctrl.Result{}, nil
	// }

	// Check if we already have an instance ID in status
	if ec2Instance.Status.InstanceID != "" {
		l.Info("Verifying existing instance state", "instanceID", ec2Instance.Status.InstanceID)

		instanceExist, instanceState, err := checkEC2InstanceExists(ctx, ec2Instance.Status.InstanceID, ec2Instance)
		if err != nil {
			l.Error(err, "Failed to check instance existence")
			return ctrl.Result{}, err
		}

		if !instanceExist {
			l.Info("Instance does not exist in AWS. Clearing status to trigger recreation", "instanceID", ec2Instance.Status.InstanceID)
			ec2Instance.Status.InstanceID = ""
			ec2Instance.Status.State = ""
			ec2Instance.Status.PublicIP = ""
			setReadyCondition(ec2Instance, false, "InstanceNotFound", "The EC2 instance is missing and will be recreated")
			if err := r.Status().Update(ctx, ec2Instance); err != nil {
				l.Error(err, "Failed to clear status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}

		observedState := ""
		if instanceState.State != nil {
			observedState = string(instanceState.State.Name)
		}
		readyStatus := metav1.ConditionFalse
		if observedState == "running" {
			readyStatus = metav1.ConditionTrue
		}
		readyCondition := apiMeta.FindStatusCondition(ec2Instance.Status.Conditions, "Ready")
		if ec2Instance.Status.State != observedState || ec2Instance.Status.PublicIP != aws.ToString(instanceState.PublicIpAddress) || readyCondition == nil || readyCondition.Status != readyStatus {
			l.Info("Detected drift in instance state/IP. Updating status",
				"oldState", ec2Instance.Status.State, "newState", observedState)

			ec2Instance.Status.State = observedState
			ec2Instance.Status.PublicIP = aws.ToString(instanceState.PublicIpAddress)
			setReadyCondition(ec2Instance, observedState == "running", "InstanceStateObserved", "Observed EC2 state: "+observedState)
			if err := r.Status().Update(ctx, ec2Instance); err != nil {
				l.Error(err, "Failed to update status after drift detection")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	l.Info("Creating new instance")

	if !controllerutil.ContainsFinalizer(ec2Instance, "ec2instance.compute.cloud.com") {
		controllerutil.AddFinalizer(ec2Instance, "ec2instance.compute.cloud.com")
		if err := r.Update(ctx, ec2Instance); err != nil {
			l.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}
	// Create a new instance.

	createdInstanceInfo, err := createEc2Instance(ctx, ec2Instance)
	if err != nil {
		l.Error(err, "Failed to create EC2 instance")
		// Kubernetes will retry with backoff
		return ctrl.Result{}, err
	}

	l.Info("=== ABOUT TO UPDATE STATUS - This will trigger reconcile loop again ===",
		"instanceID", createdInstanceInfo.InstanceID,
		"state", createdInstanceInfo.State)

	ec2Instance.Status.InstanceID = createdInstanceInfo.InstanceID
	ec2Instance.Status.State = createdInstanceInfo.State
	ec2Instance.Status.PublicIP = createdInstanceInfo.PublicIP
	ec2Instance.Status.PrivateIP = createdInstanceInfo.PrivateIP
	ec2Instance.Status.PublicDNS = createdInstanceInfo.PublicDNS
	ec2Instance.Status.PrivateDNS = createdInstanceInfo.PrivateDNS
	setReadyCondition(ec2Instance, createdInstanceInfo.State == "running", "InstanceCreated", "EC2 instance created with state: "+createdInstanceInfo.State)

	err = r.Status().Update(ctx, ec2Instance)
	if err != nil {
		l.Error(err, "Failed to update status")
		// Kubernetes will retry with backoff
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: 1 * time.Second}, nil
}

func setReadyCondition(instance *computev1.Ec2Instance, ready bool, reason, message string) {
	status := metav1.ConditionFalse
	if ready {
		status = metav1.ConditionTrue
	}
	apiMeta.SetStatusCondition(&instance.Status.Conditions, metav1.Condition{
		Type:               "Ready",
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: instance.Generation,
	})
}

// SetupWithManager sets up the controller with the Manager.
// SetupWithManager registers the Ec2InstanceReconciler with the controller manager.
// It configures the controller to watch for changes to Ec2Instance resources.
// The controller will be named "ec2instance" for logging and metrics purposes.
// The Complete(r) call finalizes the setup, associating the reconciler logic with this controller.
func (r *Ec2InstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&computev1.Ec2Instance{}).
		Named("ec2instance").
		Complete(r)
}
