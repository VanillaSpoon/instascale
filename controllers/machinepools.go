package controllers

import (
	"context"
	"fmt"
	"os"
	"strings"

	cmv1 "github.com/openshift-online/ocm-sdk-go/clustersmgmt/v1"
	arbv1 "github.com/project-codeflare/multi-cluster-app-dispatcher/pkg/apis/controller/v1beta1"

	"k8s.io/klog"
	ctrl "sigs.k8s.io/controller-runtime"
)

func (r *AppWrapperReconciler) scaleMachinePool(ctx context.Context, aw *arbv1.AppWrapper, demandPerInstanceType map[string]int) (ctrl.Result, error) {
	connection, err := r.createOCMConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating OCM connection: %v", err)
		return ctrl.Result{}, err
	}
	defer connection.Close()
	for userRequestedInstanceType := range demandPerInstanceType {
		replicas := demandPerInstanceType[userRequestedInstanceType]

		clusterMachinePools := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).MachinePools()

		response, err := clusterMachinePools.List().SendContext(ctx)
		if err != nil {
			return ctrl.Result{}, err
		}

		numberOfMachines := 0
		response.Items().Each(func(machinePool *cmv1.MachinePool) bool {
			if machinePool.InstanceType() == userRequestedInstanceType && hasAwLabel(machinePool.Labels(), aw) {
				numberOfMachines = machinePool.Replicas()
				return false
			}
			return true
		})

		if numberOfMachines != replicas {
			m := make(map[string]string)
			m[aw.Name] = aw.Name
			klog.Infof("The instanceRequired array: %v", userRequestedInstanceType)

			machinePoolID := strings.ReplaceAll(aw.Name+"-"+userRequestedInstanceType, ".", "-")
			createMachinePool, err := cmv1.NewMachinePool().ID(machinePoolID).InstanceType(userRequestedInstanceType).Replicas(replicas).Labels(m).Build()
			if err != nil {
				klog.Errorf(`Error building MachinePool: %v`, err)
			}
			klog.Infof("Built MachinePool with instance type %v and name %v", userRequestedInstanceType, createMachinePool.ID())
			response, err := clusterMachinePools.Add().Body(createMachinePool).SendContext(ctx)
			if err != nil {
				klog.Errorf(`Error creating MachinePool: %v`, err)
			}
			klog.Infof("Created MachinePool: %v", response)
		}
	}
	return ctrl.Result{Requeue: false}, nil
}

func (r *AppWrapperReconciler) deleteMachinePool(ctx context.Context, aw *arbv1.AppWrapper) (ctrl.Result, error) {
	connection, err := r.createOCMConnection()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating OCM connection: %v", err)
		return ctrl.Result{}, err
	}
	defer connection.Close()

	machinePoolsConnection := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).MachinePools().List()

	machinePoolsListResponse, _ := machinePoolsConnection.Send()
	machinePoolsList := machinePoolsListResponse.Items()
	machinePoolsList.Range(func(index int, item *cmv1.MachinePool) bool {
		id, _ := item.GetID()
		if strings.Contains(id, aw.Name) {
			targetMachinePool, err := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).MachinePools().MachinePool(id).Delete().SendContext(ctx)
			if err != nil {
				klog.Infof("Error deleting target machinepool %v", targetMachinePool)
			}
			klog.Infof("Successfully Scaled down target machinepool %v", id)
		}
		return true
	})
	return ctrl.Result{Requeue: false}, nil
}

func (r *AppWrapperReconciler) machinePoolExists() (bool, error) {
	connection, err := r.createOCMConnection()
	if err != nil {
		return false, fmt.Errorf("error creating OCM connection: %w", err)
	}
	defer connection.Close()

	machinePools := connection.ClustersMgmt().V1().Clusters().Cluster(r.ocmClusterID).MachinePools()
	return machinePools != nil, nil
}
